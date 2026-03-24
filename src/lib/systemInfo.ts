import type { LanguagesResponse, SystemInfoResponse } from "./types";

export interface SystemInfoState {
  whisperServer: boolean;
  translationEngine: boolean;
  gpu: string;
}

export function reduceSystemInfo(
  prev: SystemInfoState | null,
  response: LanguagesResponse | SystemInfoResponse
): SystemInfoState | null {
  if (response.type === "languages") {
    return prev;
  }

  return {
    whisperServer: response.whisper_server,
    translationEngine: response.translation_engine,
    gpu: response.gpu,
  };
}
