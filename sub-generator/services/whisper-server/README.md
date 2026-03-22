# whisper-server Setup

The Go sidecar expects `whisper-server` (from whisper.cpp) to be available.

## Quick Setup (Windows)

1. Download the latest whisper.cpp release from:
   https://github.com/ggerganov/whisper.cpp/releases

2. Download a GGML model file:
   - tiny: https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin (75 MB)
   - base: https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin (142 MB)
   - small: https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin (466 MB)
   - medium: https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin (1.5 GB)
   - large-v3: https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin (3.1 GB)

3. Place the binary and model in this directory:
   ```
   services/whisper-server/
   ├── whisper-server.exe
   └── models/
       └── ggml-base.bin
   ```

4. The Go sidecar will start it automatically on port 8080.

## Build from Source (for CUDA GPU support)

```bash
git clone https://github.com/ggerganov/whisper.cpp
cd whisper.cpp
cmake -B build -DGGML_CUDA=1
cmake --build build --config Release
```

The binary will be at `build/bin/Release/whisper-server.exe`.
