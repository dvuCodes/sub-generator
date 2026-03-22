interface OutputResultProps {
  outputPath: string;
  segments: number;
  durationSecs: number;
  onReset: () => void;
}

export function OutputResult({
  outputPath,
  segments,
  durationSecs,
  onReset,
}: OutputResultProps) {
  const fileName = outputPath.split(/[/\\]/).pop() ?? outputPath;
  const dir = outputPath.substring(
    0,
    outputPath.length - (fileName?.length ?? 0) - 1
  );

  const openInExplorer = async () => {
    try {
      const { open } = await import("@tauri-apps/plugin-shell");
      await open(dir);
    } catch (err) {
      console.error("Failed to open directory:", err);
    }
  };

  return (
    <div className="bg-green-500/10 border border-green-500/30 rounded-xl p-6 space-y-4">
      <div className="text-center">
        <div className="text-5xl mb-3">✅</div>
        <h2 className="text-xl font-semibold text-green-400">
          Subtitles Generated
        </h2>
      </div>

      <div className="bg-gray-800/50 rounded-lg p-4 space-y-2">
        <div className="flex justify-between text-sm">
          <span className="text-gray-400">File</span>
          <span className="text-gray-200 font-mono text-xs">{fileName}</span>
        </div>
        <div className="flex justify-between text-sm">
          <span className="text-gray-400">Segments</span>
          <span className="text-gray-200">{segments}</span>
        </div>
        <div className="flex justify-between text-sm">
          <span className="text-gray-400">Processing Time</span>
          <span className="text-gray-200">
            {durationSecs < 60
              ? `${Math.round(durationSecs)}s`
              : `${Math.floor(durationSecs / 60)}m ${Math.round(durationSecs % 60)}s`}
          </span>
        </div>
        <div className="flex justify-between text-sm">
          <span className="text-gray-400">Location</span>
          <span className="text-gray-200 font-mono text-xs truncate max-w-[250px]">
            {dir}
          </span>
        </div>
      </div>

      <div className="flex gap-3">
        <button
          onClick={openInExplorer}
          className="flex-1 px-4 py-2 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-lg transition-colors text-sm"
        >
          Open in Explorer
        </button>
        <button
          onClick={onReset}
          className="flex-1 px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors text-sm"
        >
          Generate Another
        </button>
      </div>
    </div>
  );
}
