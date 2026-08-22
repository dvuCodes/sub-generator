import type { SystemInfoResponse } from "./types";

export interface SystemInfoState {
  whisperServer: boolean;
  translationEngine: boolean;
  mlBackend: boolean;
  libretranslate: boolean;
  gpu: string;
  ffmpeg: boolean;
  vadModel: boolean;
}

export function reduceSystemInfo(
  _prev: SystemInfoState | null,
  response: SystemInfoResponse
): SystemInfoState | null {
  return {
    whisperServer: response.whisper_server,
    translationEngine: response.translation_engine,
    mlBackend: response.ml_backend,
    libretranslate: response.libretranslate,
    gpu: response.gpu,
    ffmpeg: response.ffmpeg,
    vadModel: response.vad_model,
  };
}
