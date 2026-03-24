# SubGen

SubGen is a local desktop subtitle generator built with Tauri. The app uses a React frontend, a Rust Tauri host, and a Go sidecar that coordinates transcription with `whisper-server`, optional translation with GemmaTranslate-v3 via `llama-server`, and subtitle file output in `srt`, `ass`, or `vtt`.

## Stack

- React 19 + TypeScript + Vite
- Tailwind CSS v4
- Tauri v2 with Rust
- Go sidecar
- `whisper-server` from `whisper.cpp`
- `llama-server` from `llama.cpp` (GemmaTranslate-v3 for translation)

## Repository Layout

```text
src/                React UI and sidecar IPC client
src-tauri/          Tauri host, capabilities, and desktop config
go-sidecar/         Go pipeline, service management, and subtitle writing
services/           Setup notes for whisper-server and llama-server
public/             Static frontend assets
```

## Prerequisites

- Bun 1.3+
- Rust + Cargo (`rustup` will pick the pinned `1.88.0` toolchain from `rust-toolchain.toml`)
- Go 1.26.1 on `PATH` in the active shell that runs `bunx tauri dev`
- `whisper-server` available either:
  - on `PATH`, or
  - under `services/whisper-server/`
- `llama-server` available either:
  - on `PATH`, or
  - under `services/llama-server/`
  - The GemmaTranslate-v3 GGUF model (~7 GB) downloads automatically on first translation request

The Go sidecar now checks the documented `services/whisper-server/` layout before falling back to plain `whisper-server` on `PATH`.

## whisper-server Setup

Place the server binary and models like this if you want the repo-local layout:

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

`model_size` in the UI maps to these filenames:

- `tiny` -> `ggml-tiny.bin`
- `base` -> `ggml-base.bin`
- `small` -> `ggml-small.bin`
- `medium` -> `ggml-medium.bin`
- `large-v3` -> `ggml-large-v3.bin`
- `turbo` -> `ggml-large-v3-turbo.bin`

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
3. The sidecar starts `whisper-server` for transcription, then `llama-server` (GemmaTranslate-v3) for translation as needed.
4. The sidecar transcribes, optionally translates, and writes the subtitle file next to the input video unless an explicit output path is provided.
