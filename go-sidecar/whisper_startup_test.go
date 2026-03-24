package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildWhisperCommandIncludesConvertFlag(t *testing.T) {
	cmd := buildWhisperCommand("whisper-server", "model.bin", "", 8080)

	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "--convert") {
		t.Fatalf("buildWhisperCommand() args = %q, want --convert", got)
	}
}

func TestBuildWhisperCommandIncludesVADModel(t *testing.T) {
	cmd := buildWhisperCommand("whisper-server", "model.bin", "vad.bin", 8080)

	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "--vad-model vad.bin") {
		t.Fatalf("buildWhisperCommand() args = %q, want --vad-model", got)
	}
}

func TestBuildWhisperCommandOmitsVADModelWhenEmpty(t *testing.T) {
	cmd := buildWhisperCommand("whisper-server", "model.bin", "", 8080)

	got := strings.Join(cmd.Args, " ")
	if strings.Contains(got, "--vad-model") {
		t.Fatalf("buildWhisperCommand() args = %q, should not contain --vad-model when empty", got)
	}
}

func TestResolveVADModelPathFindsInServicesDir(t *testing.T) {
	root := t.TempDir()
	vadDir := filepath.Join(root, "services", "whisper-server", "models")
	if err := os.MkdirAll(vadDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	vadPath := filepath.Join(vadDir, vadModelFilename)
	if err := os.WriteFile(vadPath, []byte("vad"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	got := resolveVADModelPath([]string{root})
	if got != vadPath {
		t.Errorf("resolveVADModelPath() = %q, want %q", got, vadPath)
	}
}

func TestResolveVADModelPathFindsInModelsDir(t *testing.T) {
	root := t.TempDir()
	modelsDir := filepath.Join(root, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	vadPath := filepath.Join(modelsDir, vadModelFilename)
	if err := os.WriteFile(vadPath, []byte("vad"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	got := resolveVADModelPath([]string{root})
	if got != vadPath {
		t.Errorf("resolveVADModelPath() = %q, want %q", got, vadPath)
	}
}

func TestResolveVADModelPathReturnsEmptyWhenNotFound(t *testing.T) {
	root := t.TempDir()
	got := resolveVADModelPath([]string{root})
	if got != "" {
		t.Errorf("resolveVADModelPath() = %q, want empty string", got)
	}
}

func TestValidateWhisperRuntimeDependenciesRequiresFFmpegWhenConverting(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", "")
	serviceDir := filepath.Join(root, "services", "whisper-server")
	modelsDir := filepath.Join(serviceDir, "models")

	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, whisperExecutableName()), []byte("bin"), 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "ggml-base.bin"), []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile(model) error: %v", err)
	}

	err := validateWhisperRuntimeDependencies(
		[]string{root},
		filepath.Join(serviceDir, whisperExecutableName()),
		filepath.Join(modelsDir, "ggml-base.bin"),
		true,
	)
	if err == nil {
		t.Fatal("validateWhisperRuntimeDependencies() error = nil, want ffmpeg guidance")
	}

	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"ffmpeg is required",
		"not found in path",
		"--convert",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("validateWhisperRuntimeDependencies() error %q missing %q", err.Error(), fragment)
		}
	}
}
