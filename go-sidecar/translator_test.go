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
