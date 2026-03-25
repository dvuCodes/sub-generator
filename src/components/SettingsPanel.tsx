import { useState } from "react";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { HugeiconsIcon } from "@hugeicons/react";
import { Settings01Icon } from "@hugeicons/core-free-icons";
import { cn } from "@/lib/utils";
import type { AudioConfig } from "@/lib/types";

interface SettingsPanelProps {
  beamSize: number;
  vadFilter: boolean;
  audioConfig: AudioConfig;
  onBeamSizeChange: (size: number) => void;
  onVadFilterChange: (enabled: boolean) => void;
  onAudioConfigChange: (config: AudioConfig) => void;
  disabled?: boolean;
}

export function SettingsPanel({
  beamSize,
  vadFilter,
  audioConfig,
  onBeamSizeChange,
  onVadFilterChange,
  onAudioConfigChange,
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

          <Separator />

          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label className="text-xs">Audio Enhancement</Label>
                <p className="text-[10px] text-muted-foreground">
                  Preprocess audio to improve transcription
                </p>
              </div>
              <Switch
                checked={audioConfig.enabled}
                onCheckedChange={(enabled) =>
                  onAudioConfigChange({ ...audioConfig, enabled })
                }
                disabled={disabled}
              />
            </div>

            {audioConfig.enabled && (
              <div className="space-y-4 pl-1">
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <Label className="text-xs text-muted-foreground">
                      Vocal Boost
                    </Label>
                    <span className="font-mono text-xs text-foreground">
                      {audioConfig.vocal_boost_db} dB
                    </span>
                  </div>
                  <Slider
                    min={0}
                    max={6}
                    step={1}
                    value={[audioConfig.vocal_boost_db]}
                    onValueChange={([val]) =>
                      onAudioConfigChange({
                        ...audioConfig,
                        vocal_boost_db: val,
                      })
                    }
                    disabled={disabled}
                  />
                  <div className="flex justify-between text-[10px] text-muted-foreground">
                    <span>Off</span>
                    <span>Max boost</span>
                  </div>
                </div>

                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <Label className="text-xs text-muted-foreground">
                      Noise Gate
                    </Label>
                    <p className="text-[10px] text-muted-foreground">
                      Suppress non-speech segments
                    </p>
                  </div>
                  <Switch
                    checked={audioConfig.noise_gate}
                    onCheckedChange={(noise_gate) =>
                      onAudioConfigChange({ ...audioConfig, noise_gate })
                    }
                    disabled={disabled}
                  />
                </div>

                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <Label className="text-xs text-muted-foreground">
                      Normalization
                    </Label>
                    <p className="text-[10px] text-muted-foreground">
                      Even out volume levels
                    </p>
                  </div>
                  <Switch
                    checked={audioConfig.normalize}
                    onCheckedChange={(normalize) =>
                      onAudioConfigChange({ ...audioConfig, normalize })
                    }
                    disabled={disabled}
                  />
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
