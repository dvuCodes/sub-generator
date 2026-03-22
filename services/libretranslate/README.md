# LibreTranslate Setup

The Go sidecar expects LibreTranslate to be available for translation.

## Option A: pip install (Recommended)

```bash
pip install libretranslate
```

The Go sidecar will start it automatically on port 5000.

## Option B: Docker

```bash
docker run -d -p 5000:5000 libretranslate/libretranslate
```

For GPU-accelerated translation:
```bash
docker run -d -p 5000:5000 --gpus all libretranslate/libretranslate
```

## Pre-download Language Models

To pre-download specific language pairs:
```bash
libretranslate --load-only en,ja
```

This will download English and Japanese models on first run.
