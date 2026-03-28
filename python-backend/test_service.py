import json
import sys
import threading
import unittest
from contextlib import contextmanager
from http.client import HTTPConnection
from pathlib import Path
from tempfile import TemporaryDirectory
from types import ModuleType
from types import SimpleNamespace

sys.path.insert(0, str(Path(__file__).resolve().parent))

import service


class ServiceTests(unittest.TestCase):
    def test_build_capabilities_defaults(self):
        capabilities = service.build_capabilities()
        self.assertEqual(capabilities["type"], "capabilities")
        self.assertEqual(capabilities["defaults"]["asr_backend"], "faster_whisper")
        self.assertEqual(capabilities["defaults"]["translation_backend"], "nllb")

    def test_build_capabilities_marks_missing_optional_backends_unavailable(self):
        original_importlib = getattr(service, "importlib", None)
        service.importlib = SimpleNamespace(
            util=SimpleNamespace(
                find_spec=lambda name: None
                if name in {"faster_whisper", "ctranslate2", "transformers", "torch"}
                else object()
            )
        )
        try:
            capabilities = service.build_capabilities()
        finally:
            if original_importlib is None:
                delattr(service, "importlib")
            else:
                service.importlib = original_importlib

        asr = {backend["id"]: backend["installed"] for backend in capabilities["backends"]["asr"]}
        translation = {
            backend["id"]: backend["installed"]
            for backend in capabilities["backends"]["translation"]
        }

        self.assertFalse(asr["faster_whisper"])
        self.assertFalse(translation["nllb"])

    def test_dominant_speaker_prefers_largest_overlap(self):
        class Turn:
            def __init__(self, start, end):
                self.start = start
                self.end = end

        class FakeDiarization:
            def itertracks(self, yield_label=False):
                yield Turn(0.0, 0.4), None, "spk_1"
                yield Turn(0.3, 1.0), None, "spk_2"

        speaker = service.dominant_speaker(FakeDiarization(), 0.2, 0.8)
        self.assertEqual(speaker, "spk_2")

    def test_translate_segments_requires_concrete_source_language(self):
        with TemporaryDirectory() as tmp:
            app = service.App(cache_dir=Path(tmp))
            with self.assertRaises(service.RequestError):
                app.translate_segments(
                    {
                        "source_lang": "auto",
                        "target_lang": "en",
                        "segments": [],
                    }
                )

    def test_detect_device_uses_ctranslate2_cuda_when_torch_is_cpu_only(self):
        original_torch_cuda_available = service.torch_cuda_available
        original_ctranslate2_cuda_available = service.ctranslate2_cuda_available
        service.torch_cuda_available = lambda: False
        service.ctranslate2_cuda_available = lambda: True
        try:
            self.assertEqual(service.detect_device(), "cuda")
        finally:
            service.torch_cuda_available = original_torch_cuda_available
            service.ctranslate2_cuda_available = original_ctranslate2_cuda_available

    def test_is_cuda_library_load_error_matches_cublas_failure(self):
        self.assertTrue(
            service.is_cuda_library_load_error(
                RuntimeError("Library cublas64_12.dll is not found or cannot be loaded")
            )
        )

    def test_load_whisper_model_falls_back_to_cpu_when_cuda_init_fails(self):
        calls = []

        class FakeWhisperModel:
            def __init__(self, model_ref, device, compute_type):
                calls.append((model_ref, device, compute_type))
                if device == "cuda":
                    raise RuntimeError("Library cublas64_12.dll is not found or cannot be loaded")

        fake_module = ModuleType("faster_whisper")
        fake_module.WhisperModel = FakeWhisperModel

        original_module = sys.modules.get("faster_whisper")
        original_detect_device = service.detect_device
        original_resolve_model_reference = service.resolve_model_reference
        sys.modules["faster_whisper"] = fake_module
        service.detect_device = lambda: "cuda"
        service.resolve_model_reference = lambda model_id, cache_root: f"resolved::{model_id}"
        try:
            with TemporaryDirectory() as tmp:
                app = service.App(cache_dir=Path(tmp))
                model = app._load_whisper_model("demo-model")
        finally:
            if original_module is None:
                del sys.modules["faster_whisper"]
            else:
                sys.modules["faster_whisper"] = original_module
            service.detect_device = original_detect_device
            service.resolve_model_reference = original_resolve_model_reference

        self.assertIsInstance(model, FakeWhisperModel)
        self.assertEqual(
            calls,
            [
                ("resolved::demo-model", "cuda", "float16"),
                ("resolved::demo-model", "cpu", "int8"),
            ],
        )

    def test_transcribe_retries_on_cpu_when_cuda_runtime_missing(self):
        calls = []

        class FakeSegment:
            def __init__(self, start, end, text):
                self.start = start
                self.end = end
                self.text = text

        class FakeWhisperModel:
            def __init__(self, model_ref, device, compute_type):
                calls.append((model_ref, device, compute_type))
                self.device = device

            def transcribe(self, path, **kwargs):
                info = SimpleNamespace(language="ja")
                if self.device == "cuda":
                    def failing_segments():
                        raise RuntimeError(
                            "Library cublas64_12.dll is not found or cannot be loaded"
                        )
                        yield

                    return failing_segments(), info

                return [FakeSegment(0.0, 1.0, "hello")], info

        fake_module = ModuleType("faster_whisper")
        fake_module.WhisperModel = FakeWhisperModel

        original_module = sys.modules.get("faster_whisper")
        original_detect_device = service.detect_device
        original_resolve_model_reference = service.resolve_model_reference
        sys.modules["faster_whisper"] = fake_module
        service.detect_device = lambda: "cuda"
        service.resolve_model_reference = lambda model_id, cache_root: f"resolved::{model_id}"
        try:
            with TemporaryDirectory() as tmp:
                input_path = Path(tmp) / "clip.wav"
                input_path.write_bytes(b"audio")
                app = service.App(cache_dir=Path(tmp))
                payload = app.transcribe(
                    {
                        "input_video": str(input_path),
                        "source_lang": "ja",
                        "model_id": "demo-model",
                    }
                )
        finally:
            if original_module is None:
                del sys.modules["faster_whisper"]
            else:
                sys.modules["faster_whisper"] = original_module
            service.detect_device = original_detect_device
            service.resolve_model_reference = original_resolve_model_reference

        self.assertEqual(payload["text"], "hello")
        self.assertEqual(payload["segments"][0]["text"], "hello")
        self.assertEqual(
            calls,
            [
                ("resolved::demo-model", "cuda", "float16"),
                ("resolved::demo-model", "cpu", "int8"),
            ],
        )


class EndpointTests(unittest.TestCase):
    @contextmanager
    def running_server(self, app):
        server = service.create_server("127.0.0.1", 0, app=app)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            yield server.server_address[1]
        finally:
            server.shutdown()
            thread.join(timeout=2)
            server.server_close()

    def get_json(self, port, path):
        conn = HTTPConnection("127.0.0.1", port)
        conn.request("GET", path)
        resp = conn.getresponse()
        payload = json.loads(resp.read())
        conn.close()
        return resp.status, payload

    def post_json(self, port, path, payload):
        conn = HTTPConnection("127.0.0.1", port)
        body = json.dumps(payload)
        conn.request("POST", path, body=body, headers={"Content-Type": "application/json"})
        resp = conn.getresponse()
        parsed = json.loads(resp.read())
        conn.close()
        return resp.status, parsed

    def test_health_endpoint(self):
        with TemporaryDirectory() as tmp:
            app = service.App(cache_dir=Path(tmp))
            with self.running_server(app) as port:
                status, payload = self.get_json(port, "/health")

        self.assertEqual(status, 200)
        self.assertEqual(payload["status"], "ok")

    def test_capabilities_endpoint(self):
        with TemporaryDirectory() as tmp:
            app = service.App(cache_dir=Path(tmp))
            with self.running_server(app) as port:
                status, payload = self.get_json(port, "/capabilities")

        self.assertEqual(status, 200)
        self.assertEqual(payload["defaults"]["asr_model_id"], service.DEFAULT_ASR_MODEL_ID)
        self.assertTrue(payload["backends"]["translation"])

    def test_transcribe_endpoint_uses_whisper_model(self):
        class FakeSegment:
            def __init__(self, start, end, text):
                self.start = start
                self.end = end
                self.text = text

        class FakeWhisperModel:
            def transcribe(self, path, **kwargs):
                self.path = path
                self.kwargs = kwargs
                return [
                    FakeSegment(0.0, 1.2, " Hello "),
                    FakeSegment(1.2, 2.4, "world"),
                ], SimpleNamespace(language="ja")

        with TemporaryDirectory() as tmp:
            input_path = Path(tmp) / "clip.wav"
            input_path.write_bytes(b"audio")

            app = service.App(cache_dir=Path(tmp))
            app._load_whisper_model = lambda model_id: FakeWhisperModel()

            with self.running_server(app) as port:
                status, payload = self.post_json(
                    port,
                    "/asr/transcribe",
                    {
                        "input_video": str(input_path),
                        "source_lang": "ja",
                        "model_id": service.DEFAULT_ASR_MODEL_ID,
                        "beam_size": 3,
                        "vad_filter": True,
                    },
                )

        self.assertEqual(status, 200)
        self.assertEqual(payload["language"], "ja")
        self.assertEqual(payload["text"], "Hello world")
        self.assertEqual(len(payload["segments"]), 2)

    def test_translate_segments_endpoint_preserves_shape(self):
        with TemporaryDirectory() as tmp:
            app = service.App(cache_dir=Path(tmp))
            app._load_nllb = lambda model_id: service.TranslationRuntime("fake", None, None)
            original_translate_text = service.translate_text
            service.translate_text = lambda runtime, text, source_lang, target_lang: f"{text} [{target_lang}]"
            try:
                with self.running_server(app) as port:
                    status, payload = self.post_json(
                        port,
                        "/translation/translate_segments",
                        {
                            "source_lang": "ja",
                            "target_lang": "en",
                            "segments": [
                                {"start": 0.0, "end": 1.0, "text": "konnichiwa"},
                                {"start": 1.0, "end": 2.0, "text": ""},
                            ],
                        },
                    )
            finally:
                service.translate_text = original_translate_text

        self.assertEqual(status, 200)
        self.assertEqual(payload["segments"][0]["text"], "konnichiwa [eng_Latn]")
        self.assertEqual(payload["segments"][1]["text"], "")

    def test_diarization_endpoint_assigns_speakers(self):
        class Turn:
            def __init__(self, start, end):
                self.start = start
                self.end = end

        class FakeDiarizationResult:
            def itertracks(self, yield_label=False):
                yield Turn(0.0, 1.0), None, "spk_a"
                yield Turn(1.0, 2.5), None, "spk_b"

        class FakeDiarizer:
            def __call__(self, path):
                return FakeDiarizationResult()

        with TemporaryDirectory() as tmp:
            audio_path = Path(tmp) / "audio.wav"
            audio_path.write_bytes(b"audio")

            app = service.App(cache_dir=Path(tmp))
            app._load_diarization = lambda: FakeDiarizer()

            with self.running_server(app) as port:
                status, payload = self.post_json(
                    port,
                    "/diarization/annotate",
                    {
                        "audio_path": str(audio_path),
                        "segments": [
                            {"start": 0.0, "end": 0.8, "text": "first"},
                            {"start": 1.2, "end": 2.0, "text": "second"},
                        ],
                    },
                )

        self.assertEqual(status, 200)
        self.assertEqual(payload["speaker_count"], 2)
        self.assertEqual(payload["segments"][0]["speaker_label"], "Speaker 1")
        self.assertEqual(payload["segments"][1]["speaker_label"], "Speaker 2")

    def test_missing_input_returns_bad_request(self):
        with TemporaryDirectory() as tmp:
            app = service.App(cache_dir=Path(tmp))
            with self.running_server(app) as port:
                status, payload = self.post_json(
                    port,
                    "/asr/transcribe",
                    {"input_video": str(Path(tmp) / "missing.wav")},
                )

        self.assertEqual(status, 400)
        self.assertIn("does not exist", payload["error"])


if __name__ == "__main__":
    unittest.main()
