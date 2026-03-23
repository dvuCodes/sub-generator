package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTranscribeForwardsConfiguredInferenceOptions(t *testing.T) {
	t.Helper()

	var gotLanguage string
	var gotBeamSize string
	var gotVADFilter string
	var gotResponseFormat string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}

		gotLanguage = r.FormValue("language")
		gotBeamSize = r.FormValue("beam_size")
		gotVADFilter = r.FormValue("vad_filter")
		gotResponseFormat = r.FormValue("response_format")

		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": "hello",
			"segments": []map[string]any{
				{"start": 0.0, "end": 1.0, "text": "hello"},
			},
		})
	}))
	defer server.Close()

	videoPath := writeTempVideoFile(t)
	sourceLang := "ja"

	transcriber := &Transcriber{
		baseURL: server.URL,
		client:  server.Client(),
	}

	_, err := transcriber.Transcribe(videoPath, &sourceLang, 7, false)
	if err != nil {
		t.Fatalf("Transcribe() error: %v", err)
	}

	if gotLanguage != "ja" {
		t.Fatalf("language = %q, want %q", gotLanguage, "ja")
	}
	if gotBeamSize != "7" {
		t.Fatalf("beam_size = %q, want %q", gotBeamSize, "7")
	}
	if gotVADFilter != "false" {
		t.Fatalf("vad_filter = %q, want %q", gotVADFilter, "false")
	}
	if gotResponseFormat != "verbose_json" {
		t.Fatalf("response_format = %q, want %q", gotResponseFormat, "verbose_json")
	}
}

func TestTranscribeParsesVerboseJSONSegmentTimestamps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": "hello world",
			"segments": []map[string]any{
				{"start": 0.15, "end": 3.52, "text": "hello world"},
			},
		})
	}))
	defer server.Close()

	transcriber := &Transcriber{
		baseURL: server.URL,
		client:  server.Client(),
	}

	result, err := transcriber.Transcribe(writeTempVideoFile(t), nil, 5, true)
	if err != nil {
		t.Fatalf("Transcribe() error: %v", err)
	}

	if len(result.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(result.Segments))
	}

	segment := result.Segments[0]
	if segment.Start != 0.15 || segment.End != 3.52 {
		t.Fatalf("segment timings = %#v, want start=0.15 end=3.52", segment)
	}
}

func TestTranscribeRequestsAutoDetectWhenSourceLanguageIsUnset(t *testing.T) {
	t.Helper()

	var gotLanguage string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}

		gotLanguage = r.FormValue("language")

		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": "hola",
			"segments": []map[string]any{
				{"start": 0.0, "end": 1.0, "text": "hola"},
			},
		})
	}))
	defer server.Close()

	transcriber := &Transcriber{
		baseURL: server.URL,
		client:  server.Client(),
	}

	_, err := transcriber.Transcribe(writeTempVideoFile(t), nil, 5, true)
	if err != nil {
		t.Fatalf("Transcribe() error: %v", err)
	}

	if gotLanguage != "auto" {
		t.Fatalf("language = %q, want %q for auto-detect", gotLanguage, "auto")
	}
}

func TestTranscribeCapturesDetectedLanguageFromVerboseJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":     "hola",
			"language": "es",
			"segments": []map[string]any{
				{"start": 0.0, "end": 1.0, "text": "hola"},
			},
		})
	}))
	defer server.Close()

	transcriber := &Transcriber{
		baseURL: server.URL,
		client:  server.Client(),
	}

	result, err := transcriber.Transcribe(writeTempVideoFile(t), nil, 5, true)
	if err != nil {
		t.Fatalf("Transcribe() error: %v", err)
	}

	if result.Language != "es" {
		t.Fatalf("language = %q, want %q", result.Language, "es")
	}
}

func TestTranscribeFallsBackToLegacyMillisecondSegments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text": "legacy",
			"segments": []map[string]any{
				{"t0": 250.0, "t1": 1750.0, "text": "legacy"},
			},
		})
	}))
	defer server.Close()

	transcriber := &Transcriber{
		baseURL: server.URL,
		client:  server.Client(),
	}

	result, err := transcriber.Transcribe(writeTempVideoFile(t), nil, 5, true)
	if err != nil {
		t.Fatalf("Transcribe() error: %v", err)
	}

	if len(result.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(result.Segments))
	}

	segment := result.Segments[0]
	if segment.Start != 0.25 || segment.End != 1.75 {
		t.Fatalf("legacy segment timings = %#v, want start=0.25 end=1.75", segment)
	}
}

func TestNewInferenceRequestStagesMultipartBodyOnDisk(t *testing.T) {
	videoPath := writeTempVideoFile(t)
	sourceLang := "ja"

	req, contentType, cleanup, err := newInferenceRequest(
		"http://localhost:8080/inference",
		videoPath,
		&sourceLang,
		7,
		false,
	)
	if err != nil {
		t.Fatalf("newInferenceRequest() error = %v", err)
	}

	bodyFile, ok := req.Body.(*os.File)
	if !ok {
		cleanup()
		t.Fatalf("request body type = %T, want *os.File for a staged multipart body", req.Body)
	}
	tempBodyPath := bodyFile.Name()
	defer cleanup()

	if contentType == "" {
		t.Fatal("contentType should not be empty")
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	body := string(bodyBytes)
	for _, fragment := range []string{
		`name="language"`,
		"\r\nja\r\n",
		`name="response_format"`,
		"\r\nverbose_json\r\n",
		`name="beam_size"`,
		"\r\n7\r\n",
		`name="vad_filter"`,
		"\r\nfalse\r\n",
		`filename="sample.mp4"`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("multipart body missing %q in %q", fragment, body)
		}
	}

	if _, err := os.Stat(tempBodyPath); err != nil {
		t.Fatalf("temp multipart body missing before cleanup: %v", err)
	}
}

func TestNewInferenceRequestCleanupRemovesMultipartTempFile(t *testing.T) {
	req, _, cleanup, err := newInferenceRequest(
		"http://localhost:8080/inference",
		writeTempVideoFile(t),
		nil,
		5,
		true,
	)
	if err != nil {
		t.Fatalf("newInferenceRequest() error = %v", err)
	}

	bodyFile, ok := req.Body.(*os.File)
	if !ok {
		cleanup()
		t.Fatalf("request body type = %T, want *os.File", req.Body)
	}
	tempBodyPath := bodyFile.Name()

	cleanup()

	if _, err := os.Stat(tempBodyPath); !os.IsNotExist(err) {
		t.Fatalf("cleanup should remove %q, stat err = %v", tempBodyPath, err)
	}
}

func TestNewTranscriberUsesDedicatedTransportWithoutKeepAlives(t *testing.T) {
	transcriber := NewTranscriber(8080)

	if transcriber.client.Timeout != 30*time.Minute {
		t.Fatalf("client timeout = %s, want %s", transcriber.client.Timeout, 30*time.Minute)
	}

	transport, ok := transcriber.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport type = %T, want *http.Transport", transcriber.client.Transport)
	}

	if !transport.DisableKeepAlives {
		t.Fatal("client transport should disable keep-alives for the local whisper-server")
	}
}

func writeTempVideoFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return path
}
