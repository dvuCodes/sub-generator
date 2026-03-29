# SubGen

SubGen is a local desktop subtitle generator built with Tauri. It pairs a React frontend with a Rust host and a Go sidecar to run local transcription with `whisper-server`, optional translation, and subtitle export in `srt`, `ass`, or `vtt`.

This repository is meant for source installs. Large models, local service binaries, and generated runtime mirrors are intentionally excluded from version control.

## What It Does

- Transcribes local audio and video into subtitle-ready segments
- Exports `srt`, `ass`, and `vtt`
- Supports local translation workflows when `llama-server` and a translation model are available
- Keeps the desktop flow local-first instead of sending media to a hosted SaaS

## Current Status

- Finished desktop application under active maintenance
- Install and run from source
- GitHub Releases are not the primary distribution channel yet

## Stack

- React 19 + TypeScript + Vite
- Tailwind CSS v4
- Tauri v2 with Rust
- Go sidecar
- `whisper-server` from `whisper.cpp`
- `llama-server` from `llama.cpp` (GemmaTranslate-v3 for translation)

## Install From Source

### Prerequisites

- Bun 1.3+
- Rust + Cargo (`rustup` will pick the pinned `1.88.0` toolchain from `rust-toolchain.toml`)
- Go 1.26.1 on `PATH` in the shell that runs `bunx tauri dev`
- `whisper-server` available either:
  - on `PATH`, or
  - under `services/whisper-server/`
- `llama-server` available either:
  - on `PATH`, or
  - under `services/llama-server/`
  - The GemmaTranslate-v3 GGUF model downloads on first translation request

### Setup

```bash
bun install
```

### Run

```bash
bunx tauri dev
```

### Build

```bash
bun run build
bunx tauri build
```

## Local Dependencies

The repo ignores downloaded binaries and models by design. If you want a repo-local layout instead of using tools already on `PATH`, place the assets here:

### `whisper-server` layout

```text
services/whisper-server/
  whisper-server.exe
  models/
    ggml-base.bin
    ggml-small.bin
    ggml-medium.bin
    ggml-large-v3.bin
    ggml-large-v3-turbo.bin
```

`model_size` maps to these filenames:

- `tiny` -> `ggml-tiny.bin`
- `base` -> `ggml-base.bin`
- `small` -> `ggml-small.bin`
- `medium` -> `ggml-medium.bin`
- `large-v3` -> `ggml-large-v3.bin`
- `turbo` -> `ggml-large-v3-turbo.bin`

## Development Checks

- Frontend:

```bash
bun run lint
bun run build
```

- Go sidecar:

```bash
go test ./...
```

- Rust host:

```bash
cargo check
```

## Repository Layout

```text
src/                React UI and sidecar IPC client
src-tauri/          Tauri host, capabilities, and desktop config
go-sidecar/         Go pipeline, service management, and subtitle writing
python-backend/     Canonical Python ML backend source
services/           Local binaries, models, and generated runtime mirrors
public/             Static frontend assets
```

## Runtime Flow

1. React connects to the Tauri host.
2. Tauri spawns the bundled Go sidecar.
3. The sidecar starts `whisper-server` for transcription, then `llama-server` (GemmaTranslate-v3) for translation as needed.
4. The sidecar transcribes the original input media, optionally translates the timed segments, and writes the subtitle file next to the input video unless an explicit output path is provided.
