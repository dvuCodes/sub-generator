package main

import (
	"strings"
	"testing"
)

func TestParseLLMTranslationsCleanWrapper(t *testing.T) {
	out, err := parseLLMTranslations(`{"translations":["a","b"]}`, []string{"x", "y"})
	if err != nil || !eqStrings(out, []string{"a", "b"}) {
		t.Fatalf("parse = %#v, %v", out, err)
	}
}

func TestParseLLMTranslationsToleratesFencesProseAndTrailingCommas(t *testing.T) {
	cases := []string{
		"```json\n{\"translations\":[\"a\",\"b\"]}\n```",
		"Sure! Here it is: {\"translations\":[\"a\",\"b\",]} hope that helps",
		"noise before {\"translations\": [\"a\",\n \"b\",],} noise after",
	}
	for _, content := range cases {
		out, err := parseLLMTranslations(content, []string{"x", "y"})
		if err != nil || !eqStrings(out, []string{"a", "b"}) {
			t.Fatalf("parse(%q) = %#v, %v", content, out, err)
		}
	}
}

func TestParseLLMTranslationsAcceptsPlainArray(t *testing.T) {
	out, err := parseLLMTranslations(`["a","b","c"]`, []string{"x", "y", "z"})
	if err != nil || !eqStrings(out, []string{"a", "b", "c"}) {
		t.Fatalf("array fallback failed: %#v, %v", out, err)
	}
}

func TestParseLLMTranslationsRejectsWrongCountAndGarbage(t *testing.T) {
	if _, err := parseLLMTranslations(`{"translations":["a"]}`, []string{"x", "y"}); err == nil {
		t.Fatal("wrong count must error")
	}
	_, err := parseLLMTranslations(`I cannot do that.`, []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "no parsable") {
		t.Fatalf("garbage should produce honest error, got %v", err)
	}
}

func TestCacheKeyDistinguishesPolicy(t *testing.T) {
	a := cacheKey("text", "ja", "en", "policy1")
	b := cacheKey("text", "ja", "en", "policy2")
	c := cacheKey("text", "ja", "en", "policy1")
	d := cacheKey("other", "ja", "en", "policy1")

	if a == b {
		t.Fatal("different policies must yield different keys")
	}
	if a != c {
		t.Fatal("same inputs must yield same key")
	}
	if a == d {
		t.Fatal("different texts must yield different keys")
	}
}
