# Context-Aware Stitch Translation Design

## Problem

The current subtitle pipeline translates each whisper segment independently, losing conversational context. This is especially damaging for Japanese, where subjects are routinely omitted, speech register carries meaning, and honorifics/sentence-final particles require surrounding dialogue to translate correctly.

## Solution

Group whisper segments into "context blocks" based on temporal proximity and translate them together with rolling history. All changes are within the Go sidecar — no new dependencies, no CGO, no architecture changes.

## Architecture

```
whisper-server → segments → StitchSegments() → ContextBlocks → TranslateBlocks() → segments → subtitles
```

### Stitcher (`stitcher.go`)

Groups consecutive segments where:
- Gap between segments <= 2.0s
- Total block span <= 30.0s
- Segment count <= 10

Blank/near-blank segments are preserved in order and never dropped.

### Contextual Translator (`translator.go`)

- Shared `sendChatCompletion(system, user)` helper for all LLM requests
- Generic base system prompt for all language pairs
- Japanese-specific suffix appended only when `sourceLang == "ja"`
- Rolling history of last 4 translated lines carried between blocks
- Strict parser: `[N]` numbering only, contiguous 1..N, no positional fallback
- Parse -> retry (fresh attempt, stricter formatting) -> per-segment fallback
- Fallback-produced translations included in rolling history
- Per-segment fallback failure stops the pipeline

### Service Config (`services.go`)

- GPU: `--ctx-size 2048`
- CPU: `--ctx-size 1024`

## Files Changed

| File | Change |
|------|--------|
| `go-sidecar/stitcher.go` | NEW — ContextBlock, StitcherConfig, StitchSegments() |
| `go-sidecar/stitcher_test.go` | NEW — 10 unit tests |
| `go-sidecar/translator.go` | Refactored: sendChatCompletion, contextual prompts, strict parser, TranslateBlocks, TranslateSegments wrapper |
| `go-sidecar/translator_test.go` | Added 18 new tests |
| `go-sidecar/services.go` | Conditional ctx-size |

## Escalation Path

If quality is insufficient:
1. Swap stitcher with VAD-based stitcher (silero-go)
2. Swap llama-server for Ollama
3. Swap translation model
4. Full Go-native rewrite with whisper.cpp bindings
