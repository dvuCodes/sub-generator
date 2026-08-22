// IPC commands (frontend -> Go sidecar)
export type ModelSize =
  | "tiny"
  | "base"
  | "small"
  | "medium"
  | "large-v3"
  | "large-v3-q5_0"
  | "turbo"
  | "turbo-q5_0"
  | "turbo-q8_0";

export type OutputFormat = "srt" | "ass" | "vtt";

export type TranslationEngine =
  | "auto"
  | "libretranslate"
  | "deepl"
  | "llm";

export type HonorificsMode = "keep" | "drop";

export interface GlossaryEntry {
  source: string;
  target: string;
}

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
  initial_prompt?: string;
  frame_rate?: number;
  translation_engine?: TranslationEngine;
  deepl_api_key?: string;
  llm_base_url?: string;
  llm_model?: string;
  llm_api_key?: string;
  synopsis?: string;
  glossary?: GlossaryEntry[];
  honorifics?: HonorificsMode;
  qa_pass?: boolean;
  shot_snap?: boolean;
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

export interface CancelCommand {
  command: "cancel";
}

export type SidecarCommand =
  | GenerateCommand
  | ListLanguagesCommand
  | SystemInfoCommand
  | StartServicesCommand
  | StopServicesCommand
  | CancelCommand;

// IPC responses (Go sidecar -> frontend)
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
  qc?: QCReport;
  preview?: CueView[];
}

export interface ErrorResponse {
  type: "error";
  message: string;
  details?: string;
}

export interface CancelledResponse {
  type: "cancelled";
  message: string;
}

export interface CueView {
  start: number;
  end: number;
  text: string;
}

export interface QCIssue {
  index: number;
  start: number;
  end: number;
  issues: string[];
}

export interface QCReport {
  cue_count: number;
  avg_cps: number;
  max_cps: number;
  summary: Record<string, number>;
  issues?: QCIssue[];
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
  ffmpeg: boolean;
  vad_model: boolean;
}

export type SidecarResponse =
  | ProgressResponse
  | StageResponse
  | CompleteResponse
  | ErrorResponse
  | CancelledResponse
  | LanguagesResponse
  | SystemInfoResponse;
