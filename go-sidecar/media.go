package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func MediaDuration(filePath string) (float64, error) {
	if filePath == "" {
		return 0, fmt.Errorf("empty file path")
	}

	out, err := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "N/A" {
		return 0, fmt.Errorf("ffprobe returned no duration for %s", filePath)
	}

	duration, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse ffprobe duration %q: %w", trimmed, err)
	}

	if duration <= 0 {
		return 0, fmt.Errorf("ffprobe returned non-positive duration: %f", duration)
	}

	return duration, nil
}
