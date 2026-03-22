package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		baseURL:    fmt.Sprintf("http://localhost:%d", port),
		client:     &http.Client{Timeout: 30 * time.Second},
		maxWorkers: 4, // Concurrent translation requests
	}
}

type translateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type translateResponse struct {
	TranslatedText string `json:"translatedText"`
}

type libreTranslateLanguage struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func (t *Translator) Translate(text, sourceLang, targetLang string) (string, error) {
	reqBody := translateRequest{
		Q:      text,
		Source: sourceLang,
		Target: targetLang,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := t.client.Post(
		t.baseURL+"/translate",
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

	var result translateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse translation response: %w", err)
	}

	return result.TranslatedText, nil
}

func (t *Translator) TranslateSegments(segments []Segment, sourceLang, targetLang string, onProgress func(current, total int)) ([]Segment, error) {
	total := len(segments)
	translated := make([]Segment, total)

	// Use a semaphore pattern for concurrent translations
	sem := make(chan struct{}, t.maxWorkers)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	completed := 0

	for i, seg := range segments {
		wg.Add(1)
		go func(idx int, s Segment) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			// Check for prior error
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
	resp, err := t.client.Get(t.baseURL + "/languages")
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

	// Build all available pairs
	var pairs []LanguagePair
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

func (t *Translator) IsHealthy() bool {
	return isServiceHealthy(t.baseURL + "/languages")
}
