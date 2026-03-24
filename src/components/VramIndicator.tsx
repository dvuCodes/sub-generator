import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

interface VramIndicatorProps {
  totalMiB: number;
  usedMiB: number;
  freeMiB: number;
}

function formatGB(mib: number): string {
  return (mib / 1024).toFixed(1);
}

export function VramIndicator({ totalMiB, usedMiB, freeMiB }: VramIndicatorProps) {
  const usedPercent = totalMiB > 0 ? Math.round((usedMiB / totalMiB) * 100) : 0;

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <div className="flex items-center gap-1.5 mt-1">
            <div className="relative h-1 w-16 overflow-hidden bg-muted">
              <div
                className={cn(
                  "h-full transition-all duration-500",
                  usedPercent > 90 ? "bg-destructive" : "bg-chart-2",
                )}
                style={{ width: `${usedPercent}%` }}
              />
            </div>
            <span className="text-[10px] text-muted-foreground">
              {formatGB(usedMiB)}/{formatGB(totalMiB)} GB
            </span>
          </div>
        </TooltipTrigger>
        <TooltipContent>
          <p>VRAM: {usedMiB} MiB used / {freeMiB} MiB free / {totalMiB} MiB total ({usedPercent}%)</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
