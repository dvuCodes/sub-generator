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

// Ensure os, exec, and filepath imports are used (needed by Task 3 tests).
var _ = os.DevNull
var _ = exec.LookPath
var _ = filepath.Join
