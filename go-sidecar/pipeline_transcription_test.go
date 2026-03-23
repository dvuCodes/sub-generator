package main

import (
	"strings"
	"testing"
)

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
