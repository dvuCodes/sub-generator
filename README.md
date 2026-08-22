# SubGen

SubGen is a local desktop subtitle generator built with Tauri. The app uses a React frontend, a Rust Tauri host, and a Go sidecar that coordinates transcription with `whisper-server`, optional translation (LibreTranslate, DeepL, or any OpenAI-compatible LLM), broadcast-style timing normalization, and subtitle file output in `srt`, `ass`, or `vtt`.

## Stack

- React 19 + TypeScript + Vite
- Tailwind CSS v4
- Tauri v2 with Rust
- Go sidecar
- `whisper-server` from `whisper.cpp` **v1.9.3+** (required for correct VAD timestamp mapping)
- LibreTranslate / DeepL API / OpenAI-compatible LLM endpoint (e.g. Ollama)
- FFmpeg (audio extraction + scene-cut detection)

## Repository Layout

```text
src/                React UI and sidecar IPC client
src-tauri/          Tauri host, capabilities, and desktop config
go-sidecar/         Go pipeline, service management, timing engine, translation engines
services/           Setup notes for whisper-server and LibreTranslate
public/             Static frontend assets
```

## Prerequisites

- Bun 1.3+
- Rust + Cargo
- Go 1.26.1
- FFmpeg on `PATH` (used to extract 16 kHz mono WAV audio before transcription and to detect scene cuts)

## whisper-server Setup

Use a recent build of [whisper.cpp](https://github.com/ggml-org/whisper.cpp) (`v1.9.3` or newer). Place the server binary and models like this if you want the repo-local layout:

```text
services/whisper-server/
  whisper-server.exe
  models/
    ggml-base.bin
    ggml-small.bin
    ggml-medium.bin
    ggml-large-v3.bin
    ggml-large-v3-turbo.bin
    ggml-large-v3-turbo-q5_0.bin   <- recommended default (half RAM, ~same quality)
    ggml-silero-v6.2.0.bin         <- Silero VAD model (enables real VAD filtering)
```

`model_size` in the UI maps to these filenames:

- `tiny` -> `ggml-tiny.bin`
- `base` -> `ggml-base.bin`
- `small` -> `ggml-small.bin`
- `medium` -> `ggml-medium.bin`
- `large-v3` -> `ggml-large-v3.bin`
- `turbo` -> `ggml-large-v3-turbo.bin`
- `large-v3-q5_0` -> `ggml-large-v3-q5_0.bin`
- `turbo-q5_0` -> `ggml-large-v3-turbo-q5_0.bin`
- `turbo-q8_0` -> `ggml-large-v3-turbo-q8_0.bin`

The Silero VAD model is downloaded from [ggml-org/whisper-vad](https://huggingface.co/ggml-org/whisper-vad) via `models/download-vad-model.sh silero-v6.2.0`. When present, SubGen starts the server with `-vm` and enables per-request VAD - this removes silence where Japanese hallucination loops live and is the single biggest quality win.

The sidecar starts the server with token-level timestamps enabled (`--dtw <preset> -nfa`) so cues can be split on word boundaries with millisecond precision.

## Translation Engines

Configured in Settings → Advanced:

| Engine | Requirements | Notes |
|---|---|---|
| LibreTranslate | Local install or Docker on port 5000 | Default fallback; batched requests |
| DeepL | API key (free keys end in `:fx`) | Uses `context` param for coherence |
| LLM | Any OpenAI-compatible endpoint (Ollama works: `http://127.0.0.1:11434`) | Sliding-window context, glossary, synopsis, optional QA review pass, disk cache across runs |

`auto` prefers LLM when a base URL is configured, then DeepL when a key is set, then LibreTranslate.

Environment fallbacks: `SUBGEN_LLM_BASE_URL`, `SUBGEN_LLM_MODEL`, `SUBGEN_LLM_API_KEY`, `SUBGEN_DEEPL_API_KEY`, `OPENAI_API_KEY`.

## Development

Install frontend dependencies:

```bash
bun install
```

Run the frontend only:

```bash
bun run dev
```

Run the desktop app:

```bash
bunx tauri dev
```

Build the frontend bundle:

```bash
bun run build
```

Build the desktop app:

```bash
bunx tauri build
```

## Checks

Frontend:

```bash
bun run lint
bun run build
```

Go sidecar:

```bash
go test ./...
```

Rust host:

```bash
cargo check
```

## Runtime Flow

1. React connects to the Tauri host.
2. Tauri spawns the bundled Go sidecar.
3. The sidecar starts `whisper-server` (with VAD + DTW flags when models are present) and LibreTranslate as needed.
4. Pipeline stages:
   - extract 16 kHz mono WAV via FFmpeg,
   - detect scene cuts (for shot-change snapping),
   - transcribe via `verbose_json` with JA-tuned decoding defaults (temperature fallback ladder, entropy threshold, capped context),
   - normalize cue timing (min/max durations, CPS-driven splitting on word timestamps, gap/overlap repair),
   - translate with sliding-window context (engine-dependent),
   - re-normalize against target-language rules (42 CPL latin / 13 full-width CJK lines, kinsoku line breaking),
   - write SRT/ASS/VTT plus a `<output>.qc.json` quality report.
5. The UI can cancel the active job at any time; completion returns a QC summary and cue preview.
