package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateWhisperStartupReturnsSetupGuidanceWhenAssetsAreMissing(t *testing.T) {
	root := t.TempDir()

	err := validateWhisperStartup([]string{root}, "whisper-server", filepath.Join("models", "ggml-base.bin"))
	if err == nil {
		t.Fatal("validateWhisperStartup() error = nil, want setup guidance")
	}

	message := err.Error()
	expectedDir := filepath.Join(root, "services", "whisper-server")
	normalizedMessage := strings.ReplaceAll(message, `\\`, `/`)

	for _, fragment := range []string{
		"whisper-server setup is incomplete",
		filepath.ToSlash(expectedDir),
		whisperExecutableName(),
		"ggml-base.bin",
		"README.md",
		"available on PATH",
	} {
		if !strings.Contains(normalizedMessage, filepath.ToSlash(fragment)) {
			t.Fatalf("validateWhisperStartup() error %q missing %q", message, fragment)
		}
	}
}

func TestNormalizeWhisperStartupErrorReturnsSetupGuidanceForPathFailure(t *testing.T) {
	root := t.TempDir()

	err := normalizeWhisperStartupError(
		[]string{root},
		"whisper-server",
		filepath.Join("models", "ggml-base.bin"),
		errors.New(`whisper-server executable "whisper-server" not found in PATH`),
	)
	if err == nil {
		t.Fatal("normalizeWhisperStartupError() error = nil, want setup guidance")
	}

	message := strings.ReplaceAll(err.Error(), `\\`, `/`)
	expectedDir := filepath.ToSlash(filepath.Join(root, "services", "whisper-server"))

	for _, fragment := range []string{
		"whisper-server setup is incomplete",
		expectedDir,
		whisperExecutableName(),
		"ggml-base.bin",
		"README.md",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("normalizeWhisperStartupError() error %q missing %q", err.Error(), fragment)
		}
	}
}
