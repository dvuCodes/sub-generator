// IPC Commands (Frontend → Go sidecar)
export interface GenerateCommand {
  command: "generate";
  input_video: string;
  source_lang: string | null;
  target_lang: string | null;
  output_format: "srt" | "ass" | "vtt";
  output_path: string | null;
  model_size: "tiny" | "base" | "small" | "medium" | "large-v3" | "turbo";
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

// IPC Responses (Go sidecar → Frontend)
export interface ProgressResponse {
  type: "progress";
  stage: string;
  percent: number;
  message: string;
}

export interface StageResponse {
  type: "stage";
  stage: string;
  message: string;
}

export interface CompleteResponse {
  type: "complete";
  output_path: string;
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
