package main

import (
	"fmt"
	"net/http"
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
