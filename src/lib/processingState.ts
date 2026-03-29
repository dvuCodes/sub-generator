import type { ProgressResponse, StageResponse } from "./types";

const STAGE_ORDER = [
  "validating",
  "downloading_model",
  "starting_services",
  "transcribing",
  "diarizing",
  "translating",
  "writing",
] as const;

export interface ProcessingState {
  stage: string;
  percent: number | null;
  message: string;
  elapsedSecs: number | null;
  etaSecs: number | null;
}

export function createInitialProcessingState(): ProcessingState {
  return {
    stage: "",
    percent: null,
    message: "",
    elapsedSecs: null,
    etaSecs: null,
  };
}

export function advanceProcessingState(
  current: ProcessingState,
  response: ProgressResponse | StageResponse
): ProcessingState {
  const stage = resolveStage(current.stage, response.stage);

  if (response.type === "progress") {
    return {
      stage,
      percent: clampPercent(response.percent),
      message: response.message,
      elapsedSecs: response.elapsed_secs ?? null,
      etaSecs: response.eta_secs ?? null,
    };
  }

  return {
    stage,
    percent: current.stage === stage ? current.percent : null,
    message: response.message,
    elapsedSecs: null,
    etaSecs: null,
  };
}

function clampPercent(percent: number) {
  return Math.max(0, Math.min(percent, 100));
}

function resolveStage(currentStage: string, nextStage: string) {
  const currentIndex = STAGE_ORDER.indexOf(
    currentStage as (typeof STAGE_ORDER)[number]
  );
  const nextIndex = STAGE_ORDER.indexOf(nextStage as (typeof STAGE_ORDER)[number]);

  if (currentIndex >= 0 && nextIndex >= 0 && nextIndex < currentIndex) {
    return currentStage;
  }

  return nextStage;
}
