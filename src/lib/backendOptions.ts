import type {
  ASRBackend,
  ASRBackendCapability,
  CapabilitiesResponse,
  ModelSize,
  TranslationBackend,
  TranslationBackendCapability,
} from "./types";

export const DEFAULT_ASR_BACKEND: ASRBackend = "faster_whisper";
export const DEFAULT_ASR_MODEL_ID =
  "deepdml/faster-whisper-large-v3-turbo-ct2";
export const DEFAULT_TRANSLATION_BACKEND: TranslationBackend = "nllb";

export function findASRBackendCapability(
  capabilities: CapabilitiesResponse | null,
  backend: string
): ASRBackendCapability | undefined {
  return capabilities?.backends.asr.find((item) => item.id === backend);
}

export function findTranslationBackendCapability(
  capabilities: CapabilitiesResponse | null,
  backend: string
): TranslationBackendCapability | undefined {
  return capabilities?.backends.translation.find((item) => item.id === backend);
}

export function resolvePreferredASRBackend(
  capabilities: CapabilitiesResponse | null,
  current: ASRBackend
): ASRBackend {
  const currentCapability = findASRBackendCapability(capabilities, current);
  if (currentCapability?.installed) {
    return current;
  }

  const preferred = capabilities?.defaults.asr_backend ?? DEFAULT_ASR_BACKEND;
  const preferredCapability = findASRBackendCapability(capabilities, preferred);
  if (preferredCapability?.installed) {
    return preferred;
  }

  return capabilities?.backends.asr.find((item) => item.installed)?.id ?? current;
}

export function resolvePreferredTranslationBackend(
  capabilities: CapabilitiesResponse | null,
  current: TranslationBackend
): TranslationBackend {
  if (current === "none") {
    return current;
  }

  const currentCapability = findTranslationBackendCapability(capabilities, current);
  if (currentCapability?.installed) {
    return current;
  }

  const preferred =
    capabilities?.defaults.translation_backend ?? DEFAULT_TRANSLATION_BACKEND;
  if (preferred !== "none") {
    const preferredCapability = findTranslationBackendCapability(
      capabilities,
      preferred
    );
    if (preferredCapability?.installed) {
      return preferred;
    }
  }

  return (
    capabilities?.backends.translation.find((item) => item.installed)?.id ?? "none"
  );
}

export function resolveASRModelId(
  capabilities: CapabilitiesResponse | null,
  backend: ASRBackend,
  whisperModelSize: ModelSize,
  currentModelId: string
): string {
  if (backend === "whisper_cpp") {
    return whisperModelSize;
  }

  const capability = findASRBackendCapability(capabilities, backend);
  return currentModelId || capability?.default_model_id || DEFAULT_ASR_MODEL_ID;
}

export function formatBackendName(value: string | undefined): string {
  if (!value) {
    return "Unknown";
  }

  return value
    .split(/[+/_-]/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}
