package main

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestClassifyProbeError_DLLNotFound(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
	}{
		{"windows dll", "error: whisper.dll not found"},
		{"msys2 shared object", "error while loading shared libraries: whisper.dll: cannot open shared object file"},
		{"windows error code", "Application failed to start (0xc0000007b)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, attach := classifyProbeError(tc.stderr)
			if code != "binary_not_runnable" {
				t.Errorf("expected binary_not_runnable, got %q", code)
			}
			if !attach {
				t.Error("expected attachActions=true for DLL error")
			}
		})
	}
}

func TestClassifyProbeError_PermissionDenied(t *testing.T) {
	code, attach := classifyProbeError("permission denied")
	if code != "binary_not_runnable" {
		t.Errorf("expected binary_not_runnable, got %q", code)
	}
	if attach {
		t.Error("expected attachActions=false for permission error")
	}
}

func TestClassifyProbeError_Unknown(t *testing.T) {
	code, attach := classifyProbeError("some random error output")
	if code != "binary_not_runnable" {
		t.Errorf("expected binary_not_runnable, got %q", code)
	}
	if attach {
		t.Error("expected attachActions=false for unknown error")
	}
}

func TestResolveAction_Valid(t *testing.T) {
	registry := &ActionRegistry{
		actions: map[string]Action{
			"whisper/install_gpu_bundle": {
				ID:              "whisper/install_gpu_bundle",
				Label:           "Install GPU bundle",
				URL:             "https://example.com/gpu.zip",
				InstallDir:      "/tmp/test",
				StripComponents: 1,
				ExpectedBinary:  "whisper-server.exe",
				ServiceID:       "whisper",
			},
		},
	}

	action, err := registry.Resolve("whisper/install_gpu_bundle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.URL != "https://example.com/gpu.zip" {
		t.Errorf("expected URL https://example.com/gpu.zip, got %q", action.URL)
	}
	if action.ServiceID != "whisper" {
		t.Errorf("expected ServiceID whisper, got %q", action.ServiceID)
	}
}

func TestResolveAction_Unknown(t *testing.T) {
	registry := &ActionRegistry{actions: map[string]Action{}}
	_, err := registry.Resolve("nonexistent/action")
	if err == nil {
		t.Fatal("expected error for unknown action_id")
	}
}

func TestCheckFFmpeg_Missing(t *testing.T) {
	status := checkFFmpegWith(func(name, display string) error {
		return fmt.Errorf("%s not found in PATH", display)
	})
	if status.State != "action_required" {
		t.Errorf("expected action_required, got %q", status.State)
	}
	if len(status.Issues) == 0 {
		t.Fatal("expected at least one issue")
	}
	if status.Issues[0].Code != "not_in_path" {
		t.Errorf("expected not_in_path, got %q", status.Issues[0].Code)
	}
	if len(status.Actions) == 0 || status.Actions[0].Kind != "manual" {
		t.Error("expected manual action for ffmpeg")
	}
}

func TestCheckFFmpeg_MissingIncludesFrontendMetadata(t *testing.T) {
	status := checkFFmpegWith(func(name, display string) error {
		return fmt.Errorf("%s not found in PATH", display)
	})

	if status.DisplayName != "ffmpeg" {
		t.Fatalf("expected display name ffmpeg, got %q", status.DisplayName)
	}
	if status.RequiredFor != "transcription" {
		t.Fatalf("expected required_for transcription, got %q", status.RequiredFor)
	}
	if len(status.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(status.Actions))
	}
	if status.Actions[0].Guidance == "" {
		t.Fatal("expected manual action guidance to be populated")
	}
}

func TestCheckFFmpeg_Present(t *testing.T) {
	status := checkFFmpegWith(func(name, display string) error {
		return nil
	})
	if status.State != "ready" {
		t.Errorf("expected ready, got %q", status.State)
	}
}

func TestCheckService_MissingBinaryIncludesActionsAndFrontendMetadata(t *testing.T) {
	registry := NewActionRegistry()
	installDir := t.TempDir()
	missingBinary := filepath.Join(t.TempDir(), "whisper-server")

	status := checkService(
		"whisper",
		"whisper-server",
		"transcription",
		missingBinary,
		whisperDownloadActions(false, installDir),
		registry,
	)

	if status.DisplayName != "whisper-server" {
		t.Fatalf("expected display name whisper-server, got %q", status.DisplayName)
	}
	if status.RequiredFor != "transcription" {
		t.Fatalf("expected required_for transcription, got %q", status.RequiredFor)
	}
	if status.State != "action_required" {
		t.Fatalf("expected action_required, got %q", status.State)
	}
	if len(status.Issues) != 1 || status.Issues[0].Code != "binary_not_found" {
		t.Fatalf("expected binary_not_found issue, got %+v", status.Issues)
	}
	if len(status.Actions) == 0 {
		t.Fatal("expected install actions for missing binary")
	}
	if status.Actions[0].Kind != "archive" {
		t.Fatalf("expected archive action kind, got %q", status.Actions[0].Kind)
	}
	if _, err := registry.Resolve(status.Actions[0].ID); err != nil {
		t.Fatalf("expected registered action, got error: %v", err)
	}
}
