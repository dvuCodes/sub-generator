package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListLanguagesUsesDeclaredTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"code":"en","name":"English","targets":["ja","fr"]},
			{"code":"ja","name":"Japanese","targets":["en"]}
		]`))
	}))
	defer server.Close()

	translator := &Translator{
		baseURL: server.URL,
		client:  server.Client(),
	}

	pairs, err := translator.ListLanguages()
	if err != nil {
		t.Fatalf("ListLanguages() error = %v", err)
	}

	want := map[LanguagePair]bool{
		{Source: "en", Target: "ja"}: true,
		{Source: "en", Target: "fr"}: true,
		{Source: "ja", Target: "en"}: true,
	}
	if len(pairs) != len(want) {
		t.Fatalf("ListLanguages() returned %d pairs, want %d", len(pairs), len(want))
	}
	for _, pair := range pairs {
		if !want[pair] {
			t.Fatalf("ListLanguages() returned unexpected pair %#v", pair)
		}
		delete(want, pair)
	}
	if len(want) != 0 {
		t.Fatalf("ListLanguages() missed pairs: %#v", want)
	}
}

func TestSupportsTranslationPairRequiresSourceSpecificTarget(t *testing.T) {
	pairs := []LanguagePair{
		{Source: "en", Target: "ja"},
		{Source: "en", Target: "fr"},
		{Source: "ja", Target: "ko"},
	}

	if !supportsTranslationPair(pairs, "ja", "ko") {
		t.Fatal("supportsTranslationPair() = false, want true for a declared pair")
	}

	if supportsTranslationPair(pairs, "ja", "en") {
		t.Fatal("supportsTranslationPair() = true, want false when the target only exists for a different source")
	}
}

func TestSupportsTranslationPairAutoSource(t *testing.T) {
	pairs := []LanguagePair{
		{Source: "en", Target: "ja"},
		{Source: "en", Target: "fr"},
		{Source: "ja", Target: "en"},
	}

	// "auto" source: should match if ANY source supports the target
	if !supportsTranslationPair(pairs, "auto", "ja") {
		t.Fatal("supportsTranslationPair(auto, ja) = false, want true (en->ja exists)")
	}
	if !supportsTranslationPair(pairs, "auto", "en") {
		t.Fatal("supportsTranslationPair(auto, en) = false, want true (ja->en exists)")
	}
	if supportsTranslationPair(pairs, "auto", "zz") {
		t.Fatal("supportsTranslationPair(auto, zz) = true, want false (no source supports zz)")
	}

	// empty string source: same as "auto"
	if !supportsTranslationPair(pairs, "", "ja") {
		t.Fatal("supportsTranslationPair('', ja) = false, want true")
	}
	if supportsTranslationPair(pairs, "", "zz") {
		t.Fatal("supportsTranslationPair('', zz) = true, want false")
	}
}
