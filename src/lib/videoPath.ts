const SUPPORTED_VIDEO_EXTENSIONS = new Set([
  ".mp4",
  ".mkv",
  ".avi",
  ".mov",
  ".webm",
  ".flv",
  ".wmv",
  ".m4v",
]);

export function isSupportedVideoPath(path: string): boolean {
  const lowerPath = path.toLowerCase();
  for (const ext of SUPPORTED_VIDEO_EXTENSIONS) {
    if (lowerPath.endsWith(ext)) {
      return true;
    }
  }
  return false;
}

export function pickDroppedVideoPath(paths: string[]): string | null {
  return paths.find(isSupportedVideoPath) ?? null;
}

export const supportedVideoExtensions = [...SUPPORTED_VIDEO_EXTENSIONS];
