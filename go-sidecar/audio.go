package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func buildFilterChain(cfg AudioConfig) string {
	filters := []string{
		"highpass=f=200",
		"lowpass=f=8000",
	}

	if cfg.VocalBoostDB > 0 {
		filters = append(filters, fmt.Sprintf(
			"equalizer=f=1000:t=h:w=2000:g=%g",
			cfg.VocalBoostDB,
		))
	}

	if cfg.Normalize {
		filters = append(filters, "loudnorm=I=-16:TP=-1.5:LRA=11")
	}

	if cfg.NoiseGate {
		filters = append(filters, "agate=threshold=0.01:attack=5:release=50")
	}

	return strings.Join(filters, ",")
}

const audioTempDir = "sub-generator-audio"

func PreprocessAudio(inputPath string, cfg AudioConfig) (string, error) {
	if _, err := os.Stat(inputPath); err != nil {
		return "", fmt.Errorf("input file not accessible: %w", err)
	}

	tempDir := filepath.Join(os.TempDir(), audioTempDir)
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath := filepath.Join(tempDir, baseName+"-enhanced.wav")

	filterChain := buildFilterChain(cfg)

	args := []string{
		"-i", inputPath,
		"-vn",
		"-af", filterChain,
		"-ar", "16000",
		"-ac", "1",
		"-f", "wav",
		"-y",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg preprocessing failed: %w", err)
	}

	if info, err := os.Stat(outputPath); err != nil || info.Size() == 0 {
		return "", fmt.Errorf("ffmpeg produced no output for %q", inputPath)
	}

	return outputPath, nil
}

func cleanupPreprocessed(wavPath string) {
	if wavPath == "" {
		return
	}
	if err := os.Remove(wavPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: failed to clean up preprocessed audio %q: %v\n", wavPath, err)
	}
}
