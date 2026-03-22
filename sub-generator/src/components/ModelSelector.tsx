interface ModelSelectorProps {
  model: string;
  onChange: (model: string) => void;
  disabled?: boolean;
}

const MODELS = [
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
    <div>
      <label className="block text-sm text-gray-400 mb-2">Whisper Model</label>
      <div className="grid grid-cols-3 gap-2">
        {MODELS.map((m) => (
          <button
            key={m.id}
            onClick={() => onChange(m.id)}
            disabled={disabled}
            className={`
              p-3 rounded-lg border text-left transition-all
              ${
                model === m.id
                  ? "border-blue-500 bg-blue-500/10 text-blue-400"
                  : "border-gray-700 hover:border-gray-500 text-gray-300"
              }
              ${disabled ? "opacity-50 cursor-not-allowed" : "cursor-pointer"}
            `}
          >
            <div className="font-medium text-sm">{m.name}</div>
            <div className="text-xs text-gray-500 mt-1">
              {m.size} &middot; {m.speed}
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}
