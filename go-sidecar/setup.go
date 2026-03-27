package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// --- IPC response types for setup_status ---

type SetupStatusResponse struct {
	Type     string          `json:"type"`
	Services []ServiceStatus `json:"services"`
}

type ServiceStatus struct {
	ID          string          `json:"id"`
	DisplayName string          `json:"display_name"`
	RequiredFor string          `json:"required_for"`
	State       string          `json:"state"` // "ready" | "action_required"
	Issues      []ServiceIssue  `json:"issues"`
	Actions     []ServiceAction `json:"actions"`
}

type ServiceIssue struct {
	Code          string `json:"code"`
	ObservedError string `json:"observed_error,omitempty"`
}

type ServiceAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Kind        string `json:"kind"` // "archive" | "manual"
	Preferred   bool   `json:"preferred,omitempty"`
	Guidance    string `json:"guidance,omitempty"`
}

// --- Internal Action type (superset of ServiceAction) ---

type Action struct {
	ID              string
	Description     string
	Kind            string
	Label           string
	URL             string
	InstallDir      string
	StripComponents int
	ExpectedBinary  string
	Preferred       bool
	Guidance        string
	ServiceID       string
}

func (a Action) toServiceAction() ServiceAction {
	return ServiceAction{
		ID:          a.ID,
		Label:       a.Label,
		Description: a.Description,
		Kind:        a.Kind,
		Preferred:   a.Preferred,
		Guidance:    a.Guidance,
	}
}

// --- ActionRegistry ---

type ActionRegistry struct {
	actions map[string]Action
}

func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{actions: make(map[string]Action)}
}

func (r *ActionRegistry) Register(a Action) {
	r.actions[a.ID] = a
}

func (r *ActionRegistry) Resolve(id string) (Action, error) {
	a, ok := r.actions[id]
	if !ok {
		return Action{}, fmt.Errorf("unknown action_id %q", id)
	}
	return a, nil
}

// --- Binary probe ---

// probeService runs binaryPath --help with a 5-second timeout.
// It succeeds (returns empty issueCode) if the process at least started and
// exited (any exit code is acceptable — the binary is loadable).
// It fails if the process could not be started at all (e.g., DLL load error).
func probeService(binaryPath string) (issueCode, stderr string) {
	if err := validateCommandAvailability(binaryPath, binaryPath); err != nil {
		return "binary_not_found", err.Error()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--help")
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stderrOut := stderrBuf.String()

	if ctx.Err() == context.DeadlineExceeded {
		// Timeout means the process started and is running — binary is loadable.
		return "", ""
	}

	if err == nil {
		return "", ""
	}

	// If the process exited with a non-zero code, it still ran — binary is loadable.
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ProcessState != nil {
		// Process ran and exited; not a load failure.
		return "", ""
	}

	// Could not start the process at all.
	return "binary_not_runnable", stderrOut
}

// classifyProbeError classifies stderr output from a failed probe.
// Returns (code, attachActions) where attachActions=true when DLL issues are
// detected (suggesting a download action would help).
func classifyProbeError(stderr string) (code string, attachActions bool) {
	lower := strings.ToLower(stderr)

	dllPatterns := []string{
		"cannot open shared object",
		"dll not found",
		"not found",
		"0xc0000007b",
		"error while loading shared libraries",
	}

	for _, pattern := range dllPatterns {
		if strings.Contains(lower, pattern) {
			return "binary_not_runnable", true
		}
	}

	return "binary_not_runnable", false
}

// --- FFmpeg checker ---

type ffmpegValidator func(name, display string) error

func checkFFmpeg() ServiceStatus {
	return checkFFmpegWith(validateCommandAvailability)
}

func checkFFmpegWith(validator ffmpegValidator) ServiceStatus {
	if err := validator("ffmpeg", "ffmpeg"); err == nil {
		return ServiceStatus{
			ID:          "ffmpeg",
			DisplayName: "ffmpeg",
			RequiredFor: "transcription",
			State:       "ready",
			Issues:      []ServiceIssue{},
			Actions:     []ServiceAction{},
		}
	}

	return ServiceStatus{
		ID:          "ffmpeg",
		DisplayName: "ffmpeg",
		RequiredFor: "transcription",
		State:       "action_required",
		Issues: []ServiceIssue{
			{
				Code: "not_in_path",
			},
		},
		Actions: []ServiceAction{
			{
				ID:          "ffmpeg/install_manual",
				Kind:        "manual",
				Label:       "Install ffmpeg",
				Description: "Install ffmpeg and add it to PATH for video transcription.",
				Guidance:    "Download ffmpeg from https://ffmpeg.org/download.html and add it to your system PATH.",
			},
		},
	}
}

// --- Download action builders ---

func whisperDownloadActions(hasGPU bool, installDir string) []Action {
	actions := make([]Action, 0, 2)

	if hasGPU {
		actions = append(actions, Action{
			ID:              "whisper/install_gpu_bundle",
			Kind:            "archive",
			Label:           "Install whisper.cpp GPU bundle (CUDA 12.4)",
			Description:     "Preferred for NVIDIA GPUs. Faster transcription.",
			URL:             "https://github.com/ggml-org/whisper.cpp/releases/download/v1.8.4/whisper-cublas-12.4.0-bin-x64.zip",
			InstallDir:      installDir,
			StripComponents: 1,
			ExpectedBinary:  whisperExecutableName(),
			Preferred:       true,
			ServiceID:       "whisper",
		})
	}

	actions = append(actions, Action{
		ID:              "whisper/install_cpu_bundle",
		Kind:            "archive",
		Label:           "Install whisper.cpp CPU bundle",
		Description:     "Works on all systems. Slower transcription.",
		URL:             "https://github.com/ggml-org/whisper.cpp/releases/download/v1.8.4/whisper-bin-x64.zip",
		InstallDir:      installDir,
		StripComponents: 1,
		ExpectedBinary:  whisperExecutableName(),
		Preferred:       !hasGPU,
		ServiceID:       "whisper",
	})

	return actions
}

func llamaDownloadActions(hasGPU bool, installDir string) []Action {
	actions := make([]Action, 0, 2)

	if hasGPU {
		actions = append(actions, Action{
			ID:              "llama/install_gpu_bundle",
			Kind:            "archive",
			Label:           "Install llama.cpp GPU bundle (CUDA 12.4)",
			Description:     "Preferred for NVIDIA GPUs. Faster translation.",
			URL:             "https://github.com/ggml-org/llama.cpp/releases/download/b5170/cudart-llama-bin-win-cu12.4-x64.zip",
			InstallDir:      installDir,
			StripComponents: 1,
			ExpectedBinary:  llamaExecutableName(),
			Preferred:       true,
			ServiceID:       "llama",
		})
	}

	actions = append(actions, Action{
		ID:              "llama/install_cpu_bundle",
		Kind:            "archive",
		Label:           "Install llama.cpp CPU bundle",
		Description:     "Works on all systems. Slower translation.",
		URL:             "https://github.com/ggml-org/llama.cpp/releases/download/b5170/llama-b5170-bin-win-noavx-x64.zip",
		InstallDir:      installDir,
		StripComponents: 1,
		ExpectedBinary:  llamaExecutableName(),
		Preferred:       !hasGPU,
		ServiceID:       "llama",
	})

	return actions
}

// --- Service checker helper ---

func checkService(
	id, displayName, requiredFor, binaryPath string,
	downloadActions []Action,
	registry *ActionRegistry,
) ServiceStatus {
	issueCode, stderr := probeService(binaryPath)
	if issueCode == "" {
		return ServiceStatus{
			ID:          id,
			DisplayName: displayName,
			RequiredFor: requiredFor,
			State:       "ready",
			Issues:      []ServiceIssue{},
			Actions:     []ServiceAction{},
		}
	}

	attachActions := issueCode == "binary_not_found"
	if issueCode == "binary_not_runnable" {
		_, attachActions = classifyProbeError(stderr)
	}

	issue := ServiceIssue{
		Code: issueCode,
	}
	if issueCode == "binary_not_runnable" {
		issue.ObservedError = stderr
	}

	status := ServiceStatus{
		ID:          id,
		DisplayName: displayName,
		RequiredFor: requiredFor,
		State:       "action_required",
		Issues:      []ServiceIssue{issue},
		Actions:     []ServiceAction{},
	}

	if attachActions && registry != nil {
		for _, a := range downloadActions {
			registry.Register(a)
			status.Actions = append(status.Actions, a.toServiceAction())
		}
	}

	return status
}

// --- Main entry point ---

// CheckSetup probes whisper, llama, and ffmpeg and returns a structured status.
func CheckSetup(config ServiceConfig, registry *ActionRegistry) SetupStatusResponse {
	hasGPU := detectGPU() != "none"

	whisperBinary, _ := resolveWhisperAssets(config.SearchRoots, "base")
	whisperInstallDir := preferredWhisperInstallDir(config.SearchRoots)
	whisperActions := whisperDownloadActions(hasGPU, whisperInstallDir)

	llamaBinary := resolveLlamaServerBinary(config.SearchRoots)
	llamaInstallDir := preferredLlamaInstallDir(config.SearchRoots)
	llamaActions := llamaDownloadActions(hasGPU, llamaInstallDir)

	whisperStatus := checkService("whisper", "whisper-server", "transcription", whisperBinary, whisperActions, registry)
	llamaStatus := checkService("llama", "llama-server", "translation", llamaBinary, llamaActions, registry)
	ffmpegStatus := checkFFmpeg()

	return SetupStatusResponse{
		Type:     "setup_status",
		Services: []ServiceStatus{whisperStatus, llamaStatus, ffmpegStatus},
	}
}
