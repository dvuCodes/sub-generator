import { useState } from "react";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { HugeiconsIcon } from "@hugeicons/react";
import { Settings01Icon } from "@hugeicons/core-free-icons";
import { cn } from "@/lib/utils";
import type { VadParams } from "@/lib/types";

interface SettingsPanelProps {
  beamSize: number;
  vadFilter: boolean;
  vadParams: VadParams;
  onBeamSizeChange: (size: number) => void;
  onVadFilterChange: (enabled: boolean) => void;
  onVadParamsChange: (params: VadParams) => void;
  disabled?: boolean;
}

export function SettingsPanel({
  beamSize,
  vadFilter,
  vadParams,
  onBeamSizeChange,
  onVadFilterChange,
  onVadParamsChange,
  disabled,
}: SettingsPanelProps) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <div className="border border-border">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="flex w-full items-center justify-between px-4 py-3 text-xs text-muted-foreground transition-colors hover:text-foreground"
      >
        <span className="flex items-center gap-2 font-medium uppercase tracking-wider">
          <HugeiconsIcon icon={Settings01Icon} className="size-3.5" strokeWidth={1.5} />
          Advanced
        </span>
        <span
          className={cn(
            "text-[10px] transition-transform duration-200",
            isOpen && "rotate-180"
          )}
        >
          &#9660;
        </span>
      </button>

      {isOpen && (
        <div className="space-y-5 border-t border-border px-4 pb-4 pt-4">
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label className="text-xs text-muted-foreground">Beam Size</Label>
              <span className="font-mono text-xs text-foreground">{beamSize}</span>
            </div>
            <Slider
              min={1}
              max={8}
              step={1}
              value={[beamSize]}
              onValueChange={([val]) => onBeamSizeChange(val)}
              disabled={disabled}
            />
            <div className="flex justify-between text-[10px] text-muted-foreground">
              <span>Faster</span>
              <span>More accurate</span>
            </div>
          </div>

          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <Label className="text-xs">VAD Filter</Label>
              <p className="text-[10px] text-muted-foreground">
                Voice Activity Detection for cleaner segments
              </p>
            </div>
            <Switch
              checked={vadFilter}
              onCheckedChange={onVadFilterChange}
              disabled={disabled}
            />
          </div>

          {vadFilter && (
            <div className="space-y-4 border-t border-border pt-4">
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-[10px] text-muted-foreground">Threshold</Label>
                  <span className="font-mono text-[10px] text-foreground">{vadParams.vad_threshold.toFixed(2)}</span>
                </div>
                <Slider
                  min={0}
                  max={1}
                  step={0.05}
                  value={[vadParams.vad_threshold]}
                  onValueChange={([val]) => onVadParamsChange({ ...vadParams, vad_threshold: val })}
                  disabled={disabled}
                />
                <div className="flex justify-between text-[10px] text-muted-foreground">
                  <span>More speech</span>
                  <span>Less speech</span>
                </div>
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-[10px] text-muted-foreground">Min Speech Duration</Label>
                  <span className="font-mono text-[10px] text-foreground">{vadParams.vad_min_speech_duration_ms}ms</span>
                </div>
                <Slider
                  min={0}
                  max={1000}
                  step={10}
                  value={[vadParams.vad_min_speech_duration_ms]}
                  onValueChange={([val]) => onVadParamsChange({ ...vadParams, vad_min_speech_duration_ms: val })}
                  disabled={disabled}
                />
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-[10px] text-muted-foreground">Min Silence Duration</Label>
                  <span className="font-mono text-[10px] text-foreground">{vadParams.vad_min_silence_duration_ms}ms</span>
                </div>
                <Slider
                  min={0}
                  max={2000}
                  step={10}
                  value={[vadParams.vad_min_silence_duration_ms]}
                  onValueChange={([val]) => onVadParamsChange({ ...vadParams, vad_min_silence_duration_ms: val })}
                  disabled={disabled}
                />
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-[10px] text-muted-foreground">Max Speech Duration</Label>
                  <span className="font-mono text-[10px] text-foreground">
                    {vadParams.vad_max_speech_duration_s === 0 ? "Unlimited" : `${vadParams.vad_max_speech_duration_s}s`}
                  </span>
                </div>
                <Slider
                  min={0}
                  max={120}
                  step={1}
                  value={[vadParams.vad_max_speech_duration_s]}
                  onValueChange={([val]) => onVadParamsChange({ ...vadParams, vad_max_speech_duration_s: val })}
                  disabled={disabled}
                />
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label className="text-[10px] text-muted-foreground">Speech Padding</Label>
                  <span className="font-mono text-[10px] text-foreground">{vadParams.vad_speech_pad_ms}ms</span>
                </div>
                <Slider
                  min={0}
                  max={500}
                  step={5}
                  value={[vadParams.vad_speech_pad_ms]}
                  onValueChange={([val]) => onVadParamsChange({ ...vadParams, vad_speech_pad_ms: val })}
                  disabled={disabled}
                />
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
