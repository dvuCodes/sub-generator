package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFilterChain_Defaults(t *testing.T) {
	cfg := DefaultAudioConfig()
	chain := buildFilterChain(cfg)

	expected := "highpass=f=200,lowpass=f=8000,equalizer=f=1000:t=h:w=2000:g=3,loudnorm=I=-16:TP=-1.5:LRA=11,agate=threshold=0.01:attack=5:release=50"
	if chain != expected {
		t.Errorf("expected %q, got %q", expected, chain)
	}
}

func TestBuildFilterChain_NoiseGateOff(t *testing.T) {
	cfg := DefaultAudioConfig()
	cfg.NoiseGate = false
	chain := buildFilterChain(cfg)

	if strings.Contains(chain, "agate") {
		t.Errorf("expected no agate filter, got %q", chain)
	}
	if !strings.Contains(chain, "loudnorm") {
		t.Errorf("expected loudnorm filter present, got %q", chain)
	}
}

func TestBuildFilterChain_NormalizeOff(t *testing.T) {
	cfg := DefaultAudioConfig()
	cfg.Normalize = false
	chain := buildFilterChain(cfg)

	if strings.Contains(chain, "loudnorm") {
		t.Errorf("expected no loudnorm filter, got %q", chain)
	}
	if !strings.Contains(chain, "agate") {
		t.Errorf("expected agate filter present, got %q", chain)
	}
}

func TestBuildFilterChain_VocalBoostZero(t *testing.T) {
	cfg := DefaultAudioConfig()
	cfg.VocalBoostDB = 0
	chain := buildFilterChain(cfg)

	if strings.Contains(chain, "equalizer") {
		t.Errorf("expected no equalizer filter when boost is 0, got %q", chain)
	}
}

func TestBuildFilterChain_VocalBoostMax(t *testing.T) {
	cfg := DefaultAudioConfig()
	cfg.VocalBoostDB = 6
	chain := buildFilterChain(cfg)

	if !strings.Contains(chain, "g=6") {
		t.Errorf("expected g=6 in equalizer filter, got %q", chain)
	}
}

func TestBuildFilterChain_SubTogglesOff(t *testing.T) {
	cfg := AudioConfig{
		Enabled:      true,
		VocalBoostDB: 0,
		NoiseGate:    false,
		Normalize:    false,
	}
	chain := buildFilterChain(cfg)

	expected := "highpass=f=200,lowpass=f=8000"
	if chain != expected {
		t.Errorf("expected %q, got %q", expected, chain)
	}
}

func TestPreprocessAudio_InvalidFile(t *testing.T) {
	cfg := DefaultAudioConfig()
	_, err := PreprocessAudio("/nonexistent/file.mp4", cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestPreprocessAudio_ValidVideo(t *testing.T) {
	// Generate a short test video with FFmpeg (2 seconds of sine wave)
	tmpDir := t.TempDir()
	testVideo := filepath.Join(tmpDir, "test.mkv")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:d=2",
		"-shortest", "-y", testVideo,
	)
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg not available or failed to create test video: %v", err)
	}

	cfg := DefaultAudioConfig()
	wavPath, err := PreprocessAudio(testVideo, cfg)
	if err != nil {
		t.Fatalf("PreprocessAudio failed: %v", err)
	}
	defer cleanupPreprocessed(wavPath)

	// Verify output exists and is non-empty
	info, err := os.Stat(wavPath)
	if err != nil {
		t.Fatalf("output WAV not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output WAV is empty")
	}
	if !strings.HasSuffix(wavPath, "-enhanced.wav") {
		t.Errorf("expected WAV filename to end with -enhanced.wav, got %q", wavPath)
	}
}

func TestPreprocessAudio_NoAudioStream(t *testing.T) {
	// Generate a video with no audio
	tmpDir := t.TempDir()
	testVideo := filepath.Join(tmpDir, "no-audio.mkv")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:d=2",
		"-an", "-y", testVideo,
	)
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg not available or failed to create test video: %v", err)
	}

	cfg := DefaultAudioConfig()
	_, err := PreprocessAudio(testVideo, cfg)
	if err == nil {
		t.Fatal("expected error for video with no audio stream")
	}
}

func TestPreprocessAudio_Cleanup(t *testing.T) {
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, "sub-generator-audio", "test-cleanup.wav")
	os.MkdirAll(filepath.Dir(testFile), 0o755)
	os.WriteFile(testFile, []byte("dummy"), 0o644)

	cleanupPreprocessed(testFile)

	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, but it still exists")
	}
}
