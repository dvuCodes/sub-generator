package main

import (
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

func TestTranscribeWithValidationFallbackRetriesRawInputAfterInvalidEnhancedResult(t *testing.T) {
	attempts := make([]string, 0, 2)
	fallbackReason := ""

	result, err := transcribeWithValidationFallback(
		"enhanced.wav",
		"video.mp4",
		func(path string) (*TranscriptionResult, error) {
			attempts = append(attempts, path)
			if path == "enhanced.wav" {
				return &TranscriptionResult{
					Text: "hello world",
					Segments: []Segment{
						{Start: 1, End: 1, Text: "hello"},
					},
				}, nil
			}

			return &TranscriptionResult{
				Text: "hello world",
				Segments: []Segment{
					{Start: 0, End: 1.5, Text: "hello world"},
				},
			}, nil
		},
		func(reason string) {
			fallbackReason = reason
		},
	)
	if err != nil {
		t.Fatalf("transcribeWithValidationFallback() error = %v, want nil", err)
	}

	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(attempts))
	}
	if attempts[0] != "enhanced.wav" || attempts[1] != "video.mp4" {
		t.Fatalf("attempt order = %#v, want [enhanced.wav video.mp4]", attempts)
	}
	if !strings.Contains(strings.ToLower(fallbackReason), "timing") {
		t.Fatalf("fallback reason = %q, want invalid timing guidance", fallbackReason)
	}
	if len(result.Segments) != 1 || result.Segments[0].Start != 0 || result.Segments[0].End != 1.5 {
		t.Fatalf("result = %#v, want validated raw-input segments", result)
	}
}

func TestTranscribeWithValidationFallbackSkipsRetryWhenEnhancedAudioSucceeds(t *testing.T) {
	attempts := make([]string, 0, 1)
	fallbackCalled := false

	result, err := transcribeWithValidationFallback(
		"enhanced.wav",
		"video.mp4",
		func(path string) (*TranscriptionResult, error) {
			attempts = append(attempts, path)
			return &TranscriptionResult{
				Text: "hello world",
				Segments: []Segment{
					{Start: 0.1, End: 1.5, Text: "hello world"},
				},
			}, nil
		},
		func(string) {
			fallbackCalled = true
		},
	)
	if err != nil {
		t.Fatalf("transcribeWithValidationFallback() error = %v, want nil", err)
	}

	if len(attempts) != 1 || attempts[0] != "enhanced.wav" {
		t.Fatalf("attempts = %#v, want only enhanced.wav", attempts)
	}
	if fallbackCalled {
		t.Fatal("fallback callback should not be called when enhanced audio succeeds")
	}
	if len(result.Segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(result.Segments))
	}
}

func TestTranscribeWithValidationFallbackReturnsOriginalErrorWhenRawRetryAlsoHasNoSpeech(t *testing.T) {
	attempts := make([]string, 0, 2)
	fallbackCalls := 0

	_, err := transcribeWithValidationFallback(
		"enhanced.wav",
		"video.mp4",
		func(path string) (*TranscriptionResult, error) {
			attempts = append(attempts, path)
			return &TranscriptionResult{}, nil
		},
		func(string) {
			fallbackCalls++
		},
	)
	if err == nil {
		t.Fatal("transcribeWithValidationFallback() error = nil, want no-speech guidance")
	}

	if len(attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(attempts))
	}
	if attempts[0] != "enhanced.wav" || attempts[1] != "video.mp4" {
		t.Fatalf("attempt order = %#v, want [enhanced.wav video.mp4]", attempts)
	}
	if fallbackCalls != 1 {
		t.Fatalf("fallbackCalls = %d, want 1", fallbackCalls)
	}

	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "no speech") {
		t.Fatalf("error = %q, want no speech guidance", err.Error())
	}
}
