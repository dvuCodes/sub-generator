# Audio Enhancement for Improved Transcription Accuracy

**Date:** 2026-03-24
**Status:** Approved
**Branch:** TBD (new feature branch)

## Problem

Anime/TV/movie content produces poor transcription because Whisper receives raw mixed audio where dialogue, background music, and sound effects compete in the same frequency space. This causes:

1. Misheard words during loud BGM/action scenes
2. Hallucinated text during silent or music-only segments
3. Quiet dialogue missed entirely
4. Character names and specific terms consistently wrong

## Solution

Combined approach: FFmpeg audio preprocessing + Whisper parameter tuning. Smart defaults that work automatically, with an Advanced Audio panel for power users.

## Architecture

### Current Pipeline

```
Video File → ffprobe (duration) → Raw upload to whisper-server → whisper-server --convert (internal FFmpeg) → Whisper inference (temp=0) → Subtitles
```

### Proposed Pipeline

```
Video File → ffprobe (duration) → FFmpeg Preprocessor (extract → EQ → normalize → high-pass → noise gate → WAV) → Upload clean WAV to whisper-server → Whisper inference (temp fallback) → Subtitles
```

The Go sidecar extracts and enhances audio before sending to Whisper. The preprocessor produces a 16kHz mono WAV optimized for speech recognition.

**`--convert` flag strategy:** The whisper-server `--convert` flag remains enabled in `buildWhisperCommand()` (services.go). When preprocessed WAV is uploaded, `--convert` is a harmless no-op (WAV passthrough). When preprocessing fails and raw video is uploaded as fallback, `--convert` is required to extract audio. Keeping it on ensures the fallback path always works.

The preprocessor is a single function in the Go sidecar that builds an FFmpeg command with a configurable filter chain. This is the extension point where AI-based vocal separation (e.g., Demucs) can be added later without rewriting the pipeline.

## FFmpeg Filter Chain

Default filters applied in order:

| # | Filter | FFmpeg | Purpose |
|---|--------|--------|---------|
| 1 | High-pass | `highpass=f=200` | Cut low rumble, bass, hum below 200Hz |
| 2 | Low-pass | `lowpass=f=8000` | Cut high-frequency hiss above 8kHz |
| 3 | Vocal EQ boost | `equalizer=f=1000:t=h:w=2000:g=3` | Boost vocal presence centered at 1kHz with 2kHz bandwidth (~0–2kHz range) by 3dB |
| 4 | Loudness normalization | `loudnorm=I=-16:TP=-1.5:LRA=11` | EBU R128 normalization for consistent volume |
| 5 | Noise gate | `agate=threshold=0.01:attack=5:release=50` | Silence quiet non-speech segments |
| 6 | Output format | `-ar 16000 -ac 1 -f wav` | 16kHz mono WAV — native Whisper input format |

Full default command:

```
ffmpeg -i input.mkv -vn \
  -af "highpass=f=200,lowpass=f=8000,equalizer=f=1000:t=h:w=2000:g=3,loudnorm=I=-16:TP=-1.5:LRA=11,agate=threshold=0.01:attack=5:release=50" \
  -ar 16000 -ac 1 -f wav output.wav
```

### User-Tunable Parameters (Advanced Audio panel)

| Setting | Control | Default | Description |
|---------|---------|---------|-------------|
| Audio Enhancement | Toggle | On | Master switch — bypass all preprocessing |
| Vocal Boost | Slider 0–6 dB | 3 | Emphasis on vocal frequencies |
| Noise Gate | Toggle | On | Suppress quiet non-speech segments |
| Normalization | Toggle | On | Even out volume levels |

## Whisper Parameter Tuning

### Temperature Fallback

Change temperature from fixed `0` to fallback list `0.0,0.2,0.4,0.6,0.8,1.0`.

Whisper starts deterministic at 0 and only escalates if it detects hallucination signs (high compression ratio > 2.4 or low average log probability < -1.0). Most segments still decode at temp=0. Only problematic segments retry at higher temperatures.

### Unchanged Parameters

- `beam_size`: Already user-configurable (1–8 slider, default 5)
- `vad`: Already user-configurable toggle
- `language`, `detect_language`, `response_format`: Unchanged

## Code Changes

### New Files

**`go-sidecar/audio.go`**
- `PreprocessAudio(inputPath string, cfg AudioConfig) (string, error)` — main entry point. Takes video/audio file, applies filter chain, returns path to cleaned WAV in temp dir.
- `buildFilterChain(cfg AudioConfig) string` — assembles the FFmpeg `-af` filter string from config.
- `cleanupPreprocessed(wavPath string)` — deletes temp WAV after upload completes.

### Modified Files

**`go-sidecar/config.go`** — Add `AudioConfig` struct:

```go
type AudioConfig struct {
    Enabled      bool    `json:"enabled"`       // default true
    VocalBoostDB float64 `json:"vocal_boost_db"` // 0-6, default 3
    NoiseGate    bool    `json:"noise_gate"`     // default true
    Normalize    bool    `json:"normalize"`      // default true
}
```

**`go-sidecar/config.go`** — Also add `AudioConfig` field to the existing `Command` struct:

```go
type Command struct {
    // ... existing fields ...
    AudioConfig AudioConfig `json:"audio_config"`
}
```

**`go-sidecar/pipeline.go`** — Orchestrates preprocessing. Call `PreprocessAudio()` before invoking the transcriber. If preprocessing succeeds, pass the clean WAV path to `Transcribe()`. If it fails, log a warning and pass the original video path (fallback). Wire `AudioConfig` defaults when not provided by frontend.

**`go-sidecar/transcriber.go`** — One change:
1. Change temperature from `"0"` to `"0.0,0.2,0.4,0.6,0.8,1.0"`

Note: `Transcribe()` signature is unchanged — it already takes a file path. The pipeline passes either the preprocessed WAV or the original video path depending on preprocessing success.

**`src/lib/types.ts`** — Add `AudioConfig` interface to `GenerateCommand`:

```typescript
interface AudioConfig {
    enabled: boolean;
    vocal_boost_db: number;
    noise_gate: boolean;
    normalize: boolean;
}
```

**`src/components/SettingsPanel.tsx`** — Add "Audio Enhancement" section to Advanced panel with master toggle, vocal boost slider, noise gate toggle, normalization toggle.

**`src/App.tsx`** — Initialize `AudioConfig` defaults in state and pass to `GenerateCommand`.

### Untouched Files

whisper-server (binary), services.go (whisper startup — `--convert` stays enabled), eta.go, media.go — all unchanged.

## Error Handling

Preprocessing is **best-effort, never blocking.** If it fails, the pipeline degrades gracefully to today's behavior (raw upload with `--convert`).

| Scenario | Behavior |
|----------|----------|
| FFmpeg preprocessing fails | Fallback to raw upload. Log error, warn user enhancement was skipped. |
| Disk full (temp WAV write fails) | Same fallback. |
| Audio enhancement disabled by user | Skip preprocessing. Temperature fallback still applies. |
| Video has no audio stream | Pipeline error surfaced to user: "No audio stream found in file." |
| Audio-only input (MP3, WAV, etc.) | Out of scope. Current `supportedVideoExts` validation rejects non-video extensions. Audio-only support can be added in a future change by expanding the allowed extensions map. |
| Temp WAV cleanup fails | Log and continue. Non-fatal. |

### Temp File Lifecycle

1. `PreprocessAudio()` creates WAV in `os.TempDir()/sub-generator-audio/`
2. Pipeline uploads WAV to whisper-server
3. Upload completes → `cleanupPreprocessed()` deletes WAV
4. If pipeline aborted mid-flight → deferred cleanup runs on pipeline exit

**Disk space estimate:** 16kHz mono 16-bit PCM WAV is approximately 1.9 MB per minute of audio (~30 MB for a 15-minute episode, ~230 MB for a 2-hour movie). The temp file is deleted immediately after upload, so only one file exists at a time.

## Testing Strategy

### Unit Tests (`go-sidecar/audio_test.go`)

- `TestBuildFilterChain_Defaults` — default config produces expected filter string
- `TestBuildFilterChain_NoiseGateOff` — noise gate disabled → agate omitted
- `TestBuildFilterChain_NormalizeOff` — normalization disabled → loudnorm omitted
- `TestBuildFilterChain_VocalBoostRange` — 0dB → EQ omitted; 6dB → gain=6
- `TestBuildFilterChain_SubTogglesOff` — master toggle ON but all three sub-toggles off (vocal_boost=0, noise_gate=off, normalize=off) → only highpass, lowpass, and output format remain. Note: master toggle OFF means no FFmpeg at all (raw upload); this test covers the sub-toggle-only case.

### Integration Tests (`go-sidecar/audio_test.go`, require FFmpeg)

- `TestPreprocessAudio_ValidVideo` — runs FFmpeg on test video, verifies 16kHz mono WAV output
- `TestPreprocessAudio_AudioOnly` — removed (audio-only input is out of scope for this feature)
- `TestPreprocessAudio_NoAudioStream` — no audio → returns descriptive error
- `TestPreprocessAudio_InvalidFile` — corrupt/nonexistent → returns error
- `TestPreprocessAudio_Cleanup` — verifies temp WAV deleted after cleanup

### Config Tests (`go-sidecar/config_test.go`)

- `TestAudioConfig_Defaults` — missing AudioConfig → defaults applied
- `TestAudioConfig_Deserialize` — frontend JSON → correctly parsed

### Manual Verification

Run full pipeline on a known anime clip (loud BGM + dialogue) and compare:
- Transcription with enhancement OFF vs ON
- Hallucination count during music-only segments
- Quiet dialogue line capture
- Word accuracy during action scenes

## Future: AI Vocal Separation

The preprocessor is architectured as a pluggable step. To add AI-based vocal separation (Demucs/UVR) later:

1. Add an alternative/additional step before or instead of the FFmpeg filter chain in `PreprocessAudio()`
2. The function signature and temp file lifecycle remain the same
3. The rest of the pipeline (upload, inference, subtitle generation) is unaffected
