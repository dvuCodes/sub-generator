# Auto Model Download Design

## Problem

When a user selects a whisper model (e.g., large-v3) that isn't downloaded, the pipeline fails with a setup error. The user must manually download ~3 GB model files and place them in the correct directory. This should be automatic.

## Design

### Trigger

When the user hits "Generate Subtitles" and the selected model is missing, the pipeline automatically downloads it before starting whisper-server. No confirmation dialog — the download starts immediately with a progress indicator.

### Download Source

Hugging Face: `https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-{model}.bin`

Model URL map (in `downloader.go`):

| Model Size | Filename | Approx Size |
|---|---|---|
| tiny | ggml-tiny.bin | 75 MB |
| base | ggml-base.bin | 141 MB |
| small | ggml-small.bin | 466 MB |
| medium | ggml-medium.bin | 1.5 GB |
| large-v3 | ggml-large-v3.bin | 3.1 GB |
| turbo | ggml-large-v3-turbo.bin | 1.6 GB |

### Pipeline Integration

The download inserts as a new step in `pipeline.go` `Run()`, between `validating` and `starting_services`:

```
validating → downloading_model (if needed) → starting_services → transcribing → [translating] → writing
```

The stage is skipped entirely if the model already exists.

**Why `pipeline.go` and not `services.go`:** The download must happen *before* `StartWhisperServer()` is called, because `validateWhisperStartup()` inside that method would reject the missing model and return an error before any download code could run. The pipeline layer also has access to `cmd.ModelSize` and the `sendStage`/`sendProgress` IPC helpers.

#### Flow in `pipeline.go` Run():

Between step 1 (validate input) and step 2 (ensure services):

1. Resolve expected model path using existing `preferredWhisperInstallDir()` + `modelFilename(cmd.ModelSize)`
2. Check if model file exists (`os.Stat`)
3. If missing:
   - Ensure parent directory exists (`os.MkdirAll`)
   - Determine download URL from `ModelDownloadURL(cmd.ModelSize)`
   - Send `StageResponse{stage: "downloading_model", message: "Downloading large-v3 model..."}`
   - Call `DownloadModel()` with progress callback
   - Progress callback sends `ProgressResponse{stage: "downloading_model", percent: X, message: "Downloaded 1.2 GB / 3.1 GB"}`
4. If download succeeds, continue to ensure services (whisper-server starts with the new model)
5. If download fails, send `ErrorResponse` with details

### Resume Support

- Downloads write to `{destPath}.part` temp file
- On startup, if `.part` file exists, send HTTP request with `Range: bytes={partSize}-` header
- If server supports range (206 response), append to existing `.part` file
- If server doesn't support range (200 response), overwrite `.part` from scratch
- On completion, rename `.part` to final filename atomically
- Cancellation (stop button kills sidecar) leaves `.part` file intact for next attempt

### Cancellation

The existing "Stop Processing" button kills the sidecar process. This interrupts the download, leaving the `.part` file. On the next generate attempt, the download resumes from where it left off.

Note: `pipeline.Run()` is called synchronously in the stdin scan loop (`main.go`), so only one pipeline can run at a time. No concurrent download race conditions.

### Error Handling

- Retry once with backoff on transient HTTP errors (429, 500, 502, 503)
- If `Content-Length` is missing, use known expected sizes from the model table for progress display
- Validate HTTPS scheme on download URL
- On disk full / write error, leave `.part` file and return clear error message

## Files to Create

### `go-sidecar/downloader.go`

New file with:

- `ModelDownloadURL(modelSize string) string` — returns Hugging Face HTTPS URL for model
- `DownloadModel(url, destPath string, onProgress func(downloaded, total int64)) error`:
  - Check for existing `.part` file, get its size
  - Create HTTP GET request with `Range` header if resuming
  - Validate response (200 or 206)
  - Read `Content-Length` (or `Content-Range`) for total size; fall back to known size
  - Stream response body to `.part` file, calling `onProgress` periodically (~500 KB)
  - Retry once on transient HTTP errors (429, 5xx)
  - On completion, `os.Rename` `.part` file to final path
  - On error, leave `.part` file intact
- `formatBytes(bytes int64) string` — human-readable size formatting (e.g., "1.2 GB")

### `go-sidecar/downloader_test.go`

Tests:
- Fresh download writes to `.part` then renames
- Resume sends Range header and appends
- Progress callback is called with correct values
- Failed download leaves `.part` file
- Completed download removes `.part` file

## Files to Modify

### `go-sidecar/pipeline.go`

In `Run()`, add a new step between validation (step 1) and ensure services (step 2):

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
}
```

### `src/components/ProcessingView.tsx`

Add to `STAGE_ORDER` (before `"starting_services"`):
```typescript
"downloading_model"
```

Add to `STAGE_LABELS`:
```typescript
downloading_model: "Download"
```

### `src/lib/processingState.ts`

No changes needed — the existing `advanceProcessingState()` is stage-agnostic.

## Verification

1. `go test ./...` passes with new downloader tests
2. Select large-v3 model (not downloaded), hit Generate:
   - "Download" stage appears in pipeline
   - Progress bar fills with "Downloaded X / Y" messages
   - After download, whisper-server starts and transcription proceeds
3. Cancel mid-download, re-trigger: download resumes from partial file
4. Select base model (already downloaded): no download stage appears
