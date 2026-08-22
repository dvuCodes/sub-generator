# SubGen

SubGen is a local-first desktop app for generating subtitle files from video and audio on your own machine. It pairs a React interface with a Tauri desktop shell, a Go orchestration sidecar, and a Python ML backend for transcription, translation, and optional speaker diarization.

SubGen currently targets source builds first. The repository is structured to help contributors run the app locally, inspect the pipeline, and improve subtitle reliability without depending on a hosted service.

## Why SubGen

- Local-first workflow for sensitive media and iterative subtitle work
- Desktop UI for running transcription and translation without wiring together multiple CLIs
- Mixed-runtime architecture that keeps the UI responsive while heavy ML work runs in dedicated services
- Source-available pipeline that is practical to debug and extend

## Current Architecture

### Default runtime path

- React 19 + TypeScript + Vite frontend
- Tauri v2 + Rust desktop host
- Go sidecar for job orchestration, setup checks, and subtitle output
- Python ML backend for:
  - Faster Whisper ASR
  - NLLB translation
  - pyannote speaker diarization

The Python backend downloads default model artifacts on demand into `python-backend/models/`. You can override the cache location with `SUBGEN_ML_CACHE`.

### Optional manual backends

The repo still contains compatibility paths for:

- `whisper-server` from `whisper.cpp`
- `llama-server` from `llama.cpp` for Gemma-based translation

Those are optional/manual backends, not the primary open-source quickstart path.

### Quality and timing pipeline

All ASR paths now share the same subtitle-quality stages:

- FFmpeg extracts a canonical 16 kHz mono WAV and detects scene cuts.
- Whisper.cpp uses `verbose_json` seconds-based segment and word timestamps, with centisecond fallback for legacy forks.
- Cue normalization applies duration, reading-speed, line-length, overlap, frame-grid, and optional shot-snap rules.
- Output includes SRT/ASS/VTT plus a sibling `<output>.qc.json` report and an in-app cue preview.
- Active jobs can be cancelled from the processing view.

The selected NLLB or Gemma backend remains the default translation path. Advanced settings can instead use LibreTranslate, DeepL, or an OpenAI-compatible LLM with sliding-window context, synopsis, glossary, honorific handling, and an optional QA pass. Environment fallbacks are `SUBGEN_LLM_BASE_URL`, `SUBGEN_LLM_MODEL`, `SUBGEN_LLM_API_KEY`, `SUBGEN_DEEPL_API_KEY`, and `OPENAI_API_KEY`.

For the optional whisper.cpp backend, use whisper.cpp v1.9.3 or newer and place assets under:

```text
services/whisper-server/
  whisper-server.exe
  models/
    ggml-large-v3.bin
    ggml-large-v3-turbo.bin
    ggml-large-v3-turbo-q5_0.bin
    ggml-silero-v6.2.0.bin
```

The Silero model enables request-level VAD. When a compatible model preset exists, the sidecar starts whisper.cpp with DTW token timestamps and flash attention disabled.

## Repository Layout

```text
src/                React UI and client-side state
src-tauri/          Tauri host, capabilities, packaging config
go-sidecar/         Go orchestration, timing, translation, QC, and subtitle output
python-backend/     Canonical Python ML backend
services/           Optional local service/model staging roots
public/             Static frontend assets
```

## Prerequisites

- Bun 1.3+
- Rust 1.88.0+ via `rustup`
- Go 1.26.1 on `PATH`
- A Python 3 runtime on `PATH` for the ML backend
- FFmpeg on `PATH` for audio extraction and scene-cut detection

Recommended:
- CUDA-capable environment if you want GPU acceleration

Optional:

- `HF_TOKEN` if your Hugging Face setup requires authentication for model downloads
- `SUBGEN_ML_CACHE` if you want model downloads outside the repo working tree

## Quickstart

1. Install frontend dependencies:

```bash
bun install
```

2. Install Python backend dependencies into the Python runtime SubGen will use:

```bash
python -m pip install -r python-backend/requirements.txt
```

3. Start the desktop app:

```bash
bun run tauri:dev
```

SubGen will use the Python backend by default and download the default Faster Whisper / NLLB / diarization assets when those features are first needed.

## Development Commands

Frontend only:

```bash
bun run dev
```

Desktop app:

```bash
bun run tauri:dev
```

Frontend production build:

```bash
bun run build
```

Desktop production build:

```bash
bun run tauri:build
```

## Verification

Frontend:

```bash
bun run lint
bun run test
bun run build
```

Go sidecar:

```bash
cd go-sidecar
go test ./...
```

Python backend:

```bash
python -m unittest discover -s python-backend -p "test_*.py"
```

Rust host:

```bash
cd src-tauri
cargo check
```

## Technical Notes

- Default ASR model: `deepdml/faster-whisper-large-v3-turbo-ct2`
- Default translation model: `JustFrederik/nllb-200-distilled-600M-ct2-int8`
- Default diarization model: `pyannote/speaker-diarization-community-1`
- Subtitle output formats: `srt`, `ass`, `vtt`
- The Go sidecar owns service lifecycle, setup checks, dependency installation guidance, and output writing

## Project Status

SubGen is an active work-in-progress focused on subtitle reliability, long-form transcription correctness, and a smoother local setup story.

What this repo is ready for:

- building from source
- inspecting and contributing to the pipeline
- testing local-first transcription and translation workflows

What is still evolving:

- packaged distribution and release assets
- smoother first-run dependency setup
- broader contributor documentation beyond the core quickstart
