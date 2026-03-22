import { useState } from "react";

interface SettingsPanelProps {
  beamSize: number;
  vadFilter: boolean;
  onBeamSizeChange: (size: number) => void;
  onVadFilterChange: (enabled: boolean) => void;
  disabled?: boolean;
}

export function SettingsPanel({
  beamSize,
  vadFilter,
  onBeamSizeChange,
  onVadFilterChange,
  disabled,
}: SettingsPanelProps) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <div className="border border-gray-700 rounded-lg">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-full px-4 py-3 flex items-center justify-between text-gray-300 hover:text-gray-100 transition-colors"
      >
        <span className="text-sm font-medium">Advanced Settings</span>
        <span
          className={`transform transition-transform ${isOpen ? "rotate-180" : ""}`}
        >
          &#9660;
        </span>
      </button>

      {isOpen && (
        <div className="px-4 pb-4 space-y-4 border-t border-gray-700 pt-4">
          <div>
            <label className="block text-sm text-gray-400 mb-1">
              Beam Size ({beamSize})
            </label>
            <input
              type="range"
              min={1}
              max={10}
              value={beamSize}
              onChange={(e) => onBeamSizeChange(parseInt(e.target.value))}
              disabled={disabled}
              className="w-full accent-blue-500"
            />
            <div className="flex justify-between text-xs text-gray-500">
              <span>Faster</span>
              <span>More accurate</span>
            </div>
          </div>

          <div className="flex items-center justify-between">
            <div>
              <label className="block text-sm text-gray-300">VAD Filter</label>
              <p className="text-xs text-gray-500">
                Voice Activity Detection for cleaner segments
              </p>
            </div>
            <button
              onClick={() => onVadFilterChange(!vadFilter)}
              disabled={disabled}
              className={`
                w-12 h-6 rounded-full transition-colors relative
                ${vadFilter ? "bg-blue-500" : "bg-gray-600"}
                ${disabled ? "opacity-50 cursor-not-allowed" : "cursor-pointer"}
              `}
            >
              <span
                className={`
                  absolute top-0.5 w-5 h-5 rounded-full bg-white transition-transform
                  ${vadFilter ? "translate-x-6" : "translate-x-0.5"}
                `}
              />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
