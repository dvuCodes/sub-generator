const WHISPER_SETUP_GUIDANCE =
  'whisper-server is not installed. Place "whisper-server.exe" at "services\\whisper-server\\whisper-server.exe" and a model such as "ggml-base.bin" at "services\\whisper-server\\models\\ggml-base.bin", or add "whisper-server" to PATH.';
const LLAMA_SERVER_SETUP_GUIDANCE =
  'llama-server is required for translation. Download it from llama.cpp releases and place it at "services\\llama-server\\llama-server.exe", or add "llama-server" to PATH.';
const ML_BACKEND_SETUP_GUIDANCE =
  'ml-backend is not launchable. In dev, keep "python-backend\\service.py" in the repo and ensure Python is available on PATH. For packaged-style runs, stage the same backend files under "services\\ml-backend\\" instead of leaving only placeholder assets there.';
const FASTER_WHISPER_DEPENDENCY_GUIDANCE =
  'faster-whisper Python dependency is missing. Install the packages from "python-backend\\requirements.txt" into the Python runtime used by the ML backend, then restart SubGen.';

function isMissingWhisperServer(details: string): boolean {
  const normalized = details.toLowerCase();

  return (
    normalized.includes("whisper-server") &&
    (normalized.includes("not found in path") ||
      normalized.includes("executable file not found"))
  );
}

function isMissingLlamaServer(details: string): boolean {
  const normalized = details.toLowerCase();

  return (
    normalized.includes("llama-server") &&
    (normalized.includes("not found in path") ||
      normalized.includes("executable file not found"))
  );
}

function isMissingMLBackend(details: string): boolean {
  const normalized = details.toLowerCase();

  return (
    normalized.includes("ml-backend setup is incomplete") ||
    normalized.includes("python runtime is required for ml-backend")
  );
}

function isMissingFasterWhisperDependency(details: string): boolean {
  const normalized = details.toLowerCase();

  return (
    normalized.includes("faster-whisper is not available") &&
    normalized.includes("no module named") &&
    normalized.includes("faster_whisper")
  );
}

export function formatRuntimeError(message: string, details?: string): string {
  if (!details) {
    return message;
  }

  if (isMissingWhisperServer(details)) {
    return `${message}: ${WHISPER_SETUP_GUIDANCE}`;
  }

  if (isMissingLlamaServer(details)) {
    return `${message}: ${LLAMA_SERVER_SETUP_GUIDANCE}`;
  }

  if (isMissingMLBackend(details)) {
    return `${message}: ${ML_BACKEND_SETUP_GUIDANCE}`;
  }

  if (isMissingFasterWhisperDependency(details)) {
    return `${message}: ${FASTER_WHISPER_DEPENDENCY_GUIDANCE}`;
  }

  return `${message}: ${details}`;
}
