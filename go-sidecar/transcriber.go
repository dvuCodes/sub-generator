package main

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Transcriber struct {
	baseURL string
	client  *http.Client
}

func NewTranscriber(port int) *Transcriber {
	return &Transcriber{
		baseURL: localServiceBaseURL(port),
		client:  &http.Client{Timeout: 30 * time.Minute}, // Long timeout for large files
	}
}

// whisper-server /inference response format
type whisperResponse struct {
	Text     string           `json:"text"`
	Segments []whisperSegment `json:"segments"`
}

type whisperSegment struct {
	Start float64 `json:"t0"`
	End   float64 `json:"t1"`
	Text  string  `json:"text"`
}

func (t *Transcriber) Transcribe(videoPath string, sourceLang *string, beamSize int, vadFilter bool) (*TranscriptionResult, error) {
	// Open the video file
	req, contentType, cleanup, err := newInferenceRequest(
		t.baseURL+"/inference",
		videoPath,
		sourceLang,
		beamSize,
		vadFilter,
	)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	req.Header.Set("Content-Type", contentType)

	// Send request
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whisper-server request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("whisper-server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var whisperResp whisperResponse
	if err := json.NewDecoder(resp.Body).Decode(&whisperResp); err != nil {
		return nil, fmt.Errorf("failed to parse whisper-server response: %w", err)
	}

	// Convert to our segment format
	segments := make([]Segment, len(whisperResp.Segments))
	for i, seg := range whisperResp.Segments {
		segments[i] = Segment{
			Start: seg.Start / 1000.0, // whisper-server returns ms, convert to seconds
			End:   seg.End / 1000.0,
			Text:  seg.Text,
		}
	}

	return &TranscriptionResult{
		Text:     whisperResp.Text,
		Segments: segments,
	}, nil
}

func (t *Transcriber) IsHealthy() bool {
	return isServiceHealthy(t.baseURL + "/health")
}

func newInferenceRequest(
	url string,
	videoPath string,
	sourceLang *string,
	beamSize int,
	vadFilter bool,
) (*http.Request, string, func(), error) {
	file, err := os.Open(videoPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to open video file: %w", err)
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := writer.FormDataContentType()

	req, err := http.NewRequest("POST", url, pipeReader)
	if err != nil {
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
		_ = file.Close()
		return nil, "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	go func() {
		defer file.Close()

		closeWithError := func(err error) {
			_ = pipeWriter.CloseWithError(err)
		}

		part, err := writer.CreateFormFile("file", filepath.Base(videoPath))
		if err != nil {
			closeWithError(fmt.Errorf("failed to create form file: %w", err))
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			closeWithError(fmt.Errorf("failed to copy file: %w", err))
			return
		}
		if err := writer.WriteField("response_format", "json"); err != nil {
			closeWithError(fmt.Errorf("failed to write response_format: %w", err))
			return
		}
		if err := writer.WriteField("temperature", "0"); err != nil {
			closeWithError(fmt.Errorf("failed to write temperature: %w", err))
			return
		}
		if err := writer.WriteField("beam_size", strconv.Itoa(beamSize)); err != nil {
			closeWithError(fmt.Errorf("failed to write beam_size: %w", err))
			return
		}
		if err := writer.WriteField("vad_filter", strconv.FormatBool(vadFilter)); err != nil {
			closeWithError(fmt.Errorf("failed to write vad_filter: %w", err))
			return
		}
		if sourceLang != nil && *sourceLang != "" {
			if err := writer.WriteField("language", *sourceLang); err != nil {
				closeWithError(fmt.Errorf("failed to write language: %w", err))
				return
			}
		}
		if err := writer.Close(); err != nil {
			closeWithError(fmt.Errorf("failed to close multipart writer: %w", err))
			return
		}
		_ = pipeWriter.Close()
	}()

	return req, contentType, func() {
		_ = req.Body.Close()
	}, nil
}
