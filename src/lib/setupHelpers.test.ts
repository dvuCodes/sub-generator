import { describe, expect, it } from "bun:test";
import { shouldDisableGenerate, formatSetupIssue } from "./setupHelpers";
import type { ServiceStatus, SetupStatusResponse } from "./types";

function makeStatus(overrides: Partial<ServiceStatus>[]): SetupStatusResponse {
  return {
    type: "setup_status",
    services: overrides.map((o) => ({
      id: o.id ?? "test",
      display_name: o.display_name ?? "test",
      required_for: o.required_for ?? "transcription",
      state: o.state ?? "ready",
      issues: o.issues ?? [],
      actions: o.actions ?? [],
    })),
  };
}

describe("shouldDisableGenerate", () => {
  it("returns false when all services ready", () => {
    const status = makeStatus([{ state: "ready", required_for: "transcription" }]);
    expect(
      shouldDisableGenerate(status, {
        asrBackend: "faster_whisper",
        translationBackend: "none",
      })
    ).toBe(false);
  });

  it("blocks faster-whisper when ml-backend needs action", () => {
    const status = makeStatus([
      { id: "ml-backend", state: "action_required", required_for: "transcription" },
    ]);
    expect(
      shouldDisableGenerate(status, {
        asrBackend: "faster_whisper",
        translationBackend: "none",
      })
    ).toBe(true);
  });

  it("allows whisper.cpp when only ml-backend needs action", () => {
    const status = makeStatus([
      { id: "ml-backend", state: "action_required", required_for: "transcription" },
    ]);
    expect(
      shouldDisableGenerate(status, {
        asrBackend: "whisper_cpp",
        translationBackend: "none",
      })
    ).toBe(false);
  });

  it("blocks gemma translation when llama needs action", () => {
    const status = makeStatus([
      { id: "llama", state: "action_required", required_for: "translation" },
    ]);
    expect(
      shouldDisableGenerate(status, {
        asrBackend: "whisper_cpp",
        translationBackend: "gemma_context",
      })
    ).toBe(true);
  });

  it("allows generation when translation is off even if translation setup is missing", () => {
    const status = makeStatus([
      { id: "llama", state: "action_required", required_for: "translation" },
    ]);
    expect(
      shouldDisableGenerate(status, {
        asrBackend: "whisper_cpp",
        translationBackend: "none",
      })
    ).toBe(false);
  });

  it("returns false when setup status is null", () => {
    expect(
      shouldDisableGenerate(null, {
        asrBackend: "faster_whisper",
        translationBackend: "none",
      })
    ).toBe(false);
  });
});

describe("formatSetupIssue", () => {
  it("returns observed_error when present", () => {
    expect(formatSetupIssue({ code: "binary_not_runnable", observed_error: "dll missing" }))
      .toContain("dll missing");
  });

  it("returns fallback for binary_not_found", () => {
    expect(formatSetupIssue({ code: "binary_not_found" }))
      .toContain("not found");
  });

  it("returns fallback for not_in_path", () => {
    expect(formatSetupIssue({ code: "not_in_path" }))
      .toContain("PATH");
  });

  it("returns fallback for binary_not_runnable without observed_error", () => {
    expect(formatSetupIssue({ code: "binary_not_runnable" }))
      .toContain("cannot start");
  });
});
