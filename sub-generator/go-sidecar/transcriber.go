package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Transcriber struct {
	baseURL string
	client  *http.Client
}

func NewTranscriber(port int) *Transcriber {
	return &Transcriber{
		baseURL: fmt.Sprintf("http://localhost:%d", port),
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

func (t *Transcriber) Transcribe(videoPath string, sourceLang *string, modelSize string, beamSize int, vadFilter bool) (*TranscriptionResult, error) {
	// Open the video file
	file, err := os.Open(videoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open video file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add the file
	part, err := writer.CreateFormFile("file", filepath.Base(videoPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Add parameters
	_ = writer.WriteField("response_format", "json")
	_ = writer.WriteField("temperature", "0")

	if sourceLang != nil && *sourceLang != "" {
		_ = writer.WriteField("language", *sourceLang)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", t.baseURL+"/inference", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

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
