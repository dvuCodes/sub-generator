package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeepLEngineOmitsSourceLangForAuto(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = parseJSONBody(r, &gotBody)
		_, _ = w.Write([]byte(`{"translations":[{"text":"ONE"},{"text":"TWO"}]}`))
	}))
	defer server.Close()

	engine := NewDeepLEngine("test-key:fx")
	engine.client = server.Client()
	engine.SetBaseURL(server.URL)

	req := TranslateRequest{SourceLang: "auto", TargetLang: "en"}
	out, err := engine.TranslateBatch(context.Background(), []string{"one", "two"}, req)
	if err != nil {
		t.Fatalf("TranslateBatch() error: %v", err)
	}
	if !eqStrings(out, []string{"ONE", "TWO"}) {
		t.Fatalf("output = %#v", out)
	}

	if gotAuth != "DeepL-Auth-Key test-key:fx" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if _, present := gotBody["source_lang"]; present {
		t.Fatalf("source_lang must be omitted for auto-detect, got %q", gotBody["source_lang"])
	}
	if gotBody["target_lang"] != "EN" {
		t.Fatalf("target_lang = %q, want EN", gotBody["target_lang"])
	}
}

func TestDeepLEngineSendsUppercaseSourceAndContext(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = parseJSONBody(r, &gotBody)
		_, _ = w.Write([]byte(`{"translations":[{"text":"HELLO"}]}`))
	}))
	defer server.Close()

	engine := NewDeepLEngine("k")
	engine.client = server.Client()
	engine.SetBaseURL(server.URL)

	req := TranslateRequest{
		SourceLang:   "ja",
		TargetLang:   "en",
		Synopsis:     "a school drama",
		ContextPairs: []ContextPair{{Source: "前の文", Target: "Previous line."}},
	}
	if _, err := engine.TranslateBatch(context.Background(), []string{"こんにちは"}, req); err != nil {
		t.Fatalf("TranslateBatch() error: %v", err)
	}

	if gotBody["source_lang"] != "JA" {
		t.Fatalf("source_lang = %q, want JA", gotBody["source_lang"])
	}
	ctxStr, _ := gotBody["context"].(string)
	if !strings.Contains(ctxStr, "a school drama") || !strings.Contains(ctxStr, "Previous line.") {
		t.Fatalf("context missing synopsis/prior lines: %q", ctxStr)
	}
}

func TestPolicyHashChangesWithSettings(t *testing.T) {
	base := TranslateRequest{SourceLang: "ja", TargetLang: "en"}

	dropHonorifics := base
	dropHonorifics.Honorifics = "drop"

	withGlossary := dropHonorifics
	withGlossary.Glossary = []GlossaryEntry{{Source: "ルルーシュ", Target: "Lelouch"}}

	reordered := withGlossary
	reordered.Glossary = []GlossaryEntry{
		{Source: "スザク", Target: "Suzaku"},
		{Source: "ルルーシュ", Target: "Lelouch"},
	}
	sameSet := withGlossary
	sameSet.Glossary = []GlossaryEntry{
		{Source: "ルルーシュ", Target: "Lelouch"},
		{Source: "スザク", Target: "Suzaku"},
	}

	if policyHash(base) == policyHash(dropHonorifics) {
		t.Fatal("honorific policy must change the cache fingerprint")
	}
	if policyHash(dropHonorifics) == policyHash(withGlossary) {
		t.Fatal("glossary must change the cache fingerprint")
	}
	if policyHash(reordered) != policyHash(sameSet) {
		t.Fatal("glossary order must not matter to the fingerprint")
	}
}
