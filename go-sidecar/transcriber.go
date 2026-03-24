package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Transcriber struct {
	baseURL string
	client  *http.Client
}

func NewTranscriber(port int) *Transcriber {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true

	return &Transcriber{
		baseURL: localServiceBaseURL(port),
		client: &http.Client{
			Timeout:   30 * time.Minute, // Long timeout for large files
			Transport: transport,
		},
	}
}

// whisper-server /inference response format
type whisperResponse struct {
	Text     string           `json:"text"`
	Language string           `json:"language"`
	Segments []whisperSegment `json:"segments"`
}

type whisperSegment struct {
	StartSeconds      *float64 `json:"start"`
	EndSeconds        *float64 `json:"end"`
	StartMilliseconds *float64 `json:"t0"`
	EndMilliseconds   *float64 `json:"t1"`
	Text              string   `json:"text"`
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
		if isConnectionReset(err) {
			if !t.IsHealthy() {
				return nil, fmt.Errorf(
					"whisper-server crashed during transcription (connection reset). "+
						"This usually means the model ran out of memory. "+
						"Try a smaller model size or shorter input file",
				)
			}
			return nil, fmt.Errorf(
				"whisper-server closed the connection unexpectedly. "+
					"The file may be too large or in an unsupported format: %w", err,
			)
		}
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
		start, end := seg.subtitleTimes()
		segments[i] = Segment{
			Start: start,
			End:   end,
			Text:  seg.Text,
		}
	}

	return &TranscriptionResult{
		Text:     whisperResp.Text,
		Segments: segments,
		Language: whisperResp.Language,
	}, nil
}

func (t *Transcriber) IsHealthy() bool {
	return isServiceHealthy(t.baseURL + "/health")
}

func (s whisperSegment) subtitleTimes() (float64, float64) {
	if s.StartSeconds != nil || s.EndSeconds != nil {
		return derefFloat64(s.StartSeconds), derefFloat64(s.EndSeconds)
	}

	if s.StartMilliseconds != nil || s.EndMilliseconds != nil {
		return derefFloat64(s.StartMilliseconds) / 1000.0, derefFloat64(s.EndMilliseconds) / 1000.0
	}

	return 0, 0
}

func derefFloat64(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func newInferenceRequest(
	url string,
	videoPath string,
	sourceLang *string,
	beamSize int,
	vadFilter bool,
) (*http.Request, string, func(), error) {
	if beamSize > 8 {
		fmt.Fprintf(os.Stderr, "warning: beam_size %d exceeds whisper-server maximum of 8, capping to 8\n", beamSize)
		beamSize = 8
	}

	videoFile, err := os.Open(videoPath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to open video file: %w", err)
	}
	defer videoFile.Close()

	bodyFile, err := os.CreateTemp("", "subgen-whisper-upload-*.multipart")
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create multipart temp file: %w", err)
	}

	cleanup := func() {
		_ = bodyFile.Close()
		_ = os.Remove(bodyFile.Name())
	}

	writer := multipart.NewWriter(bodyFile)
	contentType := writer.FormDataContentType()

	part, err := writer.CreateFormFile("file", filepath.Base(videoPath))
	if err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, videoFile); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to copy file: %w", err)
	}
	if err := writer.WriteField("response_format", "verbose_json"); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to write response_format: %w", err)
	}
	if err := writer.WriteField("temperature", "0"); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to write temperature: %w", err)
	}
	if err := writer.WriteField("beam_size", strconv.Itoa(beamSize)); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to write beam_size: %w", err)
	}
	if err := writer.WriteField("vad", strconv.FormatBool(vadFilter)); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to write vad: %w", err)
	}
	language := "auto"
	if sourceLang != nil && *sourceLang != "" {
		language = *sourceLang
	}
	if err := writer.WriteField("language", language); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to write language: %w", err)
	}
	if language == "auto" {
		if err := writer.WriteField("detect_language", "true"); err != nil {
			cleanup()
			return nil, "", nil, fmt.Errorf("failed to write detect_language: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}
	if _, err := bodyFile.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to rewind multipart temp file: %w", err)
	}

	req, err := http.NewRequest("POST", url, bodyFile)
	if err != nil {
		cleanup()
		return nil, "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	return req, contentType, cleanup, nil
}

// isConnectionReset returns true when the error indicates the remote server
// forcibly closed the TCP connection — on Windows this surfaces as WSAECONNRESET
// (syscall.ECONNRESET / errno 10054), on Unix as ECONNRESET.
func isConnectionReset(err error) bool {
	if err == nil {
		return false
	}

	// Check for syscall-level ECONNRESET (works cross-platform).
	var sysErr *os.SyscallError
	if errors.As(err, &sysErr) {
		if sysErr.Err == syscall.ECONNRESET {
			return true
		}
	}

	// Check for net.OpError wrapping a reset.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Err != nil {
			if errors.As(opErr.Err, &sysErr) && sysErr.Err == syscall.ECONNRESET {
				return true
			}
		}
	}

	// Fallback: match the Windows error message text.
	msg := err.Error()
	return strings.Contains(msg, "forcibly closed") ||
		strings.Contains(msg, "connection reset by peer")
}
