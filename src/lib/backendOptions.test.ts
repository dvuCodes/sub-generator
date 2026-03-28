import { describe, expect, it } from "bun:test";
import {
  resolveASRModelId,
  resolvePreferredASRBackend,
  resolvePreferredTranslationBackend,
} from "./backendOptions";
import type { CapabilitiesResponse } from "./types";

const capabilities: CapabilitiesResponse = {
  type: "capabilities",
  defaults: {
    asr_backend: "faster_whisper",
    asr_model_id: "deepdml/faster-whisper-large-v3-turbo-ct2",
    translation_backend: "nllb",
    diarization_enabled: false,
  },
  backends: {
    asr: [
      {
        id: "faster_whisper",
        display_name: "Faster Whisper",
        installed: false,
        default_model_id: "deepdml/faster-whisper-large-v3-turbo-ct2",
        source_languages: [],
      },
      {
        id: "whisper_cpp",
        display_name: "whisper.cpp",
        installed: true,
        default_model_id: "turbo",
        source_languages: [],
      },
    ],
    translation: [
      {
        id: "nllb",
        display_name: "NLLB",
        installed: false,
        default_model_id: "JustFrederik/nllb-200-distilled-600M-ct2-int8",
        target_languages: [],
      },
      {
        id: "gemma_context",
        display_name: "Gemma Context",
        installed: true,
        default_model_id: "GemmaTranslate-v3-12B-i1-GGUF",
        target_languages: [],
      },
    ],
  },
};

describe("resolvePreferredASRBackend", () => {
  it("preserves the current backend when capabilities are still loading", () => {
    expect(resolvePreferredASRBackend(null, "faster_whisper")).toBe(
      "faster_whisper"
    );
  });

  it("preserves the current backend when it is known but needs setup", () => {
    expect(resolvePreferredASRBackend(capabilities, "faster_whisper")).toBe(
      "faster_whisper"
    );
  });

  it("falls back when the current backend is missing from capabilities", () => {
    expect(
      resolvePreferredASRBackend(
        {
          ...capabilities,
          backends: {
            ...capabilities.backends,
            asr: [capabilities.backends.asr[1]],
          },
        },
        "faster_whisper"
      )
    ).toBe(
      "whisper_cpp"
    );
  });
});

describe("resolvePreferredTranslationBackend", () => {
  it("preserves the current translation backend when capabilities are still loading", () => {
    expect(resolvePreferredTranslationBackend(null, "nllb")).toBe("nllb");
  });

  it("preserves the current translation backend when it is known but needs setup", () => {
    expect(resolvePreferredTranslationBackend(capabilities, "nllb")).toBe(
      "nllb"
    );
  });

  it("falls back when the current translation backend is missing", () => {
    expect(
      resolvePreferredTranslationBackend(
        {
          ...capabilities,
          backends: {
            ...capabilities.backends,
            translation: [capabilities.backends.translation[1]],
          },
        },
        "nllb"
      )
    ).toBe(
      "gemma_context"
    );
  });

  it("preserves explicit none", () => {
    expect(resolvePreferredTranslationBackend(capabilities, "none")).toBe("none");
  });
});

describe("resolveASRModelId", () => {
  it("uses the whisper model size for whisper.cpp", () => {
    expect(
      resolveASRModelId(capabilities, "whisper_cpp", "large-v3", "ignored")
    ).toBe("large-v3");
  });

  it("uses the backend default model id for faster-whisper", () => {
    expect(
      resolveASRModelId(capabilities, "faster_whisper", "turbo", "")
    ).toBe("deepdml/faster-whisper-large-v3-turbo-ct2");
  });
});
