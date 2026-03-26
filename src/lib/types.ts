// IPC commands (frontend -> Go sidecar)
export type ModelSize =
  | "tiny"
  | "base"
  | "small"
  | "medium"
  | "large-v3"
  | "turbo";

export type OutputFormat = "srt" | "ass" | "vtt";

export interface AudioConfig {
  enabled: boolean;
  vocal_boost_db: number;
  noise_gate: boolean;
  normalize: boolean;
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
  audio_config: AudioConfig;
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

export interface VramInfoCommand {
  command: "vram_info";
}

export type RequiredFor = "transcription" | "translation";

export interface ServiceIssue {
  code: string;
  observed_error?: string;
}

export interface ServiceAction {
  id: string;
  label: string;
  description: string;
  kind: "archive" | "manual";
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
  | SystemInfoCommand
  | StartServicesCommand
  | StopServicesCommand
  | VramInfoCommand
  | CheckSetupCommand
  | InstallDependencyCommand;

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
  translation_engine: boolean;
  gpu: string;
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
  | LanguagesResponse
  | SystemInfoResponse
  | VramInfoResponse
  | SetupStatusResponse;
