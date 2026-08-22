import type { CueView, QCReport } from "../lib/types";

interface OutputResultProps {
  outputPath: string;
  segments: number;
  durationSecs: number;
  qc?: QCReport | null;
  preview?: CueView[] | null;
  onReset: () => void;
}

const SUMMARY_LABELS: Record<string, string> = {
  duration_over_max: "Over max duration",
  duration_under_min: "Too short",
  cps_over_limit: "Reading speed high",
  line_too_long: "Line too long",
  lines_over_max: "Too many lines",
  overlap: "Overlapping cues",
  gap_too_small: "Gap too small",
};

function formatCueTime(secs: number) {
  const minutes = Math.floor(secs / 60);
  const seconds = secs - minutes * 60;
  return `${minutes}:${seconds.toFixed(1).padStart(4, "0")}`;
}

export function OutputResult({
  outputPath,
  segments,
  durationSecs,
  qc,
  preview,
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

  // Show every violation type the backend reported, so new QC categories are
  // never silently hidden; known keys get friendly labels.
  const summaryEntries = qc
    ? Object.entries(qc.summary)
        .filter(([, count]) => count > 0)
        .map(([key, count]) => ({
          key,
          label: SUMMARY_LABELS[key] ?? key.replaceAll("_", " "),
          count,
        }))
    : [];
  const totalViolations = summaryEntries.reduce(
    (sum, entry) => sum + entry.count,
    0
  );

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

      {qc && (
        <div className="bg-gray-800/50 border border-gray-800 rounded-lg p-4 space-y-2">
          <h3 className="text-sm font-medium text-gray-300">Quality report</h3>
          <div className="grid grid-cols-3 gap-2 text-center">
            <div className="rounded-lg bg-gray-900/60 p-2">
              <div className="text-lg font-semibold text-gray-100">
                {qc.cue_count}
              </div>
              <div className="text-xs text-gray-500">Cues</div>
            </div>
            <div className="rounded-lg bg-gray-900/60 p-2">
              <div className="text-lg font-semibold text-gray-100">
                {qc.avg_cps.toFixed(1)}
              </div>
              <div className="text-xs text-gray-500">Avg CPS</div>
            </div>
            <div className="rounded-lg bg-gray-900/60 p-2">
              <div className="text-lg font-semibold text-gray-100">
                {qc.max_cps.toFixed(1)}
              </div>
              <div className="text-xs text-gray-500">Max CPS</div>
            </div>
          </div>
          {totalViolations === 0 ? (
            <p className="text-sm text-green-400">No timing violations</p>
          ) : (
            <ul className="space-y-1">
              {summaryEntries.map((entry) => (
                <li
                  key={entry.key}
                  className="flex justify-between text-sm text-gray-400"
                >
                  <span>{entry.label}</span>
                  <span
                    className={entry.count > 0 ? "text-yellow-400" : "text-gray-600"}
                  >
                    {entry.count}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      {preview && preview.length > 0 && (
        <div className="bg-gray-800/50 border border-gray-800 rounded-lg p-4 space-y-2">
          <h3 className="text-sm font-medium text-gray-300">Preview</h3>
          <div className="max-h-64 overflow-y-auto space-y-1 pr-1">
            {preview.map((cue, i) => (
              <div
                key={`${cue.start}-${cue.end}-${i}`}
                className="rounded-md bg-gray-900/60 px-3 py-2"
              >
                <div className="flex items-baseline gap-2 text-xs">
                  <span className="text-gray-500">{i + 1}</span>
                  <span className="font-mono text-blue-400/80">
                    {formatCueTime(cue.start)} &rarr; {formatCueTime(cue.end)}
                  </span>
                </div>
                <p className="mt-1 whitespace-pre-line text-sm text-gray-200">
                  {cue.text}
                </p>
              </div>
            ))}
          </div>
        </div>
      )}

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
