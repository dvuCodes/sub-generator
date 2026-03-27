import type {
  ASRBackend,
  ServiceIssue,
  SetupStatusResponse,
  TranslationBackend,
} from "./types";

export interface GenerateSelection {
  asrBackend: ASRBackend;
  translationBackend: TranslationBackend;
  diarizationEnabled?: boolean;
}

export function shouldDisableGenerate(
  setupStatus: SetupStatusResponse | null,
  selection: GenerateSelection
): boolean {
  if (!setupStatus) return false;

  const services = new Map(
    setupStatus.services.map((service) => [service.id, service])
  );
  const requires = ["ffmpeg"];

  if (selection.asrBackend === "whisper_cpp") {
    requires.push("whisper");
  } else {
    requires.push("ml-backend");
  }

  if (selection.translationBackend === "gemma_context") {
    requires.push("llama");
  } else if (selection.translationBackend === "nllb") {
    requires.push("ml-backend");
  }

  if (selection.diarizationEnabled) {
    requires.push("ml-backend");
  }

  return requires.some((id) => services.get(id)?.state === "action_required");
}

export function formatSetupIssue(issue: ServiceIssue): string {
  if (issue.observed_error) {
    return `Service cannot start: ${issue.observed_error}`;
  }

  switch (issue.code) {
    case "binary_not_found":
      return "Binary not found. Install the service using the options below.";
    case "binary_not_runnable":
      return "Service binary cannot start. Required libraries may be missing.";
    case "not_in_path":
      return "Not found in system PATH. Install it and add to PATH.";
    default:
      return `Setup issue: ${issue.code}`;
  }
}
