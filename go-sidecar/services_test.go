package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

func TestRejectUnmanagedListeningServiceRejectsRawListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	err = rejectUnmanagedListeningService("ml-backend", port, nil, false)
	if err == nil {
		t.Fatal("rejectUnmanagedListeningService() error = nil, want unmanaged listener conflict")
	}

	message := err.Error()
	for _, fragment := range []string{
		"ml-backend",
		strconv.Itoa(port),
		"already listening",
		"not managed by SubGen",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("rejectUnmanagedListeningService() error %q missing %q", message, fragment)
		}
	}
}

func TestBuildMLBackendCommandPrefersDirectPythonWhenScriptExists(t *testing.T) {
	root := t.TempDir()
	launcherPath := filepath.Join(root, mlBackendLauncherName())
	pythonPath := filepath.Join(root, "python.exe")
	scriptPath := filepath.Join(root, "service.py")

	if err := os.WriteFile(launcherPath, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatalf("write launcher: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write service.py: %v", err)
	}

	cmd, err := buildMLBackendCommand(launcherPath, pythonPath, scriptPath, 8082)
	if err != nil {
		t.Fatalf("buildMLBackendCommand() error = %v", err)
	}

	if cmd.Path != pythonPath {
		t.Fatalf("cmd.Path = %q, want %q to manage the real Python process directly", cmd.Path, pythonPath)
	}
	if len(cmd.Args) < 2 || cmd.Args[1] != scriptPath {
		t.Fatalf("cmd.Args = %#v, want service.py to be launched directly", cmd.Args)
	}
}

func TestStartMLBackendAllowsHealthyExistingService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	root := t.TempDir()
	pythonBackendDir := filepath.Join(root, "python-backend")
	if err := os.MkdirAll(pythonBackendDir, 0o755); err != nil {
		t.Fatalf("mkdir python-backend: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pythonBackendDir, "service.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("write service.py: %v", err)
	}

	sm := NewServiceManager(ServiceConfig{SearchRoots: []string{root}, MLBackendPort: port})

	if err := sm.StartMLBackend(); err != nil {
		t.Fatalf("StartMLBackend() error = %v, want nil for healthy existing service", err)
	}
}
