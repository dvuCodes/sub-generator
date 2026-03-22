# whisper-server Setup

The Go sidecar expects `whisper-server` from `whisper.cpp`.

## Quick Setup (Windows)

1. Download the latest `whisper.cpp` release:
   https://github.com/ggerganov/whisper.cpp/releases
2. Download at least one GGML model file:
   - `ggml-tiny.bin`
   - `ggml-base.bin`
   - `ggml-small.bin`
   - `ggml-medium.bin`
   - `ggml-large-v3.bin`
   - `ggml-large-v3-turbo.bin`
3. Place the binary and models in this repo layout:

```text
services/whisper-server/
  whisper-server.exe
  models/
    ggml-base.bin
```

4. The Go sidecar will prefer that layout and fall back to `whisper-server` on `PATH`.

## Build from Source (CUDA)

```bash
git clone https://github.com/ggerganov/whisper.cpp
cd whisper.cpp
cmake -B build -DGGML_CUDA=1
cmake --build build --config Release
```

The binary will be at `build/bin/Release/whisper-server.exe`.
