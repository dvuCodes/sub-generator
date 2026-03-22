package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestTranscribeForwardsConfiguredInferenceOptions(t *testing.T) {
	t.Helper()

	var gotLanguage string
	var gotBeamSize string
	var gotVADFilter string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm error: %v", err)
		}

		gotLanguage = r.FormValue("language")
		gotBeamSize = r.FormValue("beam_size")
		gotVADFilter = r.FormValue("vad_filter")

		_ = json.NewEncoder(w).Encode(whisperResponse{
			Text: "hello",
			Segments: []whisperSegment{
				{Start: 0, End: 1000, Text: "hello"},
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
}

func TestNewInferenceRequestStreamsMultipartBody(t *testing.T) {
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
		`name="language"`,
		"\r\nja\r\n",
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
}

func writeTempVideoFile(t *testing.T) string {
	t.Helper()

	path := t.TempDir() + "/sample.mp4"
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return path
}
