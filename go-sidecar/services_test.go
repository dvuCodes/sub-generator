package main

import (
	"errors"
	"os"
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

func TestShouldReuseManagedLlamaProcessRequiresTrackedProcess(t *testing.T) {
	if shouldReuseManagedLlamaProcess(nil, "gemma.gguf", "gemma.gguf", true) {
		t.Fatal("shouldReuseManagedLlamaProcess() = true, want false for untracked healthy service")
	}
}

func TestShouldReuseManagedLlamaProcessRequiresMatchingModelAndHealth(t *testing.T) {
	process := &os.Process{Pid: 1234}

	if !shouldReuseManagedLlamaProcess(process, "gemma.gguf", "gemma.gguf", true) {
		t.Fatal("shouldReuseManagedLlamaProcess() = false, want true for matching managed process")
	}
	if shouldReuseManagedLlamaProcess(process, "other.gguf", "gemma.gguf", true) {
		t.Fatal("shouldReuseManagedLlamaProcess() = true, want false for mismatched model")
	}
	if shouldReuseManagedLlamaProcess(process, "gemma.gguf", "gemma.gguf", false) {
		t.Fatal("shouldReuseManagedLlamaProcess() = true, want false for unhealthy process")
	}
}

func TestRejectUnmanagedHealthyServiceRejectsUntrackedHealthyProcess(t *testing.T) {
	err := rejectUnmanagedHealthyService("llama-server", 8081, nil, true)
	if err == nil {
		t.Fatal("rejectUnmanagedHealthyService() error = nil, want unmanaged service conflict")
	}

	message := err.Error()
	for _, fragment := range []string{
		"llama-server",
		"8081",
		"already responding",
		"not managed by SubGen",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("rejectUnmanagedHealthyService() error %q missing %q", message, fragment)
		}
	}
}

func TestRejectUnmanagedHealthyServiceAllowsTrackedOrStoppedProcess(t *testing.T) {
	if err := rejectUnmanagedHealthyService("whisper-server", 8080, nil, false); err != nil {
		t.Fatalf("rejectUnmanagedHealthyService(nil, false) error = %v, want nil", err)
	}

	process := &os.Process{Pid: 1234}
	if err := rejectUnmanagedHealthyService("whisper-server", 8080, process, true); err != nil {
		t.Fatalf("rejectUnmanagedHealthyService(process, true) error = %v, want nil", err)
	}
}
