package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// Translator is kept as the legacy façade used by main.go for language listing.
type Translator struct {
	inner *LibreEngine
}

func NewTranslator(port int) *Translator {
	return &Translator{inner: NewLibreEngine(port)}
}

func (t *Translator) ListLanguages() ([]LanguagePair, error) {
	return t.inner.ListLanguages()
}

func (t *Translator) IsHealthy() bool {
	return t.inner.IsHealthy()
}
