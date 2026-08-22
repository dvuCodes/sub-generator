package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Translation engine abstraction.
//
// Engines receive cue texts in batches and return exactly one translation
// per input, in order. The orchestrator feeds finalized translations forward
// as sliding context so pronouns and tone stay consistent across cues.

// ContextPair is one previously translated line pair used as read-only context.
type ContextPair struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// TranslateRequest carries everything an engine may need for a batch.
type TranslateRequest struct {
	SourceLang   string
	TargetLang   string
	Synopsis     string
	Glossary     []GlossaryEntry
	Honorifics   string        // "" | "keep" | "drop"
	ContextPairs []ContextPair // most recent finalized lines
}

// TranslateEngine translates cue texts in batches.
type TranslateEngine interface {
	Name() string
	BatchSize() int
	TranslateBatch(ctx context.Context, texts []string, req TranslateRequest) ([]string, error)
}

// contextWindow is how many finalized line pairs feed into each batch.
const contextWindow = 12

// ContextualTranslate runs every segment through the engine in batches,
// feeding finalized translations forward as sliding context. Batches that
// fail are retried whole once, then halved recursively down to single
// segments before giving up.
func ContextualTranslate(
	ctx context.Context,
	segments []Segment,
	engine TranslateEngine,
	req TranslateRequest,
	onProgress func(done, total int),
) ([]Segment, error) {
	out := make([]Segment, len(segments))
	sources := make([]string, 0, len(segments))
	targets := make([]string, 0, len(segments))

	batchSize := engine.BatchSize()
	if batchSize < 1 {
		batchSize = 1
	}

	done := 0
	for start := 0; start < len(segments); start += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := minInt(start+batchSize, len(segments))

		texts := make([]string, 0, end-start)
		for _, seg := range segments[start:end] {
			texts = append(texts, seg.Text)
		}

		req.ContextPairs = recentPairs(sources, targets, contextWindow)

		translated, err := translateWithRecovery(ctx, engine, texts, req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", engine.Name(), err)
		}

		for i := range texts {
			src := segments[start+i]
			final := PostProcessTranslation(translated[i], req.Honorifics)
			out[start+i] = Segment{
				Start:        src.Start,
				End:          src.End,
				Text:         final,
				SourceText:   src.Text, // keep source for the QA review pass
				Words:        src.Words,
				AvgLogprob:   src.AvgLogprob,
				NoSpeechProb: src.NoSpeechProb,
				SpeakerID:    src.SpeakerID,
				SpeakerLabel: src.SpeakerLabel,
			}
			sources = append(sources, src.Text)
			targets = append(targets, final)
		}

		done = end
		if onProgress != nil {
			onProgress(done, len(segments))
		}
	}

	return out, nil
}

func translateWithRecovery(
	ctx context.Context,
	engine TranslateEngine,
	texts []string,
	req TranslateRequest,
) ([]string, error) {
	out, engineErr := engine.TranslateBatch(ctx, texts, req)
	if engineErr == nil && len(out) == len(texts) {
		return out, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(out) != len(texts) {
		engineErr = errors.Join(engineErr, fmt.Errorf(
			"engine returned %d translations for %d inputs", len(out), len(texts),
		))
	}
	if engineErr == nil {
		engineErr = errors.New("translation failed")
	}

	if len(texts) == 1 {
		retryOut, retryErr := engine.TranslateBatch(ctx, texts, req)
		if retryErr != nil || len(retryOut) != 1 {
			return nil, fmt.Errorf("%q: %w",
				truncateForLog(texts[0], 60), errors.Join(engineErr, retryErr))
		}
		return retryOut, nil
	}

	mid := len(texts) / 2
	left, lerr := translateWithRecovery(ctx, engine, texts[:mid], req)
	if lerr != nil {
		return nil, lerr
	}
	right, rerr := translateWithRecovery(ctx, engine, texts[mid:], req)
	if rerr != nil {
		return nil, rerr
	}
	return append(left, right...), nil
}

func recentPairs(sources, targets []string, limit int) []ContextPair {
	n := minInt(len(sources), len(targets))
	start := maxInt(0, n-limit)
	pairs := make([]ContextPair, 0, n-start)
	for i := start; i < n; i++ {
		pairs = append(pairs, ContextPair{Source: sources[i], Target: targets[i]})
	}
	return pairs
}

// honorificSuffixRe strips hyphen-attached Japanese honorifics only
// ("Kaguya-san" -> "Kaguya"). Space-separated forms are deliberately NOT
// matched: "a tan coat", "in San Francisco" and surnames like "Susan Tan"
// are legitimate English and must survive the drop policy.
var honorificSuffixRe = regexp.MustCompile(
	`(?i)([A-Za-z][A-Za-z'’]{1,24})[-–—](san|kun|chan|sama|sensei|senpai|dono)\b`,
)

// asciiSpaceRun collapses runs of ASCII whitespace only, leaving CJK
// ideographic spaces (U+3000) intact.
var asciiSpaceRun = regexp.MustCompile(`[ \t\n\r\f\v]+`)

// PostProcessTranslation normalizes engine output and applies honorific policy.
func PostProcessTranslation(text string, honorifics string) string {
	text = strings.TrimSpace(text)
	text = asciiSpaceRun.ReplaceAllString(text, " ")

	if strings.EqualFold(honorifics, "drop") {
		text = honorificSuffixRe.ReplaceAllString(text, "$1")
		text = strings.TrimSpace(text)
	}
	return text
}

func truncateForLog(s string, limit int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
