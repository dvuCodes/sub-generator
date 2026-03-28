package main

import (
	"bytes"
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
