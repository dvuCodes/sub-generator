# SubGen ML Backend

This directory contains the Python HTTP service used by the Go sidecar for:

- `faster-whisper` ASR
- `NLLB` subtitle translation
- `pyannote` speaker diarization

The packaged app stages this tree into `services/ml-backend/`.

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
