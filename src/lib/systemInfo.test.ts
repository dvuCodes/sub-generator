import { describe, expect, it } from "bun:test";
import { reduceSystemInfo } from "./systemInfo";
import type { SystemInfoResponse } from "./types";

describe("reduceSystemInfo", () => {
  it("applies explicit system_info availability", () => {
    const next = reduceSystemInfo(null, {
      type: "system_info",
      whisper_server: true,
      translation_engine: true,
      ml_backend: true,
      gpu: "RTX 4090",
    } satisfies SystemInfoResponse);

    expect(next).toEqual({
      whisperServer: true,
      translationEngine: true,
      mlBackend: true,
      gpu: "RTX 4090",
    });
  });
});
