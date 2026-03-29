package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestPipelineRunStopsServicesOnValidationFailure(t *testing.T) {
	cleanupCalled := false
	pipeline := &Pipeline{
		cleanupServices: func() {
			cleanupCalled = true
		},
	}

	pipeline.Run(Command{})

	if !cleanupCalled {
		t.Fatal("Pipeline.Run() did not stop managed services on validation failure")
	}
}

func TestValidateTranscriptionResultRejectsEmptySegments(t *testing.T) {
	err := validateTranscriptionResult(&TranscriptionResult{})
	if err == nil {
		t.Fatal("validateTranscriptionResult() error = nil, want no-speech guidance")
	}

	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "no speech") {
		t.Fatalf("validateTranscriptionResult() error = %q, want no speech guidance", err.Error())
	}
}

func TestValidateTranscriptionResultExplainsMissingTimestampsWhenTextExists(t *testing.T) {
	err := validateTranscriptionResult(&TranscriptionResult{
		Text: "hello world",
	})
	if err == nil {
		t.Fatal("validateTranscriptionResult() error = nil, want timestamp guidance")
	}

	message := strings.ToLower(err.Error())
	for _, fragment := range []string{"timestamp", "segments"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("validateTranscriptionResult() error = %q missing %q", err.Error(), fragment)
		}
	}
}

func TestValidateTranscriptionResultAllowsTimestampedSegments(t *testing.T) {
	err := validateTranscriptionResult(&TranscriptionResult{
		Segments: []Segment{{Start: 0, End: 1, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("validateTranscriptionResult() error = %v, want nil", err)
	}
}

func TestValidateTranscriptionResultDropsUnusableSegmentsWhenTimedSegmentsRemain(t *testing.T) {
	result := &TranscriptionResult{
		Text: "hello there",
		Segments: []Segment{
			{Start: 0, End: 1, Text: "hello"},
			{Start: 1109, End: 1109, Text: "there"},
		},
	}

	err := validateTranscriptionResult(result)
	if err != nil {
		t.Fatalf("validateTranscriptionResult() error = %v, want nil", err)
	}

	if len(result.Segments) != 1 {
		t.Fatalf("segments = %d, want 1 usable segment", len(result.Segments))
	}

	if got := result.Segments[0]; got.Start != 0 || got.End != 1 || got.Text != "hello" {
		t.Fatalf("remaining segment = %#v, want the valid timed segment to be preserved", got)
	}
}

func TestValidateTranscriptionResultDropsOverlongSegmentsWhenTimedSegmentsRemain(t *testing.T) {
	result := &TranscriptionResult{
		Text: "hello there",
		Segments: []Segment{
			{Start: 0, End: 1, Text: "hello"},
			{Start: 10, End: 700, Text: "there"},
		},
	}

	err := validateTranscriptionResult(result)
	if err != nil {
		t.Fatalf("validateTranscriptionResult() error = %v, want nil", err)
	}

	if len(result.Segments) != 1 {
		t.Fatalf("segments = %d, want 1 usable segment", len(result.Segments))
	}

	if got := result.Segments[0]; got.Start != 0 || got.End != 1 || got.Text != "hello" {
		t.Fatalf("remaining segment = %#v, want the valid timed segment to be preserved", got)
	}
}

func TestValidateTranscriptionResultRejectsInvalidSegmentTimings(t *testing.T) {
	err := validateTranscriptionResult(&TranscriptionResult{
		Text: "hello world",
		Segments: []Segment{
			{Start: 1, End: 1, Text: "hello"},
		},
	})
	if err == nil {
		t.Fatal("validateTranscriptionResult() error = nil, want invalid timing guidance")
	}

	message := strings.ToLower(err.Error())
	for _, fragment := range []string{"timing", "segment"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("validateTranscriptionResult() error = %q missing %q", err.Error(), fragment)
		}
	}
}

func TestPipelineTranscribeUsesOriginalInputWhenInitialResultIsValid(t *testing.T) {
	attempts := make([]string, 0, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/asr/transcribe" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var req asrTranscribeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		attempts = append(attempts, req.InputVideo)

		result := TranscriptionResult{
			Text: "hello world",
			Segments: []Segment{
				{Start: 0, End: 1.5, Text: "hello world"},
			},
			Language: "en",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(server.URL) error = %v", err)
	}
	port, err := strconv.Atoi(parsedURL.Port())
	if err != nil {
		t.Fatalf("strconv.Atoi(parsedURL.Port()) error = %v", err)
	}

	pipeline := &Pipeline{
		svcManager: NewServiceManager(ServiceConfig{MLBackendPort: port}),
	}

	result, err := pipeline.transcribe(
		Command{
			InputVideo: "video.mp4",
			BeamSize:   5,
			VADFilter:  true,
		},
		"faster_whisper",
		"deepdml/faster-whisper-large-v3-turbo-ct2",
	)
	if err != nil {
		t.Fatalf("Pipeline.transcribe() error = %v, want nil", err)
	}

	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(attempts))
	}
	if attempts[0] != "video.mp4" {
		t.Fatalf("attempt order = %#v, want [video.mp4]", attempts)
	}
	if len(result.Segments) != 1 || result.Segments[0].End != 1.5 {
		t.Fatalf("result = %#v, want validated original-input segments", result)
	}
}

func TestPipelineTranscribeRetriesWithoutVADWhenInitialResultDropsHugeSegments(t *testing.T) {
	type attempt struct {
		inputVideo string
		vadFilter  bool
	}

	attempts := make([]attempt, 0, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/asr/transcribe" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var req asrTranscribeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		attempts = append(attempts, attempt{
			inputVideo: req.InputVideo,
			vadFilter:  req.VADFilter,
		})

		result := TranscriptionResult{
			Text: "hello there",
			Segments: []Segment{
				{Start: 0, End: 1, Text: "hello"},
				{Start: 12, End: 569, Text: "collapsed giant segment"},
				{Start: 2, End: 3, Text: "there"},
			},
			Language: "ja",
		}
		if !req.VADFilter {
			result.Segments = []Segment{
				{Start: 0, End: 2, Text: "hello"},
				{Start: 2, End: 4, Text: "there"},
				{Start: 4, End: 8, Text: "tail recovered"},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(server.URL) error = %v", err)
	}
	port, err := strconv.Atoi(parsedURL.Port())
	if err != nil {
		t.Fatalf("strconv.Atoi(parsedURL.Port()) error = %v", err)
	}

	pipeline := &Pipeline{
		svcManager: NewServiceManager(ServiceConfig{MLBackendPort: port}),
	}

	result, err := pipeline.transcribe(
		Command{
			InputVideo: "video.mp4",
			BeamSize:   5,
			VADFilter:  true,
		},
		"faster_whisper",
		"deepdml/faster-whisper-large-v3-turbo-ct2",
	)
	if err != nil {
		t.Fatalf("Pipeline.transcribe() error = %v, want nil", err)
	}

	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(attempts))
	}
	if attempts[0] != (attempt{inputVideo: "video.mp4", vadFilter: true}) {
		t.Fatalf("attempts[0] = %#v, want vad-enabled initial request", attempts[0])
	}
	if attempts[1] != (attempt{inputVideo: "video.mp4", vadFilter: false}) {
		t.Fatalf("attempts[1] = %#v, want vad-disabled retry", attempts[1])
	}
	if len(result.Segments) != 3 || result.Segments[2].Text != "tail recovered" {
		t.Fatalf("result = %#v, want vad-disabled retry result", result)
	}
}

func TestRetryImprovesTranscriptionRejectsLargeTailRegression(t *testing.T) {
	baseline := transcriptionValidation{
		KeptSegments:    2,
		LastKeptEnd:     180,
		UsableDuration:  20,
		DroppedSegments: 1,
	}
	candidate := transcriptionValidation{
		KeptSegments:   3,
		LastKeptEnd:    15,
		UsableDuration: 15,
	}

	if retryImprovesTranscription(candidate, baseline) {
		t.Fatal("retryImprovesTranscription() = true, want false when tail coverage regresses badly")
	}
}

func TestPipelineTranscribeKeepsOriginalInputWhenRetryingWithoutVAD(t *testing.T) {
	type attempt struct {
		inputVideo string
		vadFilter  bool
	}

	attempts := make([]attempt, 0, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/asr/transcribe" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var req asrTranscribeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		attempts = append(attempts, attempt{
			inputVideo: req.InputVideo,
			vadFilter:  req.VADFilter,
		})

		result := TranscriptionResult{
			Text:     "baseline",
			Language: "ja",
			Segments: []Segment{
				{Start: 0, End: 10, Text: "intro"},
				{Start: 20, End: 90, Text: "collapsed giant segment"},
				{Start: 170, End: 180, Text: "late baseline tail"},
			},
		}

		if req.InputVideo == "video.mp4" && !req.VADFilter {
			result.Text = "raw retry"
			result.Segments = []Segment{
				{Start: 0, End: 5, Text: "short 1"},
				{Start: 5, End: 10, Text: "short 2"},
				{Start: 10, End: 15, Text: "short 3"},
				{Start: 185, End: 190, Text: "recovered tail"},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(server.URL) error = %v", err)
	}
	port, err := strconv.Atoi(parsedURL.Port())
	if err != nil {
		t.Fatalf("strconv.Atoi(parsedURL.Port()) error = %v", err)
	}

	pipeline := &Pipeline{
		svcManager: NewServiceManager(ServiceConfig{MLBackendPort: port}),
	}

	result, err := pipeline.transcribe(
		Command{
			InputVideo: "video.mp4",
			BeamSize:   5,
			VADFilter:  true,
		},
		"faster_whisper",
		"deepdml/faster-whisper-large-v3-turbo-ct2",
	)
	if err != nil {
		t.Fatalf("Pipeline.transcribe() error = %v, want nil", err)
	}

	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(attempts))
	}
	if attempts[0] != (attempt{inputVideo: "video.mp4", vadFilter: true}) {
		t.Fatalf("attempts[0] = %#v, want initial transcription on original input", attempts[0])
	}
	if attempts[1] != (attempt{inputVideo: "video.mp4", vadFilter: false}) {
		t.Fatalf("attempts[1] = %#v, want original-input retry without VAD", attempts[1])
	}
	if len(result.Segments) != 4 || result.Segments[3].Text != "recovered tail" {
		t.Fatalf("result = %#v, want original-input retry result with recovered tail", result)
	}
}
