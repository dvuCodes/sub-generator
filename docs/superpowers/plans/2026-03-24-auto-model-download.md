# Auto Model Download Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically download missing whisper GGML models from Hugging Face when the user hits Generate, with resume support and progress UI.

**Architecture:** New `downloader.go` handles HTTP download with `.part` file resume. `pipeline.go` checks if the model exists before starting services and triggers the download if needed. The existing stage/progress IPC sends download progress to the frontend, which adds a "Download" step to the processing pipeline UI.

**Tech Stack:** Go (net/http, io, os), React/TypeScript (existing shadcn/ui ProcessingView)

**Spec:** `docs/superpowers/specs/2026-03-24-auto-model-download-design.md`

---

## File Structure

| Action | File | Responsibility |
|---|---|---|
| Create | `go-sidecar/downloader.go` | HTTP download with resume, progress callbacks, URL mapping |
| Create | `go-sidecar/downloader_test.go` | Tests for download, resume, progress, error handling |
| Modify | `go-sidecar/pipeline.go` | Insert download step between validate and ensure services |
| Modify | `src/components/ProcessingView.tsx` | Add `downloading_model` stage to pipeline UI |

---

### Task 1: Create downloader with URL mapping and formatBytes

**Files:**
- Create: `go-sidecar/downloader.go`
- Create: `go-sidecar/downloader_test.go`

- [ ] **Step 1: Write failing tests for ModelDownloadURL and formatBytes**

In `go-sidecar/downloader_test.go`:

```go
package main

import (
	"testing"
)

func TestModelDownloadURL(t *testing.T) {
	tests := []struct {
		modelSize string
		wantURL   string
	}{
		{"base", "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin"},
		{"large-v3", "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin"},
		{"turbo", "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin"},
	}
	for _, tt := range tests {
		got := ModelDownloadURL(tt.modelSize)
		if got != tt.wantURL {
			t.Errorf("ModelDownloadURL(%q) = %q, want %q", tt.modelSize, got, tt.wantURL)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1536 * 1024 * 1024, "1.5 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go-sidecar && go test -run "TestModelDownloadURL|TestFormatBytes" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement ModelDownloadURL and formatBytes**

In `go-sidecar/downloader.go`:

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const modelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

func ModelDownloadURL(modelSize string) string {
	return modelBaseURL + modelFilename(modelSize)
}

func formatBytes(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-sidecar && go test -run "TestModelDownloadURL|TestFormatBytes" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-sidecar/downloader.go go-sidecar/downloader_test.go
git commit -m "feat: add model download URL mapping and byte formatting"
```

---

### Task 2: Implement DownloadModel with resume support

**Files:**
- Modify: `go-sidecar/downloader.go`
- Modify: `go-sidecar/downloader_test.go`

- [ ] **Step 1: Write failing test for fresh download**

Merge imports into existing import block in `go-sidecar/downloader_test.go`, then add test:

```go
// Add to existing import block: "fmt", "net/http", "net/http/httptest", "os", "path/filepath"

func TestDownloadModelFreshDownload(t *testing.T) {
	content := []byte("fake-model-data-1234567890")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}))
	defer server.Close()

	destPath := filepath.Join(t.TempDir(), "model.bin")
	var progressCalls int
	err := DownloadModel(server.URL+"/model.bin", destPath, func(downloaded, total int64) {
		progressCalls++
	})
	if err != nil {
		t.Fatalf("DownloadModel() error: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("downloaded content = %q, want %q", got, content)
	}

	// .part file should be cleaned up
	if _, err := os.Stat(destPath + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part file should be removed after successful download")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-sidecar && go test -run TestDownloadModelFreshDownload -v`
Expected: FAIL — DownloadModel not defined

- [ ] **Step 3: Implement DownloadModel**

Add to `go-sidecar/downloader.go`:

```go
func DownloadModel(url, destPath string, onProgress func(downloaded, total int64)) error {
	// Validate HTTPS
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("model download requires HTTPS, got: %s", url)
	}

	partPath := destPath + ".part"

	// Check for existing partial download
	var existingSize int64
	if info, err := os.Stat(partPath); err == nil {
		existingSize = info.Size()
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("cannot create models directory: %w", err)
	}

	// Try download with one retry on transient errors
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
			// Re-check .part size in case first attempt wrote partial data
			if info, err := os.Stat(partPath); err == nil {
				existingSize = info.Size()
			}
		}

		lastErr = downloadToFile(url, partPath, existingSize, onProgress)
		if lastErr == nil {
			break
		}

		// Only retry on transient errors
		if !isTransientError(lastErr) {
			return lastErr
		}
	}
	if lastErr != nil {
		return lastErr
	}

	// Rename .part to final path
	if err := os.Rename(partPath, destPath); err != nil {
		return fmt.Errorf("failed to finalize model file: %w", err)
	}

	return nil
}

func downloadToFile(url, partPath string, existingSize int64, onProgress func(downloaded, total int64)) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	if existingSize > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingSize))
	}

	client := &http.Client{Timeout: 2 * time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	// Retry on transient HTTP status codes
	if resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode <= 599) {
		return &transientError{status: resp.StatusCode}
	}

	var totalSize int64
	var fileFlags int

	switch resp.StatusCode {
	case http.StatusOK:
		totalSize = resp.ContentLength
		existingSize = 0
		fileFlags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	case http.StatusPartialContent:
		totalSize = existingSize + resp.ContentLength
		fileFlags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	default:
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	partFile, err := os.OpenFile(partPath, fileFlags, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open part file: %w", err)
	}

	buf := make([]byte, 512*1024)
	downloaded := existingSize

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := partFile.Write(buf[:n]); writeErr != nil {
				partFile.Close()
				return fmt.Errorf("failed to write model data: %w", writeErr)
			}
			downloaded += int64(n)
			if onProgress != nil && totalSize > 0 {
				onProgress(downloaded, totalSize)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			partFile.Close()
			return fmt.Errorf("download interrupted: %w", readErr)
		}
	}

	return partFile.Close()
}

type transientError struct {
	status int
}

func (e *transientError) Error() string {
	return fmt.Sprintf("transient HTTP error: status %d", e.status)
}

func isTransientError(err error) bool {
	_, ok := err.(*transientError)
	return ok
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go-sidecar && go test -run TestDownloadModelFreshDownload -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-sidecar/downloader.go go-sidecar/downloader_test.go
git commit -m "feat: implement DownloadModel with streaming and progress"
```

---

### Task 3: Add resume and error handling tests

**Files:**
- Modify: `go-sidecar/downloader_test.go`

- [ ] **Step 1: Write test for resume download**

Append to `go-sidecar/downloader_test.go`:

```go
func TestDownloadModelResumesFromPartFile(t *testing.T) {
	fullContent := "AAAAABBBBB" // 10 bytes total
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "bytes=5-" {
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusPartialContent)
			w.Write([]byte("BBBBB"))
		} else {
			w.Header().Set("Content-Length", "10")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fullContent))
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "model.bin")

	// Write partial .part file (first 5 bytes)
	if err := os.WriteFile(destPath+".part", []byte("AAAAA"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := DownloadModel(server.URL+"/model.bin", destPath, nil)
	if err != nil {
		t.Fatalf("DownloadModel() error: %v", err)
	}

	got, _ := os.ReadFile(destPath)
	if string(got) != fullContent {
		t.Fatalf("content = %q, want %q", got, fullContent)
	}
}

func TestDownloadModelFailureLeavesPartFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial")) // Write less than Content-Length then close
	}))
	defer server.Close()

	destPath := filepath.Join(t.TempDir(), "model.bin")
	err := DownloadModel(server.URL+"/model.bin", destPath, nil)

	// Final file must NOT exist (download was incomplete)
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Fatal("final model file should not exist after failed download")
	}

	// .part file should be preserved with partial data
	partInfo, statErr := os.Stat(destPath + ".part")
	if err != nil && os.IsNotExist(statErr) {
		t.Fatal(".part file should be preserved on download failure")
	}
	if partInfo != nil && partInfo.Size() == 0 {
		t.Fatal(".part file should contain partial data")
	}
}

func TestDownloadModelRejectsNonHTTPS(t *testing.T) {
	err := DownloadModel("http://example.com/model.bin", filepath.Join(t.TempDir(), "model.bin"), nil)
	if err == nil {
		t.Fatal("expected error for non-HTTPS URL")
	}
}
```

- [ ] **Step 2: Run tests**

Run: `cd go-sidecar && go test -run "TestDownloadModelResumes|TestDownloadModelFailure" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add go-sidecar/downloader_test.go
git commit -m "test: add resume and error handling tests for model download"
```

---

### Task 4: Integrate download step into pipeline

**Files:**
- Modify: `go-sidecar/pipeline.go:30-45`

- [ ] **Step 1: Add download step to Pipeline.Run()**

In `go-sidecar/pipeline.go`, insert between step 1 (validate) and step 2 (ensure services). The new block goes after line 38 (`}`) and before line 40 (`// Step 2`):

```go
	// Step 1.5: Download model if needed
	modelFile := modelFilename(cmd.ModelSize)
	installDir := preferredWhisperInstallDir(p.svcManager.config.SearchRoots)
	modelPath := filepath.Join(installDir, "models", modelFile)

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		sendStage("downloading_model", fmt.Sprintf("Downloading %s model...", cmd.ModelSize))
		url := ModelDownloadURL(cmd.ModelSize)
		if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
			sendError("Download failed", fmt.Sprintf("cannot create models directory: %s", err))
			return
		}
		if err := DownloadModel(url, modelPath, func(downloaded, total int64) {
			pct := float64(downloaded) / float64(total) * 100
			sendProgress("downloading_model", pct, fmt.Sprintf("Downloaded %s / %s", formatBytes(downloaded), formatBytes(total)))
		}); err != nil {
			sendError("Model download failed", err.Error())
			return
		}
		sendProgress("downloading_model", 100, "Model downloaded")
	}
```

- [ ] **Step 2: Run all Go tests**

Run: `cd go-sidecar && go test ./... -v`
Expected: PASS (all existing + new tests)

- [ ] **Step 3: Commit**

```bash
git add go-sidecar/pipeline.go
git commit -m "feat: integrate model auto-download into pipeline"
```

---

### Task 5: Add downloading_model stage to frontend

**Files:**
- Modify: `src/components/ProcessingView.tsx:16-30`

- [ ] **Step 1: Add downloading_model to STAGE_ORDER and STAGE_LABELS**

In `src/components/ProcessingView.tsx`, update the two constants:

```typescript
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
```

- [ ] **Step 2: Verify the app builds**

Run: `cd /c/Users/datvu/Documents/code/sub-generator && bunx tsc --noEmit`
Expected: No type errors

- [ ] **Step 3: Commit**

```bash
git add src/components/ProcessingView.tsx
git commit -m "feat: add Download stage to processing pipeline UI"
```

---

### Task 6: Rebuild sidecar and end-to-end test

**Files:**
- Build: `go-sidecar/` → `src-tauri/subgen-sidecar-x86_64-pc-windows-msvc.exe`

- [ ] **Step 1: Rebuild Go sidecar**

Run: `cd go-sidecar && GOOS=windows GOARCH=amd64 go build -o ../src-tauri/subgen-sidecar-x86_64-pc-windows-msvc.exe .`

- [ ] **Step 2: Verify binary was updated**

Run: `ls -la src-tauri/subgen-sidecar-x86_64-pc-windows-msvc.exe`
Expected: Timestamp should be current

- [ ] **Step 3: Manual end-to-end test**

1. Start the app (`cargo tauri dev` or launch the app)
2. Select a video file
3. Set model to "large-v3" (which is NOT yet downloaded)
4. Hit "Generate Subtitles"
5. Verify:
   - "Download" stage appears in the pipeline UI
   - Progress bar shows "Downloaded X / Y" with percentage
   - After download completes, whisper-server starts and transcription proceeds
   - Subtitle file is generated successfully

- [ ] **Step 4: Test resume by canceling mid-download**

1. Start another generation with a different missing model
2. Cancel mid-download via "Stop Processing"
3. Re-trigger generation
4. Verify download resumes from where it left off (progress starts > 0%)

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "build: rebuild sidecar with auto model download"
```
