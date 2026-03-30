import { useEffect, useMemo, useRef } from "react";
import { cn } from "@/lib/utils";
import { Progress } from "@/components/ui/progress";
import { Button } from "@/components/ui/button";
import { HugeiconsIcon } from "@hugeicons/react";
import { StopCircleIcon } from "@hugeicons/core-free-icons";

export interface ProcessingLogEntry {
  id: string;
  time: string;
  level: "info" | "warn" | "error" | "success";
  label?: string;
  message: string;
}

interface ProcessingDrawerProps {
  stage: string;
  percent: number | null;
  message: string;
  elapsedSecs: number | null;
  etaSecs: number | null;
  logEntries: ProcessingLogEntry[];
  onStop?: () => void;
  stopDisabled?: boolean;
  stopLabel?: string;
  stageLabels?: Record<string, string>;
}

const DEFAULT_STAGE_LABELS: Record<string, string> = {
  validating: "Validate",
  downloading_model: "Download",
  starting_services: "Services",
  transcribing: "Transcribe",
  diarizing: "Speakers",
  translating: "Translate",
  writing: "Write",
};

function formatTime(secs: number): string {
  const total = Math.round(secs);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0)
    return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export function ProcessingDrawer({
  stage,
  percent,
  message,
  elapsedSecs,
  etaSecs,
  logEntries,
  onStop,
  stopDisabled = false,
  stopLabel = "Stop",
  stageLabels,
}: ProcessingDrawerProps) {
  const labels = stageLabels ?? DEFAULT_STAGE_LABELS;
  const containerRef = useRef<HTMLDivElement | null>(null);
  const displayPercent = typeof percent === "number" ? Math.round(percent) : null;
  const stageLabel = labels[stage] ?? (stage || "Running");

  const headerMeta = useMemo(() => {
    if (elapsedSecs != null && etaSecs != null) {
      return `${formatTime(elapsedSecs)} elapsed • ${formatTime(etaSecs)} remaining`;
    }
    if (elapsedSecs != null) {
      return `${formatTime(elapsedSecs)} elapsed`;
    }
    if (displayPercent != null) {
      return `${displayPercent}%`;
    }
    return "Initializing";
  }, [displayPercent, elapsedSecs, etaSecs]);

  useEffect(() => {
    const node = containerRef.current;
    if (!node) return;
    node.scrollTop = node.scrollHeight;
  }, [logEntries]);

  return (
    <section className="tui-console overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-4 border-b border-border px-5 py-4">
        <div className="min-w-0">
          <p className="text-[10px] uppercase tracking-[0.28em] text-muted-foreground">
            SubGen Runtime Console
          </p>
          <div className="mt-1 flex flex-wrap items-center gap-3">
            <span className="text-sm font-medium">{stageLabel}</span>
            <span className="text-[11px] text-muted-foreground">{headerMeta}</span>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{message}</p>
        </div>
        {onStop && (
          <Button
            variant="destructive"
            size="sm"
            className="shrink-0"
            onClick={onStop}
            disabled={stopDisabled}
          >
            <HugeiconsIcon icon={StopCircleIcon} className="size-4" strokeWidth={1.5} />
            {stopLabel}
          </Button>
        )}
      </div>

      <div className="px-5 py-4">
        {displayPercent != null ? (
          <Progress value={displayPercent} />
        ) : (
          <div className="relative h-1 w-full overflow-hidden rounded-full bg-muted">
            <div className="processing-indeterminate h-full w-2/5 bg-primary" />
          </div>
        )}
      </div>

      <div className="px-5 pb-5">
        <div className="rounded-xl border border-foreground/10 bg-foreground text-background">
          <div className="flex items-center gap-2 border-b border-background/10 px-4 py-2 text-[10px] uppercase tracking-[0.3em] text-background/70">
            <span className="inline-block size-1.5 rounded-full bg-chart-1" />
            Live output
          </div>
          <div
            ref={containerRef}
            className="max-h-64 overflow-auto px-4 py-3 font-mono text-[11px] leading-relaxed"
          >
            {logEntries.length === 0 ? (
              <div className="text-background/60">Waiting for activity...</div>
            ) : (
              logEntries.map((entry) => (
                <div key={entry.id} className="flex gap-3 py-1">
                  <span className="text-background/50">[{entry.time}]</span>
                  <span
                    className={cn(
                      "shrink-0 text-[10px] uppercase tracking-[0.18em]",
                      entry.level === "success" && "text-chart-1",
                      entry.level === "warn" && "text-chart-4",
                      entry.level === "error" && "text-destructive",
                      entry.level === "info" && "text-background/70"
                    )}
                  >
                    {entry.label ?? entry.level}
                  </span>
                  <span className="text-background/90">{entry.message}</span>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
