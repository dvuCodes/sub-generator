package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestTranslateDoesNotSendFixedMaxTokensCap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if _, ok := raw["max_tokens"]; ok {
			t.Fatalf("request unexpectedly included max_tokens: %#v", raw["max_tokens"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	translator := &Translator{
		baseURL:    server.URL,
		client:     server.Client(),
		maxWorkers: 1,
	}

	if _, err := translator.Translate(string(bytes.Repeat([]byte("a"), 4000)), "en", "ja"); err != nil {
		t.Fatalf("Translate() error = %v", err)
	}
}

// --- Contextual prompt tests ---

func TestContextualSystemPromptGeneric(t *testing.T) {
	prompt := contextualSystemPrompt("en", "es")
	if !strings.Contains(prompt, "English") || !strings.Contains(prompt, "Spanish") {
		t.Fatal("generic prompt should contain language names")
	}
	if strings.Contains(prompt, "JAPANESE") {
		t.Fatal("non-Japanese prompt should not contain Japanese-specific rules")
	}
}

func TestContextualSystemPromptJapanese(t *testing.T) {
	prompt := contextualSystemPrompt("ja", "en")
	if !strings.Contains(prompt, "Japanese") || !strings.Contains(prompt, "English") {
		t.Fatal("Japanese prompt should contain language names")
	}
	if !strings.Contains(prompt, "ADDITIONAL RULES FOR JAPANESE") {
		t.Fatal("Japanese prompt should contain Japanese-specific rules")
	}
	if !strings.Contains(prompt, "honorifics") {
		t.Fatal("Japanese rules should mention honorifics")
	}
}

func TestContextualSystemPromptAutoLanguage(t *testing.T) {
	prompt := contextualSystemPrompt("auto", "en")
	if !strings.Contains(prompt, "the source language") {
		t.Fatal("auto source should use 'the source language' label, got: " + prompt[:100])
	}
	if strings.Contains(prompt, "JAPANESE") {
		t.Fatal("auto source should not trigger Japanese rules")
	}
}

func TestBuildContextualUserPromptNoHistory(t *testing.T) {
	block := ContextBlock{
		Segments: []Segment{
			{Text: "hello"},
			{Text: "world"},
		},
	}
	prompt := buildContextualUserPrompt(block, nil)

	if strings.Contains(prompt, "PREVIOUS CONTEXT") {
		t.Fatal("first block should not have history section")
	}
	if !strings.Contains(prompt, "[1] hello") || !strings.Contains(prompt, "[2] world") {
		t.Fatal("prompt should contain numbered segments")
	}
}

func TestBuildContextualUserPromptWithHistory(t *testing.T) {
	block := ContextBlock{
		Segments: []Segment{{Text: "next line"}},
	}
	history := []string{"He went home.", "She stayed."}
	prompt := buildContextualUserPrompt(block, history)

	if !strings.Contains(prompt, "PREVIOUS CONTEXT") {
		t.Fatal("should have history section")
	}
	if !strings.Contains(prompt, "> He went home.") {
		t.Fatal("should contain history lines with > prefix")
	}
	if !strings.Contains(prompt, "[1] next line") {
		t.Fatal("should contain numbered segment")
	}
}

func TestBuildContextualUserPromptBlankSegments(t *testing.T) {
	block := ContextBlock{
		Segments: []Segment{
			{Text: "hello"},
			{Text: ""},
			{Text: "..."},
		},
	}
	prompt := buildContextualUserPrompt(block, nil)

	if !strings.Contains(prompt, "[1] hello") {
		t.Fatal("should contain segment 1")
	}
	if !strings.Contains(prompt, "[2] \n") {
		t.Fatal("blank segment should still be numbered as [2]")
	}
	if !strings.Contains(prompt, "[3] ...") {
		t.Fatal("punctuation segment should be numbered as [3]")
	}
}

// --- Parser tests ---

func TestParseBlockTranslationValid(t *testing.T) {
	response := "[1] Hello\n[2] World\n[3] How are you"
	translations, err := parseBlockTranslation(response, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if translations[0] != "Hello" || translations[1] != "World" || translations[2] != "How are you" {
		t.Fatalf("unexpected translations: %v", translations)
	}
}

func TestParseBlockTranslationWithEmptyText(t *testing.T) {
	response := "[1] Hello\n[2] \n[3] World"
	translations, err := parseBlockTranslation(response, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if translations[1] != "" {
		t.Fatalf("expected empty translation for [2], got %q", translations[1])
	}
}

func TestParseBlockTranslationWithBareMarker(t *testing.T) {
	response := "[1] Hello\n[2]\n[3] World"
	translations, err := parseBlockTranslation(response, 3)
	if err != nil {
		t.Fatalf("unexpected error for bare [2] marker: %v", err)
	}
	if translations[1] != "" {
		t.Fatalf("expected empty translation for bare [2], got %q", translations[1])
	}
}

func TestParseBlockTranslationCountMismatch(t *testing.T) {
	response := "[1] Hello\n[2] World"
	_, err := parseBlockTranslation(response, 3)
	if err == nil {
		t.Fatal("expected error for count mismatch")
	}
}

func TestParseBlockTranslationDuplicateIndex(t *testing.T) {
	response := "[1] Hello\n[1] Again\n[2] World"
	_, err := parseBlockTranslation(response, 2)
	if err == nil {
		t.Fatal("expected error for duplicate index")
	}
}

func TestParseBlockTranslationMissingIndex(t *testing.T) {
	response := "[1] Hello\n[3] World"
	_, err := parseBlockTranslation(response, 3)
	if err == nil {
		t.Fatal("expected error for missing index")
	}
}

func TestParseBlockTranslationOutOfRange(t *testing.T) {
	response := "[1] Hello\n[5] World"
	_, err := parseBlockTranslation(response, 2)
	if err == nil {
		t.Fatal("expected error for out of range index")
	}
}

func TestParseBlockTranslationNoNumbering(t *testing.T) {
	response := "Hello\nWorld"
	_, err := parseBlockTranslation(response, 2)
	if err == nil {
		t.Fatal("expected error: no positional fallback, unnumbered lines should fail")
	}
}

func TestParseBlockTranslationBlankLines(t *testing.T) {
	response := "\n[1] Hello\n\n[2] World\n\n"
	translations, err := parseBlockTranslation(response, 2)
	if err != nil {
		t.Fatalf("blank lines should be skipped: %v", err)
	}
	if translations[0] != "Hello" || translations[1] != "World" {
		t.Fatalf("unexpected: %v", translations)
	}
}

// --- Source language resolution tests ---

func TestResolveLanguageName(t *testing.T) {
	if got := resolveLanguageName("ja"); got != "Japanese" {
		t.Errorf("resolveLanguageName(ja) = %q, want Japanese", got)
	}
	if got := resolveLanguageName("auto"); got != "the source language" {
		t.Errorf("resolveLanguageName(auto) = %q, want 'the source language'", got)
	}
	if got := resolveLanguageName(""); got != "the source language" {
		t.Errorf("resolveLanguageName('') = %q, want 'the source language'", got)
	}
	if got := resolveLanguageName("xx"); got != "xx" {
		t.Errorf("resolveLanguageName(xx) = %q, want xx (passthrough)", got)
	}
}

// --- TranslateBlocks integration tests ---

func mockLlamaServer(t *testing.T, responses []string) *httptest.Server {
	t.Helper()
	callIdx := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if callIdx >= len(responses) {
			t.Fatalf("unexpected request #%d (only %d responses configured)", callIdx+1, len(responses))
		}
		content := responses[callIdx]
		callIdx++

		resp := chatCompletionResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: content}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestTranslateBlocksEndToEnd(t *testing.T) {
	server := mockLlamaServer(t, []string{
		"[1] Good morning\n[2] Tanaka, good morning",
		"[1] He went\n[2] Who?",
	})
	defer server.Close()

	translator := &Translator{baseURL: server.URL, client: server.Client(), maxWorkers: 1}

	blocks := []ContextBlock{
		{
			Segments: []Segment{
				{Start: 0.0, End: 1.2, Text: "おはようございます"},
				{Start: 1.5, End: 2.8, Text: "田中くん、おはよう"},
			},
			Start: 0.0, End: 2.8,
		},
		{
			Segments: []Segment{
				{Start: 15.0, End: 17.0, Text: "行った"},
				{Start: 17.3, End: 18.5, Text: "誰が？"},
			},
			Start: 15.0, End: 18.5,
		},
	}

	var progressCalls []int
	result, err := translator.TranslateBlocks(blocks, "ja", "en", func(current, total int) {
		progressCalls = append(progressCalls, current)
	})
	if err != nil {
		t.Fatalf("TranslateBlocks() error = %v", err)
	}

	if len(result) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(result))
	}

	// Verify timestamps preserved
	if result[0].Start != 0.0 || result[0].End != 1.2 {
		t.Errorf("segment 0 timestamps: got %f-%f, want 0.0-1.2", result[0].Start, result[0].End)
	}
	if result[2].Start != 15.0 || result[2].End != 17.0 {
		t.Errorf("segment 2 timestamps: got %f-%f, want 15.0-17.0", result[2].Start, result[2].End)
	}

	// Verify translated text
	if result[0].Text != "Good morning" {
		t.Errorf("segment 0 text = %q, want 'Good morning'", result[0].Text)
	}
	if result[3].Text != "Who?" {
		t.Errorf("segment 3 text = %q, want 'Who?'", result[3].Text)
	}

	// Verify progress: should be called after each block with cumulative counts
	if len(progressCalls) != 2 {
		t.Fatalf("expected 2 progress calls, got %d", len(progressCalls))
	}
	if progressCalls[0] != 2 || progressCalls[1] != 4 {
		t.Errorf("progress calls = %v, want [2, 4]", progressCalls)
	}
}

func TestTranslateBlocksFallbackOnParseError(t *testing.T) {
	server := mockLlamaServer(t, []string{
		"garbled nonsense",    // first attempt for block 0 → parse fail
		"still garbled",       // retry for block 0 → parse fail again
		"Good morning",        // fallback: segment 0 individually
		"Tanaka, good morning", // fallback: segment 1 individually
	})
	defer server.Close()

	translator := &Translator{baseURL: server.URL, client: server.Client(), maxWorkers: 1}

	blocks := []ContextBlock{
		{
			Segments: []Segment{
				{Start: 0.0, End: 1.2, Text: "おはよう"},
				{Start: 1.5, End: 2.8, Text: "田中くん"},
			},
			Start: 0.0, End: 2.8,
		},
	}

	result, err := translator.TranslateBlocks(blocks, "ja", "en", nil)
	if err != nil {
		t.Fatalf("TranslateBlocks() should not error on graceful fallback: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(result))
	}
	if result[0].Text != "Good morning" {
		t.Errorf("segment 0 = %q, want 'Good morning'", result[0].Text)
	}
	if result[1].Text != "Tanaka, good morning" {
		t.Errorf("segment 1 = %q, want 'Tanaka, good morning'", result[1].Text)
	}
}

func TestTranslateBlocksFallbackFailureStopsPipeline(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			// First two: block attempts return garbled (parse will fail)
			resp := chatCompletionResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{
					{Message: chatMessage{Role: "assistant", Content: "garbled"}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Fallback per-segment requests: return server error
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server down"))
	}))
	defer server.Close()

	translator := &Translator{baseURL: server.URL, client: server.Client(), maxWorkers: 1}
	blocks := []ContextBlock{
		{
			Segments: []Segment{
				{Start: 0.0, End: 1.0, Text: "test"},
				{Start: 1.5, End: 2.0, Text: "test2"},
			},
			Start: 0.0, End: 2.0,
		},
	}

	_, err := translator.TranslateBlocks(blocks, "ja", "en", nil)
	if err == nil {
		t.Fatal("expected error when fallback per-segment translation fails")
	}
}

func TestTranslateBlocksRequestFailureDoesNotFallback(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("llama unavailable"))
			return
		}

		resp := chatCompletionResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "unexpected fallback success"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	translator := &Translator{baseURL: server.URL, client: server.Client(), maxWorkers: 1}
	blocks := []ContextBlock{
		{
			Segments: []Segment{
				{Start: 0.0, End: 1.0, Text: "テスト"},
				{Start: 1.2, End: 2.0, Text: "です"},
			},
			Start: 0.0, End: 2.0,
		},
	}

	_, err := translator.TranslateBlocks(blocks, "ja", "en", nil)
	if err == nil {
		t.Fatal("expected block request failure to abort instead of falling back")
	}
	if callCount != 1 {
		t.Fatalf("expected exactly 1 request with no fallback, got %d", callCount)
	}
}

func TestTranslateBlocksRollingHistory(t *testing.T) {
	var requests []chatCompletionRequest
	callIdx := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		requests = append(requests, req)

		responses := []string{
			"[1] Hello\n[2] World",
			"[1] Goodbye",
		}
		content := responses[callIdx]
		callIdx++

		resp := chatCompletionResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: content}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	translator := &Translator{baseURL: server.URL, client: server.Client(), maxWorkers: 1}

	blocks := []ContextBlock{
		{
			Segments: []Segment{
				{Start: 0.0, End: 1.0, Text: "こんにちは"},
				{Start: 1.5, End: 2.0, Text: "世界"},
			},
			Start: 0.0, End: 2.0,
		},
		{
			Segments: []Segment{
				{Start: 5.0, End: 6.0, Text: "さようなら"},
			},
			Start: 5.0, End: 6.0,
		},
	}

	_, err := translator.TranslateBlocks(blocks, "ja", "en", nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// Second request should contain history from block 1
	if len(requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requests))
	}
	secondUserMsg := requests[1].Messages[len(requests[1].Messages)-1].Content
	if !strings.Contains(secondUserMsg, "PREVIOUS CONTEXT") {
		t.Fatal("second block should contain PREVIOUS CONTEXT section")
	}
	if !strings.Contains(secondUserMsg, "> Hello") || !strings.Contains(secondUserMsg, "> World") {
		t.Fatal("second block should contain translations from first block as history")
	}
}

func TestTranslateBlocksRequestShape(t *testing.T) {
	var req chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&req)
		resp := chatCompletionResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "[1] Hello"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	translator := &Translator{baseURL: server.URL, client: server.Client(), maxWorkers: 1}
	blocks := []ContextBlock{
		{Segments: []Segment{{Start: 0, End: 1, Text: "test"}}, Start: 0, End: 1},
	}

	translator.TranslateBlocks(blocks, "ja", "en", nil)

	// Block translation should use [system, user] message shape
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages [system, user], got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want 'system'", req.Messages[0].Role)
	}
	if req.Messages[1].Role != "user" {
		t.Errorf("second message role = %q, want 'user'", req.Messages[1].Role)
	}

	// Should not set max_tokens
	if req.MaxTokens != nil {
		t.Errorf("max_tokens should be nil, got %v", *req.MaxTokens)
	}
}

func TestTranslateSegmentsUsesStitching(t *testing.T) {
	// TranslateSegments should internally stitch and translate blocks.
	// With 2 close segments, they should be in one block → one LLM call.
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := chatCompletionResponse{
			Choices: []struct {
				Message chatMessage `json:"message"`
			}{
				{Message: chatMessage{Role: "assistant", Content: "[1] A\n[2] B"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	translator := &Translator{baseURL: server.URL, client: server.Client(), maxWorkers: 1}
	segments := []Segment{
		{Start: 0.0, End: 1.0, Text: "a"},
		{Start: 1.5, End: 2.0, Text: "b"}, // gap 0.5s → same block
	}

	result, err := translator.TranslateSegments(segments, "ja", "en", nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(result))
	}
	// Should have been one block → one LLM call (not 2)
	if callCount != 1 {
		t.Errorf("expected 1 LLM call (stitched block), got %d", callCount)
	}
}

func TestNewTranslatorUsesLongerTimeoutForStitchedRequests(t *testing.T) {
	translator := NewTranslator(8081)

	if translator.client.Timeout < 5*time.Minute {
		t.Fatalf("translator timeout = %s, want at least %s for stitched translation", translator.client.Timeout, 5*time.Minute)
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
