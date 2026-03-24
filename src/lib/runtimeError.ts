const WHISPER_SETUP_GUIDANCE =
  'whisper-server is not installed. Place "whisper-server.exe" at "services\\whisper-server\\whisper-server.exe" and a model such as "ggml-base.bin" at "services\\whisper-server\\models\\ggml-base.bin", or add "whisper-server" to PATH.';
const LLAMA_SERVER_SETUP_GUIDANCE =
  'llama-server is required for translation. Download it from llama.cpp releases and place it at "services\\llama-server\\llama-server.exe", or add "llama-server" to PATH.';

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

  return `${message}: ${details}`;
}
