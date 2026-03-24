package main

import "testing"

func TestModelSpeedRatioKnownModels(t *testing.T) {
	tests := []struct {
		model  string
		hasGPU bool
		want   float64
	}{
		{"tiny", true, 30.0},
		{"base", true, 15.0},
		{"small", true, 8.0},
		{"medium", true, 4.0},
		{"large-v3", true, 2.0},
		{"turbo", true, 10.0},
		{"tiny", false, 8.0},
		{"base", false, 4.0},
		{"small", false, 2.0},
		{"medium", false, 0.8},
		{"large-v3", false, 0.3},
		{"turbo", false, 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := ModelSpeedRatio(tt.model, tt.hasGPU)
			if got != tt.want {
				t.Errorf("ModelSpeedRatio(%q, %v) = %v, want %v", tt.model, tt.hasGPU, got, tt.want)
			}
		})
	}
}

func TestModelSpeedRatioUnknownModel(t *testing.T) {
	got := ModelSpeedRatio("nonexistent", true)
	if got != 0 {
		t.Errorf("ModelSpeedRatio unknown model = %v, want 0", got)
	}
}

func TestEstimateTranscriptionSeconds(t *testing.T) {
	got := EstimateTranscriptionSeconds(60.0, "base", true)
	if got != 4.0 {
		t.Errorf("EstimateTranscriptionSeconds(60, base, GPU) = %v, want 4.0", got)
	}
}

func TestEstimateTranscriptionSecondsZeroDuration(t *testing.T) {
	got := EstimateTranscriptionSeconds(0, "base", true)
	if got != 0 {
		t.Errorf("EstimateTranscriptionSeconds(0, ...) = %v, want 0", got)
	}
}

func TestEstimateTranscriptionSecondsUnknownModel(t *testing.T) {
	got := EstimateTranscriptionSeconds(60, "nonexistent", true)
	if got != 0 {
		t.Errorf("EstimateTranscriptionSeconds with unknown model = %v, want 0", got)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		secs float64
		want string
	}{
		{0, "0:00"},
		{5, "0:05"},
		{59, "0:59"},
		{60, "1:00"},
		{90, "1:30"},
		{3599, "59:59"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{-5, "0:00"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.secs)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.secs, got, tt.want)
			}
		})
	}
}
