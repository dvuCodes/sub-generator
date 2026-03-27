import type { ServiceIssue, SetupStatusResponse } from "./types";

export function shouldDisableGenerate(
  setupStatus: SetupStatusResponse | null,
  targetLang: string
): boolean {
  if (!setupStatus) return false;

  for (const service of setupStatus.services) {
    if (service.state !== "action_required") continue;

    if (service.required_for === "transcription") return true;
    if (service.required_for === "translation" && targetLang) return true;
  }

  return false;
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
