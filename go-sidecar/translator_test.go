package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListLanguagesReturnsStaticPairs(t *testing.T) {
	translator := &Translator{
		baseURL: "http://unused",
		client:  http.DefaultClient,
	}

	pairs, err := translator.ListLanguages()
	if err != nil {
		t.Fatalf("ListLanguages() error = %v", err)
	}

	if len(pairs) == 0 {
		t.Fatal("ListLanguages() returned empty list")
	}

	// With 55+ languages, expect at least 55*54 = 2970 pairs
	if len(pairs) < 2000 {
		t.Fatalf("ListLanguages() returned only %d pairs, expected 2000+", len(pairs))
	}

	// Verify a known pair exists
	found := false
	for _, pair := range pairs {
		if pair.Source == "en" && pair.Target == "ja" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("ListLanguages() missing en->ja pair")
	}
}

func TestSupportsTranslationPairWithKnownLanguages(t *testing.T) {
	if !supportsTranslationPair("ja", "ko") {
		t.Fatal("supportsTranslationPair(ja, ko) = false, want true")
	}

	if !supportsTranslationPair("en", "ja") {
		t.Fatal("supportsTranslationPair(en, ja) = false, want true")
	}

	// Same language should fail
	if supportsTranslationPair("en", "en") {
		t.Fatal("supportsTranslationPair(en, en) = true, want false for same language")
	}

	// Unknown language should fail
	if supportsTranslationPair("en", "zz") {
		t.Fatal("supportsTranslationPair(en, zz) = true, want false for unknown language")
	}
}

func TestSupportsTranslationPairAutoSource(t *testing.T) {
	// "auto" source: should match if the target language is supported
	if !supportsTranslationPair("auto", "ja") {
		t.Fatal("supportsTranslationPair(auto, ja) = false, want true")
	}
	if !supportsTranslationPair("auto", "en") {
		t.Fatal("supportsTranslationPair(auto, en) = false, want true")
	}
	if supportsTranslationPair("auto", "zz") {
		t.Fatal("supportsTranslationPair(auto, zz) = true, want false")
	}

	// empty string source: same as "auto"
	if !supportsTranslationPair("", "ja") {
		t.Fatal("supportsTranslationPair('', ja) = false, want true")
	}
	if supportsTranslationPair("", "zz") {
		t.Fatal("supportsTranslationPair('', zz) = true, want false")
	}
}

func TestBuildTranslationPrompt(t *testing.T) {
	prompt := buildTranslationPrompt("Hello world", "en", "ja")
	if prompt == "" {
		t.Fatal("buildTranslationPrompt returned empty string")
	}

	// Should contain source and target language names
	if got := prompt; got == "" {
		t.Fatal("prompt should not be empty")
	}

	// Should contain the text to translate
	if !containsString(prompt, "Hello world") {
		t.Fatal("prompt should contain the input text")
	}

	// Should contain language names
	if !containsString(prompt, "English") {
		t.Fatal("prompt should contain 'English'")
	}
	if !containsString(prompt, "Japanese") {
		t.Fatal("prompt should contain 'Japanese'")
	}
}

func TestTranslateParsesOpenAIChatResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Fatalf("expected 1 user message, got %v", req.Messages)
		}

		resp := chatCompletionResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "こんにちは世界"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	translator := &Translator{
		baseURL:    server.URL,
		client:     server.Client(),
		maxWorkers: 1,
	}

	result, err := translator.Translate("Hello world", "en", "ja")
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
	if result != "こんにちは世界" {
		t.Fatalf("Translate() = %q, want %q", result, "こんにちは世界")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && searchString(s, substr))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
