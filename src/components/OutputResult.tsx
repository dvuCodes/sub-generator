import type { CueView, QCReport } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  CheckmarkCircle02Icon,
  FolderOpenIcon,
  RefreshIcon,
} from "@hugeicons/core-free-icons";
import { deriveOutputDirectory, explorerOpenTarget } from "@/lib/outputPath";

const QC_LABELS: Record<string, string> = {
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

interface OutputResultProps {
  outputPath: string;
  transcriptionLog?: string;
  segments: number;
  durationSecs: number;
  backendSummary?: string;
  selectedASRBackend?: string;
  diarizationRan?: boolean;
  speakerCount?: number;
  qc?: QCReport | null;
  preview?: CueView[] | null;
  onReset: () => void;
}

export function OutputResult({
  outputPath,
  transcriptionLog,
  segments,
  durationSecs,
  backendSummary,
  selectedASRBackend,
  diarizationRan,
  speakerCount,
  qc,
  preview,
  onReset,
}: OutputResultProps) {
  const fileName = outputPath.split(/[/\\]/).pop() ?? outputPath;
  const dir = deriveOutputDirectory(outputPath);

  const openInExplorer = async () => {
    try {
      const { open } = await import("@tauri-apps/plugin-shell");
      await open(explorerOpenTarget(outputPath));
    } catch (err) {
      console.error("Failed to open directory:", err);
    }
  };

  const formatDuration = (secs: number) =>
    secs < 60
      ? `${Math.round(secs)}s`
      : `${Math.floor(secs / 60)}m ${Math.round(secs % 60)}s`;

  // Show every violation type the backend reported, so new QC categories are
  // never silently hidden; known keys get friendly labels.
  const summaryEntries = qc
    ? Object.entries(qc.summary)
        .filter(([, count]) => count > 0)
        .map(([key, count]) => ({
          key,
          label: QC_LABELS[key] ?? key.replaceAll("_", " "),
          count,
        }))
    : [];
  const totalViolations = summaryEntries.reduce(
    (sum, entry) => sum + entry.count,
    0
  );

  return (
    <Card className="border-chart-1/30 bg-chart-1/5">
      <CardContent className="space-y-5">
        <div className="flex flex-col items-center gap-3 pt-2">
          <div className="flex size-14 items-center justify-center border border-chart-1/30 bg-chart-1/10">
            <HugeiconsIcon
              icon={CheckmarkCircle02Icon}
              className="size-7 text-chart-1"
              strokeWidth={1.5}
            />
          </div>
          <div className="text-center">
            <h2 className="text-sm font-medium text-foreground">
              Subtitles Generated
            </h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Processing complete
            </p>
          </div>
        </div>

        <Separator />

        <div className="space-y-2.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">File</span>
            <span className="font-mono text-foreground">{fileName}</span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">Segments</span>
            <span className="font-mono text-foreground">{segments}</span>
          </div>
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">Duration</span>
            <span className="font-mono text-foreground">
              {formatDuration(durationSecs)}
            </span>
          </div>
          {backendSummary && (
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Backends</span>
              <span className="max-w-[220px] truncate font-mono text-foreground">
                {backendSummary}
              </span>
            </div>
          )}
          {selectedASRBackend && (
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">ASR Backend</span>
              <span className="font-mono text-foreground">{selectedASRBackend}</span>
            </div>
          )}
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">Speaker Labels</span>
            <span className="font-mono text-foreground">
              {diarizationRan ? "On" : "Off"}
            </span>
          </div>
          {typeof speakerCount === "number" && (
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Speakers</span>
              <span className="font-mono text-foreground">{speakerCount}</span>
            </div>
          )}
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">Location</span>
            <span className="max-w-[220px] truncate font-mono text-foreground">
              {dir}
            </span>
          </div>
          {transcriptionLog && (
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">Transcription Log</span>
              <span className="max-w-[220px] truncate font-mono text-foreground">
                {transcriptionLog.split(/[/\\]/).pop()}
              </span>
            </div>
          )}
        </div>

        {qc && (
          <>
            <Separator />
            <div className="space-y-3">
              <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Quality report
              </h3>
              <div className="grid grid-cols-3 gap-2">
                {[
                  ["Cues", qc.cue_count],
                  ["Avg CPS", qc.avg_cps.toFixed(1)],
                  ["Max CPS", qc.max_cps.toFixed(1)],
                ].map(([label, value]) => (
                  <div key={label} className="border border-border bg-muted/30 p-2 text-center">
                    <div className="font-mono text-sm text-foreground">{value}</div>
                    <div className="text-[10px] text-muted-foreground">{label}</div>
                  </div>
                ))}
              </div>
              {totalViolations === 0 ? (
                <p className="text-xs text-chart-1">No timing violations</p>
              ) : (
                <div className="grid grid-cols-2 gap-x-4 gap-y-1">
                  {summaryEntries.map((entry) => (
                    <div
                      key={entry.key}
                      className="flex justify-between text-[10px] text-muted-foreground"
                    >
                      <span>{entry.label}</span>
                      <span className="text-chart-4">{entry.count}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </>
        )}

        {preview && preview.length > 0 && (
          <>
            <Separator />
            <div className="space-y-2">
              <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Cue preview
              </h3>
              <div className="max-h-64 space-y-1 overflow-y-auto pr-1">
                {preview.map((cue, index) => (
                  <div key={`${cue.start}-${cue.end}-${index}`} className="border border-border bg-muted/20 px-3 py-2">
                    <div className="flex gap-2 font-mono text-[10px] text-muted-foreground">
                      <span>{index + 1}</span>
                      <span>{formatCueTime(cue.start)} → {formatCueTime(cue.end)}</span>
                    </div>
                    <p className="mt-1 whitespace-pre-line text-xs text-foreground">{cue.text}</p>
                  </div>
                ))}
              </div>
            </div>
          </>
        )}

        <div className="flex gap-2">
          <Button
            variant="outline"
            size="lg"
            className="flex-1"
            onClick={openInExplorer}
          >
            <HugeiconsIcon icon={FolderOpenIcon} className="size-4" strokeWidth={1.5} />
            Open Folder
          </Button>
          <Button size="lg" className="flex-1" onClick={onReset}>
            <HugeiconsIcon icon={RefreshIcon} className="size-4" strokeWidth={1.5} />
            New File
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
