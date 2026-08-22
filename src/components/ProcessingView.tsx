interface ProcessingViewProps {
  stage: string;
  percent: number;
  message: string;
  onCancel?: () => void;
  cancelRequested?: boolean;
}

const STAGE_ORDER = [
  "validating",
  "starting_services",
  "preparing",
  "transcribing",
  "timing",
  "translating",
  "writing",
];

const STAGE_LABELS: Record<string, string> = {
  validating: "Validating",
  starting_services: "Starting Services",
  preparing: "Preparing audio & scene detection",
  transcribing: "Transcribing",
  timing: "Timing normalization",
  translating: "Translating",
  writing: "Writing File",
};

export function ProcessingView({
  stage,
  percent,
  message,
  onCancel,
  cancelRequested = false,
}: ProcessingViewProps) {
  const currentIndex = STAGE_ORDER.indexOf(stage);
  const indeterminate = percent < 0;

  return (
    <div className="space-y-6">
      {/* Stage indicators */}
      <div className="flex items-center justify-between">
        {STAGE_ORDER.map((s, i) => {
          const isActive = i === currentIndex;
          const isComplete = i < currentIndex;

          return (
            <div key={s} className="flex items-center gap-2">
              <div
                className={`
                  w-8 h-8 rounded-full flex items-center justify-center text-xs font-medium transition-all
                  ${isComplete ? "bg-green-500 text-white" : ""}
                  ${isActive ? "bg-blue-500 text-white animate-pulse" : ""}
                  ${!isActive && !isComplete ? "bg-gray-700 text-gray-400" : ""}
                `}
              >
                {isComplete ? "✓" : i + 1}
              </div>
              <span
                className={`text-xs hidden sm:inline ${isActive ? "text-blue-400" : "text-gray-500"}`}
              >
                {STAGE_LABELS[s] ?? s}
              </span>
              {i < STAGE_ORDER.length - 1 && (
                <div
                  className={`w-8 h-0.5 ${isComplete ? "bg-green-500" : "bg-gray-700"}`}
                />
              )}
            </div>
          );
        })}
      </div>

      {/* Progress bar */}
      <div>
        <div className="flex justify-between text-sm mb-1">
          <span className="text-gray-300">{message}</span>
          {!indeterminate && (
            <span className="text-gray-400">{Math.round(percent)}%</span>
          )}
        </div>
        <div className="w-full bg-gray-700 rounded-full h-2 overflow-hidden">
          {indeterminate ? (
            <div className="bg-blue-500 h-full w-full rounded-full animate-pulse" />
          ) : (
            <div
              className="bg-blue-500 h-full rounded-full transition-all duration-300"
              style={{ width: `${Math.min(percent, 100)}%` }}
            />
          )}
        </div>
      </div>

      {/* Cancel */}
      {onCancel && (
        <button
          onClick={onCancel}
          disabled={cancelRequested}
          className={`
            w-full rounded-lg py-2 text-sm font-medium transition-colors border
            ${
              cancelRequested
                ? "cursor-not-allowed border-gray-700 bg-gray-800 text-gray-500"
                : "border-red-500/40 bg-red-500/10 text-red-400 hover:bg-red-500/20"
            }
          `}
        >
          {cancelRequested ? "Cancelling..." : "Cancel"}
        </button>
      )}
    </div>
  );
}
