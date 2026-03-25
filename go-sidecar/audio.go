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

// Placeholder references so imports survive until Task 3 adds PreprocessAudio
// and cleanupPreprocessed which use them directly.
var _ = os.DevNull
var _ = exec.LookPath
var _ = filepath.Join
