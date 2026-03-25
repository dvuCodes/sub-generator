package main

import (
	"fmt"
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

func TestCheckFFmpeg_Present(t *testing.T) {
	status := checkFFmpegWith(func(name, display string) error {
		return nil
	})
	if status.State != "ready" {
		t.Errorf("expected ready, got %q", status.State)
	}
}
