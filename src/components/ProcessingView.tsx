import { cn } from "@/lib/utils";
import { Progress } from "@/components/ui/progress";
import { Button } from "@/components/ui/button";
import { HugeiconsIcon } from "@hugeicons/react";
import { StopCircleIcon, Tick02Icon } from "@hugeicons/core-free-icons";

interface ProcessingViewProps {
  stage: string;
  percent: number | null;
  message: string;
  elapsedSecs: number | null;
  etaSecs: number | null;
  onStop?: () => void;
  stopDisabled?: boolean;
  stopLabel?: string;
  stageOrder?: string[];
  stageLabels?: Record<string, string>;
}

const DEFAULT_STAGE_ORDER = [
  "validating",
  "downloading_model",
  "starting_services",
  "preprocessing",
  "transcribing",
  "diarizing",
  "translating",
  "writing",
];

const DEFAULT_STAGE_LABELS: Record<string, string> = {
  validating: "Validate",
  downloading_model: "Download",
  starting_services: "Services",
  preprocessing: "Enhance",
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

export function ProcessingView({
  stage,
  percent,
  message,
  elapsedSecs,
  etaSecs,
  onStop,
  stopDisabled = false,
  stopLabel = "Stop",
  stageOrder,
  stageLabels,
}: ProcessingViewProps) {
  void etaSecs;
  const stages = stageOrder ?? DEFAULT_STAGE_ORDER;
  const labels = stageLabels ?? DEFAULT_STAGE_LABELS;
  const currentIndex = stages.indexOf(stage);
  const isDeterminate = typeof percent === "number";
  const displayPercent = isDeterminate ? Math.round(percent) : null;

  return (
    <div className="space-y-5">
      {/* Stage pipeline */}
      <div className="flex items-center gap-1">
        {stages.map((s, i) => {
          const isActive = i === currentIndex;
          const isComplete = i < currentIndex;

          return (
            <div key={s} className="flex flex-1 items-center gap-1">
              <div className="flex flex-1 flex-col items-center gap-1.5">
                <div
                  className={cn(
                    "flex size-7 items-center justify-center text-[10px] font-medium transition-all",
                    isComplete && "bg-chart-1 text-background",
                    isActive && "bg-primary text-primary-foreground animate-pulse",
                    !isActive && !isComplete && "bg-muted text-muted-foreground"
                  )}
                >
                  {isComplete ? (
                    <HugeiconsIcon icon={Tick02Icon} className="size-3.5" strokeWidth={2.5} />
                  ) : (
                    i + 1
                  )}
                </div>
                <span
                  className={cn(
                    "text-[10px] font-medium uppercase tracking-wider",
                    isActive ? "text-foreground" : "text-muted-foreground"
                  )}
                >
                  {labels[s] ?? s}
                </span>
              </div>
              {i < stages.length - 1 && (
                <div
                  className={cn(
                    "mb-5 h-px flex-1 transition-colors",
                    isComplete ? "bg-chart-1" : "bg-border"
                  )}
                />
              )}
            </div>
          );
        })}
      </div>

      {/* Progress */}
      <div className="space-y-2">
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">{message}</span>
          <span className="font-mono text-foreground">
            {elapsedSecs != null
              ? formatTime(elapsedSecs)
              : isDeterminate
                ? `${displayPercent}%`
                : "---"}
          </span>
        </div>
        {isDeterminate ? (
          <Progress value={displayPercent ?? 0} />
        ) : (
          <div className="relative h-1 w-full overflow-hidden bg-muted">
            <div className="processing-indeterminate h-full w-2/5 bg-primary" />
          </div>
        )}
      </div>

      {onStop && (
        <Button
          variant="destructive"
          size="lg"
          className="w-full"
          onClick={onStop}
          disabled={stopDisabled}
        >
          <HugeiconsIcon icon={StopCircleIcon} className="size-4" strokeWidth={1.5} />
          {stopLabel}
        </Button>
      )}
    </div>
  );
}
