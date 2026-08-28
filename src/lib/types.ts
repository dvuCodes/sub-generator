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
export type ASRBackend = "faster_whisper" | "whisper_cpp";
export type TranslationBackend = "nllb" | "gemma_context" | "none";
export type TranslationEngine = "backend" | "auto" | "libretranslate" | "deepl" | "llm";
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
  asr_backend: ASRBackend;
  asr_model_id: string;
  model_size: ModelSize;
  translation_backend: TranslationBackend;
  diarization_enabled: boolean;
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

export interface CapabilitiesCommand {
  command: "capabilities";
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

export interface VramInfoCommand {
  command: "vram_info";
}

export interface CheckSetupCommand {
  command: "check_setup";
}

export interface InstallDependencyCommand {
  command: "install_dependency";
  action_id: string;
}

export type SidecarCommand =
  | GenerateCommand
  | ListLanguagesCommand
  | CapabilitiesCommand
  | SystemInfoCommand
  | StartServicesCommand
  | StopServicesCommand
  | CancelCommand
  | VramInfoCommand
  | CheckSetupCommand
  | InstallDependencyCommand;

export type RequiredFor = "transcription" | "translation";

export interface ServiceIssue {
  code: string;
  observed_error?: string;
}

export interface ServiceAction {
  id: string;
  label: string;
  description: string;
  kind: "archive" | "manual" | "command";
  preferred?: boolean;
  guidance?: string;
}

export interface ServiceStatus {
  id: string;
  display_name: string;
  required_for: RequiredFor;
  state: "ready" | "action_required";
  issues: ServiceIssue[];
  actions: ServiceAction[];
}

export interface SetupStatusResponse {
  type: "setup_status";
  services: ServiceStatus[];
}

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
  backend_summary?: string;
  selected_asr_backend?: string;
  diarization_ran?: boolean;
  speaker_count?: number;
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

export interface LanguageOption {
  code: string;
  name: string;
}

export interface BackendDefaults {
  asr_backend: ASRBackend;
  asr_model_id: string;
  translation_backend: TranslationBackend;
  diarization_enabled: boolean;
}

export interface ASRBackendCapability {
  id: ASRBackend;
  display_name: string;
  installed: boolean;
  default_model_id?: string;
  source_languages: LanguageOption[];
}

export interface TranslationBackendCapability {
  id: Exclude<TranslationBackend, "none">;
  display_name: string;
  installed: boolean;
  default_model_id?: string;
  target_languages: LanguageOption[];
}

export interface CapabilitiesResponse {
  type: "capabilities";
  defaults: BackendDefaults;
  backends: {
    asr: ASRBackendCapability[];
    translation: TranslationBackendCapability[];
  };
}

export interface SystemInfoResponse {
  type: "system_info";
  whisper_server: boolean;
  translation_engine: boolean;
  ml_backend: boolean;
  libretranslate: boolean;
  gpu: string;
  ffmpeg: boolean;
  vad_model: boolean;
}

export interface VramInfo {
  total_mib: number;
  used_mib: number;
  free_mib: number;
}

export interface VramInfoResponse {
  type: "vram_info";
  vram: VramInfo | null;
}

export type SidecarResponse =
  | ProgressResponse
  | StageResponse
  | CompleteResponse
  | ErrorResponse
  | CancelledResponse
  | LanguagesResponse
  | CapabilitiesResponse
  | SystemInfoResponse
  | VramInfoResponse
  | SetupStatusResponse;
