# Dependency Validation & Auto-Install Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect missing DLLs/binaries at app launch, show clear setup warnings, and let users download+install service bundles with one click.

**Architecture:** New `check_setup` IPC command probes service binaries via exec and checks ffmpeg in PATH. Frontend shows a SetupBanner with download actions. `install_dependency` command downloads zip archives, extracts atomically, and re-probes. Go mutex prevents concurrent install and generate.

**Tech Stack:** Go (sidecar), React/TypeScript (frontend), shadcn/ui (UI), `archive/zip` (extraction)

**Spec:** `docs/superpowers/specs/2026-03-25-dependency-validation-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `go-sidecar/config.go` | Modify (line 11-25) | Add `ActionID` field to `Command` struct |
| `go-sidecar/setup.go` | Create | `CheckSetup`, `probeService`, `classifyProbeError`, action registry, download URLs |
| `go-sidecar/setup_test.go` | Create | Unit tests for probe classification, setup checks, action resolution |
| `go-sidecar/archive.go` | Create | `DownloadAndExtractArchive`, `extractZip`, atomic swap |
| `go-sidecar/archive_test.go` | Create | Unit tests for zip extraction, atomic swap, failure modes |
| `go-sidecar/main.go` | Modify (lines 15-16, 68-113) | Add `check_setup`/`install_dependency` routing, `sync.Mutex` for serialization |
| `src/lib/types.ts` | Modify (lines 52-58, 118-125) | Add setup types, update command/response unions |
| `src/lib/setupHelpers.ts` | Create | `shouldDisableGenerate`, `formatSetupIssue` pure functions |
| `src/lib/setupHelpers.test.ts` | Create | Tests for helper functions |
| `src/lib/installState.ts` | Create | Install reducer: `advanceInstallState`, `createInitialInstallState` |
| `src/lib/installState.test.ts` | Create | Tests for install state machine |
| `src/components/ProcessingView.tsx` | Modify (lines 18-34) | Accept stage config as props |
| `src/components/SetupBanner.tsx` | Create | Setup warning banner with download actions |
| `src/App.tsx` | Modify | Wire setup status, install state, SetupBanner, generate-disable |

---

### Task 1: Add ActionID to Go Command struct

**Files:**
- Modify: `go-sidecar/config.go:11-25`

- [ ] **Step 1: Add ActionID field to Command**

In `go-sidecar/config.go`, add `ActionID` after `AudioConfig` in the `Command` struct (line 21):

```go
type Command struct {
	Command      string       `json:"command"`
	InputVideo   string       `json:"input_video,omitempty"`
	SourceLang   *string      `json:"source_lang,omitempty"`
	TargetLang   *string      `json:"target_lang,omitempty"`
	OutputFormat string       `json:"output_format,omitempty"`
	OutputPath   *string      `json:"output_path,omitempty"`
	ModelSize    string       `json:"model_size,omitempty"`
	BeamSize     int          `json:"beam_size,omitempty"`
	VADFilter    bool         `json:"vad_filter,omitempty"`
	AudioConfig  *AudioConfig `json:"audio_config,omitempty"`
	ActionID     string       `json:"action_id,omitempty"`
	// install_language fields
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd go-sidecar && go build ./...`

- [ ] **Step 3: Commit**

```bash
git add go-sidecar/config.go
git commit -m "feat(setup): add ActionID field to Command struct"
```

---

### Task 2: Implement setup checker with probe and classification (TDD)

**Files:**
- Create: `go-sidecar/setup.go`
- Create: `go-sidecar/setup_test.go`

- [ ] **Step 1: Write failing tests**

Create `go-sidecar/setup_test.go`:

```go
package main

import (
	"fmt"
	"testing"
)

func TestClassifyProbeError_DLLNotFound(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
	}{
		{"windows dll", "error: whisper.dll not found"},
		{"msys2 shared object", "error while loading shared libraries: whisper.dll: cannot open shared object file"},
		{"windows error code", "Application failed to start (0xc0000007b)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, attach := classifyProbeError(tc.stderr)
			if code != "binary_not_runnable" {
				t.Errorf("expected binary_not_runnable, got %q", code)
			}
			if !attach {
				t.Error("expected attachActions=true for DLL error")
			}
		})
	}
}

func TestClassifyProbeError_PermissionDenied(t *testing.T) {
	code, attach := classifyProbeError("permission denied")
	if code != "binary_not_runnable" {
		t.Errorf("expected binary_not_runnable, got %q", code)
	}
	if attach {
		t.Error("expected attachActions=false for permission error")
	}
}

func TestClassifyProbeError_Unknown(t *testing.T) {
	code, attach := classifyProbeError("some random error output")
	if code != "binary_not_runnable" {
		t.Errorf("expected binary_not_runnable, got %q", code)
	}
	if attach {
		t.Error("expected attachActions=false for unknown error")
	}
}

func TestResolveAction_Valid(t *testing.T) {
	registry := &ActionRegistry{
		actions: map[string]Action{
			"whisper/install_gpu_bundle": {
				ID:              "whisper/install_gpu_bundle",
				Label:           "Install GPU bundle",
				URL:             "https://example.com/gpu.zip",
				InstallDir:      "/tmp/test",
				StripComponents: 1,
				ExpectedBinary:  "whisper-server.exe",
				ServiceID:       "whisper",
			},
		},
	}

	action, err := registry.Resolve("whisper/install_gpu_bundle")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.URL != "https://example.com/gpu.zip" {
		t.Errorf("expected URL https://example.com/gpu.zip, got %q", action.URL)
	}
	if action.ServiceID != "whisper" {
		t.Errorf("expected ServiceID whisper, got %q", action.ServiceID)
	}
}

func TestResolveAction_Unknown(t *testing.T) {
	registry := &ActionRegistry{actions: map[string]Action{}}
	_, err := registry.Resolve("nonexistent/action")
	if err == nil {
		t.Fatal("expected error for unknown action_id")
	}
}

func TestCheckFFmpeg_Missing(t *testing.T) {
	status := checkFFmpegWith(func(name, display string) error {
		return fmt.Errorf("%s not found in PATH", display)
	})
	if status.State != "action_required" {
		t.Errorf("expected action_required, got %q", status.State)
	}
	if len(status.Issues) == 0 {
		t.Fatal("expected at least one issue")
	}
	if status.Issues[0].Code != "not_in_path" {
		t.Errorf("expected not_in_path, got %q", status.Issues[0].Code)
	}
	if len(status.Actions) == 0 || status.Actions[0].Kind != "manual" {
		t.Error("expected manual action for ffmpeg")
	}
}

func TestCheckFFmpeg_Present(t *testing.T) {
	status := checkFFmpegWith(func(name, display string) error {
		return nil
	})
	if status.State != "ready" {
		t.Errorf("expected ready, got %q", status.State)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go-sidecar && go test -run "TestClassify|TestResolve|TestCheckFFmpeg" -v`
Expected: FAIL — types and functions undefined.

- [ ] **Step 3: Implement setup.go**

Create `go-sidecar/setup.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// --- IPC Response Types ---

type SetupStatusResponse struct {
	Type     string          `json:"type"`
	Services []ServiceStatus `json:"services"`
}

type ServiceStatus struct {
	ID          string          `json:"id"`
	DisplayName string          `json:"display_name"`
	RequiredFor string          `json:"required_for"`
	State       string          `json:"state"`
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
	Kind        string `json:"kind"`
	Preferred   bool   `json:"preferred,omitempty"`
	Guidance    string `json:"guidance,omitempty"`
}

// --- Internal Action (superset of ServiceAction) ---

type Action struct {
	ID              string
	Label           string
	Description     string
	Kind            string
	Preferred       bool
	Guidance        string
	URL             string
	InstallDir      string
	StripComponents int
	ExpectedBinary  string
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

// --- Action Registry ---

type ActionRegistry struct {
	actions map[string]Action
}

func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{actions: make(map[string]Action)}
}

func (r *ActionRegistry) Register(action Action) {
	r.actions[action.ID] = action
}

func (r *ActionRegistry) Resolve(actionID string) (Action, error) {
	action, ok := r.actions[actionID]
	if !ok {
		return Action{}, fmt.Errorf("unknown install action: %q", actionID)
	}
	return action, nil
}

// --- Probe ---

func probeService(binaryPath string) (issueCode string, stderr string) {
	if _, err := os.Stat(binaryPath); err != nil {
		return "binary_not_found", ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--help")
	output, err := cmd.CombinedOutput()
	if err == nil {
		return "", ""
	}

	// Process started and exited (even with non-zero) — that's fine
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return "", ""
	}

	return "binary_not_runnable", string(output)
}

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

// --- FFmpeg Check ---

func checkFFmpeg() ServiceStatus {
	return checkFFmpegWith(validateCommandAvailability)
}

func checkFFmpegWith(validator func(string, string) error) ServiceStatus {
	status := ServiceStatus{
		ID:          "ffmpeg",
		DisplayName: "ffmpeg",
		RequiredFor: "transcription",
		State:       "ready",
		Issues:      []ServiceIssue{},
		Actions:     []ServiceAction{},
	}

	if err := validator("ffmpeg", "ffmpeg"); err != nil {
		status.State = "action_required"
		status.Issues = append(status.Issues, ServiceIssue{
			Code:          "not_in_path",
			ObservedError: err.Error(),
		})
		status.Actions = append(status.Actions, ServiceAction{
			ID:       "ffmpeg/install_manual",
			Label:    "Install ffmpeg",
			Kind:     "manual",
			Guidance: "Download ffmpeg from https://ffmpeg.org/download.html and add it to your system PATH.",
		})
	}

	return status
}

// --- Download Actions ---

func whisperDownloadActions(hasGPU bool, installDir string) []Action {
	actions := []Action{
		{
			ID:              "whisper/install_gpu_bundle",
			Label:           "Install GPU (CUDA) bundle",
			Description:     "Preferred for NVIDIA GPUs. Faster transcription.",
			Kind:            "archive",
			Preferred:       hasGPU,
			URL:             "https://github.com/ggml-org/whisper.cpp/releases/download/v1.8.4/whisper-cublas-12.4.0-bin-x64.zip",
			InstallDir:      installDir,
			StripComponents: 1,
			ExpectedBinary:  whisperExecutableName(),
			ServiceID:       "whisper",
		},
		{
			ID:              "whisper/install_cpu_bundle",
			Label:           "Install CPU bundle",
			Description:     "Works on all systems. Slower transcription.",
			Kind:            "archive",
			URL:             "https://github.com/ggml-org/whisper.cpp/releases/download/v1.8.4/whisper-bin-x64.zip",
			InstallDir:      installDir,
			StripComponents: 1,
			ExpectedBinary:  whisperExecutableName(),
			ServiceID:       "whisper",
		},
	}
	return actions
}

func llamaDownloadActions(hasGPU bool, installDir string) []Action {
	actions := []Action{
		{
			ID:              "llama/install_gpu_bundle",
			Label:           "Install GPU (CUDA) bundle",
			Description:     "Preferred for NVIDIA GPUs. Faster translation.",
			Kind:            "archive",
			Preferred:       hasGPU,
			// NOTE: spec used releases/latest placeholder URLs; these are real artifact names from b5170 release
			URL:             "https://github.com/ggml-org/llama.cpp/releases/download/b5170/cudart-llama-bin-win-cu12.4-x64.zip",
			InstallDir:      installDir,
			StripComponents: 1,
			ExpectedBinary:  llamaExecutableName(),
			ServiceID:       "llama",
		},
		{
			ID:              "llama/install_cpu_bundle",
			Label:           "Install CPU bundle",
			Description:     "Works on all systems. Slower translation.",
			Kind:            "archive",
			URL:             "https://github.com/ggml-org/llama.cpp/releases/download/b5170/llama-bin-win-avx2-x64.zip",
			InstallDir:      installDir,
			StripComponents: 1,
			ExpectedBinary:  llamaExecutableName(),
			ServiceID:       "llama",
		},
	}
	return actions
}

// --- Main Check ---

func CheckSetup(config ServiceConfig, registry *ActionRegistry) SetupStatusResponse {
	hasGPU := detectGPU() != "none"

	whisperBinary, _ := resolveWhisperAssets(config.SearchRoots, "base")
	whisperInstallDir := preferredWhisperInstallDir(config.SearchRoots)
	whisperStatus := checkService("whisper", "whisper-server", "transcription", whisperBinary, hasGPU, whisperInstallDir, whisperDownloadActions, registry)

	llamaBinary := resolveLlamaServerBinary(config.SearchRoots)
	llamaInstallDir := preferredLlamaInstallDir(config.SearchRoots)
	llamaStatus := checkService("llama", "llama-server", "translation", llamaBinary, hasGPU, llamaInstallDir, llamaDownloadActions, registry)

	ffmpegStatus := checkFFmpeg()

	return SetupStatusResponse{
		Type:     "setup_status",
		Services: []ServiceStatus{whisperStatus, llamaStatus, ffmpegStatus},
	}
}

func checkService(
	id, displayName, requiredFor, binaryPath string,
	hasGPU bool,
	installDir string,
	actionsFn func(bool, string) []Action,
	registry *ActionRegistry,
) ServiceStatus {
	status := ServiceStatus{
		ID:          id,
		DisplayName: displayName,
		RequiredFor: requiredFor,
		State:       "ready",
		Issues:      []ServiceIssue{},
		Actions:     []ServiceAction{},
	}

	issueCode, stderr := probeService(binaryPath)
	if issueCode == "" {
		return status
	}

	status.State = "action_required"
	issue := ServiceIssue{Code: issueCode}

	attachActions := false
	if issueCode == "binary_not_found" {
		attachActions = true
	} else if issueCode == "binary_not_runnable" {
		_, attachActions = classifyProbeError(stderr)
		issue.ObservedError = stderr
	}

	status.Issues = append(status.Issues, issue)

	if attachActions {
		actions := actionsFn(hasGPU, installDir)
		for _, action := range actions {
			registry.Register(action)
			status.Actions = append(status.Actions, action.toServiceAction())
		}
	}

	return status
}

// NOTE: preferredWhisperInstallDir already exists in services.go — reuse it directly.
// NOTE: preferredLlamaInstallDir already exists in config.go — reuse it directly.
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-sidecar && go test -run "TestClassify|TestResolve|TestCheckFFmpeg" -v`
Expected: All tests PASS.

- [ ] **Step 5: Run full test suite**

Run: `cd go-sidecar && go test -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add go-sidecar/setup.go go-sidecar/setup_test.go
git commit -m "feat(setup): implement dependency checker with probe, classification, and action registry"
```

---

### Task 3: Implement archive extraction with atomic swap (TDD)

**Files:**
- Create: `go-sidecar/archive.go`
- Create: `go-sidecar/archive_test.go`

- [ ] **Step 1: Write failing tests**

Create `go-sidecar/archive_test.go`:

```go
package main

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func createTestZip(t *testing.T, dest string, files map[string]string) {
	t.Helper()
	f, err := os.Create(dest)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("add file: %v", err)
		}
		fw.Write([]byte(content))
	}
	w.Close()
}

func TestExtractZip_StripComponents0(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	destDir := filepath.Join(tmpDir, "out")

	createTestZip(t, zipPath, map[string]string{
		"folder/binary.exe": "exe content",
		"folder/lib.dll":    "dll content",
	})

	err := extractZip(zipPath, destDir, 0)
	if err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "folder", "binary.exe")); err != nil {
		t.Error("expected folder/binary.exe to exist")
	}
}

func TestExtractZip_StripComponents1(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	destDir := filepath.Join(tmpDir, "out")

	createTestZip(t, zipPath, map[string]string{
		"top-folder/binary.exe": "exe content",
		"top-folder/lib.dll":    "dll content",
	})

	err := extractZip(zipPath, destDir, 1)
	if err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}

	// Should be flattened — no top-folder
	if _, err := os.Stat(filepath.Join(destDir, "binary.exe")); err != nil {
		t.Error("expected binary.exe at root of destDir after strip")
	}
}

func TestExtractZip_MissingBinary(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	destDir := filepath.Join(tmpDir, "out")

	createTestZip(t, zipPath, map[string]string{
		"top-folder/other.txt": "not a binary",
	})

	err := extractZip(zipPath, destDir, 1)
	if err != nil {
		t.Fatalf("extractZip itself should not fail: %v", err)
	}

	// Validation is separate — check expected binary
	if _, err := os.Stat(filepath.Join(destDir, "whisper-server.exe")); !os.IsNotExist(err) {
		t.Error("expected binary to be absent")
	}
}

func TestAtomicSwap_Success(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "service")
	tempDir := filepath.Join(tmpDir, ".service-install-tmp")

	os.MkdirAll(installDir, 0o755)
	os.WriteFile(filepath.Join(installDir, "old.txt"), []byte("old"), 0o644)
	os.MkdirAll(tempDir, 0o755)
	os.WriteFile(filepath.Join(tempDir, "new.txt"), []byte("new"), 0o644)

	err := atomicSwapDirs(installDir, tempDir)
	if err != nil {
		t.Fatalf("atomicSwapDirs failed: %v", err)
	}

	// New content should be in installDir
	if _, err := os.Stat(filepath.Join(installDir, "new.txt")); err != nil {
		t.Error("expected new.txt in installDir after swap")
	}
	// Old backup should be deleted
	if _, err := os.Stat(installDir + ".bak"); !os.IsNotExist(err) {
		t.Error("expected .bak to be cleaned up")
	}
}

func TestAtomicSwap_StepAFails(t *testing.T) {
	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "nonexistent-dir")
	tempDir := filepath.Join(tmpDir, ".tmp")
	os.MkdirAll(tempDir, 0o755)

	err := atomicSwapDirs(installDir, tempDir)
	if err == nil {
		t.Fatal("expected error when installDir doesn't exist")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go-sidecar && go test -run "TestExtractZip|TestAtomicSwap" -v`
Expected: FAIL — functions undefined.

- [ ] **Step 3: Implement archive.go**

Create `go-sidecar/archive.go`:

```go
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractZip(zipPath, destDir string, stripComponents int) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		name := f.Name
		if stripComponents > 0 {
			parts := strings.SplitN(name, "/", stripComponents+1)
			if len(parts) <= stripComponents {
				continue // skip files at or above the strip level
			}
			name = parts[stripComponents]
			if name == "" {
				continue
			}
		}

		targetPath := filepath.Join(destDir, filepath.FromSlash(name))

		// Prevent zip slip
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0o755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", name, err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip entry %s: %w", name, err)
		}

		outFile, err := os.Create(targetPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create %s: %w", targetPath, err)
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return fmt.Errorf("failed to write %s: %w", targetPath, err)
		}
	}

	return nil
}

func atomicSwapDirs(installDir, tempDir string) error {
	bakDir := installDir + ".bak"

	// Clean up any stale backup
	os.RemoveAll(bakDir)

	// Check if installDir exists (first-time install has no existing dir)
	_, existsErr := os.Stat(installDir)
	hasExisting := existsErr == nil

	if hasExisting {
		// Step A: rename current → .bak
		if err := os.Rename(installDir, bakDir); err != nil {
			return fmt.Errorf("install_locked: cannot move current directory (is a file in use?): %w", err)
		}
	}

	// Step B: rename temp → installDir
	if err := os.Rename(tempDir, installDir); err != nil {
		if hasExisting {
			// Restore .bak
			restoreErr := os.Rename(bakDir, installDir)
			if restoreErr != nil {
				return fmt.Errorf("install_locked: failed to install AND failed to restore backup: move=%w restore=%v", err, restoreErr)
			}
		}
		return fmt.Errorf("install_locked: cannot place new directory (close any programs using the service directory and try again): %w", err)
	}

	// Success — clean up backup if it exists
	if hasExisting {
		os.RemoveAll(bakDir)
	}
	return nil
}

func DownloadAndExtractArchive(
	action Action,
	svcManager *ServiceManager,
	progress func(downloaded, total int64),
) error {
	// Stop managed service for this action's service
	switch action.ServiceID {
	case "whisper":
		svcManager.StopWhisperServer()
	case "llama":
		svcManager.StopLlamaServer()
	}

	// Download to temp file
	tmpZip := filepath.Join(os.TempDir(), "subgen-install-"+action.ServiceID+".zip")
	defer os.Remove(tmpZip)

	sendStage("downloading_dependency", fmt.Sprintf("Downloading %s...", action.Label))
	if err := DownloadModel(action.URL, tmpZip, progress); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Extract to temp sibling directory (same volume for rename)
	parentDir := filepath.Dir(action.InstallDir)
	tempExtractDir := filepath.Join(parentDir, "."+filepath.Base(action.InstallDir)+"-install-tmp")
	os.RemoveAll(tempExtractDir)
	defer os.RemoveAll(tempExtractDir)

	sendStage("extracting", "Extracting files...")
	if err := extractZip(tmpZip, tempExtractDir, action.StripComponents); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Validate expected binary exists
	sendStage("validating", "Validating installation...")
	expectedPath := filepath.Join(tempExtractDir, action.ExpectedBinary)
	if _, err := os.Stat(expectedPath); err != nil {
		return fmt.Errorf("installation validation failed: expected binary %q not found in extracted archive", action.ExpectedBinary)
	}

	// Atomic swap
	if err := atomicSwapDirs(action.InstallDir, tempExtractDir); err != nil {
		return err
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-sidecar && go test -run "TestExtractZip|TestAtomicSwap" -v`
Expected: All tests PASS.

- [ ] **Step 5: Run full test suite**

Run: `cd go-sidecar && go test -v`

- [ ] **Step 6: Commit**

```bash
git add go-sidecar/archive.go go-sidecar/archive_test.go
git commit -m "feat(setup): implement zip extraction and atomic directory swap"
```

---

### Task 4: Wire check_setup and install_dependency in main.go

**Files:**
- Modify: `go-sidecar/main.go`

- [ ] **Step 1: Add global action registry and pipeline mutex**

At the top of `main.go`, after the existing `stdoutMu` and `runGenerate` declarations (lines 15-18), add:

```go
var pipelineMu sync.Mutex
var actionRegistry = NewActionRegistry()
```

- [ ] **Step 2: Add command routing**

In `handleCommand` (line 68), add two new cases before the `default` case:

```go
	case "check_setup":
		result := CheckSetup(svcManager.config, actionRegistry)
		sendJSON(result)

	case "install_dependency":
		if cmd.ActionID == "" {
			sendError("Invalid command", "install_dependency requires action_id")
			return
		}
		go runInstallDependency(cmd.ActionID, svcManager)
```

- [ ] **Step 3: Add runInstallDependency function and mutex to runGenerate**

Add after `handleCommand`:

```go
func runInstallDependency(actionID string, svcManager *ServiceManager) {
	if !pipelineMu.TryLock() {
		sendError("Install failed", "Cannot install while processing. Stop processing first.")
		return
	}
	defer pipelineMu.Unlock()

	action, err := actionRegistry.Resolve(actionID)
	if err != nil {
		sendError("Install failed", err.Error())
		return
	}

	if err := DownloadAndExtractArchive(action, svcManager, func(downloaded, total int64) {
		if total > 0 {
			pct := float64(downloaded) / float64(total) * 100
			sendProgress("downloading_dependency", pct, fmt.Sprintf("Downloading %s / %s", formatBytes(downloaded), formatBytes(total)))
		} else {
			sendProgress("downloading_dependency", 0, fmt.Sprintf("Downloading %s...", formatBytes(downloaded)))
		}
	}); err != nil {
		sendError("Install failed", err.Error())
		return
	}

	sendProgress("validating", 100, "Installation complete")

	// Re-check setup and send fresh status
	result := CheckSetup(svcManager.config, actionRegistry)
	sendJSON(result)
}
```

Also wrap the existing `runGenerate` with TryLock. Change lines 16-18 from:

```go
var runGenerate = func(pipeline *Pipeline, cmd Command) {
	pipeline.Run(cmd)
}
```

to:

```go
var runGenerate = func(pipeline *Pipeline, cmd Command) {
	if !pipelineMu.TryLock() {
		sendError("Generate failed", "Cannot generate while installing. Wait for installation to complete.")
		return
	}
	defer pipelineMu.Unlock()
	pipeline.Run(cmd)
}
```

- [ ] **Step 4: Verify it compiles**

Run: `cd go-sidecar && go build ./...`

- [ ] **Step 5: Run full test suite**

Run: `cd go-sidecar && go test -v`

- [ ] **Step 6: Commit**

```bash
git add go-sidecar/main.go
git commit -m "feat(setup): wire check_setup and install_dependency commands with mutex serialization"
```

---

### Task 5: Add setup types to frontend and helper functions (TDD)

**Files:**
- Modify: `src/lib/types.ts`
- Create: `src/lib/setupHelpers.ts`
- Create: `src/lib/setupHelpers.test.ts`

- [ ] **Step 1: Add types to types.ts**

In `src/lib/types.ts`, add the new interfaces before the `SidecarCommand` union (before line 52), and update both unions:

Add before `SidecarCommand`:

```typescript
export type RequiredFor = "transcription" | "translation";

export interface ServiceIssue {
  code: string;
  observed_error?: string;
}

export interface ServiceAction {
  id: string;
  label: string;
  description: string;
  kind: "archive" | "manual";
  preferred?: boolean;
  guidance?: string;
}

export interface ServiceStatus {
  id: string;
  display_name: string;
  required_for: RequiredFor;
  state: "ready" | "action_required";
  issues: ServiceIssue[];
  actions: ServiceAction[];
}

export interface SetupStatusResponse {
  type: "setup_status";
  services: ServiceStatus[];
}

export interface CheckSetupCommand {
  command: "check_setup";
}

export interface InstallDependencyCommand {
  command: "install_dependency";
  action_id: string;
}
```

Update `SidecarCommand` to include:

```typescript
export type SidecarCommand =
  | GenerateCommand
  | ListLanguagesCommand
  | SystemInfoCommand
  | StartServicesCommand
  | StopServicesCommand
  | VramInfoCommand
  | CheckSetupCommand
  | InstallDependencyCommand;
```

Update `SidecarResponse` to include:

```typescript
export type SidecarResponse =
  | ProgressResponse
  | StageResponse
  | CompleteResponse
  | ErrorResponse
  | LanguagesResponse
  | SystemInfoResponse
  | VramInfoResponse
  | SetupStatusResponse;
```

- [ ] **Step 2: Write failing tests for setupHelpers**

Create `src/lib/setupHelpers.test.ts`:

```typescript
import { describe, expect, it } from "vitest";
import { shouldDisableGenerate, formatSetupIssue } from "./setupHelpers";
import type { ServiceStatus, SetupStatusResponse } from "./types";

function makeStatus(overrides: Partial<ServiceStatus>[]): SetupStatusResponse {
  return {
    type: "setup_status",
    services: overrides.map((o) => ({
      id: o.id ?? "test",
      display_name: o.display_name ?? "test",
      required_for: o.required_for ?? "transcription",
      state: o.state ?? "ready",
      issues: o.issues ?? [],
      actions: o.actions ?? [],
    })),
  };
}

describe("shouldDisableGenerate", () => {
  it("returns false when all services ready", () => {
    const status = makeStatus([{ state: "ready", required_for: "transcription" }]);
    expect(shouldDisableGenerate(status, "")).toBe(false);
  });

  it("returns true when transcription service needs action", () => {
    const status = makeStatus([{ state: "action_required", required_for: "transcription" }]);
    expect(shouldDisableGenerate(status, "")).toBe(true);
  });

  it("returns true when translation service needs action and targetLang set", () => {
    const status = makeStatus([{ state: "action_required", required_for: "translation" }]);
    expect(shouldDisableGenerate(status, "en")).toBe(true);
  });

  it("returns false when translation service needs action but no targetLang", () => {
    const status = makeStatus([{ state: "action_required", required_for: "translation" }]);
    expect(shouldDisableGenerate(status, "")).toBe(false);
  });

  it("returns false when setup status is null", () => {
    expect(shouldDisableGenerate(null, "en")).toBe(false);
  });
});

describe("formatSetupIssue", () => {
  it("returns observed_error when present", () => {
    expect(formatSetupIssue({ code: "binary_not_runnable", observed_error: "dll missing" }))
      .toContain("dll missing");
  });

  it("returns fallback for binary_not_found", () => {
    expect(formatSetupIssue({ code: "binary_not_found" }))
      .toContain("not found");
  });

  it("returns fallback for not_in_path", () => {
    expect(formatSetupIssue({ code: "not_in_path" }))
      .toContain("PATH");
  });

  it("returns fallback for binary_not_runnable without observed_error", () => {
    expect(formatSetupIssue({ code: "binary_not_runnable" }))
      .toContain("cannot start");
  });
});
```

- [ ] **Step 3: Implement setupHelpers.ts**

Create `src/lib/setupHelpers.ts`:

```typescript
import type { ServiceIssue, SetupStatusResponse } from "./types";

export function shouldDisableGenerate(
  setupStatus: SetupStatusResponse | null,
  targetLang: string
): boolean {
  if (!setupStatus) return false;

  for (const service of setupStatus.services) {
    if (service.state !== "action_required") continue;

    if (service.required_for === "transcription") return true;
    if (service.required_for === "translation" && targetLang) return true;
  }

  return false;
}

export function formatSetupIssue(issue: ServiceIssue): string {
  if (issue.observed_error) {
    return `Service cannot start: ${issue.observed_error}`;
  }

  switch (issue.code) {
    case "binary_not_found":
      return "Binary not found. Install the service using the options below.";
    case "binary_not_runnable":
      return "Service binary cannot start. Required libraries may be missing.";
    case "not_in_path":
      return "Not found in system PATH. Install it and add to PATH.";
    default:
      return `Setup issue: ${issue.code}`;
  }
}
```

- [ ] **Step 4: Run tests**

Run: `npx vitest run src/lib/setupHelpers.test.ts`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add src/lib/types.ts src/lib/setupHelpers.ts src/lib/setupHelpers.test.ts
git commit -m "feat(setup): add setup types and helper functions with tests"
```

---

### Task 6: Implement install state reducer (TDD)

**Files:**
- Create: `src/lib/installState.ts`
- Create: `src/lib/installState.test.ts`

- [ ] **Step 1: Write failing tests**

Create `src/lib/installState.test.ts`:

```typescript
import { describe, expect, it } from "vitest";
import {
  advanceInstallState,
  createInitialInstallState,
  isInstallComplete,
  type InstallState,
} from "./installState";
import type { ProgressResponse, StageResponse, SetupStatusResponse } from "./types";

describe("advanceInstallState", () => {
  it("advances through download stage", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const response: ProgressResponse = {
      type: "progress",
      stage: "downloading_dependency",
      percent: 50,
      message: "Downloading...",
    };
    const next = advanceInstallState(state, response);
    expect(next.stage).toBe("downloading_dependency");
    expect(next.percent).toBe(50);
  });

  it("advances to extracting stage", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const response: StageResponse = {
      type: "stage",
      stage: "extracting",
      message: "Extracting...",
    };
    const next = advanceInstallState(state, response);
    expect(next.stage).toBe("extracting");
  });

  it("preserves pending action id", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const response: ProgressResponse = {
      type: "progress",
      stage: "downloading_dependency",
      percent: 10,
      message: "...",
    };
    const next = advanceInstallState(state, response);
    expect(next.pendingActionId).toBe("whisper/install_gpu_bundle");
    expect(next.targetServiceId).toBe("whisper");
  });
});

describe("isInstallComplete", () => {
  it("returns true when target service is ready", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const setupStatus: SetupStatusResponse = {
      type: "setup_status",
      services: [
        { id: "whisper", display_name: "whisper-server", required_for: "transcription", state: "ready", issues: [], actions: [] },
      ],
    };
    expect(isInstallComplete(state, setupStatus)).toBe(true);
  });

  it("returns false when target service still has issues", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const setupStatus: SetupStatusResponse = {
      type: "setup_status",
      services: [
        { id: "whisper", display_name: "whisper-server", required_for: "transcription", state: "action_required", issues: [{ code: "binary_not_runnable" }], actions: [] },
      ],
    };
    expect(isInstallComplete(state, setupStatus)).toBe(false);
  });

  it("returns false when no pending action", () => {
    const state = createInitialInstallState("", "");
    const setupStatus: SetupStatusResponse = {
      type: "setup_status",
      services: [],
    };
    expect(isInstallComplete(state, setupStatus)).toBe(false);
  });
});
```

- [ ] **Step 2: Implement installState.ts**

Create `src/lib/installState.ts`:

```typescript
import type { ProgressResponse, StageResponse, SetupStatusResponse } from "./types";

export interface InstallState {
  stage: string;
  percent: number | null;
  message: string;
  pendingActionId: string;
  targetServiceId: string;
}

export function createInitialInstallState(
  actionId: string,
  serviceId: string
): InstallState {
  return {
    stage: "",
    percent: null,
    message: "Starting installation...",
    pendingActionId: actionId,
    targetServiceId: serviceId,
  };
}

export function advanceInstallState(
  current: InstallState,
  response: ProgressResponse | StageResponse
): InstallState {
  if (response.type === "progress") {
    return {
      ...current,
      stage: response.stage,
      percent: Math.max(0, Math.min(response.percent, 100)),
      message: response.message,
    };
  }

  return {
    ...current,
    stage: response.stage,
    percent: null,
    message: response.message,
  };
}

export function isInstallComplete(
  state: InstallState,
  setupStatus: SetupStatusResponse
): boolean {
  if (!state.pendingActionId || !state.targetServiceId) return false;

  const service = setupStatus.services.find(
    (s) => s.id === state.targetServiceId
  );
  return service?.state === "ready";
}
```

- [ ] **Step 3: Run tests**

Run: `npx vitest run src/lib/installState.test.ts`
Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add src/lib/installState.ts src/lib/installState.test.ts
git commit -m "feat(setup): implement install state reducer with tests"
```

---

### Task 7: Parameterize ProcessingView stage labels

**Files:**
- Modify: `src/components/ProcessingView.tsx`

- [ ] **Step 1: Add optional stageOrder and stageLabels props**

In `ProcessingView.tsx`, update the props interface to accept optional stage config:

```typescript
interface ProcessingViewProps {
  stage: string;
  percent: number | null;
  message: string;
  elapsedSecs: number | null;
  etaSecs: number | null;
  onStop?: () => void;
  stopDisabled?: boolean;
  stopLabel?: string;
  stageOrder?: string[];
  stageLabels?: Record<string, string>;
}
```

Change the module-level constants to defaults:

```typescript
const DEFAULT_STAGE_ORDER = [
  "validating",
  "downloading_model",
  "starting_services",
  "preprocessing",
  "transcribing",
  "translating",
  "writing",
];

const DEFAULT_STAGE_LABELS: Record<string, string> = {
  validating: "Validate",
  downloading_model: "Download",
  starting_services: "Services",
  preprocessing: "Enhance",
  transcribing: "Transcribe",
  translating: "Translate",
  writing: "Write",
};
```

In the component, use props with fallback to defaults:

```typescript
  const stages = stageOrder ?? DEFAULT_STAGE_ORDER;
  const labels = stageLabels ?? DEFAULT_STAGE_LABELS;
  const currentIndex = stages.indexOf(stage);
```

Replace all references to `STAGE_ORDER` with `stages` and `STAGE_LABELS` with `labels` in the JSX.

- [ ] **Step 2: Verify TypeScript compiles**

Run: `npm run build`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add src/components/ProcessingView.tsx
git commit -m "feat(setup): parameterize ProcessingView stage labels for install flow"
```

---

### Task 8: Create SetupBanner component

**Files:**
- Create: `src/components/SetupBanner.tsx`

- [ ] **Step 1: Create SetupBanner**

Create `src/components/SetupBanner.tsx`:

```tsx
import { Button } from "@/components/ui/button";
import { HugeiconsIcon } from "@hugeicons/react";
import { Alert02Icon } from "@hugeicons/core-free-icons";
import { formatSetupIssue } from "@/lib/setupHelpers";
import type { SetupStatusResponse } from "@/lib/types";

interface SetupBannerProps {
  setupStatus: SetupStatusResponse;
  onInstall: (actionId: string) => void;
  disabled?: boolean;
}

export function SetupBanner({ setupStatus, onInstall, disabled }: SetupBannerProps) {
  const servicesWithIssues = setupStatus.services.filter(
    (s) => s.state === "action_required"
  );

  if (servicesWithIssues.length === 0) return null;

  return (
    <div className="space-y-3">
      {servicesWithIssues.map((service) => (
        <div
          key={service.id}
          className="border border-chart-4/30 bg-chart-4/5 p-4 space-y-3"
        >
          <div className="flex items-start gap-3">
            <HugeiconsIcon
              icon={Alert02Icon}
              className="mt-0.5 size-4 shrink-0 text-chart-4"
              strokeWidth={2}
            />
            <div className="flex-1 space-y-1">
              <p className="text-xs font-medium text-foreground">
                {service.display_name}{" "}
                <span className="text-muted-foreground font-normal">
                  — required for {service.required_for}
                </span>
              </p>
              {service.issues.map((issue, i) => (
                <p key={i} className="text-[11px] text-muted-foreground">
                  {formatSetupIssue(issue)}
                </p>
              ))}
            </div>
          </div>

          {service.actions.length > 0 && (
            <div className="flex flex-wrap gap-2 pl-7">
              {service.actions.map((action) =>
                action.kind === "archive" ? (
                  <Button
                    key={action.id}
                    size="sm"
                    variant={action.preferred ? "default" : "outline"}
                    className="text-[11px]"
                    onClick={() => onInstall(action.id)}
                    disabled={disabled}
                  >
                    {action.label}
                  </Button>
                ) : (
                  <p
                    key={action.id}
                    className="text-[11px] text-muted-foreground"
                  >
                    {action.guidance}
                  </p>
                )
              )}
            </div>
          )}

          {service.actions.some((a) => a.kind === "archive") && (
            <div className="pl-7 space-y-0.5">
              {service.actions
                .filter((a) => a.kind === "archive")
                .map((a) => (
                  <p key={a.id} className="text-[10px] text-muted-foreground">
                    {a.preferred ? "▸ " : "  "}
                    {a.description}
                  </p>
                ))}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `npm run build`

- [ ] **Step 3: Commit**

```bash
git add src/components/SetupBanner.tsx
git commit -m "feat(setup): create SetupBanner component for dependency warnings"
```

---

### Task 9: Wire everything in App.tsx

**Files:**
- Modify: `src/App.tsx`

- [ ] **Step 1: Add imports**

Add to existing imports in `App.tsx`:

```typescript
import { SetupBanner } from "./components/SetupBanner";
import { shouldDisableGenerate } from "./lib/setupHelpers";
import {
  advanceInstallState,
  createInitialInstallState,
  isInstallComplete,
  type InstallState,
} from "./lib/installState";
import type { SetupStatusResponse } from "./lib/types";
```

- [ ] **Step 2: Update AppState type and add state**

Change the AppState type (line 34):

```typescript
type AppState = "idle" | "processing" | "complete" | "error" | "installing";
```

After the `isStopping` state (line 69), add:

```typescript
  const [setupStatus, setSetupStatus] = useState<SetupStatusResponse | null>(null);
  const [installState, setInstallState] = useState<InstallState | null>(null);
```

Add refs for values read inside the `onResponse` closure (same pattern as `handleVramResponseRef` and `sendCommandRef`):

```typescript
  const appStateRef = useRef(appState);
  appStateRef.current = appState;

  const installStateRef = useRef(installState);
  installStateRef.current = installState;
```

- [ ] **Step 3: Send check_setup on connect**

In the `useEffect` that sends `system_info` and `list_languages` (around line 150), add:

```typescript
    sendCommand({ command: "check_setup" }).catch((err) => {
      console.error("Failed to check setup:", err);
    });
```

- [ ] **Step 4: Handle setup_status and install progress in onResponse**

In the `onResponse` switch (around line 97), add a case for `"setup_status"` before the `"error"` case:

```typescript
        case "setup_status":
          setSetupStatus(response);
          // Check if install completed (use refs to avoid stale closure)
          if (appStateRef.current === "installing" && installStateRef.current) {
            if (isInstallComplete(installStateRef.current, response)) {
              setAppState("idle");
              setInstallState(null);
            }
          }
          break;
```

Update the `"progress"` and `"stage"` cases to route based on appState:

```typescript
        case "progress":
          if (appStateRef.current === "installing") {
            setInstallState((current) =>
              current ? advanceInstallState(current, response) : current
            );
          } else {
            setProcessing((current) => advanceProcessingState(current, response));
          }
          break;
        case "stage":
          if (appStateRef.current === "installing") {
            setInstallState((current) =>
              current ? advanceInstallState(current, response) : current
            );
          } else {
            setProcessing((current) => advanceProcessingState(current, response));
          }
          break;
```

- [ ] **Step 5: Add install handler**

After `handleStopProcessing`, add:

```typescript
  const handleInstall = useCallback(
    async (actionId: string) => {
      const serviceId = actionId.split("/")[0];
      setAppState("installing");
      setInstallState(createInitialInstallState(actionId, serviceId));
      setErrorMsg("");

      try {
        await sendCommand({ command: "install_dependency", action_id: actionId });
      } catch (err) {
        setErrorMsg(`Failed to start install: ${err}`);
        setAppState("error");
        setInstallState(null);
      }
    },
    [sendCommand]
  );
```

- [ ] **Step 6: Update render — add SetupBanner and installing state**

In the JSX, add the installing state handler alongside the processing view (after the `isProcessing ?` ternary, around line 316):

```tsx
        ) : appState === "installing" && installState ? (
          <ProcessingView
            stage={installState.stage}
            percent={installState.percent}
            message={installState.message}
            elapsedSecs={null}
            etaSecs={null}
            stageOrder={["downloading_dependency", "extracting", "validating"]}
            stageLabels={{
              downloading_dependency: "Download",
              extracting: "Extract",
              validating: "Validate",
            }}
          />
```

Add the SetupBanner before VideoDropzone (inside the idle `<>` block, around line 328):

```tsx
            {setupStatus && (
              <SetupBanner
                setupStatus={setupStatus}
                onInstall={handleInstall}
              />
            )}
```

Update the Generate button disabled condition (around line 385):

```typescript
              disabled={
                !videoPath ||
                !connected ||
                shouldDisableGenerate(setupStatus, targetLang)
              }
```

- [ ] **Step 7: Verify TypeScript compiles**

Run: `npm run build`
Expected: No errors.

- [ ] **Step 8: Commit**

```bash
git add src/App.tsx
git commit -m "feat(setup): wire setup status, install flow, and SetupBanner into App"
```

---

### Task 10: End-to-end verification

- [ ] **Step 1: Run Go test suite**

Run: `cd go-sidecar && go test -v`
Expected: All tests pass.

- [ ] **Step 2: Run frontend build**

Run: `npm run build`
Expected: No errors.

- [ ] **Step 3: Run frontend tests**

Run: `npx vitest run`
Expected: All tests pass.

- [ ] **Step 4: Review all changes**

Run: `git diff main --stat`

Expected new/modified files:
- `go-sidecar/setup.go` (new)
- `go-sidecar/setup_test.go` (new)
- `go-sidecar/archive.go` (new)
- `go-sidecar/archive_test.go` (new)
- `go-sidecar/config.go` (modified)
- `go-sidecar/main.go` (modified)
- `src/lib/types.ts` (modified)
- `src/lib/setupHelpers.ts` (new)
- `src/lib/setupHelpers.test.ts` (new)
- `src/lib/installState.ts` (new)
- `src/lib/installState.test.ts` (new)
- `src/components/ProcessingView.tsx` (modified)
- `src/components/SetupBanner.tsx` (new)
- `src/App.tsx` (modified)
