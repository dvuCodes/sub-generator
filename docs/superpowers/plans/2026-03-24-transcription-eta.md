# Transcription Elapsed Time + ETA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the "---" placeholder during transcription with a live elapsed timer and estimated time remaining.

**Architecture:** A Go goroutine sends progress updates every second while `Transcribe()` blocks. Audio duration comes from `ffprobe`; ETA is estimated using conservative model-size speed ratios. Frontend propagates new `elapsed_secs`/`eta_secs` fields and renders a timer display.

**Tech Stack:** Go (goroutine + ticker), ffprobe CLI, React/TypeScript, Tauri IPC (stdin/stdout JSON lines)

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `go-sidecar/media.go` | `MediaDuration()` — probe file duration via ffprobe |
| Create | `go-sidecar/media_test.go` | Tests for MediaDuration |
| Create | `go-sidecar/eta.go` | Speed ratios, ETA estimation, duration formatting |
| Create | `go-sidecar/eta_test.go` | Tests for speed ratios, estimation, formatting |
| Modify | `go-sidecar/config.go:28-33` | Add `ElapsedSecs`/`ETASecs` to `ProgressResponse` |
| Modify | `go-sidecar/main.go:118-125` | Add stdout mutex + `sendTimerProgress` helper |
| Modify | `go-sidecar/pipeline.go:97-116` | Goroutine timer wrapping `Transcribe()` call |
| Modify | `src/lib/types.ts:48-53` | Add optional `elapsed_secs`/`eta_secs` fields |
| Modify | `src/lib/processingState.ts` | Extend `ProcessingState` + propagation logic |
| Modify | `src/components/ProcessingView.tsx:7-14,94-108` | Timer display + `formatTime` helper |
| Modify | `src/App.tsx:152-156,332-339` | Pass new props, include in initial state |

---

### Task 1: MediaDuration — ffprobe integration

**Files:**
- Create: `go-sidecar/media.go`
- Create: `go-sidecar/media_test.go`

- [ ] **Step 1: Write the test file**

```go
// go-sidecar/media_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMediaDurationReturnsErrorForMissingFile(t *testing.T) {
	_, err := MediaDuration("/nonexistent/file.mp4")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestMediaDurationReturnsErrorForEmptyPath(t *testing.T) {
	_, err := MediaDuration("")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestMediaDurationReturnsErrorForDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := MediaDuration(dir)
	if err == nil {
		t.Fatal("expected error for directory, got nil")
	}
}

func TestMediaDurationReturnsErrorForTextFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-media.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := MediaDuration(path)
	if err == nil {
		t.Fatal("expected error for text file, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-sidecar && go test -run TestMediaDuration -v`
Expected: compilation error — `MediaDuration` undefined

- [ ] **Step 3: Write the implementation**

```go
// go-sidecar/media.go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-sidecar && go test -run TestMediaDuration -v`
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
cd go-sidecar && git add media.go media_test.go
git commit -m "feat: add MediaDuration ffprobe integration for audio duration probing"
```

---

### Task 2: ETA estimation — speed ratios + duration formatting

**Files:**
- Create: `go-sidecar/eta.go`
- Create: `go-sidecar/eta_test.go`

- [ ] **Step 1: Write the test file**

```go
// go-sidecar/eta_test.go
package main

import "testing"

func TestModelSpeedRatioKnownModels(t *testing.T) {
	tests := []struct {
		model  string
		hasGPU bool
		want   float64
	}{
		{"tiny", true, 30.0},
		{"base", true, 15.0},
		{"small", true, 8.0},
		{"medium", true, 4.0},
		{"large-v3", true, 2.0},
		{"turbo", true, 10.0},
		{"tiny", false, 8.0},
		{"base", false, 4.0},
		{"small", false, 2.0},
		{"medium", false, 0.8},
		{"large-v3", false, 0.3},
		{"turbo", false, 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := ModelSpeedRatio(tt.model, tt.hasGPU)
			if got != tt.want {
				t.Errorf("ModelSpeedRatio(%q, %v) = %v, want %v", tt.model, tt.hasGPU, got, tt.want)
			}
		})
	}
}

func TestModelSpeedRatioUnknownModel(t *testing.T) {
	got := ModelSpeedRatio("nonexistent", true)
	if got != 0 {
		t.Errorf("ModelSpeedRatio unknown model = %v, want 0", got)
	}
}

func TestEstimateTranscriptionSeconds(t *testing.T) {
	// 60 seconds of audio, base model, GPU (ratio=15) => 60/15 = 4 seconds
	got := EstimateTranscriptionSeconds(60.0, "base", true)
	if got != 4.0 {
		t.Errorf("EstimateTranscriptionSeconds(60, base, GPU) = %v, want 4.0", got)
	}
}

func TestEstimateTranscriptionSecondsZeroDuration(t *testing.T) {
	got := EstimateTranscriptionSeconds(0, "base", true)
	if got != 0 {
		t.Errorf("EstimateTranscriptionSeconds(0, ...) = %v, want 0", got)
	}
}

func TestEstimateTranscriptionSecondsUnknownModel(t *testing.T) {
	got := EstimateTranscriptionSeconds(60, "nonexistent", true)
	if got != 0 {
		t.Errorf("EstimateTranscriptionSeconds with unknown model = %v, want 0", got)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		secs float64
		want string
	}{
		{0, "0:00"},
		{5, "0:05"},
		{59, "0:59"},
		{60, "1:00"},
		{90, "1:30"},
		{3599, "59:59"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{-5, "0:00"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.secs)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.secs, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go-sidecar && go test -run "TestModelSpeedRatio|TestEstimate|TestFormatDuration" -v`
Expected: compilation error — functions undefined

- [ ] **Step 3: Write the implementation**

```go
// go-sidecar/eta.go
package main

import "fmt"

var gpuSpeedRatios = map[string]float64{
	"tiny":     30.0,
	"base":     15.0,
	"small":    8.0,
	"medium":   4.0,
	"large-v3": 2.0,
	"turbo":    10.0,
}

var cpuSpeedRatios = map[string]float64{
	"tiny":     8.0,
	"base":     4.0,
	"small":    2.0,
	"medium":   0.8,
	"large-v3": 0.3,
	"turbo":    3.0,
}

func ModelSpeedRatio(modelSize string, hasGPU bool) float64 {
	ratios := cpuSpeedRatios
	if hasGPU {
		ratios = gpuSpeedRatios
	}
	return ratios[modelSize]
}

func EstimateTranscriptionSeconds(audioDurationSecs float64, modelSize string, hasGPU bool) float64 {
	ratio := ModelSpeedRatio(modelSize, hasGPU)
	if ratio <= 0 || audioDurationSecs <= 0 {
		return 0
	}
	return audioDurationSecs / ratio
}

func formatDuration(secs float64) string {
	total := int(secs)
	if total < 0 {
		total = 0
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-sidecar && go test -run "TestModelSpeedRatio|TestEstimate|TestFormatDuration" -v`
Expected: all 9 tests PASS

- [ ] **Step 5: Commit**

```bash
cd go-sidecar && git add eta.go eta_test.go
git commit -m "feat: add model speed ratios and ETA estimation for transcription"
```

---

### Task 3: Extend ProgressResponse with timer fields

**Files:**
- Modify: `go-sidecar/config.go:28-33`

- [ ] **Step 1: Add optional pointer fields to ProgressResponse**

In `go-sidecar/config.go`, change `ProgressResponse` (lines 28-33) from:

```go
type ProgressResponse struct {
	Type    string  `json:"type"`
	Stage   string  `json:"stage"`
	Percent float64 `json:"percent"`
	Message string  `json:"message"`
}
```

to:

```go
type ProgressResponse struct {
	Type        string   `json:"type"`
	Stage       string   `json:"stage"`
	Percent     float64  `json:"percent"`
	Message     string   `json:"message"`
	ElapsedSecs *float64 `json:"elapsed_secs,omitempty"`
	ETASecs     *float64 `json:"eta_secs,omitempty"`
}
```

- [ ] **Step 2: Verify existing tests still pass**

Run: `cd go-sidecar && go test ./... -v`
Expected: all existing tests PASS (omitempty means zero change to existing JSON output)

- [ ] **Step 3: Commit**

```bash
cd go-sidecar && git add config.go
git commit -m "feat: add elapsed_secs and eta_secs optional fields to ProgressResponse"
```

---

### Task 4: Thread-safe stdout + sendTimerProgress helper

**Files:**
- Modify: `go-sidecar/main.go:1-10,118-125`

- [ ] **Step 1: Add sync import and stdout mutex**

In `go-sidecar/main.go`, add `"sync"` to the import block (line 4-10):

```go
import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)
```

Add the mutex variable after the imports, before `main()`:

```go
var stdoutMu sync.Mutex
```

- [ ] **Step 2: Wrap sendJSON with mutex**

Change `sendJSON` (lines 118-125) from:

```go
func sendJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
```

to:

```go
func sendJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON marshal error: %v\n", err)
		return
	}
	stdoutMu.Lock()
	fmt.Println(string(data))
	stdoutMu.Unlock()
}
```

- [ ] **Step 3: Add sendTimerProgress helper**

Add after `sendStage` (after line 150):

```go
func sendTimerProgress(stage string, percent float64, message string, elapsedSecs float64, etaSecs float64) {
	resp := ProgressResponse{
		Type:        "progress",
		Stage:       stage,
		Percent:     percent,
		Message:     message,
		ElapsedSecs: &elapsedSecs,
	}
	if etaSecs > 0 {
		resp.ETASecs = &etaSecs
	}
	sendJSON(resp)
}
```

- [ ] **Step 4: Verify compilation and existing tests**

Run: `cd go-sidecar && go test ./... -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
cd go-sidecar && git add main.go
git commit -m "feat: add stdout mutex for thread safety and sendTimerProgress helper"
```

---

### Task 5: Goroutine timer in pipeline transcription step

**Files:**
- Modify: `go-sidecar/pipeline.go:1-9,97-116`

- [ ] **Step 1: Update import block**

No change needed — `"fmt"`, `"time"` are already imported at lines 4-8.

- [ ] **Step 2: Replace the transcription block**

Replace lines 97-116 (the `// Step 4: Transcribe` section) with:

```go
	// Step 4: Transcribe
	sendStage("transcribing", "Transcribing speech...")

	mediaDuration, probeErr := MediaDuration(cmd.InputVideo)
	if probeErr != nil {
		fmt.Fprintf(os.Stderr, "ffprobe duration probe failed (ETA unavailable): %v\n", probeErr)
	}

	hasGPU := detectGPU() != "none"
	estimatedSecs := EstimateTranscriptionSeconds(mediaDuration, cmd.ModelSize, hasGPU)

	done := make(chan struct{})
	transcribeStart := time.Now()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed := time.Since(transcribeStart).Seconds()

				var percent float64
				var etaSecs float64
				var msg string

				if estimatedSecs > 0 {
					percent = (elapsed / estimatedSecs) * 100
					if percent > 99 {
						percent = 99
					}
					etaSecs = estimatedSecs - elapsed
					if etaSecs < 0 {
						etaSecs = 0
					}

					if elapsed > estimatedSecs*1.2 {
						msg = fmt.Sprintf("Transcribing... %s elapsed (taking longer than expected)", formatDuration(elapsed))
					} else {
						msg = fmt.Sprintf("Transcribing... %s elapsed / ~%s remaining", formatDuration(elapsed), formatDuration(etaSecs))
					}
				} else {
					msg = fmt.Sprintf("Transcribing... %s elapsed", formatDuration(elapsed))
				}

				sendTimerProgress("transcribing", percent, msg, elapsed, etaSecs)
			}
		}
	}()

	transcriber := NewTranscriber(p.svcManager.config.WhisperPort)
	result, err := transcriber.Transcribe(
		cmd.InputVideo,
		cmd.SourceLang,
		cmd.BeamSize,
		cmd.VADFilter,
	)
	close(done)

	if err != nil {
		sendError("Transcription failed", err.Error())
		return
	}

	if err := validateTranscriptionResult(result); err != nil {
		sendError("Transcription failed", err.Error())
		return
	}

	sendProgress("transcribing", 100, fmt.Sprintf("Transcribed %d segments", len(result.Segments)))
```

- [ ] **Step 3: Verify compilation and existing tests**

Run: `cd go-sidecar && go build -o go-sidecar.exe . && go test ./... -v`
Expected: build succeeds, all tests PASS

- [ ] **Step 4: Commit**

```bash
cd go-sidecar && git add pipeline.go
git commit -m "feat: add goroutine timer for elapsed/ETA progress during transcription"
```

---

### Task 6: Frontend types — add timer fields

**Files:**
- Modify: `src/lib/types.ts:48-53`

- [ ] **Step 1: Extend ProgressResponse**

In `src/lib/types.ts`, change `ProgressResponse` (lines 48-53) from:

```typescript
export interface ProgressResponse {
  type: "progress";
  stage: string;
  percent: number;
  message: string;
}
```

to:

```typescript
export interface ProgressResponse {
  type: "progress";
  stage: string;
  percent: number;
  message: string;
  elapsed_secs?: number;
  eta_secs?: number;
}
```

- [ ] **Step 2: Verify frontend compiles**

Run: `npm run build`
Expected: build succeeds

- [ ] **Step 3: Commit**

```bash
git add src/lib/types.ts
git commit -m "feat: add elapsed_secs and eta_secs to ProgressResponse type"
```

---

### Task 7: Processing state — propagate timer fields

**Files:**
- Modify: `src/lib/processingState.ts`

- [ ] **Step 1: Extend ProcessingState and update functions**

Replace the entire `src/lib/processingState.ts` with:

```typescript
import type { ProgressResponse, StageResponse } from "./types";

export interface ProcessingState {
  stage: string;
  percent: number | null;
  message: string;
  elapsedSecs: number | null;
  etaSecs: number | null;
}

export function createInitialProcessingState(): ProcessingState {
  return {
    stage: "",
    percent: null,
    message: "",
    elapsedSecs: null,
    etaSecs: null,
  };
}

export function advanceProcessingState(
  current: ProcessingState,
  response: ProgressResponse | StageResponse
): ProcessingState {
  if (response.type === "progress") {
    return {
      stage: response.stage,
      percent: clampPercent(response.percent),
      message: response.message,
      elapsedSecs: response.elapsed_secs ?? null,
      etaSecs: response.eta_secs ?? null,
    };
  }

  return {
    stage: response.stage,
    percent: current.stage === response.stage ? current.percent : null,
    message: response.message,
    elapsedSecs: null,
    etaSecs: null,
  };
}

function clampPercent(percent: number) {
  return Math.max(0, Math.min(percent, 100));
}
```

- [ ] **Step 2: Verify frontend compiles**

Run: `npm run build`
Expected: build FAILS — `ProcessingView` and `App.tsx` don't pass new required props yet (this is expected, we'll fix in next tasks)

- [ ] **Step 3: Commit**

```bash
git add src/lib/processingState.ts
git commit -m "feat: extend ProcessingState with elapsedSecs and etaSecs fields"
```

---

### Task 8: ProcessingView — render elapsed timer

**Files:**
- Modify: `src/components/ProcessingView.tsx:7-14,94-108`

- [ ] **Step 1: Add formatTime helper and update props**

Replace the entire `src/components/ProcessingView.tsx` with:

```tsx
import { cn } from "@/lib/utils";
import { Progress } from "@/components/ui/progress";
import { Button } from "@/components/ui/button";
import { HugeiconsIcon } from "@hugeicons/react";
import { StopCircleIcon, Tick02Icon } from "@hugeicons/core-free-icons";

interface ProcessingViewProps {
  stage: string;
  percent: number | null;
  message: string;
  elapsedSecs: number | null;
  etaSecs: number | null;
  onStop?: () => void;
  stopDisabled?: boolean;
  stopLabel?: string;
}

const STAGE_ORDER = [
  "validating",
  "downloading_model",
  "starting_services",
  "transcribing",
  "translating",
  "writing",
];

const STAGE_LABELS: Record<string, string> = {
  validating: "Validate",
  downloading_model: "Download",
  starting_services: "Services",
  transcribing: "Transcribe",
  translating: "Translate",
  writing: "Write",
};

function formatTime(secs: number): string {
  const total = Math.round(secs);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0)
    return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export function ProcessingView({
  stage,
  percent,
  message,
  elapsedSecs,
  etaSecs,
  onStop,
  stopDisabled = false,
  stopLabel = "Stop",
}: ProcessingViewProps) {
  const currentIndex = STAGE_ORDER.indexOf(stage);
  const isDeterminate = typeof percent === "number";
  const displayPercent = isDeterminate ? Math.round(percent) : null;

  return (
    <div className="space-y-5">
      {/* Stage pipeline */}
      <div className="flex items-center gap-1">
        {STAGE_ORDER.map((s, i) => {
          const isActive = i === currentIndex;
          const isComplete = i < currentIndex;

          return (
            <div key={s} className="flex flex-1 items-center gap-1">
              <div className="flex flex-1 flex-col items-center gap-1.5">
                <div
                  className={cn(
                    "flex size-7 items-center justify-center text-[10px] font-medium transition-all",
                    isComplete && "bg-chart-1 text-background",
                    isActive && "bg-primary text-primary-foreground animate-pulse",
                    !isActive && !isComplete && "bg-muted text-muted-foreground"
                  )}
                >
                  {isComplete ? (
                    <HugeiconsIcon icon={Tick02Icon} className="size-3.5" strokeWidth={2.5} />
                  ) : (
                    i + 1
                  )}
                </div>
                <span
                  className={cn(
                    "text-[10px] font-medium uppercase tracking-wider",
                    isActive ? "text-foreground" : "text-muted-foreground"
                  )}
                >
                  {STAGE_LABELS[s] ?? s}
                </span>
              </div>
              {i < STAGE_ORDER.length - 1 && (
                <div
                  className={cn(
                    "mb-5 h-px flex-1 transition-colors",
                    isComplete ? "bg-chart-1" : "bg-border"
                  )}
                />
              )}
            </div>
          );
        })}
      </div>

      {/* Progress */}
      <div className="space-y-2">
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">{message}</span>
          <span className="font-mono text-foreground">
            {elapsedSecs != null
              ? formatTime(elapsedSecs)
              : isDeterminate
                ? `${displayPercent}%`
                : "---"}
          </span>
        </div>
        {isDeterminate ? (
          <Progress value={displayPercent ?? 0} />
        ) : (
          <div className="relative h-1 w-full overflow-hidden bg-muted">
            <div className="processing-indeterminate h-full w-2/5 bg-primary" />
          </div>
        )}
      </div>

      {onStop && (
        <Button
          variant="destructive"
          size="lg"
          className="w-full"
          onClick={onStop}
          disabled={stopDisabled}
        >
          <HugeiconsIcon icon={StopCircleIcon} className="size-4" strokeWidth={1.5} />
          {stopLabel}
        </Button>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify frontend compiles**

Run: `npm run build`
Expected: build FAILS — `App.tsx` doesn't pass `elapsedSecs`/`etaSecs` yet (fixed in next task)

- [ ] **Step 3: Commit**

```bash
git add src/components/ProcessingView.tsx
git commit -m "feat: render elapsed timer and ETA in ProcessingView"
```

---

### Task 9: App.tsx — wire up new props

**Files:**
- Modify: `src/App.tsx:152-156,332-339`

- [ ] **Step 1: Update initial processing state in handleGenerate**

In `src/App.tsx`, change lines 152-156 from:

```typescript
    setProcessing({
      stage: "validating",
      percent: null,
      message: "Starting...",
    });
```

to:

```typescript
    setProcessing({
      stage: "validating",
      percent: null,
      message: "Starting...",
      elapsedSecs: null,
      etaSecs: null,
    });
```

- [ ] **Step 2: Pass new props to ProcessingView**

Change lines 332-339 from:

```tsx
                <ProcessingView
                  stage={processing.stage}
                  percent={processing.percent}
                  message={processing.message}
                  onStop={handleStopProcessing}
                  stopDisabled={isStopping}
                  stopLabel={isStopping ? "Stopping..." : "Stop Processing"}
                />
```

to:

```tsx
                <ProcessingView
                  stage={processing.stage}
                  percent={processing.percent}
                  message={processing.message}
                  elapsedSecs={processing.elapsedSecs}
                  etaSecs={processing.etaSecs}
                  onStop={handleStopProcessing}
                  stopDisabled={isStopping}
                  stopLabel={isStopping ? "Stopping..." : "Stop Processing"}
                />
```

- [ ] **Step 3: Verify frontend compiles**

Run: `npm run build`
Expected: build succeeds

- [ ] **Step 4: Commit**

```bash
git add src/App.tsx
git commit -m "feat: pass elapsedSecs and etaSecs props to ProcessingView"
```

---

### Task 10: Full build verification

**Files:** None (verification only)

- [ ] **Step 1: Run all Go tests**

Run: `cd go-sidecar && go test ./... -v`
Expected: all tests PASS

- [ ] **Step 2: Build Go sidecar**

Run: `cd go-sidecar && go build -o go-sidecar.exe .`
Expected: build succeeds

- [ ] **Step 3: Build frontend**

Run: `npm run build`
Expected: build succeeds with no TypeScript errors

- [ ] **Step 4: Final commit if any loose changes**

Run: `git status` to check for any unstaged changes, clean up if needed.
