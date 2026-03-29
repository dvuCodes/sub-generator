# SubGen ML Backend

This directory contains the canonical Python HTTP service used by the Go sidecar for:

- `faster-whisper` ASR
- `NLLB` subtitle translation
- `pyannote` speaker diarization

SubGen uses this backend as the default runtime path during development. Packaged builds may stage a copy of these files under `services/ml-backend/`, but this directory is the source of truth.

## Setup

Install the backend dependencies into the Python runtime that SubGen will launch:

```bash
python -m pip install -r python-backend/requirements.txt
```

Optional environment variables:

- `HF_TOKEN` for Hugging Face-authenticated model downloads when required
- `SUBGEN_ML_CACHE` to override the default repo-local cache root

Default model IDs:

- ASR: `deepdml/faster-whisper-large-v3-turbo-ct2`
- Translation: `JustFrederik/nllb-200-distilled-600M-ct2-int8`
- Diarization: `pyannote/speaker-diarization-community-1`

Entrypoints:

- `service.py` is the canonical HTTP server used in dev and when the Go sidecar
  launches Python directly.
- `run.bat` and `run.sh` are lightweight launchers for a bundled Python runtime.

Endpoint contract:

- `GET /health`
- `GET /capabilities`
- `POST /asr/transcribe`
- `POST /translation/translate_segments`
- `POST /diarization/annotate`
