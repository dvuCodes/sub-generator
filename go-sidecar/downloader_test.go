package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestModelDownloadURL verifies that URLs are composed correctly for known model sizes.
func TestModelDownloadURL(t *testing.T) {
	tests := []struct {
		modelSize  string
		wantSuffix string
	}{
		{"base", "ggml-base.bin"},
		{"large-v3", "ggml-large-v3.bin"},
		{"turbo", "ggml-large-v3-turbo.bin"},
	}

	baseURL := "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

	for _, tt := range tests {
		t.Run(tt.modelSize, func(t *testing.T) {
			got := ModelDownloadURL(tt.modelSize)
			want := baseURL + tt.wantSuffix
			if got != want {
				t.Errorf("ModelDownloadURL(%q) = %q, want %q", tt.modelSize, got, want)
			}
		})
	}
}

// TestFormatBytes verifies human-readable byte formatting.
func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{int64(1.5 * 1024 * 1024 * 1024), "1.5 GB"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.bytes), func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

// TestDownloadModelFreshDownload verifies a clean download writes the file and removes .part.
func TestDownloadModelFreshDownload(t *testing.T) {
	content := "hello world content"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		fmt.Fprint(w, content)
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "model.bin")

	// Override the HTTP client to trust the TLS test server certificate.
	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	var lastDownloaded, lastTotal int64
	err := DownloadModel(server.URL, destPath, func(downloaded, total int64) {
		lastDownloaded = downloaded
		lastTotal = total
	})
	if err != nil {
		t.Fatalf("DownloadModel returned error: %v", err)
	}

	// Verify the final file has the correct content.
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content = %q, want %q", string(got), content)
	}

	// Verify .part file is cleaned up.
	partPath := destPath + ".part"
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Errorf(".part file should not exist after successful download, but it does")
	}

	// Verify progress callback was called.
	if lastDownloaded == 0 {
		t.Errorf("progress callback was never called (lastDownloaded=0)")
	}
	_ = lastTotal
}

// TestDownloadModelResumesFromPartFile verifies resume from an existing .part file.
func TestDownloadModelResumesFromPartFile(t *testing.T) {
	firstHalf := "hello"
	secondHalf := "world"
	fullContent := firstHalf + secondHalf // 10 bytes total

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "bytes=5-" {
			// Resume: send second half with 206 Partial Content.
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(secondHalf)))
			w.WriteHeader(http.StatusPartialContent)
			fmt.Fprint(w, secondHalf)
		} else {
			// Fresh start: send full content.
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullContent)))
			fmt.Fprint(w, fullContent)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "model.bin")
	partPath := destPath + ".part"

	// Write the first half as an existing .part file.
	if err := os.WriteFile(partPath, []byte(firstHalf), 0644); err != nil {
		t.Fatalf("WriteFile part: %v", err)
	}

	// Override HTTP client.
	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	err := DownloadModel(server.URL, destPath, nil)
	if err != nil {
		t.Fatalf("DownloadModel returned error: %v", err)
	}

	// Verify final file has combined content.
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != fullContent {
		t.Errorf("file content = %q, want %q", string(got), fullContent)
	}

	// Verify .part file was cleaned up.
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Errorf(".part file should not exist after successful download")
	}
}

// TestDownloadModelFailureLeavesPartFile verifies that on error the .part file is preserved.
func TestDownloadModelFailureLeavesPartFile(t *testing.T) {
	// Server claims 100 bytes but only sends 10.
	shortContent := strings.Repeat("x", 10)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		fmt.Fprint(w, shortContent)
		// Connection closes here, causing a short read detected by Content-Length check.
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "model.bin")
	partPath := destPath + ".part"

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	err := DownloadModel(server.URL, destPath, nil)
	// We expect an error because the download was incomplete.
	if err == nil {
		t.Fatal("expected error for incomplete download, got nil")
	}

	// .part file should still exist (for resume later).
	if _, err := os.Stat(partPath); os.IsNotExist(err) {
		t.Errorf(".part file should be preserved on error for resume, but it's gone")
	}

	// Final file should NOT exist.
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Errorf("destination file should not exist after failed download")
	}
}

// TestVADModelDownloadURL verifies the VAD model URL is correct.
func TestVADModelDownloadURL(t *testing.T) {
	got := VADModelDownloadURL()
	want := "https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v5.1.2.bin"
	if got != want {
		t.Errorf("VADModelDownloadURL() = %q, want %q", got, want)
	}
}

// TestDownloadModelRetries429 verifies that a 429 on first attempt retries and succeeds.
func TestDownloadModelRetries429(t *testing.T) {
	content := "retry content"
	attempt := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		fmt.Fprint(w, content)
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "model.bin")

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	err := DownloadModel(server.URL, destPath, nil)
	if err != nil {
		t.Fatalf("DownloadModel returned error: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content = %q, want %q", string(got), content)
	}
	if attempt != 2 {
		t.Errorf("expected 2 attempts, got %d", attempt)
	}
}

// TestDownloadModelRetries5xx verifies that a 503 on first attempt retries and succeeds.
func TestDownloadModelRetries5xx(t *testing.T) {
	content := "retry 5xx content"
	attempt := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		fmt.Fprint(w, content)
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "model.bin")

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	err := DownloadModel(server.URL, destPath, nil)
	if err != nil {
		t.Fatalf("DownloadModel returned error: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content = %q, want %q", string(got), content)
	}
}

// TestDownloadModelDoesNotRetry404 verifies that a 404 fails immediately without retry.
func TestDownloadModelDoesNotRetry404(t *testing.T) {
	attempt := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "model.bin")

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	err := DownloadModel(server.URL, destPath, nil)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if attempt != 1 {
		t.Errorf("404 should not be retried, expected 1 attempt, got %d", attempt)
	}
}

// TestDownloadModelRejectsNonHTTPS verifies that http:// URLs are rejected.
func TestDownloadModelRejectsNonHTTPS(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "model.bin")

	err := DownloadModel("http://example.com/model.bin", destPath, nil)
	if err == nil {
		t.Fatal("expected error for http:// URL, got nil")
	}

	// Verify the error message mentions https.
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "https") {
		t.Errorf("error message %q should mention 'https'", err.Error())
	}

	// Neither the final file nor a .part file should exist.
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Errorf("destination file should not exist after rejected download")
	}
	if _, statErr := os.Stat(destPath + ".part"); !os.IsNotExist(statErr) {
		t.Errorf(".part file should not exist after rejected download")
	}
}
