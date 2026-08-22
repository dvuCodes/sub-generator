package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LLMEngine translates via any OpenAI-compatible chat completions endpoint
// (Ollama, llama.cpp server, LM Studio, or a cloud provider). It translates
// cue batches with sliding-window context, a glossary, and strict JSON
// output, caching results on disk across runs.
type LLMEngine struct {
	baseURL     string
	model       string
	apiKey      string
	client      *http.Client
	batchSize   int
	temperature float64
	cache       *translationCache
}

func NewLLMEngine(baseURL, model, apiKey string) *LLMEngine {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL != "" && !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	if model == "" {
		model = "qwen2.5:7b-instruct"
	}

	engine := &LLMEngine{
		baseURL:     baseURL,
		model:       model,
		apiKey:      strings.TrimSpace(apiKey),
		client:      &http.Client{Timeout: 10 * time.Minute},
		batchSize:   10,
		temperature: 0.2,
	}
	engine.cache = loadTranslationCache(model)
	return engine
}

func (e *LLMEngine) Name() string   { return "llm:" + e.model }
func (e *LLMEngine) BatchSize() int { return e.batchSize }

// --- request/response types ---

type llmChatRequest struct {
	Model       string           `json:"model"`
	Messages    []llmChatMessage `json:"messages"`
	Temperature float64          `json:"temperature"`
	Stream      bool             `json:"stream"`
}

type llmChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (e *LLMEngine) TranslateBatch(ctx context.Context, texts []string, req TranslateRequest) ([]string, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if e.baseURL == "" {
		return nil, errors.New("no LLM endpoint configured")
	}

	out := make([]string, len(texts))
	pending := make([]int, 0, len(texts))
	policy := policyHash(req)
	for i, text := range texts {
		if cached, ok := e.cache.get(text, req.SourceLang, req.TargetLang, policy); ok {
			out[i] = cached
		} else {
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 {
		return out, nil
	}

	pendingTexts := make([]string, len(pending))
	for i, idx := range pending {
		pendingTexts[i] = texts[idx]
	}

	fresh, err := e.translateFresh(ctx, pendingTexts, req)
	if err != nil {
		return nil, err
	}
	for i, idx := range pending {
		out[idx] = fresh[i]
		e.cache.put(texts[idx], req.SourceLang, req.TargetLang, policy, fresh[i])
	}
	e.cache.save()
	return out, nil
}

func (e *LLMEngine) translateFresh(ctx context.Context, texts []string, req TranslateRequest) ([]string, error) {
	content, err := e.doChat(ctx, []llmChatMessage{
		{Role: "system", Content: buildLLMSystemPrompt(req)},
		{Role: "user", Content: buildLLMUserPrompt(texts, req)},
	})
	if err != nil {
		return nil, err
	}
	return parseLLMTranslations(content, texts)
}

func (e *LLMEngine) doChat(ctx context.Context, messages []llmChatMessage) (string, error) {
	payload, err := json.Marshal(llmChatRequest{
		Model:       e.model,
		Messages:    messages,
		Temperature: e.temperature,
		Stream:      false,
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, e.baseURL+"/chat/completions", bytes.NewReader(payload),
	)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("LLM request failed (%s): %w", e.baseURL, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		var apiErr llmChatResponse
		_ = json.Unmarshal(respBody, &apiErr)
		if apiErr.Error != nil && apiErr.Error.Message != "" {
			return "", fmt.Errorf("LLM error %d: %s", resp.StatusCode, truncateForLog(apiErr.Error.Message, 300))
		}
		return "", fmt.Errorf("LLM returned status %d: %s", resp.StatusCode, truncateForLog(string(respBody), 200))
	}

	var chatResp llmChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse LLM response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", errors.New("LLM response contained no choices")
	}
	return chatResp.Choices[0].Message.Content, nil
}

// parseLLMTranslations extracts the translations array from model output,
// tolerating code fences, surrounding prose and trailing commas.
func parseLLMTranslations(content string, texts []string) ([]string, error) {
	content = stripCodeFences(content)

	expect := func(got []string, err error) ([]string, error) {
		if err != nil {
			return nil, err
		}
		if len(got) != len(texts) {
			return nil, fmt.Errorf("LLM returned %d translations for %d lines", len(got), len(texts))
		}
		return got, nil
	}

	if objStart := strings.Index(content, "{"); objStart >= 0 {
		var wrapper struct {
			Translations []string `json:"translations"`
		}
		err := decodeLenientJSON(content[objStart:], &wrapper)
		if out, perr := expect(wrapper.Translations, err); perr == nil {
			return out, nil
		} else if err == nil {
			// Valid JSON but wrong cardinality - report it directly.
			return nil, perr
		}
	}
	if arrStart := strings.Index(content, "["); arrStart >= 0 {
		var list []string
		err := decodeLenientJSON(content[arrStart:], &list)
		if out, perr := expect(list, err); perr == nil {
			return out, nil
		}
	}

	return nil, fmt.Errorf("LLM returned no parsable translations for %d lines: %q",
		len(texts), truncateForLog(content, 160))
}

// stripCodeFences removes markdown fences around a JSON payload.
func stripCodeFences(content string) string {
	content = strings.TrimSpace(content)
	for _, open := range []string{"```json", "```JSON", "```"} {
		if strings.HasPrefix(content, open) {
			content = content[len(open):]
			break
		}
	}
	content = strings.TrimSuffix(strings.TrimSpace(content), "```")
	return strings.TrimSpace(content)
}

// decodeLenientJSON decodes the first JSON value from s, tolerating trailing
// prose after the value and trailing commas inside arrays/objects. The plain
// decode is attempted first so valid JSON is never touched by the lenient
// path.
func decodeLenientJSON(s string, v any) error {
	if err := json.NewDecoder(strings.NewReader(s)).Decode(v); err == nil {
		return nil
	}
	s = stripTrailingCommas(s)
	return json.NewDecoder(strings.NewReader(s)).Decode(v)
}

// stripTrailingCommas removes commas that directly precede a closing brace or
// bracket, ignoring anything inside JSON strings (a state machine rather than
// a regex, so text like "He said \",] ok\"" survives untouched).
func stripTrailingCommas(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			b.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
			b.WriteByte(c)
		case ',':
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				continue // drop the trailing comma
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// --- prompt construction ---

// RefinePass implements the optional QA pass: an LLM review of every already
// translated cue with wider context, correcting mistranslations and
// unnatural phrasing. It satisfies the pipeline's refiner interface.
func (e *LLMEngine) RefinePass(
	ctx context.Context,
	segments []Segment,
	req TranslateRequest,
	onProgress func(done, total int),
) ([]Segment, error) {
	if len(segments) == 0 {
		return nil, nil
	}

	out := make([]Segment, len(segments))
	copy(out, segments)

	batchSize := e.batchSize * 2 // review tolerates larger batches than drafting

	for start := 0; start < len(out); start += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := minInt(start+batchSize, len(out))
		batch := out[start:end]

		current := make([]string, 0, len(batch))
		for _, seg := range batch {
			current = append(current, seg.Text)
		}

		content, err := e.doChat(ctx, []llmChatMessage{
			{Role: "system", Content: buildLLMReviewSystemPrompt(req)},
			{Role: "user", Content: buildLLMReviewUserPrompt(batch, req)},
		})
		if err != nil {
			return nil, err
		}

		reviewed, err := parseLLMTranslations(content, current)
		if err != nil {
			return nil, fmt.Errorf("QA pass: %w", err)
		}

		for i := range reviewed {
			out[start+i].Text = PostProcessTranslation(reviewed[i], req.Honorifics)
		}

		if onProgress != nil {
			onProgress(end, len(out))
		}
	}

	return out, nil
}

func buildLLMReviewSystemPrompt(req TranslateRequest) string {
	var sb strings.Builder
	sb.WriteString("You are a senior subtitle QC editor reviewing ")
	sb.WriteString(languageDisplayName(req.SourceLang))
	sb.WriteString(" to ")
	sb.WriteString(languageDisplayName(req.TargetLang))
	sb.WriteString(" subtitle translations.\n\n")
	sb.WriteString("For each numbered pair you will see the source line and its current English translation. Correct:\n")
	sb.WriteString("- mistranslations or dropped/added meaning,\n")
	sb.WriteString("- unnatural or stiff phrasing (subtitles should sound like spoken dialogue),\n")
	sb.WriteString("- pronoun or name inconsistencies with earlier lines.\n")
	sb.WriteString("Never merge or split lines; return exactly one corrected line per input.\n")

	switch strings.ToLower(req.Honorifics) {
	case "keep":
		sb.WriteString("Keep Japanese honorifics where they were kept.\n")
	case "drop":
		sb.WriteString("Honorifics must be removed.\n")
	}

	if len(req.Glossary) > 0 {
		sb.WriteString("\nGlossary (enforce exactly):\n")
		for _, entry := range req.Glossary {
			entry.Source = strings.TrimSpace(entry.Source)
			entry.Target = strings.TrimSpace(entry.Target)
			if entry.Source == "" || entry.Target == "" {
				continue
			}
			fmt.Fprintf(&sb, "- %s => %s\n", entry.Source, entry.Target)
		}
	}

	sb.WriteString("\nReturn ONLY valid JSON: {\"translations\": [\"...\", ...]} in input order.")
	return sb.String()
}

func buildLLMReviewUserPrompt(batch []Segment, req TranslateRequest) string {
	var sb strings.Builder

	if synopsis := strings.TrimSpace(req.Synopsis); synopsis != "" {
		sb.WriteString("Series/film synopsis (context only):\n")
		sb.WriteString(synopsis)
		sb.WriteString("\n\n")
	}

	fmt.Fprintf(&sb, "Review and correct these %d subtitle pairs:\n", len(batch))
	for i, seg := range batch {
		source := seg.SourceText
		if source == "" {
			source = "(source unavailable - polish phrasing only)"
		}
		fmt.Fprintf(&sb, "%d. %s\n   => %s\n", i+1, source, seg.Text)
	}
	return sb.String()
}

func buildLLMSystemPrompt(req TranslateRequest) string {
	var sb strings.Builder

	srcName := languageDisplayName(req.SourceLang)
	tgtName := languageDisplayName(req.TargetLang)

	sb.WriteString("You are a professional subtitle translator specializing in ")
	sb.WriteString(srcName)
	sb.WriteString(" to ")
	sb.WriteString(tgtName)
	sb.WriteString(" audiovisual translation.\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- Translate each numbered line into natural, concise spoken English suitable for subtitles.\n")
	sb.WriteString("- Preserve meaning and tone; prefer natural dialogue over literal wording.\n")
	sb.WriteString("- Keep the same number of lines; never merge, split, reorder, or skip lines.\n")
	sb.WriteString("- Subtitles must be brief. Omit filler words (um, uh, ah) unless dramatically meaningful.\n")
	sb.WriteString("- Do not add explanations, notes, romanizations, or quotes around the translation.\n")

	switch strings.ToLower(req.Honorifics) {
	case "keep":
		sb.WriteString("- Keep Japanese honorifics (-san, -kun, -chan, -sama) attached to names where natural.\n")
	case "drop":
		sb.WriteString("- Remove Japanese honorifics (-san, -kun, -chan, -sama); use plain names.\n")
	}

	if len(req.Glossary) > 0 {
		sb.WriteString("\nGlossary (use these exact renderings):\n")
		for _, entry := range req.Glossary {
			entry.Source = strings.TrimSpace(entry.Source)
			entry.Target = strings.TrimSpace(entry.Target)
			if entry.Source == "" || entry.Target == "" {
				continue
			}
			fmt.Fprintf(&sb, "- %s => %s\n", entry.Source, entry.Target)
		}
	}

	sb.WriteString("\nReturn ONLY valid JSON: {\"translations\": [\"...\", ...]} matching the input order exactly.")
	return sb.String()
}

func buildLLMUserPrompt(texts []string, req TranslateRequest) string {
	var sb strings.Builder

	if synopsis := strings.TrimSpace(req.Synopsis); synopsis != "" {
		sb.WriteString("Series/film synopsis (context only):\n")
		sb.WriteString(synopsis)
		sb.WriteString("\n\n")
	}

	if len(req.ContextPairs) > 0 {
		sb.WriteString("Previously translated lines (context only, do NOT re-translate):\n")
		for _, pair := range req.ContextPairs {
			source := pair.Source
			if source == "" {
				source = "(same)"
			}
			fmt.Fprintf(&sb, "%s -> %s\n", source, pair.Target)
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Translate these %d lines:\n", len(texts)))
	for i, text := range texts {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, text)
	}
	return sb.String()
}

var displayLanguageNames = map[string]string{
	"ja": "Japanese", "en": "English", "zh": "Chinese", "ko": "Korean",
	"es": "Spanish", "fr": "French", "de": "German", "it": "Italian",
	"pt": "Portuguese", "ru": "Russian", "ar": "Arabic", "hi": "Hindi",
	"th": "Thai", "vi": "Vietnamese", "id": "Indonesian", "auto": "the detected source",
}

func languageDisplayName(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if name, ok := displayLanguageNames[code]; ok {
		return name
	}
	if code == "" {
		return "the detected source"
	}
	return code
}

// --- disk cache ---

type translationCache struct {
	mu      sync.Mutex
	model   string
	entries map[string]string
	path    string
	dirty   bool
}

// policyHash fingerprints everything besides the raw text that influences a
// translation (honorific policy, glossary, synopsis) so cache entries are
// invalidated when settings change.
func policyHash(req TranslateRequest) string {
	var sb strings.Builder
	sb.WriteString(strings.ToLower(strings.TrimSpace(req.Honorifics)))

	entries := append([]GlossaryEntry(nil), req.Glossary...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Source != entries[j].Source {
			return entries[i].Source < entries[j].Source
		}
		return entries[i].Target < entries[j].Target
	})
	for _, e := range entries {
		source := strings.TrimSpace(e.Source)
		target := strings.TrimSpace(e.Target)
		if source == "" || target == "" {
			// Mirror the prompt builders, which also skip half-empty rows.
			continue
		}
		sb.WriteString("\x1f")
		sb.WriteString(source)
		sb.WriteString("=")
		sb.WriteString(target)
	}

	if synopsis := strings.TrimSpace(req.Synopsis); synopsis != "" {
		sum := sha256.Sum256([]byte(synopsis))
		sb.WriteString("\x1esyn:")
		sb.WriteString(hex.EncodeToString(sum[:8]))
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:8])
}

// cacheKey hashes every dimension that determines a translation: source and
// target language, the policy fingerprint (honorifics/glossary/synopsis) and
// the source text itself.
func cacheKey(text, sourceLang, targetLang, policy string) string {
	sum := sha256.Sum256([]byte(sourceLang + "\x00" + targetLang + "\x00" + policy + "\x00" + text))
	return hex.EncodeToString(sum[:])
}

func (c *translationCache) get(text, sourceLang, targetLang, policy string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.entries[cacheKey(text, sourceLang, targetLang, policy)]
	return v, ok
}

func (c *translationCache) put(text, sourceLang, targetLang, policy, translation string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := cacheKey(text, sourceLang, targetLang, policy)
	if c.entries[key] != translation {
		c.entries[key] = translation
		c.dirty = true
	}
}

func (c *translationCache) save() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty || c.path == "" {
		return
	}
	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, c.path); err != nil {
		// Keep dirty so the next save retries; drop the orphaned temp file.
		_ = os.Remove(tmp)
		return
	}
	c.dirty = false
}

func loadTranslationCache(model string) *translationCache {
	cache := &translationCache{model: model, entries: map[string]string{}}

	if base, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(base, "subgen")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			safeModel := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(model)
			cache.path = filepath.Join(dir, "translations-"+safeModel+".json")
		}
	}

	data, err := os.ReadFile(cache.path)
	if err != nil {
		return cache
	}
	if err := json.Unmarshal(data, &cache.entries); err != nil {
		fmt.Fprintf(os.Stderr, "[subgen] discarding corrupt translation cache %s: %v\n", cache.path, err)
		cache.entries = map[string]string{}
	}
	return cache
}
