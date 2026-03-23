import type { OutputFormat } from "../lib/types";
import { cn } from "@/lib/utils";
import { Label } from "@/components/ui/label";

interface FormatSelectorProps {
  format: OutputFormat;
  onChange: (format: OutputFormat) => void;
  disabled?: boolean;
}

const FORMATS: { id: OutputFormat; name: string; desc: string }[] = [
  {
    id: "srt",
    name: "SRT",
    desc: "Universal",
  },
  {
    id: "ass",
    name: "ASS",
    desc: "Styled / CJK",
  },
  {
    id: "vtt",
    name: "WebVTT",
    desc: "Web standard",
  },
];

export function FormatSelector({
  format,
  onChange,
  disabled,
}: FormatSelectorProps) {
  return (
    <div className="space-y-2">
      <Label className="text-xs text-muted-foreground uppercase tracking-wider">
        Output Format
      </Label>
      <div className="flex gap-1.5">
        {FORMATS.map((f) => (
          <button
            key={f.id}
            onClick={() => onChange(f.id)}
            disabled={disabled}
            className={cn(
              "flex-1 border px-4 py-2.5 text-center text-xs transition-all",
              format === f.id
                ? "border-primary bg-primary/10 text-foreground"
                : "border-border hover:border-muted-foreground hover:bg-muted/30 text-muted-foreground hover:text-foreground",
              disabled && "opacity-50 cursor-not-allowed"
            )}
          >
            <div className="font-medium">{f.name}</div>
            <div className="mt-0.5 text-[10px] opacity-60">{f.desc}</div>
          </button>
        ))}
      </div>
    </div>
  );
}
