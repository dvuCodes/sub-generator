package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveOutputPath(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		format     string
		targetLang *string
		want       string
	}{
		{
			name:       "SRT without translation",
			input:      "C:/Videos/movie.mp4",
			format:     "srt",
			targetLang: nil,
			want:       "C:/Videos/movie.srt",
		},
		{
			name:       "SRT with Japanese target",
			input:      "C:/Videos/movie.mp4",
			format:     "srt",
			targetLang: strPtr("ja"),
			want:       "C:/Videos/movie.ja.srt",
		},
		{
			name:       "ASS with English target",
			input:      "/home/user/anime.mkv",
			format:     "ass",
			targetLang: strPtr("en"),
			want:       "/home/user/anime.en.ass",
		},
		{
			name:       "VTT without target",
			input:      "video.webm",
			format:     "vtt",
			targetLang: nil,
			want:       "video.vtt",
		},
		{
			name:       "Empty target lang string",
			input:      "test.mp4",
			format:     "srt",
			targetLang: strPtr(""),
			want:       "test.srt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveOutputPath(tt.input, tt.format, tt.targetLang)
			if got != tt.want {
				t.Errorf("DeriveOutputPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubtitleWriterSRT(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.srt")

	segments := []Segment{
		{Start: 0.0, End: 2.5, Text: "Hello world"},
		{Start: 3.0, End: 5.5, Text: "This is a test"},
		{Start: 6.0, End: 8.0, Text: "Goodbye"},
	}

	writer := NewSubtitleWriter()
	err := writer.Write(segments, outPath, "srt", nil)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	srtContent := string(content)

	// Verify SRT structure
	if !strings.Contains(srtContent, "Hello world") {
		t.Error("SRT missing 'Hello world'")
	}
	if !strings.Contains(srtContent, "This is a test") {
		t.Error("SRT missing 'This is a test'")
	}
	if !strings.Contains(srtContent, "Goodbye") {
		t.Error("SRT missing 'Goodbye'")
	}
	// SRT uses timestamps like 00:00:00,000
	if !strings.Contains(srtContent, "-->") {
		t.Error("SRT missing timestamp arrows")
	}
}

func TestSubtitleWriterVTT(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.vtt")

	segments := []Segment{
		{Start: 1.0, End: 3.5, Text: "First subtitle"},
		{Start: 4.0, End: 6.0, Text: "Second subtitle"},
	}

	writer := NewSubtitleWriter()
	err := writer.Write(segments, outPath, "vtt", nil)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	vttContent := string(content)

	// WebVTT must start with WEBVTT
	if !strings.HasPrefix(vttContent, "WEBVTT") {
		t.Error("VTT missing WEBVTT header")
	}
	if !strings.Contains(vttContent, "First subtitle") {
		t.Error("VTT missing 'First subtitle'")
	}
}

func TestSubtitleWriterASS(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.ass")

	segments := []Segment{
		{Start: 0.0, End: 2.0, Text: "テスト字幕"},
	}

	ja := "ja"
	writer := NewSubtitleWriter()
	err := writer.Write(segments, outPath, "ass", &ja)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	assContent := string(content)

	// ASS has section headers
	if !strings.Contains(assContent, "[Script Info]") {
		t.Error("ASS missing [Script Info] section")
	}
	if !strings.Contains(assContent, "テスト字幕") {
		t.Error("ASS missing Japanese text")
	}
}

func TestSubtitleWriterUnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.xyz")

	writer := NewSubtitleWriter()
	err := writer.Write([]Segment{}, outPath, "xyz", nil)
	if err == nil {
		t.Error("Expected error for unsupported format")
	}
}

func TestIsCJKLanguage(t *testing.T) {
	if !isCJKLanguage("ja") {
		t.Error("Japanese should be CJK")
	}
	if !isCJKLanguage("zh") {
		t.Error("Chinese should be CJK")
	}
	if !isCJKLanguage("ko") {
		t.Error("Korean should be CJK")
	}
	if isCJKLanguage("en") {
		t.Error("English should not be CJK")
	}
}

func TestDeriveTranscriptionLogPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"mp4", "C:/Videos/movie.mp4", "C:/Videos/movie.transcription.txt"},
		{"mkv", "/home/user/anime.mkv", "/home/user/anime.transcription.txt"},
		{"no directory", "video.webm", "video.transcription.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveTranscriptionLogPath(tt.input)
			if got != tt.want {
				t.Errorf("DeriveTranscriptionLogPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteTranscriptionLog(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.transcription.txt")

	segments := []Segment{
		{Start: 0.0, End: 2.5, Text: "Hello world"},
		{Start: 63.5, End: 65.0, Text: "Second segment"},
	}

	err := WriteTranscriptionLog(segments, outPath, "en")
	if err != nil {
		t.Fatalf("WriteTranscriptionLog() error: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "# Transcription (source language: en)") {
		t.Error("Missing header line")
	}
	if !strings.Contains(text, "[00:00:00.000 --> 00:00:02.500] Hello world") {
		t.Errorf("Missing or malformed first segment, got:\n%s", text)
	}
	if !strings.Contains(text, "[00:01:03.500 --> 00:01:05.000] Second segment") {
		t.Errorf("Missing or malformed second segment, got:\n%s", text)
	}
}

func TestWriteTranscriptionLogEmptySegments(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.transcription.txt")

	err := WriteTranscriptionLog([]Segment{}, outPath, "en")
	if err != nil {
		t.Fatalf("WriteTranscriptionLog() error: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "# Transcription (source language: en)") {
		t.Error("Missing header line for empty segments")
	}
	// Should only have header + blank line, no segment lines
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 1 {
		t.Errorf("Expected 1 line (header only) for empty segments, got %d lines", len(lines))
	}
}

func TestWriteTranscriptionLogCJK(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.transcription.txt")

	segments := []Segment{
		{Start: 0.0, End: 2.0, Text: "こんにちは世界"},
		{Start: 3.0, End: 5.0, Text: "テスト字幕"},
	}

	err := WriteTranscriptionLog(segments, outPath, "ja")
	if err != nil {
		t.Fatalf("WriteTranscriptionLog() error: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	text := string(content)

	if !strings.Contains(text, "source language: ja") {
		t.Error("Missing Japanese language in header")
	}
	if !strings.Contains(text, "こんにちは世界") {
		t.Error("Missing Japanese text: こんにちは世界")
	}
	if !strings.Contains(text, "テスト字幕") {
		t.Error("Missing Japanese text: テスト字幕")
	}
}

func strPtr(s string) *string {
	return &s
}
