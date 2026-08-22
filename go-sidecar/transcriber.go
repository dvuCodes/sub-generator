package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// TranscribeOptions controls a single /inference call against the
// whisper.cpp server (>= v1.9.x, response_format=verbose_json).
type TranscribeOptions struct {
	SourceLang     string  // ISO 639-1 code; "" leaves the server default
	BeamSize       int     // >1 switches decoding to beam search
	BestOf         int     // greedy candidates kept per step
	VADEnabled     bool    // requires the server started with a Silero VAD model
	InitialPrompt  string  // domain vocabulary / character names
	Temperature    float64 // decoding start temperature (0 = deterministic)
	TemperatureInc float64 // fallback ladder step used on decoder failure
	EntropyThold   float64 // entropy trigger for temperature fallback
	LogProbThold   float64 // <=0 keeps the server default (-1.00)
	MaxContext     int     // text tokens carried between 30s windows
	SplitOnWord    bool    // segment on word/token boundaries
	MaxLen         int     // max characters per segment (server-side wrapping)
}

func DefaultTranscribeOptions() TranscribeOptions {
	return TranscribeOptions{
		BeamSize:       5,
		BestOf:         5,
		Temperature:    0,
		TemperatureInc: 0.2,
		EntropyThold:   2.8, // raised from 2.4: cuts Japanese repetition loops
		MaxContext:     64,  // breaks self-reinforcing repetition context
		SplitOnWord:    true,
	}
}

type Transcriber struct {
	baseURL string
	client  *http.Client
}

func NewTranscriber(port int) *Transcriber {
	return &Transcriber{
		baseURL: localServiceBaseURL(port),
		client:  &http.Client{Timeout: 60 * time.Minute}, // long jobs on CPU-only machines
	}
}

// --- whisper-server verbose_json response types ---

type whisperResponse struct {
	Text                        string           `json:"text"`
	Duration                    float64          `json:"duration"`
	Language                    string           `json:"language"`          // full name, e.g. "japanese"
	DetectedLanguage            string           `json:"detected_language"` // full name
	DetectedLanguageProbability float64          `json:"detected_language_probability"`
	Segments                    []whisperSegment `json:"segments"`
}

type whisperSegment struct {
	Start        float64       `json:"start"` // seconds (upstream >= 1.7)
	End          float64       `json:"end"`   // seconds
	T0           *float64      `json:"t0"`    // legacy forks: centiseconds
	T1           *float64      `json:"t1"`    // legacy forks: centiseconds
	Text         string        `json:"text"`
	AvgLogprob   float64       `json:"avg_logprob"`
	NoSpeechProb float64       `json:"no_speech_prob"`
	Words        []whisperWord `json:"words"`
}

type whisperWord struct {
	Word  string   `json:"word"`
	Start *float64 `json:"start"`
	End   *float64 `json:"end"`
}

// Transcribe converts an audio file into timestamped segments.
func (t *Transcriber) Transcribe(ctx context.Context, audioPath string, opts TranscribeOptions) (*TranscriptionResult, error) {
	req, contentType, cleanup, err := newInferenceRequest(
		ctx,
		t.baseURL+"/inference",
		audioPath,
		opts,
	)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	req.Header.Set("Content-Type", contentType)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whisper-server request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("whisper-server returned status %d: %s", resp.StatusCode, string(body))
	}

	var whisperResp whisperResponse
	if err := json.NewDecoder(resp.Body).Decode(&whisperResp); err != nil {
		return nil, fmt.Errorf("failed to parse whisper-server response: %w", err)
	}

	segments := convertWhisperSegments(whisperResp.Segments)

	language := WhisperLangToCode(whisperResp.DetectedLanguage)
	if language == "" {
		language = WhisperLangToCode(whisperResp.Language)
	}
	if opts.SourceLang != "" {
		language = opts.SourceLang
	}

	return &TranscriptionResult{
		Text:     whisperResp.Text,
		Segments: segments,
		Language: language,
		Duration: whisperResp.Duration,
	}, nil
}

// convertWhisperSegments maps upstream verbose_json segments onto our Segment
// type. Responses from older community servers that expose only raw t0/t1
// fields (centiseconds) are converted with a unit-detection heuristic.
func convertWhisperSegments(raw []whisperSegment) []Segment {
	if len(raw) == 0 {
		return nil
	}

	scale := detectLegacyTimeScale(raw)

	segments := make([]Segment, 0, len(raw))
	for i := range raw {
		seg := &raw[i]
		start, end := seg.Start, seg.End
		if start == 0 && end == 0 && seg.T0 != nil && seg.T1 != nil && (*seg.T0 != 0 || *seg.T1 != 0) {
			start = *seg.T0 / scale
			end = *seg.T1 / scale
		}
		if end < start {
			end = start
		}

		converted := Segment{
			Start:        start,
			End:          end,
			Text:         strings.TrimSpace(seg.Text),
			NoSpeechProb: seg.NoSpeechProb,
			AvgLogprob:   seg.AvgLogprob,
		}
		if words := convertWhisperWords(seg.Words, start, end); len(words) > 0 {
			converted.Words = words
		}
		if converted.Text == "" && len(converted.Words) == 0 {
			continue
		}
		segments = append(segments, converted)
	}

	return segments
}

func convertWhisperWords(words []whisperWord, segStart, segEnd float64) []Word {
	out := make([]Word, 0, len(words))
	for _, w := range words {
		text := strings.TrimSpace(w.Word)
		if text == "" || w.Start == nil || w.End == nil {
			continue
		}
		start, end := *w.Start, *w.End
		if end < start {
			continue
		}
		start = clampf(start, segStart, segEnd)
		end = clampf(end, segStart, segEnd)
		if end-start <= 0 {
			continue
		}
		out = append(out, Word{Text: text, Start: start, End: end})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func clampf(v, lo, hi float64) float64 {
	if lo > hi {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// detectLegacyTimeScale guesses whether raw t0/t1 values are centiseconds
// (whisper.cpp native) or milliseconds by scoring which interpretation yields
// plausible segment spans.
func detectLegacyTimeScale(raw []whisperSegment) float64 {
	spans := make([]float64, 0, len(raw))
	for i := range raw {
		seg := &raw[i]
		if seg.T0 == nil || seg.T1 == nil {
			continue
		}
		if span := *seg.T1 - *seg.T0; span > 0 {
			spans = append(spans, span)
		}
	}
	if len(spans) == 0 {
		return 100
	}

	bestScale, bestScore := 100.0, -1
	for _, scale := range []float64{100, 1000} {
		score := 0
		for _, span := range spans {
			if s := span / scale; s >= 0.1 && s <= 90 {
				score++
			}
		}
		if score > bestScore {
			bestScale, bestScore = scale, score
		}
	}
	return bestScale
}

func (t *Transcriber) IsHealthy() bool {
	return isServiceHealthy(t.baseURL + "/health")
}

func newInferenceRequest(
	ctx context.Context,
	url string,
	audioPath string,
	opts TranscribeOptions,
) (*http.Request, string, func(), error) {
	file, err := os.Open(audioPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to open audio file: %w", err)
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := writer.FormDataContentType()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
		_ = file.Close()
		return nil, "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	go func() {
		closeWithError := func(err error) {
			_ = pipeWriter.CloseWithError(err)
		}

		part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
		if err != nil {
			closeWithError(fmt.Errorf("failed to create form file: %w", err))
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			closeWithError(fmt.Errorf("failed to copy file: %w", err))
			return
		}

		fields := map[string]string{
			"response_format": "verbose_json",
			"temperature":     strconv.FormatFloat(opts.Temperature, 'f', -1, 64),
			"vad":             strconv.FormatBool(opts.VADEnabled),
			"split_on_word":   strconv.FormatBool(opts.SplitOnWord),
		}
		if opts.TemperatureInc > 0 {
			fields["temperature_inc"] = strconv.FormatFloat(opts.TemperatureInc, 'f', -1, 64)
		}
		if opts.EntropyThold > 0 {
			fields["entropy_thold"] = strconv.FormatFloat(opts.EntropyThold, 'f', -1, 64)
		}
		if opts.LogProbThold != 0 {
			fields["logprob_thold"] = strconv.FormatFloat(opts.LogProbThold, 'f', -1, 64)
		}
		if opts.MaxContext > 0 {
			fields["max_context"] = strconv.Itoa(opts.MaxContext)
		}
		if opts.BeamSize > 0 {
			fields["beam_size"] = strconv.Itoa(opts.BeamSize)
		}
		if opts.BestOf > 0 {
			fields["best_of"] = strconv.Itoa(opts.BestOf)
		}
		if opts.MaxLen > 0 {
			fields["max_len"] = strconv.Itoa(opts.MaxLen)
		}
		if opts.SourceLang != "" {
			fields["language"] = opts.SourceLang
		}
		if opts.InitialPrompt != "" {
			fields["prompt"] = opts.InitialPrompt
			fields["carry_initial_prompt"] = "true"
		}

		for key, value := range fields {
			if err := writer.WriteField(key, value); err != nil {
				closeWithError(fmt.Errorf("failed to write field %q: %w", key, err))
				return
			}
		}

		if err := writer.Close(); err != nil {
			closeWithError(fmt.Errorf("failed to close multipart writer: %w", err))
			return
		}
		_ = pipeWriter.Close()
		_ = file.Close()
	}()

	cleanup := func() {
		_ = req.Body.Close()
	}

	return req, contentType, cleanup, nil
}
