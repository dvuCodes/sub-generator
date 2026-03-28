package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMLBackendClientCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/capabilities" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CapabilitiesResponse{
			Type: "capabilities",
			Defaults: BackendDefaults{
				ASRBackend:         "faster_whisper",
				ASRModelID:         "deepdml/faster-whisper-large-v3-turbo-ct2",
				TranslationBackend: "nllb",
			},
			Backends: BackendCapabilities{
				ASR: []ASRBackendCapability{
					{
						ID:             "faster_whisper",
						DisplayName:    "Faster Whisper",
						Installed:      true,
						DefaultModelID: "deepdml/faster-whisper-large-v3-turbo-ct2",
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewMLBackendClient(server.URL)

	got, err := client.Capabilities()
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}

	if got.Defaults.ASRBackend != "faster_whisper" {
		t.Errorf("Defaults.ASRBackend = %q, want faster_whisper", got.Defaults.ASRBackend)
	}
	if len(got.Backends.ASR) != 1 || got.Backends.ASR[0].ID != "faster_whisper" {
		t.Fatalf("unexpected ASR backends: %#v", got.Backends.ASR)
	}
}

func TestMLBackendClientUsesExtendedRequestTimeout(t *testing.T) {
	client := NewMLBackendClient("http://127.0.0.1:8082")

	if client.client.Timeout < 30*time.Minute {
		t.Fatalf("client timeout = %s, want at least 30m", client.client.Timeout)
	}
}

func TestMLBackendClientAnnotateDiarization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/diarization/annotate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var req diarizationAnnotateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if len(req.Segments) != 1 {
			t.Fatalf("segments = %d, want 1", len(req.Segments))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(diarizationAnnotateResponse{
			Segments: []Segment{
				{
					Start:        req.Segments[0].Start,
					End:          req.Segments[0].End,
					Text:         req.Segments[0].Text,
					SpeakerID:    "spk_1",
					SpeakerLabel: "Speaker 1",
				},
			},
			SpeakerCount: 1,
		})
	}))
	defer server.Close()

	client := NewMLBackendClient(server.URL)
	segments, speakerCount, err := client.AnnotateDiarization("audio.wav", []Segment{{Start: 0, End: 1, Text: "hello"}})
	if err != nil {
		t.Fatalf("AnnotateDiarization() error = %v", err)
	}

	if speakerCount != 1 {
		t.Errorf("speakerCount = %d, want 1", speakerCount)
	}
	if len(segments) != 1 || segments[0].SpeakerLabel != "Speaker 1" {
		t.Fatalf("unexpected segments: %#v", segments)
	}
}

func TestMLBackendClientTranscribe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/asr/transcribe" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var req asrTranscribeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.ModelID != "deepdml/faster-whisper-large-v3-turbo-ct2" {
			t.Fatalf("ModelID = %q, want default faster-whisper model", req.ModelID)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TranscriptionResult{
			Text: "hello world",
			Segments: []Segment{
				{Start: 0, End: 1, Text: "hello"},
			},
			Language: "en",
		})
	}))
	defer server.Close()

	client := NewMLBackendClient(server.URL)
	result, err := client.Transcribe("clip.mp4", nil, "deepdml/faster-whisper-large-v3-turbo-ct2", 5, true)
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}

	if result.Language != "en" || len(result.Segments) != 1 {
		t.Fatalf("unexpected transcription result: %#v", result)
	}
}

func TestMLBackendClientTranslateSegments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/translation/translate_segments" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var req translateSegmentsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.TargetLang != "eng_Latn" {
			t.Fatalf("TargetLang = %q, want eng_Latn", req.TargetLang)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(translateSegmentsResponse{
			Segments: []Segment{
				{Start: 0, End: 1, Text: "hello"},
			},
		})
	}))
	defer server.Close()

	client := NewMLBackendClient(server.URL)
	segments, err := client.TranslateSegments([]Segment{{Start: 0, End: 1, Text: "hola"}}, "spa_Latn", "eng_Latn", "JustFrederik/nllb-200-distilled-600M-ct2-int8")
	if err != nil {
		t.Fatalf("TranslateSegments() error = %v", err)
	}

	if len(segments) != 1 || segments[0].Text != "hello" {
		t.Fatalf("unexpected translated segments: %#v", segments)
	}
}
