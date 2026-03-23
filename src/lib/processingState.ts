import type { ProgressResponse, StageResponse } from "./types";

export interface ProcessingState {
  stage: string;
  percent: number | null;
  message: string;
}

export function createInitialProcessingState(): ProcessingState {
  return {
    stage: "",
    percent: null,
    message: "",
  };
}

export function advanceProcessingState(
  current: ProcessingState,
  response: ProgressResponse | StageResponse
): ProcessingState {
  if (response.type === "progress") {
    return {
      stage: response.stage,
      percent: clampPercent(response.percent),
      message: response.message,
    };
  }

  return {
    stage: response.stage,
    percent: current.stage === response.stage ? current.percent : null,
    message: response.message,
  };
}

function clampPercent(percent: number) {
  return Math.max(0, Math.min(percent, 100));
}
