export function deriveOutputDirectory(outputPath: string) {
  const lastSeparator = Math.max(
    outputPath.lastIndexOf("/"),
    outputPath.lastIndexOf("\\")
  );

  if (lastSeparator < 0) {
    return ".";
  }

  if (lastSeparator === 0) {
    return outputPath[0];
  }

  const dir = outputPath.slice(0, lastSeparator);
  if (/^[A-Za-z]:$/.test(dir)) {
    return `${dir}\\`;
  }

  return dir;
}

export function explorerOpenTarget(outputPath: string) {
  const dir = deriveOutputDirectory(outputPath);
  const normalized = dir.replace(/\\/g, "/");
  return normalized.startsWith("/")
    ? `file://${normalized}`
    : `file:///${normalized}`;
}
