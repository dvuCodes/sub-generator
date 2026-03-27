import { describe, expect, it } from "bun:test";
import type { CapabilitiesResponse } from "./types";
import { buildLanguageOptions } from "./languageOptions";

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
        installed: true,
        default_model_id: "deepdml/faster-whisper-large-v3-turbo-ct2",
        source_languages: [
          { code: "auto", name: "Auto-detect" },
          { code: "en", name: "English" },
          { code: "ja", name: "Japanese" },
        ],
      },
      {
        id: "whisper_cpp",
        display_name: "whisper.cpp",
        installed: true,
        default_model_id: "turbo",
        source_languages: [{ code: "en", name: "English" }],
      },
    ],
    translation: [
      {
        id: "nllb",
        display_name: "NLLB",
        installed: true,
        default_model_id: "JustFrederik/nllb-200-distilled-600M-ct2-int8",
        target_languages: [
          { code: "en", name: "English" },
          { code: "fr", name: "French" },
        ],
      },
      {
        id: "gemma_context",
        display_name: "Gemma Context",
        installed: true,
        default_model_id: "GemmaTranslate-v3-12B-i1-GGUF",
        target_languages: [{ code: "ja", name: "Japanese" }],
      },
    ],
  },
};

describe("buildLanguageOptions", () => {
  it("uses the selected backend language coverage when available", () => {
    const opts = buildLanguageOptions(capabilities, "faster_whisper", "nllb");

    expect(opts.source).toEqual([
      { code: "auto", name: "Auto-detect" },
      { code: "en", name: "English" },
      { code: "ja", name: "Japanese" },
    ]);
    expect(opts.target).toEqual([
      { code: "en", name: "English" },
      { code: "fr", name: "French" },
    ]);
  });

  it("falls back to the built-in source list when backend discovery is missing", () => {
    const opts = buildLanguageOptions(capabilities, "missing_backend", "nllb");

    expect(opts.source[0]).toEqual({ code: "auto", name: "Auto-detect" });
    expect(opts.source.length).toBeGreaterThan(55);
    expect(opts.target).toEqual([
      { code: "en", name: "English" },
      { code: "fr", name: "French" },
    ]);
  });

  it("falls back to the built-in target list when translation coverage is missing", () => {
    const opts = buildLanguageOptions(capabilities, "faster_whisper", "missing_backend");

    expect(opts.target.length).toBeGreaterThan(54);
    expect(opts.target.find((lang) => lang.code === "auto")).toBeUndefined();
    expect(opts.target.find((lang) => lang.code === "ko")).toEqual({
      code: "ko",
      name: "Korean",
    });
  });
});
