package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Translator struct {
	baseURL    string
	client     *http.Client
	maxWorkers int
}

func NewTranslator(port int) *Translator {
	return &Translator{
		baseURL:    localServiceBaseURL(port),
		client:     &http.Client{Timeout: 120 * time.Second},
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

func buildTranslationPrompt(text, sourceLang, targetLang string) string {
	sourceName := gemmaLanguages[sourceLang]
	targetName := gemmaLanguages[targetLang]

	if sourceName == "" {
		sourceName = sourceLang
	}
	if targetName == "" {
		targetName = targetLang
	}

	return fmt.Sprintf(
		"Translate the following text from %s to %s. Output only the translation, nothing else.\n\n%s",
		sourceName,
		targetName,
		text,
	)
}

func (t *Translator) Translate(text, sourceLang, targetLang string) (string, error) {
	prompt := buildTranslationPrompt(text, sourceLang, targetLang)

	reqBody := chatCompletionRequest{
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
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

func (t *Translator) TranslateSegments(segments []Segment, sourceLang, targetLang string, onProgress func(current, total int)) ([]Segment, error) {
	total := len(segments)
	translated := make([]Segment, total)

	sem := make(chan struct{}, t.maxWorkers)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	completed := 0

	for i, seg := range segments {
		wg.Add(1)
		go func(idx int, s Segment) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			mu.Lock()
			if firstErr != nil {
				mu.Unlock()
				return
			}
			mu.Unlock()

			result, err := t.Translate(s.Text, sourceLang, targetLang)

			mu.Lock()
			defer mu.Unlock()

			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("segment %d: %w", idx, err)
				return
			}

			translated[idx] = Segment{
				Start: s.Start,
				End:   s.End,
				Text:  result,
			}
			completed++

			if onProgress != nil {
				onProgress(completed, total)
			}
		}(i, seg)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return translated, nil
}

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
