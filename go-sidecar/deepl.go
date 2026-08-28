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

// DeepLEngine translates via the DeepL REST API. Free-tier keys end in ":fx"
// and use api-free.deepl.com; pro keys use api.deepl.com.
type DeepLEngine struct {
	apiKey          string
	client          *http.Client
	batchSize       int
	baseURLOverride string // used by tests to point at a stub server
}

func NewDeepLEngine(apiKey string) *DeepLEngine {
	return &DeepLEngine{
		apiKey:    strings.TrimSpace(apiKey),
		client:    &http.Client{Timeout: 60 * time.Second},
		batchSize: 25,
	}
}

func (e *DeepLEngine) Name() string   { return "deepl" }
func (e *DeepLEngine) BatchSize() int { return e.batchSize }

// SetBaseURL overrides the endpoint (test hook).
func (e *DeepLEngine) SetBaseURL(url string) { e.baseURLOverride = url }

func (e *DeepLEngine) baseURL() string {
	if e.baseURLOverride != "" {
		return e.baseURLOverride
	}
	if strings.HasSuffix(e.apiKey, ":fx") {
		return "https://api-free.deepl.com"
	}
	return "https://api.deepl.com"
}

type deeplTranslateRequest struct {
	Text       []string `json:"text"`
	TargetLang string   `json:"target_lang"`
	SourceLang string   `json:"source_lang,omitempty"`
	Context    string   `json:"context,omitempty"`
}

type deeplTranslateResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
}

func (e *DeepLEngine) TranslateBatch(ctx context.Context, texts []string, req TranslateRequest) ([]string, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	payload := deeplTranslateRequest{
		Text:       texts,
		TargetLang: strings.ToUpper(req.TargetLang),
	}

	// DeepL auto-detects by omitting source_lang entirely ("auto" is rejected).
	if src := strings.TrimSpace(req.SourceLang); src != "" && !strings.EqualFold(src, "auto") {
		payload.SourceLang = strings.ToUpper(src)
	}

	// DeepL's context parameter biases translation without being translated.
	if len(req.ContextPairs) > 0 || req.Synopsis != "" {
		var sb strings.Builder
		if req.Synopsis != "" {
			sb.WriteString("Context: ")
			sb.WriteString(req.Synopsis)
			sb.WriteString("\n")
		}
		sb.WriteString("Previous lines:\n")
		for _, pair := range req.ContextPairs {
			sb.WriteString("- ")
			sb.WriteString(pair.Target)
			sb.WriteString("\n")
		}
		payload.Context = strings.TrimSpace(sb.String())
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, e.baseURL()+"/v2/translate", bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+e.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepl request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to decoding
	case http.StatusTooManyRequests:
		retryAfter := resp.Header.Get("Retry-After")
		return nil, fmt.Errorf("deepl rate limited (retry after %s)", retryAfter)
	case http.StatusForbidden:
		return nil, fmt.Errorf("deepl rejected the API key (403)")
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("deepl returned status %d: %s", resp.StatusCode, truncateForLog(string(respBody), 200))
	}

	var result deeplTranslateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse deepl response: %w", err)
	}
	if len(result.Translations) != len(texts) {
		return nil, fmt.Errorf("expected %d translations, got %d", len(texts), len(result.Translations))
	}

	out := make([]string, len(texts))
	for i, t := range result.Translations {
		out[i] = t.Text
	}
	return out, nil
}
