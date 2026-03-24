package main

import (
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestListAvailableLanguagesReturnsStaticPairs(t *testing.T) {
	langs, err := listAvailableLanguages()
	if err != nil {
		t.Fatalf("listAvailableLanguages() error = %v", err)
	}
	if len(langs) == 0 {
		t.Fatal("listAvailableLanguages() returned empty list")
	}

	// Verify a known pair exists
	found := false
	for _, pair := range langs {
		if pair.Source == "en" && pair.Target == "ja" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("listAvailableLanguages() missing en->ja pair")
	}
}

func TestLocalServiceBaseURLUsesIPv4Loopback(t *testing.T) {
	if got := localServiceBaseURL(5000); got != "http://127.0.0.1:5000" {
		t.Fatalf("localServiceBaseURL() = %q, want %q", got, "http://127.0.0.1:5000")
	}
}

func TestHandleCommandGenerateRunsSynchronously(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() {
		runtime.GOMAXPROCS(oldProcs)
	})

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = originalStdout
	})

	handleCommand(Command{Command: "generate"}, &Pipeline{}, nil)

	os.Stdout = originalStdout
	_ = writer.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if !strings.Contains(string(data), `"message":"Validation failed"`) {
		t.Fatalf("handleCommand(generate) should emit validation failure before returning, got %q", string(data))
	}
}
