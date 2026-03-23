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
