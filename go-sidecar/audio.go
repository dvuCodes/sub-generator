package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const neutralAudioTempDir = "sub-generator-audio"

var stageNeutralAudio = stageNeutralAudioWithFFmpeg

func stageNeutralAudioWithFFmpeg(inputPath string) (string, func(), error) {
	if _, err := os.Stat(inputPath); err != nil {
		return "", nil, fmt.Errorf("input file not accessible: %w", err)
	}

	tempDir := filepath.Join(os.TempDir(), neutralAudioTempDir)
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	tempFile, err := os.CreateTemp(tempDir, baseName+"-neutral-*.wav")
	if err != nil {
		return "", nil, fmt.Errorf("failed to allocate staged audio path: %w", err)
	}
	outputPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		return "", nil, fmt.Errorf("failed to close staged audio handle: %w", err)
	}

	args := []string{
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		"-y",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		_ = os.Remove(outputPath)
		return "", nil, fmt.Errorf("ffmpeg neutral staging failed: %w", err)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		_ = os.Remove(outputPath)
		return "", nil, fmt.Errorf("cannot access staged audio output: %w", err)
	}
	if info.Size() == 0 {
		_ = os.Remove(outputPath)
		return "", nil, fmt.Errorf("ffmpeg produced no staged audio for %q", inputPath)
	}

	return outputPath, func() {
		if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: failed to clean up staged audio %q: %v\n", outputPath, err)
		}
	}, nil
}
