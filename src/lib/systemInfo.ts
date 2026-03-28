import type { SystemInfoResponse } from "./types";

export interface SystemInfoState {
  whisperServer: boolean;
  translationEngine: boolean;
  mlBackend: boolean;
  gpu: string;
}

export function reduceSystemInfo(
  _prev: SystemInfoState | null,
  response: SystemInfoResponse
): SystemInfoState | null {
  return {
    whisperServer: response.whisper_server,
    translationEngine: response.translation_engine,
    mlBackend: response.ml_backend,
    gpu: response.gpu,
  };
}
