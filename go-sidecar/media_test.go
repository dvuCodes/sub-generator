package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMediaDurationReturnsErrorForMissingFile verifies that a non-existent path returns an error.
func TestMediaDurationReturnsErrorForMissingFile(t *testing.T) {
	_, err := MediaDuration("/nonexistent/file.mp4")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestMediaDurationReturnsErrorForEmptyPath verifies that an empty path returns an error.
func TestMediaDurationReturnsErrorForEmptyPath(t *testing.T) {
	_, err := MediaDuration("")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

// TestMediaDurationReturnsErrorForDirectory verifies that a directory path returns an error.
func TestMediaDurationReturnsErrorForDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := MediaDuration(dir)
	if err == nil {
		t.Fatalf("expected error for directory path %q, got nil", dir)
	}
}

// TestMediaDurationReturnsErrorForTextFile verifies that a plain text file returns an error from ffprobe.
func TestMediaDurationReturnsErrorForTextFile(t *testing.T) {
	dir := t.TempDir()
	textFile := filepath.Join(dir, "not-a-video.txt")
	if err := os.WriteFile(textFile, []byte("this is not a media file"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := MediaDuration(textFile)
	if err == nil {
		t.Fatalf("expected error for text file %q, got nil", textFile)
	}
}
