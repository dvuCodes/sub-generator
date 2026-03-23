import type { ModelSize } from "../lib/types";
import { cn } from "@/lib/utils";
import { Label } from "@/components/ui/label";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface ModelSelectorProps {
  model: ModelSize;
  onChange: (model: ModelSize) => void;
  disabled?: boolean;
}

const MODELS: {
  id: ModelSize;
  name: string;
  size: string;
  speed: string;
  desc: string;
}[] = [
  {
    id: "tiny",
    name: "Tiny",
    size: "75 MB",
    speed: "Fastest",
    desc: "Quick draft, lower accuracy",
  },
  {
    id: "base",
    name: "Base",
    size: "142 MB",
    speed: "Fast",
    desc: "Good balance of speed and accuracy",
  },
  {
    id: "small",
    name: "Small",
    size: "466 MB",
    speed: "Medium",
    desc: "Better accuracy for most content",
  },
  {
    id: "medium",
    name: "Medium",
    size: "1.5 GB",
    speed: "Slower",
    desc: "High accuracy, needs more VRAM",
  },
  {
    id: "large-v3",
    name: "Large v3",
    size: "3.1 GB",
    speed: "Slowest",
    desc: "Best accuracy, requires 10+ GB VRAM",
  },
  {
    id: "turbo",
    name: "Turbo",
    size: "809 MB",
    speed: "Fast",
    desc: "Near large-v3 quality at 8x speed",
  },
];

export function ModelSelector({
  model,
  onChange,
  disabled,
}: ModelSelectorProps) {
  return (
    <div className="space-y-2">
      <Label className="text-xs text-muted-foreground uppercase tracking-wider">
        Model
      </Label>
      <div className="grid grid-cols-3 gap-1.5">
        {MODELS.map((m) => (
          <Tooltip key={m.id}>
            <TooltipTrigger asChild>
              <button
                onClick={() => onChange(m.id)}
                disabled={disabled}
                className={cn(
                  "relative px-3 py-2.5 text-left text-xs transition-all border",
                  model === m.id
                    ? "border-primary bg-primary/10 text-foreground"
                    : "border-border hover:border-muted-foreground hover:bg-muted/30 text-muted-foreground hover:text-foreground",
                  disabled && "opacity-50 cursor-not-allowed"
                )}
              >
                <div className="font-medium">{m.name}</div>
                <div className="mt-0.5 text-[10px] opacity-60">
                  {m.size}
                </div>
              </button>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              <p>{m.desc}</p>
              <p className="text-muted-foreground mt-0.5">Speed: {m.speed}</p>
            </TooltipContent>
          </Tooltip>
        ))}
      </div>
    </div>
  );
}
