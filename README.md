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

## Repository Layout

```text
src/                React UI and client-side state
src-tauri/          Tauri host, capabilities, packaging config
go-sidecar/         Go orchestration pipeline and subtitle writer
python-backend/     Canonical Python ML backend
services/           Optional local service/model staging roots
public/             Static frontend assets
```

## Prerequisites

- Bun 1.3+
- Rust 1.88.0+ via `rustup`
- Go 1.26.1 on `PATH`
- A Python 3 runtime on `PATH` for the ML backend

Recommended:

- FFmpeg on `PATH` for video transcription workflows
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
