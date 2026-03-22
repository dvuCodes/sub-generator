package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func writeTempVideoFile(t *testing.T) string {
	t.Helper()

	path := t.TempDir() + "/sample.mp4"
	if err := os.WriteFile(path, []byte("video"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return path
}
