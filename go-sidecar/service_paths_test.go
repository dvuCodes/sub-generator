package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWhisperAssetsFindsDocumentedServicesLayout(t *testing.T) {
	root := t.TempDir()
	serviceDir := filepath.Join(root, "services", "whisper-server")
	modelsDir := filepath.Join(serviceDir, "models")

	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, "whisper-server.exe"), []byte("bin"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "ggml-large-v3-turbo.bin"), []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	binPath, modelPath := resolveWhisperAssets([]string{root}, "turbo")

	if want := filepath.Join(serviceDir, "whisper-server.exe"); binPath != want {
		t.Fatalf("binPath = %q, want %q", binPath, want)
	}
	if want := filepath.Join(modelsDir, "ggml-large-v3-turbo.bin"); modelPath != want {
		t.Fatalf("modelPath = %q, want %q", modelPath, want)
	}
}

func TestModelFilenameFallsBackToBaseForUnknownModel(t *testing.T) {
	if got := modelFilename("mystery"); got != "ggml-base.bin" {
		t.Fatalf("modelFilename() = %q, want %q", got, "ggml-base.bin")
	}
}
