import type { ProgressResponse, StageResponse, SetupStatusResponse } from "./types";

export interface InstallState {
  stage: string;
  percent: number | null;
  message: string;
  pendingActionId: string;
  targetServiceId: string;
}

export function createInitialInstallState(
  actionId: string,
  serviceId: string
): InstallState {
  return {
    stage: "",
    percent: null,
    message: "Starting installation...",
    pendingActionId: actionId,
    targetServiceId: serviceId,
  };
}

export function advanceInstallState(
  current: InstallState,
  response: ProgressResponse | StageResponse
): InstallState {
  if (response.type === "progress") {
    return {
      ...current,
      stage: response.stage,
      percent: Math.max(0, Math.min(response.percent, 100)),
      message: response.message,
    };
  }

  return {
    ...current,
    stage: response.stage,
    percent: null,
    message: response.message,
  };
}

export function isInstallComplete(
  state: InstallState,
  setupStatus: SetupStatusResponse
): boolean {
  if (!state.pendingActionId || !state.targetServiceId) return false;

  const service = setupStatus.services.find(
    (s) => s.id === state.targetServiceId
  );
  return service?.state === "ready";
}
