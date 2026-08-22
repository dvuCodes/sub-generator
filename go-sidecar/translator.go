package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LibreEngine translates via a local LibreTranslate server. Requests are
// batched: LibreTranslate accepts string arrays for q and mirrors them back
// in translatedText.
type LibreEngine struct {
	baseURL   string
	client    *http.Client
	batchSize int
}

func NewLibreEngine(port int) *LibreEngine {
	return &LibreEngine{
		baseURL:   localServiceBaseURL(port),
		client:    &http.Client{Timeout: 120 * time.Second},
		batchSize: 16,
	}
}

func (e *LibreEngine) Name() string   { return "libretranslate" }
func (e *LibreEngine) BatchSize() int { return e.batchSize }

type libreTranslateRequest struct {
	Q      []string `json:"q"`
	Source string   `json:"source"`
	Target string   `json:"target"`
	Format string   `json:"format,omitempty"`
}

type libreTranslateResponse struct {
	TranslatedText json.RawMessage `json:"translatedText"`
}

func (e *LibreEngine) TranslateBatch(ctx context.Context, texts []string, req TranslateRequest) ([]string, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(libreTranslateRequest{
		Q:      texts,
		Source: normalizeLangCode(req.SourceLang),
		Target: normalizeLangCode(req.TargetLang),
		Format: "text",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/translate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("translation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("translation returned status %d: %s", resp.StatusCode, truncateForLog(string(respBody), 200))
	}

	var result libreTranslateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse translation response: %w", err)
	}

	return decodeTranslatedText(result.TranslatedText, len(texts))
}

// decodeTranslatedText handles array replies (batch requests). A single
// string reply is only accepted for single-text requests: splitting joined
// strings by newline risks misassociating cues when counts coincide.
func decodeTranslatedText(raw json.RawMessage, want int) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty translation response")
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if want == 1 {
			return []string{single}, nil
		}
		return nil, fmt.Errorf("expected %d translations, got a single string", want)
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("unparsable translatedText: %w", err)
	}
	if len(list) != want {
		return nil, fmt.Errorf("expected %d translations, got %d", want, len(list))
	}
	return list, nil
}

type libreTranslateLanguage struct {
	Code    string   `json:"code"`
	Name    string   `json:"name"`
	Targets []string `json:"targets"`
}

// ListLanguages returns the installed language pair matrix from LibreTranslate.
func (e *LibreEngine) ListLanguages() ([]LanguagePair, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/languages", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build languages request: %w", err)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list languages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list languages returned status %d: %s", resp.StatusCode, string(body))
	}

	var languages []libreTranslateLanguage
	if err := json.NewDecoder(resp.Body).Decode(&languages); err != nil {
		return nil, fmt.Errorf("failed to parse languages response: %w", err)
	}

	// Prefer the explicit target matrix from LibreTranslate when available.
	var pairs []LanguagePair
	hasDeclaredTargets := false
	for _, src := range languages {
		if len(src.Targets) > 0 {
			hasDeclaredTargets = true
		}
		for _, tgt := range src.Targets {
			if src.Code != tgt {
				pairs = append(pairs, LanguagePair{
					Source: src.Code,
					Target: tgt,
				})
			}
		}
	}

	if hasDeclaredTargets {
		return pairs, nil
	}

	// Older LibreTranslate responses may omit targets; fall back to a full matrix then.
	for _, src := range languages {
		for _, tgt := range languages {
			if src.Code != tgt.Code {
				pairs = append(pairs, LanguagePair{
					Source: src.Code,
					Target: tgt.Code,
				})
			}
		}
	}

	return pairs, nil
}

func (e *LibreEngine) IsHealthy() bool {
	return isServiceHealthy(e.baseURL + "/languages")
}

func normalizeLangCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || strings.EqualFold(code, "auto") {
		return "auto"
	}
	return code
}

type Translator struct {
	baseURL    string
	client     *http.Client
	maxWorkers int
}

const translationRequestTimeout = 10 * time.Minute

func NewTranslator(port int) *Translator {
	return &Translator{
		baseURL:    localServiceBaseURL(port),
		client:     &http.Client{Timeout: translationRequestTimeout},
		maxWorkers: 1, // LLM inference is GPU-bound; no benefit from concurrency
	}
}

// --- OpenAI-compatible chat completion types ---

type chatCompletionRequest struct {
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// --- Supported languages (GemmaTranslate-v3) ---

var gemmaLanguages = map[string]string{
	"en": "English",
	"ja": "Japanese",
	"zh": "Chinese",
	"ko": "Korean",
	"es": "Spanish",
	"fr": "French",
	"de": "German",
	"pt": "Portuguese",
	"ru": "Russian",
	"ar": "Arabic",
	"hi": "Hindi",
	"vi": "Vietnamese",
	"th": "Thai",
	"it": "Italian",
	"nl": "Dutch",
	"pl": "Polish",
	"tr": "Turkish",
	"sv": "Swedish",
	"da": "Danish",
	"fi": "Finnish",
	"no": "Norwegian",
	"cs": "Czech",
	"el": "Greek",
	"he": "Hebrew",
	"hu": "Hungarian",
	"id": "Indonesian",
	"ms": "Malay",
	"ro": "Romanian",
	"sk": "Slovak",
	"uk": "Ukrainian",
	"bg": "Bulgarian",
	"hr": "Croatian",
	"lt": "Lithuanian",
	"lv": "Latvian",
	"et": "Estonian",
	"sl": "Slovenian",
	"sr": "Serbian",
	"ca": "Catalan",
	"gl": "Galician",
	"eu": "Basque",
	"mk": "Macedonian",
	"sq": "Albanian",
	"ka": "Georgian",
	"hy": "Armenian",
	"az": "Azerbaijani",
	"kk": "Kazakh",
	"uz": "Uzbek",
	"tl": "Filipino",
	"sw": "Swahili",
	"ta": "Tamil",
	"te": "Telugu",
	"bn": "Bengali",
	"ur": "Urdu",
	"fa": "Persian",
	"ne": "Nepali",
	"si": "Sinhala",
	"my": "Myanmar",
}

func supportsTranslationPair(sourceLang, targetLang string) bool {
	if targetLang == "" {
		return true
	}

	// For auto-detect, just check that the target language is supported
	if sourceLang == "" || sourceLang == "auto" {
		_, ok := gemmaLanguages[targetLang]
		return ok
	}

	_, srcOK := gemmaLanguages[sourceLang]
	_, tgtOK := gemmaLanguages[targetLang]
	return srcOK && tgtOK && sourceLang != targetLang
}

// --- Shared HTTP helper ---

// sendChatCompletion sends a [system, user] chat completion request to llama-server.
// If systemPrompt is empty, only the user message is sent.
func (t *Translator) sendChatCompletion(systemPrompt, userPrompt string) (string, error) {
	var messages []chatMessage
	if systemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, chatMessage{Role: "user", Content: userPrompt})

	reqBody := chatCompletionRequest{
		Messages:    messages,
		Temperature: 0.1,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := t.client.Post(
		t.baseURL+"/v1/chat/completions",
		"application/json",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return "", fmt.Errorf("translation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("translation returned status %d: %s", resp.StatusCode, string(body))
	}

	var result chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse translation response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("translation returned no choices")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// --- Language name resolution ---

func resolveLanguageName(langCode string) string {
	if langCode == "" || langCode == "auto" {
		return "the source language"
	}
	if name, ok := gemmaLanguages[langCode]; ok {
		return name
	}
	return langCode
}

// --- Single-segment translation ---

func buildTranslationPrompt(text, sourceLang, targetLang string) string {
	sourceName := resolveLanguageName(sourceLang)
	targetName := resolveLanguageName(targetLang)

	return fmt.Sprintf(
		"Translate the following text from %s to %s. Output only the translation, nothing else.\n\n%s",
		sourceName,
		targetName,
		text,
	)
}

func (t *Translator) Translate(text, sourceLang, targetLang string) (string, error) {
	prompt := buildTranslationPrompt(text, sourceLang, targetLang)
	return t.sendChatCompletion("", prompt)
}

// --- Contextual block translation ---

const maxHistoryLines = 4

func contextualSystemPrompt(sourceLang, targetLang string) string {
	sourceName := resolveLanguageName(sourceLang)
	targetName := resolveLanguageName(targetLang)

	prompt := fmt.Sprintf(`You are a professional subtitle translator. Translate dialogue from %s to %s.

RULES:
1. Translate each numbered line separately. Output exactly the same number of numbered lines.
2. Use the conversation context and history to resolve omitted subjects and pronouns.
3. Preserve the speaker's tone and register.
4. Output ONLY the numbered translations in [1], [2], ... format. No explanations or notes.`, sourceName, targetName)

	if sourceLang == "ja" {
		prompt += `

ADDITIONAL RULES FOR JAPANESE:
- Japanese frequently omits subjects (I, you, he/she). Use surrounding context to determine the correct subject.
- Preserve honorifics when they carry meaning that would be lost (-san, -sama, -kun, -chan, senpai).
- Translate sentence-final particles (よ, ね, わ, ぞ, etc.) into natural phrasing that conveys the same nuance.
- Maintain speech register differences (keigo vs casual vs rough).`
	}

	return prompt
}

func buildContextualUserPrompt(block ContextBlock, history []string) string {
	var sb strings.Builder

	if len(history) > 0 {
		sb.WriteString("PREVIOUS CONTEXT (for reference, do not re-translate):\n")
		for _, line := range history {
			sb.WriteString("> ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("TRANSLATE THE FOLLOWING:\n")
	for i, seg := range block.Segments {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, seg.Text))
	}

	return sb.String()
}

func buildStricterUserPrompt(block ContextBlock, history []string) string {
	var sb strings.Builder

	if len(history) > 0 {
		sb.WriteString("PREVIOUS CONTEXT (for reference, do not re-translate):\n")
		for _, line := range history {
			sb.WriteString("> ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("TRANSLATE THE FOLLOWING (output EXACTLY %d lines, each starting with [N]):\n", len(block.Segments)))
	for i, seg := range block.Segments {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, seg.Text))
	}

	return sb.String()
}

// --- Strict response parser ---

var numberedLineRe = regexp.MustCompile(`^\[(\d+)\]\s?(.*)$`)

func parseBlockTranslation(response string, expectedCount int) ([]string, error) {
	lines := strings.Split(strings.TrimSpace(response), "\n")

	translations := make([]string, expectedCount)
	seen := make(map[int]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		m := numberedLineRe.FindStringSubmatch(trimmed)
		if m == nil {
			return nil, fmt.Errorf("line does not match [N] format: %q", trimmed)
		}

		idx, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse index from %q: %w", trimmed, err)
		}

		if idx < 1 || idx > expectedCount {
			return nil, fmt.Errorf("index %d out of range [1..%d]", idx, expectedCount)
		}

		if seen[idx] {
			return nil, fmt.Errorf("duplicate index %d", idx)
		}
		seen[idx] = true

		translations[idx-1] = strings.TrimSpace(m[2])
	}

	if len(seen) != expectedCount {
		missing := []int{}
		for i := 1; i <= expectedCount; i++ {
			if !seen[i] {
				missing = append(missing, i)
			}
		}
		return nil, fmt.Errorf("missing indices: %v (got %d of %d)", missing, len(seen), expectedCount)
	}

	return translations, nil
}

// --- Block translation with retry/fallback ---

type blockParseFailure struct {
	cause error
}

func (e *blockParseFailure) Error() string { return e.cause.Error() }
func (e *blockParseFailure) Unwrap() error { return e.cause }

func (t *Translator) TranslateBlocks(
	blocks []ContextBlock,
	sourceLang, targetLang string,
	onProgress func(current, total int),
) ([]Segment, error) {
	totalSegments := 0
	for _, b := range blocks {
		totalSegments += len(b.Segments)
	}

	var allSegments []Segment
	var history []string
	completedSegments := 0

	sysPrompt := contextualSystemPrompt(sourceLang, targetLang)

	for blockIdx, block := range blocks {
		translations, err := t.translateBlockWithRetry(sysPrompt, block, history)

		if err != nil {
			var parseFailure *blockParseFailure
			if !errors.As(err, &parseFailure) {
				return nil, err
			}

			// Fallback: translate each segment individually
			fmt.Fprintf(os.Stderr, "warning: context-aware translation failed for block %d (segments %d-%d), falling back to per-segment translation: %v\n",
				blockIdx, completedSegments+1, completedSegments+len(block.Segments), err)

			translations = make([]string, len(block.Segments))
			for i, seg := range block.Segments {
				result, translateErr := t.Translate(seg.Text, sourceLang, targetLang)
				if translateErr != nil {
					return nil, fmt.Errorf("fallback translation failed for segment %d: %w", completedSegments+i, translateErr)
				}
				translations[i] = result
			}
		}

		for i, seg := range block.Segments {
			allSegments = append(allSegments, Segment{
				Start:        seg.Start,
				End:          seg.End,
				Text:         translations[i],
				SpeakerID:    seg.SpeakerID,
				SpeakerLabel: seg.SpeakerLabel,
			})
		}

		// Update rolling history with the translations actually emitted
		for _, tr := range translations {
			history = append(history, tr)
		}
		if len(history) > maxHistoryLines {
			history = history[len(history)-maxHistoryLines:]
		}

		completedSegments += len(block.Segments)
		if onProgress != nil {
			onProgress(completedSegments, totalSegments)
		}
	}

	return allSegments, nil
}

func (t *Translator) translateBlockWithRetry(sysPrompt string, block ContextBlock, history []string) ([]string, error) {
	expectedCount := len(block.Segments)

	// First attempt
	userPrompt := buildContextualUserPrompt(block, history)
	response, err := t.sendChatCompletion(sysPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("block translation request failed: %w", err)
	}

	translations, parseErr := parseBlockTranslation(response, expectedCount)
	if parseErr == nil {
		return translations, nil
	}

	// Retry with stricter formatting instruction (fresh attempt, no reference to bad output)
	fmt.Fprintf(os.Stderr, "warning: block translation parse failed (%v), retrying with stricter formatting\n", parseErr)

	stricterPrompt := buildStricterUserPrompt(block, history)
	response, err = t.sendChatCompletion(sysPrompt, stricterPrompt)
	if err != nil {
		return nil, fmt.Errorf("retry translation request failed: %w", err)
	}

	translations, parseErr = parseBlockTranslation(response, expectedCount)
	if parseErr != nil {
		return nil, &blockParseFailure{cause: fmt.Errorf("retry parse also failed: %w", parseErr)}
	}

	return translations, nil
}

// --- TranslateSegments (stable wrapper) ---

func (t *Translator) TranslateSegments(segments []Segment, sourceLang, targetLang string, onProgress func(current, total int)) ([]Segment, error) {
	blocks := StitchSegments(segments, DefaultStitcherConfig())
	fmt.Fprintf(os.Stderr, "stitched %d segments into %d context blocks\n", len(segments), len(blocks))

	return t.TranslateBlocks(blocks, sourceLang, targetLang, onProgress)
}

// --- Language pairs ---

func (t *Translator) ListLanguages() ([]LanguagePair, error) {
	return StaticLanguagePairs(), nil
}

func StaticLanguagePairs() []LanguagePair {
	codes := make([]string, 0, len(gemmaLanguages))
	for code := range gemmaLanguages {
		codes = append(codes, code)
	}

	pairs := make([]LanguagePair, 0, len(codes)*(len(codes)-1))
	for _, src := range codes {
		for _, tgt := range codes {
			if src != tgt {
				pairs = append(pairs, LanguagePair{Source: src, Target: tgt})
			}
		}
	}

	return pairs
}

func (t *Translator) IsHealthy() bool {
	return isServiceHealthy(t.baseURL + "/health")
}
