import { describe, expect, it } from "bun:test";
import { reduceSystemInfo } from "./systemInfo";
import type { LanguagesResponse, SystemInfoResponse } from "./types";

describe("reduceSystemInfo", () => {
  it("does not mark translation ready from static language metadata alone", () => {
    const prev = {
      whisperServer: true,
      translationEngine: false,
      gpu: "RTX 4090",
    };

    const next = reduceSystemInfo(prev, {
      type: "languages",
      installed: [{ source: "en", target: "ja" }],
    } satisfies LanguagesResponse);

    expect(next).toEqual(prev);
  });

  it("applies explicit system_info availability", () => {
    const next = reduceSystemInfo(null, {
      type: "system_info",
      whisper_server: true,
      translation_engine: true,
      gpu: "RTX 4090",
    } satisfies SystemInfoResponse);

    expect(next).toEqual({
      whisperServer: true,
      translationEngine: true,
      gpu: "RTX 4090",
    });
  });
});
