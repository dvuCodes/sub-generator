import type { OutputFormat } from "../lib/types";

interface FormatSelectorProps {
  format: OutputFormat;
  onChange: (format: OutputFormat) => void;
  disabled?: boolean;
}

const FORMATS: { id: OutputFormat; name: string; desc: string }[] = [
  {
    id: "srt",
    name: "SRT",
    desc: "Most compatible, works everywhere",
  },
  {
    id: "ass",
    name: "ASS",
    desc: "Advanced styling, great for CJK",
  },
  {
    id: "vtt",
    name: "WebVTT",
    desc: "Web standard format",
  },
];

export function FormatSelector({
  format,
  onChange,
  disabled,
}: FormatSelectorProps) {
  return (
    <div>
      <label className="block text-sm text-gray-400 mb-2">Output Format</label>
      <div className="flex gap-2">
        {FORMATS.map((f) => (
          <button
            key={f.id}
            onClick={() => onChange(f.id)}
            disabled={disabled}
            className={`
              flex-1 px-4 py-2 rounded-lg border text-center transition-all
              ${
                format === f.id
                  ? "border-blue-500 bg-blue-500/10 text-blue-400"
                  : "border-gray-700 hover:border-gray-500 text-gray-300"
              }
              ${disabled ? "opacity-50 cursor-not-allowed" : "cursor-pointer"}
            `}
          >
            <div className="font-medium text-sm">{f.name}</div>
            <div className="text-xs text-gray-500 mt-1">{f.desc}</div>
          </button>
        ))}
      </div>
    </div>
  );
}
