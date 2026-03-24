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
