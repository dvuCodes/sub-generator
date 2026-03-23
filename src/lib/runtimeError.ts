const WHISPER_SETUP_GUIDANCE =
  'whisper-server is not installed. Place "whisper-server.exe" at "services\\whisper-server\\whisper-server.exe" and a model such as "ggml-base.bin" at "services\\whisper-server\\models\\ggml-base.bin", or add "whisper-server" to PATH.';
const LIBRETRANSLATE_SETUP_GUIDANCE =
  'LibreTranslate is not installed. Install the "libretranslate" executable and add it to PATH, or start LibreTranslate manually before requesting translation languages.';

function isMissingWhisperServer(details: string): boolean {
  const normalized = details.toLowerCase();

  return (
    normalized.includes("whisper-server") &&
    (normalized.includes("not found in path") ||
      normalized.includes("executable file not found"))
  );
}

function isMissingLibreTranslate(details: string): boolean {
  const normalized = details.toLowerCase();

  return (
    normalized.includes("libretranslate") &&
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

  if (isMissingLibreTranslate(details)) {
    return `${message}: ${LIBRETRANSLATE_SETUP_GUIDANCE}`;
  }

  return `${message}: ${details}`;
}
