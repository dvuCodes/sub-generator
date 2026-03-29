package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareTranscriptionInputStagesNeutralAudioForFasterWhisper(t *testing.T) {
	originalStageNeutralAudio := stageNeutralAudio
	defer func() {
		stageNeutralAudio = originalStageNeutralAudio
	}()

	cleanupCalled := false
	stageNeutralAudio = func(inputPath string) (string, func(), error) {
		if inputPath != "video.mp4" {
			t.Fatalf("stageNeutralAudio() input = %q, want %q", inputPath, "video.mp4")
		}
		return "video-staged.wav", func() {
			cleanupCalled = true
		}, nil
	}

	prepared := prepareTranscriptionInput(Command{InputVideo: "video.mp4"}, "faster_whisper")

	if prepared.TranscriptionPath != "video-staged.wav" {
		t.Fatalf("TranscriptionPath = %q, want staged WAV", prepared.TranscriptionPath)
	}
	if prepared.DiarizationPath != "video-staged.wav" {
		t.Fatalf("DiarizationPath = %q, want staged WAV", prepared.DiarizationPath)
	}

	prepared.Cleanup()
	if !cleanupCalled {
		t.Fatal("Cleanup() did not call staged-audio cleanup")
	}
}

func TestPrepareTranscriptionInputFallsBackToOriginalWhenStagingFails(t *testing.T) {
	originalStageNeutralAudio := stageNeutralAudio
	defer func() {
		stageNeutralAudio = originalStageNeutralAudio
	}()

	stageNeutralAudio = func(inputPath string) (string, func(), error) {
		return "", nil, errors.New("ffmpeg unavailable")
	}

	prepared := prepareTranscriptionInput(Command{InputVideo: "video.mp4"}, "faster_whisper")

	if prepared.TranscriptionPath != "video.mp4" {
		t.Fatalf("TranscriptionPath = %q, want original input fallback", prepared.TranscriptionPath)
	}
	if prepared.DiarizationPath != "video.mp4" {
		t.Fatalf("DiarizationPath = %q, want original input fallback", prepared.DiarizationPath)
	}
	if prepared.Cleanup == nil {
		t.Fatal("Cleanup should always be set")
	}
}

func TestPrepareTranscriptionInputLeavesWhisperCppUnchanged(t *testing.T) {
	originalStageNeutralAudio := stageNeutralAudio
	defer func() {
		stageNeutralAudio = originalStageNeutralAudio
	}()

	stageNeutralAudio = func(inputPath string) (string, func(), error) {
		t.Fatal("stageNeutralAudio() should not run for whisper_cpp")
		return "", nil, nil
	}

	prepared := prepareTranscriptionInput(Command{InputVideo: "video.mp4"}, "whisper_cpp")

	if prepared.TranscriptionPath != "video.mp4" {
		t.Fatalf("TranscriptionPath = %q, want original input", prepared.TranscriptionPath)
	}
	if prepared.DiarizationPath != "video.mp4" {
		t.Fatalf("DiarizationPath = %q, want original input", prepared.DiarizationPath)
	}
}

func TestEnsureASRAssetsSkipsWhisperCppSetupForFasterWhisper(t *testing.T) {
	root := t.TempDir()

	err := ensureASRAssets([]string{root}, Command{
		ModelSize: "base",
		VADFilter: true,
	}, "faster_whisper")
	if err != nil {
		t.Fatalf("ensureASRAssets() error = %v, want nil for faster_whisper", err)
	}

	modelsDir := filepath.Join(root, "services", "whisper-server", "models")
	if _, statErr := os.Stat(modelsDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("modelsDir stat error = %v, want not-exist because whisper_cpp assets should be skipped", statErr)
	}
}
