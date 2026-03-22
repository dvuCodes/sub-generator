package main

import "testing"

func TestListAvailableLanguagesReturnsEmptyWhenLibreTranslateIsIdle(t *testing.T) {
	svcManager := NewServiceManager(ServiceConfig{
		LibreTranslatePort: 1,
	})

	langs, err := listAvailableLanguages(svcManager)
	if err != nil {
		t.Fatalf("listAvailableLanguages() error = %v", err)
	}
	if len(langs) != 0 {
		t.Fatalf("listAvailableLanguages() returned %d languages, want 0", len(langs))
	}
}

func TestLocalServiceBaseURLUsesIPv4Loopback(t *testing.T) {
	if got := localServiceBaseURL(5000); got != "http://127.0.0.1:5000" {
		t.Fatalf("localServiceBaseURL() = %q, want %q", got, "http://127.0.0.1:5000")
	}
}
