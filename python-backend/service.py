#!/usr/bin/env python3
from __future__ import annotations

import argparse
import importlib.util
import json
import os
import re
import sys
import threading
import traceback
from dataclasses import dataclass, field
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any


DEFAULT_ASR_MODEL_ID = "deepdml/faster-whisper-large-v3-turbo-ct2"
DEFAULT_TRANSLATION_MODEL_ID = "JustFrederik/nllb-200-distilled-600M-ct2-int8"
DEFAULT_DIARIZATION_MODEL_ID = "pyannote/speaker-diarization-community-1"
GEMMA_TRANSLATION_BACKEND = "gemma_context"
NLLB_FALLBACK_TOKENIZER = "facebook/nllb-200-distilled-600M"

COMMON_LANGUAGES = [
    {"code": "en", "name": "English"},
    {"code": "ja", "name": "Japanese"},
    {"code": "zh", "name": "Chinese"},
    {"code": "ko", "name": "Korean"},
    {"code": "es", "name": "Spanish"},
    {"code": "fr", "name": "French"},
    {"code": "de", "name": "German"},
    {"code": "pt", "name": "Portuguese"},
    {"code": "ru", "name": "Russian"},
    {"code": "ar", "name": "Arabic"},
    {"code": "hi", "name": "Hindi"},
    {"code": "vi", "name": "Vietnamese"},
    {"code": "th", "name": "Thai"},
    {"code": "it", "name": "Italian"},
    {"code": "nl", "name": "Dutch"},
    {"code": "pl", "name": "Polish"},
    {"code": "tr", "name": "Turkish"},
]

NLLB_LANG_MAP = {
    "en": "eng_Latn",
    "ja": "jpn_Jpan",
    "zh": "zho_Hans",
    "ko": "kor_Hang",
    "es": "spa_Latn",
    "fr": "fra_Latn",
    "de": "deu_Latn",
    "pt": "por_Latn",
    "ru": "rus_Cyrl",
    "ar": "arb_Arab",
    "hi": "hin_Deva",
    "vi": "vie_Latn",
    "th": "tha_Thai",
    "it": "ita_Latn",
    "nl": "nld_Latn",
    "pl": "pol_Latn",
    "tr": "tur_Latn",
}

CUDA_DLL_ERROR_MARKERS = (
    "cublas64_",
    "cublaslt64_",
    "cudart64_",
    "cudnn",
    "cuda",
)

CUDA_DLL_FILE_NAMES = (
    "cublas64_12.dll",
    "cublasLt64_12.dll",
    "cudart64_12.dll",
)

WINDOWS_DLL_DIRECTORY_HANDLES: list[Any] = []
REGISTERED_WINDOWS_DLL_DIRECTORIES: set[str] = set()


class RequestError(ValueError):
    pass


class BackendUnavailableError(RuntimeError):
    pass


@dataclass
class TranslationRuntime:
    engine: str
    translator: Any
    tokenizer: Any


@dataclass
class LoadedModels:
    whisper: dict[str, Any] = field(default_factory=dict)
    translators: dict[str, TranslationRuntime] = field(default_factory=dict)
    diarization: dict[str, Any] = field(default_factory=dict)


def detect_device() -> str:
    if torch_cuda_available() or ctranslate2_cuda_available():
        return "cuda"
    return "cpu"


def torch_cuda_available() -> bool:
    try:
        import torch  # type: ignore

        return bool(torch.cuda.is_available())
    except Exception:
        return False


def ctranslate2_cuda_available() -> bool:
    try:
        import ctranslate2  # type: ignore

        return bool(ctranslate2.get_supported_compute_types("cuda"))
    except Exception:
        return False


def module_available(name: str) -> bool:
    return importlib.util.find_spec(name) is not None


def is_cuda_library_load_error(exc: BaseException) -> bool:
    message = str(exc).lower()
    if "dll" not in message:
        return False
    if "cannot be loaded" not in message and "not found" not in message:
        return False
    return any(marker in message for marker in CUDA_DLL_ERROR_MARKERS)


def candidate_cuda_runtime_dirs() -> list[Path]:
    script_dir = Path(__file__).resolve().parent
    configured = os.environ.get("SUBGEN_CUDA_DLL_DIR")
    candidates = [
        Path(configured).expanduser() if configured else None,
        script_dir / "llama-server",
        script_dir.parent / "llama-server",
        script_dir / "services" / "llama-server",
        script_dir.parent / "services" / "llama-server",
        script_dir.parent.parent / "services" / "llama-server",
    ]

    resolved: list[Path] = []
    seen: set[str] = set()
    for candidate in candidates:
        if candidate is None:
            continue
        try:
            path = candidate.resolve()
        except FileNotFoundError:
            continue
        path_key = str(path)
        if path_key in seen or not path.is_dir():
            continue
        if not any((path / dll_name).exists() for dll_name in CUDA_DLL_FILE_NAMES):
            continue
        seen.add(path_key)
        resolved.append(path)
    return resolved


def configure_windows_cuda_runtime_dirs() -> None:
    if os.name != "nt":
        return

    path_entries = os.environ.get("PATH", "").split(os.pathsep)
    for directory in candidate_cuda_runtime_dirs():
        directory_str = str(directory)
        if directory_str not in path_entries:
            path_entries.insert(0, directory_str)
        if directory_str in REGISTERED_WINDOWS_DLL_DIRECTORIES:
            continue
        add_dll_directory = getattr(os, "add_dll_directory", None)
        if add_dll_directory is None:
            continue
        WINDOWS_DLL_DIRECTORY_HANDLES.append(add_dll_directory(directory_str))
        REGISTERED_WINDOWS_DLL_DIRECTORIES.add(directory_str)

    os.environ["PATH"] = os.pathsep.join(path_entries)


def faster_whisper_available() -> bool:
    return module_available("faster_whisper")


def nllb_available() -> bool:
    return module_available("transformers") and (
        module_available("ctranslate2") or module_available("torch")
    )


def build_capabilities() -> dict[str, Any]:
    asr_available = faster_whisper_available()
    translation_available = nllb_available()

    return {
        "type": "capabilities",
        "defaults": {
            "asr_backend": "faster_whisper",
            "asr_model_id": DEFAULT_ASR_MODEL_ID,
            "translation_backend": "nllb",
            "diarization_enabled": False,
        },
        "backends": {
            "asr": [
                {
                    "id": "faster_whisper",
                    "display_name": "Faster Whisper",
                    "installed": asr_available,
                    "default_model_id": DEFAULT_ASR_MODEL_ID,
                    "source_languages": [{"code": "auto", "name": "Auto-detect"}, *COMMON_LANGUAGES],
                },
                {
                    "id": "whisper_cpp",
                    "display_name": "whisper.cpp",
                    "installed": False,
                    "default_model_id": "turbo",
                    "source_languages": [{"code": "auto", "name": "Auto-detect"}, *COMMON_LANGUAGES],
                },
            ],
            "translation": [
                {
                    "id": "nllb",
                    "display_name": "NLLB",
                    "installed": translation_available,
                    "default_model_id": DEFAULT_TRANSLATION_MODEL_ID,
                    "target_languages": COMMON_LANGUAGES,
                },
                {
                    "id": GEMMA_TRANSLATION_BACKEND,
                    "display_name": "Gemma Context",
                    "installed": False,
                    "default_model_id": "",
                    "target_languages": COMMON_LANGUAGES,
                },
            ],
        },
    }


def model_cache_dir() -> Path:
    root = os.environ.get("SUBGEN_ML_CACHE")
    if root:
        return Path(root).expanduser().resolve()
    return Path(__file__).resolve().parent / "models"


def repo_cache_dir(cache_root: Path, model_id: str) -> Path:
    safe_name = re.sub(r"[^A-Za-z0-9._-]+", "--", model_id).strip("-") or "model"
    return cache_root / safe_name


def resolve_model_reference(model_id: str, cache_root: Path) -> str:
    candidate = Path(model_id)
    if candidate.exists():
        return str(candidate)

    if "/" not in model_id:
        return model_id

    local_dir = repo_cache_dir(cache_root, model_id)
    local_dir.mkdir(parents=True, exist_ok=True)

    try:
        from huggingface_hub import snapshot_download  # type: ignore
    except Exception:
        return model_id

    return snapshot_download(
        repo_id=model_id,
        local_dir=str(local_dir),
        local_dir_use_symlinks=False,
        token=os.environ.get("HF_TOKEN"),
    )


def require_object(payload: Any) -> dict[str, Any]:
    if not isinstance(payload, dict):
        raise RequestError("request body must be a JSON object")
    return payload


def require_path(payload: dict[str, Any], key: str) -> Path:
    value = payload.get(key)
    if not isinstance(value, str) or not value.strip():
        raise RequestError(f"{key} is required")

    path = Path(value)
    if not path.exists():
        raise RequestError(f"{key} does not exist: {value}")
    if not path.is_file():
        raise RequestError(f"{key} must be a file: {value}")
    return path


def require_language(payload: dict[str, Any], key: str) -> str:
    value = payload.get(key)
    if not isinstance(value, str) or not value.strip():
        raise RequestError(f"{key} is required")
    return value.strip()


def normalize_segments(raw_segments: Any) -> list[dict[str, Any]]:
    if not isinstance(raw_segments, list):
        raise RequestError("segments must be an array")

    normalized: list[dict[str, Any]] = []
    for index, segment in enumerate(raw_segments):
        if not isinstance(segment, dict):
            raise RequestError(f"segments[{index}] must be an object")
        try:
            start = float(segment["start"])
            end = float(segment["end"])
        except (KeyError, TypeError, ValueError) as exc:
            raise RequestError(f"segments[{index}] must include numeric start/end values") from exc

        if end < start:
            raise RequestError(f"segments[{index}] end must be greater than or equal to start")

        text = segment.get("text", "")
        if text is None:
            text = ""
        if not isinstance(text, str):
            raise RequestError(f"segments[{index}].text must be a string")

        normalized_segment = dict(segment)
        normalized_segment["start"] = start
        normalized_segment["end"] = end
        normalized_segment["text"] = text
        normalized.append(normalized_segment)

    return normalized


def resolve_nllb_language(code: str, field_name: str) -> str:
    normalized = code.strip()
    if normalized == "auto":
        raise RequestError(f"{field_name} must be a concrete language code for NLLB")
    return NLLB_LANG_MAP.get(normalized, normalized)


def dominant_speaker(diarization: Any, start: float, end: float) -> str | None:
    best_label = None
    best_overlap = 0.0
    for turn, _, speaker in diarization.itertracks(yield_label=True):
        overlap = min(end, float(turn.end)) - max(start, float(turn.start))
        if overlap > best_overlap:
            best_overlap = overlap
            best_label = speaker
    return best_label


def translate_text(runtime: TranslationRuntime, text: str, source_lang: str, target_lang: str) -> str:
    if runtime.engine == "ctranslate2":
        tokenizer = runtime.tokenizer
        tokenizer.src_lang = source_lang
        token_ids = tokenizer.encode(text)
        source_tokens = tokenizer.convert_ids_to_tokens(token_ids)
        results = runtime.translator.translate_batch([source_tokens], target_prefix=[[target_lang]])
        target_tokens = [token for token in results[0].hypotheses[0] if token != target_lang]
        target_ids = tokenizer.convert_tokens_to_ids(target_tokens)
        return tokenizer.decode(target_ids, skip_special_tokens=True).strip()

    tokenizer = runtime.tokenizer
    tokenizer.src_lang = source_lang
    encoded = tokenizer(text, return_tensors="pt")
    forced_bos_token_id = tokenizer.convert_tokens_to_ids(target_lang)
    generated = runtime.translator.generate(
        **encoded,
        forced_bos_token_id=forced_bos_token_id,
        max_new_tokens=256,
    )
    return tokenizer.batch_decode(generated, skip_special_tokens=True)[0].strip()


@dataclass
class App:
    cache_dir: Path = field(default_factory=model_cache_dir)
    loaded: LoadedModels = field(default_factory=LoadedModels)

    def __post_init__(self) -> None:
        self.cache_dir.mkdir(parents=True, exist_ok=True)
        self._lock = threading.Lock()

    def health(self) -> dict[str, Any]:
        return {"status": "ok"}

    def capabilities(self) -> dict[str, Any]:
        return build_capabilities()

    def transcribe(self, payload: Any) -> dict[str, Any]:
        request = require_object(payload)
        input_path = require_path(request, "input_video")
        model_id = request.get("model_id") or DEFAULT_ASR_MODEL_ID
        language = request.get("source_lang")
        if language == "auto":
            language = None

        try:
            return self._transcribe_with_model(model_id, input_path, request, language)
        except Exception as exc:
            if not is_cuda_library_load_error(exc):
                raise
            print(
                "[ml-backend] Faster Whisper CUDA runtime is unavailable during transcription; retrying on CPU.",
                file=sys.stderr,
            )
            with self._lock:
                self.loaded.whisper.pop(model_id, None)
            return self._transcribe_with_model(
                model_id,
                input_path,
                request,
                language,
                force_device="cpu",
            )

    def translate_segments(self, payload: Any) -> dict[str, Any]:
        request = require_object(payload)
        source_lang = resolve_nllb_language(require_language(request, "source_lang"), "source_lang")
        target_lang = resolve_nllb_language(require_language(request, "target_lang"), "target_lang")
        segments = normalize_segments(request.get("segments"))
        runtime = self._load_nllb(request.get("model_id") or DEFAULT_TRANSLATION_MODEL_ID)

        translated_segments = []
        for segment in segments:
            text = (segment.get("text") or "").strip()
            if not text:
                translated_segments.append(segment)
                continue

            translated_segment = dict(segment)
            translated_segment["text"] = translate_text(runtime, text, source_lang, target_lang)
            translated_segments.append(translated_segment)

        return {"segments": translated_segments}

    def annotate_diarization(self, payload: Any) -> dict[str, Any]:
        request = require_object(payload)
        audio_path = require_path(request, "audio_path")
        segments = normalize_segments(request.get("segments"))
        diarization = self._load_diarization()(str(audio_path))
        speaker_map: dict[str, str] = {}

        for segment in segments:
            speaker = dominant_speaker(diarization, segment["start"], segment["end"])
            if not speaker:
                continue
            speaker_map.setdefault(speaker, f"Speaker {len(speaker_map) + 1}")
            segment["speaker_id"] = speaker
            segment["speaker_label"] = speaker_map[speaker]

        return {"segments": segments, "speaker_count": len(speaker_map)}

    def _transcribe_with_model(
        self,
        model_id: str,
        input_path: Path,
        request: dict[str, Any],
        language: str | None,
        force_device: str | None = None,
    ) -> dict[str, Any]:
        if force_device is None:
            model = self._load_whisper_model(model_id)
        else:
            model = self._load_whisper_model(model_id, force_device=force_device)
        segments, info = model.transcribe(
            str(input_path),
            language=language,
            beam_size=int(request.get("beam_size") or 5),
            vad_filter=bool(request.get("vad_filter", True)),
            word_timestamps=False,
        )
        normalized_segments = []
        texts = []
        for segment in segments:
            text = (getattr(segment, "text", "") or "").strip()
            normalized_segments.append(
                {
                    "start": float(getattr(segment, "start")),
                    "end": float(getattr(segment, "end")),
                    "text": text,
                }
            )
            if text:
                texts.append(text)

        return {
            "text": " ".join(texts).strip(),
            "segments": normalized_segments,
            "language": getattr(info, "language", language or "") or "",
        }

    def _load_whisper_model(self, model_id: str, force_device: str | None = None) -> Any:
        with self._lock:
            existing = self.loaded.whisper.get(model_id)
            if existing is not None:
                return existing

            try:
                from faster_whisper import WhisperModel  # type: ignore
            except Exception as exc:
                raise BackendUnavailableError(f"faster-whisper is not available: {exc}") from exc

            model_ref = resolve_model_reference(model_id, self.cache_dir / "whisper")
            configure_windows_cuda_runtime_dirs()
            device = force_device or detect_device()
            compute_type = "float16" if device == "cuda" else "int8"
            try:
                model = WhisperModel(model_ref, device=device, compute_type=compute_type)
            except Exception as exc:
                if device != "cuda" or not is_cuda_library_load_error(exc):
                    raise
                print(
                    "[ml-backend] Faster Whisper CUDA runtime is unavailable during model load; falling back to CPU.",
                    file=sys.stderr,
                )
                model = WhisperModel(model_ref, device="cpu", compute_type="int8")
            self.loaded.whisper[model_id] = model
            return model

    def _load_nllb(self, model_id: str) -> TranslationRuntime:
        with self._lock:
            existing = self.loaded.translators.get(model_id)
            if existing is not None:
                return existing

            model_ref = resolve_model_reference(model_id, self.cache_dir / "translation")
            token = os.environ.get("HF_TOKEN")

            try:
                import ctranslate2  # type: ignore
                from transformers import AutoTokenizer  # type: ignore

                tokenizer = self._load_tokenizer(AutoTokenizer, model_ref, token)
                runtime = TranslationRuntime(
                    engine="ctranslate2",
                    translator=ctranslate2.Translator(
                        model_ref,
                        device="cuda" if detect_device() == "cuda" else "cpu",
                    ),
                    tokenizer=tokenizer,
                )
                self.loaded.translators[model_id] = runtime
                return runtime
            except Exception:
                pass

            try:
                from transformers import AutoModelForSeq2SeqLM, AutoTokenizer  # type: ignore
            except Exception as exc:
                raise BackendUnavailableError(f"NLLB dependencies are not available: {exc}") from exc

            tokenizer = self._load_tokenizer(AutoTokenizer, model_ref, token)
            runtime = TranslationRuntime(
                engine="transformers",
                translator=AutoModelForSeq2SeqLM.from_pretrained(model_ref, token=token),
                tokenizer=tokenizer,
            )
            self.loaded.translators[model_id] = runtime
            return runtime

    def _load_tokenizer(self, tokenizer_cls: Any, model_ref: str, token: str | None) -> Any:
        try:
            return tokenizer_cls.from_pretrained(model_ref, token=token)
        except Exception:
            return tokenizer_cls.from_pretrained(NLLB_FALLBACK_TOKENIZER, token=token)

    def _load_diarization(self) -> Any:
        with self._lock:
            existing = self.loaded.diarization.get(DEFAULT_DIARIZATION_MODEL_ID)
            if existing is not None:
                return existing

            try:
                from pyannote.audio import Pipeline  # type: ignore
            except Exception as exc:
                raise BackendUnavailableError(f"pyannote.audio is not available: {exc}") from exc

            token = os.environ.get("HF_TOKEN")
            diarization = Pipeline.from_pretrained(DEFAULT_DIARIZATION_MODEL_ID, use_auth_token=token)
            if detect_device() == "cuda":
                try:
                    import torch  # type: ignore

                    diarization.to(torch.device("cuda"))
                except Exception:
                    pass

            self.loaded.diarization[DEFAULT_DIARIZATION_MODEL_ID] = diarization
            return diarization


def make_handler(app: App) -> type[BaseHTTPRequestHandler]:
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            if self.path == "/health":
                self.respond(app.health())
                return
            if self.path == "/capabilities":
                self.respond(app.capabilities())
                return
            self.respond({"error": "not found"}, status=HTTPStatus.NOT_FOUND)

        def do_POST(self) -> None:
            try:
                payload = self._read_json_payload()

                if self.path == "/asr/transcribe":
                    self.respond(app.transcribe(payload))
                    return
                if self.path == "/translation/translate_segments":
                    self.respond(app.translate_segments(payload))
                    return
                if self.path == "/diarization/annotate":
                    self.respond(app.annotate_diarization(payload))
                    return
                self.respond({"error": "not found"}, status=HTTPStatus.NOT_FOUND)
            except RequestError as exc:
                self.respond({"error": str(exc)}, status=HTTPStatus.BAD_REQUEST)
            except BackendUnavailableError as exc:
                self.respond({"error": str(exc)}, status=HTTPStatus.SERVICE_UNAVAILABLE)
            except Exception as exc:
                traceback.print_exc()
                self.respond({"error": f"internal backend error: {exc}"}, status=HTTPStatus.INTERNAL_SERVER_ERROR)

        def _read_json_payload(self) -> dict[str, Any]:
            try:
                length = int(self.headers.get("Content-Length", "0"))
            except ValueError as exc:
                raise RequestError(f"invalid Content-Length: {exc}") from exc

            raw = self.rfile.read(length)
            try:
                return require_object(json.loads(raw or b"{}"))
            except json.JSONDecodeError as exc:
                raise RequestError(f"invalid JSON body: {exc}") from exc

        def log_message(self, fmt: str, *args: Any) -> None:
            sys.stderr.write("[ml-backend] " + (fmt % args) + "\n")

        def respond(self, payload: dict[str, Any], status: HTTPStatus = HTTPStatus.OK) -> None:
            body = json.dumps(payload).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    return Handler


def create_server(host: str, port: int, app: App | None = None) -> ThreadingHTTPServer:
    return ThreadingHTTPServer((host, port), make_handler(app or App()))


def serve(host: str = "127.0.0.1", port: int = 8082, app: App | None = None) -> None:
    server = create_server(host, port, app=app)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8082)
    args = parser.parse_args()
    serve(host=args.host, port=args.port)


if __name__ == "__main__":
    main()
