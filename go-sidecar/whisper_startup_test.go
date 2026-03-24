package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildWhisperCommandIncludesConvertFlag(t *testing.T) {
	cmd := buildWhisperCommand("whisper-server", "model.bin", "", 8080, nil)

	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "--convert") {
		t.Fatalf("buildWhisperCommand() args = %q, want --convert", got)
	}
}

func TestBuildWhisperCommandIncludesVADModel(t *testing.T) {
	cmd := buildWhisperCommand("whisper-server", "model.bin", "vad.bin", 8080, nil)

	got := strings.Join(cmd.Args, " ")
	if !strings.Contains(got, "--vad-model vad.bin") {
		t.Fatalf("buildWhisperCommand() args = %q, want --vad-model", got)
	}
}

func TestBuildWhisperCommandOmitsVADModelWhenEmpty(t *testing.T) {
	cmd := buildWhisperCommand("whisper-server", "model.bin", "", 8080, nil)

	got := strings.Join(cmd.Args, " ")
	if strings.Contains(got, "--vad-model") {
		t.Fatalf("buildWhisperCommand() args = %q, should not contain --vad-model when empty", got)
	}
}

func TestBuildWhisperCommandIncludesVADParams(t *testing.T) {
	params := &VADParams{
		Threshold:            0.30,
		MinSpeechDurationMs:  100,
		MinSilenceDurationMs: 200,
		MaxSpeechDurationS:   60,
		SpeechPadMs:          50,
	}
	cmd := buildWhisperCommand("whisper-server", "model.bin", "vad.bin", 8080, params)
	got := strings.Join(cmd.Args, " ")

	for _, fragment := range []string{
		"--vad-threshold 0.30",
		"--vad-min-speech-duration-ms 100",
		"--vad-min-silence-duration-ms 200",
		"--vad-max-speech-duration-s 60.0",
		"--vad-speech-pad-ms 50",
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("buildWhisperCommand() args = %q, missing %q", got, fragment)
		}
	}
}

func TestBuildWhisperCommandOmitsMaxSpeechDurationWhenZero(t *testing.T) {
	params := &VADParams{
		Threshold:            0.5,
		MinSpeechDurationMs:  250,
		MinSilenceDurationMs: 100,
		MaxSpeechDurationS:   0,
		SpeechPadMs:          30,
	}
	cmd := buildWhisperCommand("whisper-server", "model.bin", "vad.bin", 8080, params)
	got := strings.Join(cmd.Args, " ")

	if strings.Contains(got, "--vad-max-speech-duration-s") {
		t.Fatalf("buildWhisperCommand() args = %q, should not contain --vad-max-speech-duration-s when 0", got)
	}
}

func TestBuildWhisperCommandOmitsVADParamsWhenNil(t *testing.T) {
	cmd := buildWhisperCommand("whisper-server", "model.bin", "vad.bin", 8080, nil)
	got := strings.Join(cmd.Args, " ")

	if strings.Contains(got, "--vad-threshold") {
		t.Fatalf("buildWhisperCommand() args = %q, should not contain --vad-threshold when params nil", got)
	}
}

func TestVadParamsEqual(t *testing.T) {
	a := &VADParams{Threshold: 0.5, MinSpeechDurationMs: 250, MinSilenceDurationMs: 100, MaxSpeechDurationS: 0, SpeechPadMs: 30}
	b := &VADParams{Threshold: 0.5, MinSpeechDurationMs: 250, MinSilenceDurationMs: 100, MaxSpeechDurationS: 0, SpeechPadMs: 30}

	if !vadParamsEqual(a, b) {
		t.Fatal("vadParamsEqual(a, b) = false, want true for identical params")
	}
	if !vadParamsEqual(nil, nil) {
		t.Fatal("vadParamsEqual(nil, nil) = false, want true")
	}

	b.Threshold = 0.3
	if vadParamsEqual(a, b) {
		t.Fatal("vadParamsEqual(a, b) = true, want false for different threshold")
	}
	if vadParamsEqual(a, nil) {
		t.Fatal("vadParamsEqual(a, nil) = true, want false")
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
