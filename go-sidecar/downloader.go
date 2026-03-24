package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// httpClient is the HTTP client used for downloads. It is a package-level var
// so that tests can substitute a TLS-aware client from httptest.NewTLSServer.
var httpClient = &http.Client{
	Timeout: 0, // No timeout — large model files can take a long time.
}

// ModelDownloadURL returns the Hugging Face download URL for the given model size.
func ModelDownloadURL(modelSize string) string {
	const base = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"
	return base + modelFilename(modelSize)
}

// VADModelDownloadURL returns the Hugging Face download URL for the Silero VAD model.
func VADModelDownloadURL() string {
	return "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin"
}

const vadModelFilename = "ggml-silero-v5.1.2.bin"

// formatBytes converts a byte count into a human-readable string.
// Examples: 0 -> "0 B", 1024 -> "1.0 KB", 1536 -> "1.5 KB", etc.
func formatBytes(bytes int64) string {
	const (
		KB = int64(1024)
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes < KB:
		return fmt.Sprintf("%d B", bytes)
	case bytes < MB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	case bytes < GB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	default:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	}
}

// DownloadModel downloads the file at url to destPath with resume support.
//
// Behaviour:
//   - Writes to destPath+".part" and renames to destPath on completion.
//   - If a .part file already exists, sends a Range header to resume.
//   - On 206 Partial Content the .part file is opened for append.
//   - On 200 OK the .part file is truncated and rewritten from scratch; any other status is an error.
//   - Retries once (with a 2-second pause) on 429 or 5xx responses.
//   - Progress callback is invoked on every buffer read (~512 KB chunks).
//   - Only https:// URLs are accepted.
//   - On error the .part file is left intact for a future resume attempt.
func DownloadModel(url, destPath string, onProgress func(downloaded, total int64)) error {
	// HTTPS validation — must happen before any file or network work.
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("only https:// URLs are allowed for downloads, got: %s", url)
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(parentDir(destPath), 0755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	partPath := destPath + ".part"

	// Determine how many bytes we already have in a previous .part file.
	var existingBytes int64
	if info, err := os.Stat(partPath); err == nil {
		existingBytes = info.Size()
	}

	const maxAttempts = 2
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
			// Re-check .part file size in case previous attempt modified it.
			if info, err := os.Stat(partPath); err == nil {
				existingBytes = info.Size()
			}
		}

		lastErr = downloadAttempt(url, destPath, partPath, existingBytes, onProgress)
		if lastErr == nil {
			return nil
		}

		// Only retry on transient errors flagged by a sentinel type.
		if !isRetryableError(lastErr) {
			return lastErr
		}
	}

	return lastErr
}

// retryableError wraps an error to signal that the caller should retry.
type retryableError struct{ cause error }

func (e *retryableError) Error() string { return e.cause.Error() }
func (e *retryableError) Unwrap() error { return e.cause }

func isRetryableError(err error) bool {
	_, ok := err.(*retryableError)
	return ok
}

const bufferSize = 512 * 1024 // 512 KB

// downloadAttempt performs a single HTTP download attempt.
func downloadAttempt(url, destPath, partPath string, existingBytes int64, onProgress func(downloaded, total int64)) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if existingBytes > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingBytes))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// Handle retryable HTTP statuses.
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return &retryableError{
			cause: fmt.Errorf("server returned status %d", resp.StatusCode),
		}
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	// Open the .part file — append on 206, truncate otherwise.
	var partFile *os.File
	if resp.StatusCode == http.StatusPartialContent {
		partFile, err = os.OpenFile(partPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
	} else {
		// 200: server is sending the full file; start fresh.
		existingBytes = 0
		partFile, err = os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	}
	if err != nil {
		return fmt.Errorf("open part file: %w", err)
	}
	// Note: we intentionally do NOT remove the .part file on error —
	// we leave it intact so the download can be resumed later.

	totalSize := resp.ContentLength // may be -1 if unknown
	if resp.StatusCode == http.StatusPartialContent && totalSize >= 0 {
		totalSize += existingBytes
	}

	downloaded := existingBytes
	buf := make([]byte, bufferSize)

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := partFile.Write(buf[:n]); writeErr != nil {
				partFile.Close()
				return fmt.Errorf("write to part file: %w", writeErr)
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(downloaded, totalSize)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			partFile.Close()
			return &retryableError{
				cause: fmt.Errorf("read response body: %w", readErr),
			}
		}
	}

	if err := partFile.Close(); err != nil {
		return fmt.Errorf("close part file: %w", err)
	}

	// Verify we received everything the server promised (Content-Length check).
	if resp.ContentLength >= 0 {
		received := downloaded - existingBytes
		if received < resp.ContentLength {
			return fmt.Errorf("incomplete download: received %d bytes, expected %d", received, resp.ContentLength)
		}
	}

	// Rename .part -> final destination.
	if err := os.Rename(partPath, destPath); err != nil {
		return fmt.Errorf("rename part file: %w", err)
	}

	return nil
}

// parentDir returns the directory component of path.
// It never returns an empty string — it falls back to ".".
func parentDir(path string) string {
	if idx := lastSeparator(path); idx >= 0 {
		return path[:idx]
	}
	return "."
}

// lastSeparator returns the index of the last path separator in s, or -1.
func lastSeparator(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' || s[i] == '\\' {
			return i
		}
	}
	return -1
}
