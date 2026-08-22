package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- PostProcessTranslation ---

func TestPostProcessTranslationDropsAttachedHonorificsOnly(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Kaguya-san, welcome back.", "Kaguya, welcome back."},
		{"Where is Taro-kun?", "Where is Taro?"},
		{"He wore a tan coat", "He wore a tan coat"},
		{"We visited San Francisco", "We visited San Francisco"},
		{"Susan Tan joined today", "Susan Tan joined today"},
		{"The sensei explained it", "The sensei explained it"},
	}
	for _, tc := range cases {
		if got := PostProcessTranslation(tc.in, "drop"); got != tc.want {
			t.Errorf("PostProcessTranslation(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPostProcessTranslationKeepsHonorificsInKeepMode(t *testing.T) {
	in := "Kaguya-san, welcome."
	if got := PostProcessTranslation(in, "keep"); got != in {
		t.Fatalf("keep mode altered text: %q", got)
	}
}

func TestPostProcessTranslationPreservesIdeographicSpace(t *testing.T) {
	in := "hello\u3000world\tagain"
	got := PostProcessTranslation(in, "")
	if !strings.ContainsRune(got, '\u3000') {
		t.Fatalf("ideographic space lost: %q", got)
	}
	if strings.ContainsRune(got, '\t') {
		t.Fatalf("ascii whitespace not collapsed: %q", got)
	}
}

// --- decodeTranslatedText ---

func TestDecodeTranslatedText(t *testing.T) {
	array := []byte(`["one","two"]`)
	got, err := decodeTranslatedText(array, 2)
	if err != nil || !eqStrings(got, []string{"one", "two"}) {
		t.Fatalf("decodeTranslatedText(array) = %#v, %v", got, err)
	}

	single := []byte(`"only"`)
	if _, err := decodeTranslatedText(single, 1); err != nil {
		t.Fatalf("single-string single-text failed: %v", err)
	}
	if _, err := decodeTranslatedText(single, 3); err == nil {
		t.Fatal("single string must be rejected for multi-text requests")
	}
	if _, err := decodeTranslatedText([]byte(`["a"]`), 2); err == nil {
		t.Fatal("count mismatch should error")
	}
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- fake engine for orchestrator tests ---

type fakeEngine struct {
	name      string
	batchSize int
	fn        func(texts []string, req TranslateRequest) ([]string, error)

	seenBatches [][]string
	seenContext [][]ContextPair
}

func (e *fakeEngine) Name() string   { return e.name }
func (e *fakeEngine) BatchSize() int { return e.batchSize }
func (e *fakeEngine) TranslateBatch(ctx context.Context, texts []string, req TranslateRequest) ([]string, error) {
	e.seenBatches = append(e.seenBatches, append([]string(nil), texts...))
	e.seenContext = append(e.seenContext, req.ContextPairs)
	return e.fn(texts, req)
}

func upperAll(texts []string, _ TranslateRequest) ([]string, error) {
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = strings.ToUpper(t)
	}
	return out, nil
}

func TestContextualTranslatePreservesOrderTimesAndSource(t *testing.T) {
	engine := &fakeEngine{name: "fake", batchSize: 2, fn: upperAll}
	segs := []Segment{
		{Start: 0, End: 1, Text: "one"},
		{Start: 1, End: 2, Text: "two"},
		{Start: 2, End: 3, Text: "three"},
	}

	var progress []int
	out, err := ContextualTranslate(context.Background(), segs, engine,
		TranslateRequest{TargetLang: "en"}, func(done, total int) { progress = append(progress, done) })
	if err != nil {
		t.Fatalf("ContextualTranslate() error: %v", err)
	}
	if len(out) != 3 || out[0].Text != "ONE" || out[2].Text != "THREE" {
		t.Fatalf("order broken: %#v", out)
	}
	if out[0].Start != 0 || out[0].End != 1 || out[2].Start != 2 || out[2].End != 3 {
		t.Fatalf("times not preserved: %#v", out)
	}
	if out[0].SourceText != "one" || out[2].SourceText != "three" {
		t.Fatalf("source text not preserved: %#v", out)
	}
	if !eqInts(progress, []int{2, 3}) {
		t.Fatalf("progress = %v, want [2 3]", progress)
	}
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestContextualTranslateFeedsSlidingWindowContext(t *testing.T) {
	engine := &fakeEngine{name: "fake", batchSize: 2, fn: upperAll}
	segs := []Segment{
		{Start: 0, End: 1, Text: "a"},
		{Start: 1, End: 2, Text: "b"},
		{Start: 2, End: 3, Text: "c"},
		{Start: 3, End: 4, Text: "d"},
	}

	_, err := ContextualTranslate(context.Background(), segs, engine, TranslateRequest{}, nil)
	if err != nil {
		t.Fatalf("ContextualTranslate() error: %v", err)
	}

	// Batch 1 has no context; batch 2 sees batch 1's pairs.
	if len(engine.seenContext[0]) != 0 {
		t.Fatalf("first batch context = %#v, want empty", engine.seenContext[0])
	}
	ctx2 := engine.seenContext[1]
	if len(ctx2) != 2 || ctx2[0].Target != "A" || ctx2[1].Target != "B" {
		t.Fatalf("second batch context = %#v, want prior translations", ctx2)
	}
}

type flakyEngine struct {
	failFirst int
	calls     int
}

func (e *flakyEngine) Name() string   { return "flaky" }
func (e *flakyEngine) BatchSize() int { return 10 }
func (e *flakyEngine) TranslateBatch(_ context.Context, texts []string, _ TranslateRequest) ([]string, error) {
	e.calls++
	if e.calls <= e.failFirst {
		return nil, errors.New("boom")
	}
	return upperAll(texts, TranslateRequest{})
}

func TestTranslateWithRecoveryHalvesFailingBatches(t *testing.T) {
	engine := &flakyEngine{failFirst: 1} // whole batch fails once, halves succeed

	texts := []string{"aa", "bb", "cc", "dd"}
	out, err := translateWithRecovery(context.Background(), engine, texts, TranslateRequest{})
	if err != nil {
		t.Fatalf("translateWithRecovery() error: %v", err)
	}
	if !eqStrings(out, []string{"AA", "BB", "CC", "DD"}) {
		t.Fatalf("recovery output wrong: %#v", out)
	}
}

func TestTranslateWithRecoveryReportsErrorOnSingleFailure(t *testing.T) {
	engine := &flakyEngine{failFirst: 1000} // always fails
	_, err := translateWithRecovery(context.Background(), engine, []string{"x"}, TranslateRequest{})
	if err == nil {
		t.Fatal("expected error when every attempt fails")
	}
}

func TestLibreEngineBatchedArrayRequest(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = parseJSONBody(r, &gotBody)
		_, _ = w.Write([]byte(`{"translatedText":["ONE","TWO"]}`))
	}))
	defer server.Close()

	engine := &LibreEngine{baseURL: server.URL, client: server.Client(), batchSize: 16}
	out, err := engine.TranslateBatch(context.Background(), []string{"one", "two"},
		TranslateRequest{SourceLang: "ja", TargetLang: "en"})
	if err != nil {
		t.Fatalf("TranslateBatch() error: %v", err)
	}
	if !eqStrings(out, []string{"ONE", "TWO"}) {
		t.Fatalf("output = %#v", out)
	}
	if got, _ := json.Marshal(gotBody["q"]); string(got) != `["one","two"]` {
		t.Fatalf("expected array q payload, got %s", got)
	}
}

func parseJSONBody(r *http.Request, into any) error {
	return json.NewDecoder(r.Body).Decode(into)
}
