package main

import (
	"fmt"
	"testing"
)

// TestModelDownloadURL verifies that URLs are composed correctly for known model sizes.
func TestModelDownloadURL(t *testing.T) {
	tests := []struct {
		modelSize  string
		wantSuffix string
	}{
		{"base", "ggml-base.bin"},
		{"large-v3", "ggml-large-v3.bin"},
		{"turbo", "ggml-large-v3-turbo.bin"},
	}

	baseURL := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

	for _, tt := range tests {
		t.Run(tt.modelSize, func(t *testing.T) {
			got := ModelDownloadURL(tt.modelSize)
			want := baseURL + tt.wantSuffix
			if got != want {
				t.Errorf("ModelDownloadURL(%q) = %q, want %q", tt.modelSize, got, want)
			}
		})
	}
}

// TestFormatBytes verifies human-readable byte formatting.
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{int64(1.5 * 1024 * 1024 * 1024), "1.5 GB"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.bytes), func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
