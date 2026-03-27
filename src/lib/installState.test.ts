import { describe, expect, it } from "vitest";
import {
  advanceInstallState,
  createInitialInstallState,
  isInstallComplete,
} from "./installState";
import type { ProgressResponse, StageResponse, SetupStatusResponse } from "./types";

describe("advanceInstallState", () => {
  it("advances through download stage", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const response: ProgressResponse = {
      type: "progress",
      stage: "downloading_dependency",
      percent: 50,
      message: "Downloading...",
    };
    const next = advanceInstallState(state, response);
    expect(next.stage).toBe("downloading_dependency");
    expect(next.percent).toBe(50);
  });

  it("advances to extracting stage", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const response: StageResponse = {
      type: "stage",
      stage: "extracting",
      message: "Extracting...",
    };
    const next = advanceInstallState(state, response);
    expect(next.stage).toBe("extracting");
  });

  it("preserves pending action id", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const response: ProgressResponse = {
      type: "progress",
      stage: "downloading_dependency",
      percent: 10,
      message: "...",
    };
    const next = advanceInstallState(state, response);
    expect(next.pendingActionId).toBe("whisper/install_gpu_bundle");
    expect(next.targetServiceId).toBe("whisper");
  });
});

describe("isInstallComplete", () => {
  it("returns true when target service is ready", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const setupStatus: SetupStatusResponse = {
      type: "setup_status",
      services: [
        { id: "whisper", display_name: "whisper-server", required_for: "transcription", state: "ready", issues: [], actions: [] },
      ],
    };
    expect(isInstallComplete(state, setupStatus)).toBe(true);
  });

  it("returns false when target service still has issues", () => {
    const state = createInitialInstallState("whisper/install_gpu_bundle", "whisper");
    const setupStatus: SetupStatusResponse = {
      type: "setup_status",
      services: [
        { id: "whisper", display_name: "whisper-server", required_for: "transcription", state: "action_required", issues: [{ code: "binary_not_runnable" }], actions: [] },
      ],
    };
    expect(isInstallComplete(state, setupStatus)).toBe(false);
  });

  it("returns false when no pending action", () => {
    const state = createInitialInstallState("", "");
    const setupStatus: SetupStatusResponse = {
      type: "setup_status",
      services: [],
    };
    expect(isInstallComplete(state, setupStatus)).toBe(false);
  });
});
