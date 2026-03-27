package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestMLBackendDownloadActions(t *testing.T) {
	actions := mlBackendDownloadActions(filepath.Join(t.TempDir(), "services", "ml-backend"))
	if len(actions) != 2 {
		t.Fatalf("expected 2 ml-backend actions, got %d", len(actions))
	}
	if actions[0].ServiceID != "ml-backend" {
		t.Fatalf("expected ServiceID ml-backend, got %q", actions[0].ServiceID)
	}
	if actions[0].Kind != "command" {
		t.Fatalf("expected first action to be command, got %q", actions[0].Kind)
	}
	if actions[1].Kind != "manual" {
		t.Fatalf("expected second action to be manual, got %q", actions[1].Kind)
	}
}

func TestCheckSetupIncludesMLBackendService(t *testing.T) {
	root := t.TempDir()
	registry := NewActionRegistry()

	result := CheckSetup(resolveServiceConfig(root), registry)

	found := false
	for _, service := range result.Services {
		if service.ID != "ml-backend" {
			continue
		}
		found = true
		if service.RequiredFor != "transcription" {
			t.Fatalf("expected ml-backend required_for transcription, got %q", service.RequiredFor)
		}
		if service.State != "action_required" {
			t.Fatalf("expected ml-backend action_required when missing, got %q", service.State)
		}
	}

	if !found {
		t.Fatal("expected ml-backend to be reported in setup status")
	}
}

func TestPreferredMLBackendInstallDirPrefersCompleteBackendOverPlaceholder(t *testing.T) {
	root := t.TempDir()

	placeholderDir := filepath.Join(root, "services", "ml-backend")
	if err := os.MkdirAll(filepath.Join(placeholderDir, "models"), 0o755); err != nil {
		t.Fatalf("mkdir placeholder: %v", err)
	}

	pythonBackendDir := filepath.Join(root, "python-backend")
	if err := os.MkdirAll(pythonBackendDir, 0o755); err != nil {
		t.Fatalf("mkdir python-backend: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pythonBackendDir, "service.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write service.py: %v", err)
	}

	if got := preferredMLBackendInstallDir([]string{root}); got != pythonBackendDir {
		t.Fatalf("expected python-backend dir %q, got %q", pythonBackendDir, got)
	}
}

func TestCheckMLBackendReportsMissingFasterWhisperDependency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/capabilities" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CapabilitiesResponse{
			Type: "capabilities",
			Defaults: BackendDefaults{
				ASRBackend:         "faster_whisper",
				ASRModelID:         "deepdml/faster-whisper-large-v3-turbo-ct2",
				TranslationBackend: "nllb",
			},
			Backends: BackendCapabilities{
				ASR: []ASRBackendCapability{
					{
						ID:          "faster_whisper",
						DisplayName: "Faster Whisper",
						Installed:   false,
					},
				},
			},
		})
	}))
	defer server.Close()

	root := t.TempDir()
	pythonBackendDir := filepath.Join(root, "python-backend")
	if err := os.MkdirAll(pythonBackendDir, 0o755); err != nil {
		t.Fatalf("mkdir python-backend: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pythonBackendDir, "service.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write service.py: %v", err)
	}

	portText := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse port %q: %v", portText, err)
	}

	status := checkMLBackend(
		ServiceConfig{SearchRoots: []string{root}, MLBackendPort: port},
		NewActionRegistry(),
		mlBackendDownloadActions(pythonBackendDir),
	)

	if status.State != "action_required" {
		t.Fatalf("expected action_required, got %q", status.State)
	}
	if len(status.Issues) == 0 || !strings.Contains(status.Issues[0].ObservedError, "faster-whisper Python dependency is missing") {
		t.Fatalf("expected faster-whisper dependency guidance, got %+v", status.Issues)
	}
	if len(status.Actions) < 2 {
		t.Fatalf("expected command and manual setup actions, got %+v", status.Actions)
	}
	if status.Actions[0].Kind != "command" {
		t.Fatalf("expected first setup action to be command, got %+v", status.Actions)
	}
	if status.Actions[1].Kind != "manual" {
		t.Fatalf("expected second setup action to be manual, got %+v", status.Actions)
	}
}
