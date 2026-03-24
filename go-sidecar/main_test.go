package main

import "testing"

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

func TestHandleCommandGenerateRunsAsynchronously(t *testing.T) {
	originalRunGenerate := runGenerate
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	runGenerate = func(_ *Pipeline, _ Command) {
		close(started)
		<-release
		close(done)
	}
	t.Cleanup(func() {
		runGenerate = originalRunGenerate
	})

	returned := make(chan struct{})
	go func() {
		handleCommand(Command{Command: "generate"}, &Pipeline{}, nil)
		close(returned)
	}()

	<-started

	select {
	case <-returned:
		// handleCommand should return before generate finishes.
	default:
		t.Fatal("handleCommand(generate) blocked until the pipeline finished")
	}

	select {
	case <-done:
		t.Fatal("generate runner finished before the test released it")
	default:
	}

	close(release)
	<-done
}
