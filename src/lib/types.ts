// IPC commands (frontend -> Go sidecar)
export type ModelSize =
  | "tiny"
  | "base"
  | "small"
  | "medium"
  | "large-v3"
  | "turbo";

export type OutputFormat = "srt" | "ass" | "vtt";

export interface GenerateCommand {
  command: "generate";
  input_video: string;
  source_lang: string | null;
  target_lang: string | null;
  output_format: OutputFormat;
  output_path: string | null;
  model_size: ModelSize;
  beam_size: number;
  vad_filter: boolean;
}

export interface ListLanguagesCommand {
  command: "list_languages";
}

export interface SystemInfoCommand {
  command: "system_info";
}

export interface StartServicesCommand {
  command: "start_services";
}

export interface StopServicesCommand {
  command: "stop_services";
}

export type SidecarCommand =
  | GenerateCommand
  | ListLanguagesCommand
  | SystemInfoCommand
  | StartServicesCommand
  | StopServicesCommand;

// IPC responses (Go sidecar -> frontend)
export interface ProgressResponse {
  type: "progress";
  stage: string;
  percent: number;
  message: string;
  elapsed_secs?: number;
  eta_secs?: number;
}

export interface StageResponse {
  type: "stage";
  stage: string;
  message: string;
}

export interface CompleteResponse {
  type: "complete";
  output_path: string;
  transcription_log?: string;
  segments: number;
  duration_secs: number;
}

export interface ErrorResponse {
  type: "error";
  message: string;
  details?: string;
}

export interface LanguagePair {
  source: string;
  target: string;
}

export interface LanguagesResponse {
  type: "languages";
  installed: LanguagePair[];
}

export interface SystemInfoResponse {
  type: "system_info";
  whisper_server: boolean;
  libretranslate: boolean;
  gpu: string;
}

export type SidecarResponse =
  | ProgressResponse
  | StageResponse
  | CompleteResponse
  | ErrorResponse
  | LanguagesResponse
  | SystemInfoResponse;
