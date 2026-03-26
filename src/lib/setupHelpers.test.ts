import { describe, expect, it } from "vitest";
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
    expect(shouldDisableGenerate(status, "")).toBe(false);
  });

  it("returns true when transcription service needs action", () => {
    const status = makeStatus([{ state: "action_required", required_for: "transcription" }]);
    expect(shouldDisableGenerate(status, "")).toBe(true);
  });

  it("returns true when translation service needs action and targetLang set", () => {
    const status = makeStatus([{ state: "action_required", required_for: "translation" }]);
    expect(shouldDisableGenerate(status, "en")).toBe(true);
  });

  it("returns false when translation service needs action but no targetLang", () => {
    const status = makeStatus([{ state: "action_required", required_for: "translation" }]);
    expect(shouldDisableGenerate(status, "")).toBe(false);
  });

  it("returns false when setup status is null", () => {
    expect(shouldDisableGenerate(null, "en")).toBe(false);
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
