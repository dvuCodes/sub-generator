package main

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTranscribeForwardsConfiguredInferenceOptions(t *testing.T) {
	t.Helper()

	var gotForm map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("ParseMultipartForm error: %v", err)
		}
		gotForm = map[string]string{}
		for key := range r.MultipartForm.Value {
			gotForm[key] = r.FormValue(key)
		}

		start := 0.5
		end := 2.25
		wordStart := 0.5
		wordEnd := 1.1
		_ = json.NewEncoder(w).Encode(whisperResponse{
			Text:                        " hello ",
			DetectedLanguage:            "japanese",
			Duration:                    4.2,
			DetectedLanguageProbability: 0.98,
			Segments: []whisperSegment{
				{
					Start: start,
					End:   end,
					Text:  " こんにちは ",
					Words: []whisperWord{
						{Word: " こん", Start: &wordStart, End: &wordEnd},
						{Word: "にちは", Start: &wordEnd, End: &end},
					},
				},
			},
		})
	}))
	defer server.Close()

	audioPath := writeTempAudioFile(t)

	transcriber := &Transcriber{
		baseURL: server.URL,
		client:  server.Client(),
	}

	opts := DefaultTranscribeOptions()
	opts.SourceLang = "ja"
	opts.VADEnabled = true
	opts.InitialPrompt = "アニメ用語"

	result, err := transcriber.Transcribe(context.Background(), audioPath, opts)
	if err != nil {
		t.Fatalf("Transcribe() error: %v", err)
	}

	wantFields := map[string]string{
		"response_format":      "verbose_json",
		"language":             "ja",
		"temperature":          "0",
		"temperature_inc":      "0.2",
		"entropy_thold":        "2.8",
		"max_context":          "64",
		"beam_size":            "5",
		"best_of":              "5",
		"vad":                  "true",
		"split_on_word":        "true",
		"prompt":               "アニメ用語",
		"carry_initial_prompt": "true",
	}
	for key, want := range wantFields {
		if got := gotForm[key]; got != want {
			t.Fatalf("form field %q = %q, want %q (all fields: %v)", key, got, want, gotForm)
		}
	}

	if result.Language != "ja" {
		t.Fatalf("result.Language = %q, want %q", result.Language, "ja")
	}
	if len(result.Segments) != 1 {
		t.Fatalf("len(result.Segments) = %d, want 1", len(result.Segments))
	}
	seg := result.Segments[0]
	if seg.Text != "こんにちは" {
		t.Fatalf("segment text = %q, want %q", seg.Text, "こんにちは")
	}
	if len(seg.Words) != 2 || seg.Words[0].Text != "こん" {
		t.Fatalf("words not parsed: %#v", seg.Words)
	}
}

func TestTranscribeConvertsLegacyForkCentisecondTimestamps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"text":"legacy","segments":[` +
			`{"t0":120,"t1":480,"text":" legacy one "},` +
			`{"t0":500,"t1":1250,"text":"legacy two"}]}`))
	}))
	defer server.Close()

	transcriber := &Transcriber{
		baseURL: server.URL,
		client:  server.Client(),
	}

	result, err := transcriber.Transcribe(
		context.Background(), writeTempAudioFile(t), DefaultTranscribeOptions(),
	)
	if err != nil {
		t.Fatalf("Transcribe() error: %v", err)
	}

	if len(result.Segments) != 2 {
		t.Fatalf("len(segments) = %d, want 2", len(result.Segments))
	}
	first := result.Segments[0]
	if math.Abs(first.Start-1.2) > 1e-9 || math.Abs(first.End-4.8) > 1e-9 {
		t.Fatalf("first segment times = (%v, %v), want (1.2, 4.8)", first.Start, first.End)
	}
}

func TestDetectLegacyTimeScalePrefersPlausibleSpans(t *testing.T) {
	csSegments := []whisperSegment{
		{T0: floatPtr(100), T1: floatPtr(350)},
		{T0: floatPtr(400), T1: floatPtr(1900)},
	}
	if got := detectLegacyTimeScale(csSegments); got != 100 {
		t.Fatalf("detectLegacyTimeScale() = %v, want 100 (centiseconds)", got)
	}

	msSegments := []whisperSegment{
		// 30000 raw units: 300s (centiseconds) is implausible, 30s (ms) is not.
		{T0: floatPtr(1000), T1: floatPtr(31000)},
		{T0: floatPtr(32000), T1: floatPtr(47000)},
	}
	if got := detectLegacyTimeScale(msSegments); got != 1000 {
		t.Fatalf("detectLegacyTimeScale() = %v, want 1000 (milliseconds)", got)
	}
}

func TestConvertWhisperWordsDropsInvalidEntries(t *testing.T) {
	a, b := 1.0, 2.0
	below := 0.5
	invertedStart, invertedEnd := 3.0, 2.5
	words := convertWhisperWords([]whisperWord{
		{Word: " keep", Start: &a, End: &b},
		{Word: "", Start: &a, End: &b},
		{Word: "no-end", Start: &a},
		{Word: "inverted", Start: &invertedStart, End: &invertedEnd},
		{Word: "clamped", Start: &below, End: &b},
	}, a, b)

	if len(words) != 2 {
		t.Fatalf("len(words) = %d, want 2: %#v", len(words), words)
	}
	if words[0].Text != "keep" || words[0].Start != 1.0 {
		t.Fatalf("words[0] = %#v, want clamped entry starting at 1.0", words[0])
	}
}

func TestNewInferenceRequestStreamsMultipartBody(t *testing.T) {
	audioPath := writeTempAudioFile(t)

	req, contentType, cleanup, err := newInferenceRequest(
		context.Background(),
		"http://localhost:8080/inference",
		audioPath,
		DefaultTranscribeOptions(),
	)
	if err != nil {
		t.Fatalf("newInferenceRequest() error = %v", err)
	}
	defer cleanup()

	if got := reflect.TypeOf(req.Body).String(); got != "*io.PipeReader" {
		t.Fatalf("request body type = %s, want *io.PipeReader for a streaming body", got)
	}
	if contentType == "" {
		t.Fatal("contentType should not be empty")
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	body := string(bodyBytes)
	for _, fragment := range []string{
		`name="response_format"`,
		"\r\nverbose_json\r\n",
		`name="split_on_word"`,
		"\r\ntrue\r\n",
		`filename="sample.mp4"`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("multipart body missing %q in %q", fragment, body)
		}
	}
}

func TestWhisperLangToCode(t *testing.T) {
	cases := map[string]string{
		"japanese": "ja",
		"English":  "en",
		"korean":   "ko",
		"ja":       "ja",
		"unknown":  "",
	}
	for input, want := range cases {
		if got := WhisperLangToCode(input); got != want {
			t.Fatalf("WhisperLangToCode(%q) = %q, want %q", input, got, want)
		}
	}
}

func floatPtr(v float64) *float64 { return &v }

func writeTempAudioFile(t *testing.T) string {
	t.Helper()

	path := t.TempDir() + "/sample.mp4"
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return path
}

func TestNewTranscriberUsesDedicatedTransportWithoutKeepAlives(t *testing.T) {
	transcriber := NewTranscriber(8080)

	if transcriber.client.Timeout != 60*time.Minute {
		t.Fatalf("client timeout = %s, want %s", transcriber.client.Timeout, 60*time.Minute)
	}

	transport, ok := transcriber.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport type = %T, want *http.Transport", transcriber.client.Transport)
	}
	if !transport.DisableKeepAlives {
		t.Fatal("client transport should disable keep-alives for the local whisper-server")
	}
}

func TestNewInferenceRequestCapsBeamSizeAtEight(t *testing.T) {
	opts := DefaultTranscribeOptions()
	opts.BeamSize = 12

	req, _, cleanup, err := newInferenceRequest(
		context.Background(),
		"http://localhost:8080/inference",
		writeTempAudioFile(t),
		opts,
	)
	if err != nil {
		t.Fatalf("newInferenceRequest() error = %v", err)
	}
	defer cleanup()

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	body := string(bodyBytes)
	if !strings.Contains(body, "\r\n8\r\n") {
		t.Fatalf("beam_size should be capped to 8, got body: %s", body)
	}
	if strings.Contains(body, "\r\n12\r\n") {
		t.Fatal("beam_size 12 should have been capped to 8")
	}
}
