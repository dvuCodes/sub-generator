import { useState } from "react";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { HugeiconsIcon } from "@hugeicons/react";
import { Settings01Icon } from "@hugeicons/core-free-icons";
import { cn } from "@/lib/utils";
import type { HonorificsMode, TranslationEngine } from "@/lib/types";

interface SettingsPanelProps {
  beamSize: number;
  vadFilter: boolean;
  initialPrompt: string;
  translationEngine: TranslationEngine;
  deeplApiKey: string;
  llmBaseUrl: string;
  llmModel: string;
  llmApiKey: string;
  synopsis: string;
  glossaryText: string;
  honorifics: HonorificsMode;
  qaPass: boolean;
  shotSnap: boolean;
  onBeamSizeChange: (size: number) => void;
  onVadFilterChange: (enabled: boolean) => void;
  onInitialPromptChange: (value: string) => void;
  onTranslationEngineChange: (engine: TranslationEngine) => void;
  onDeeplApiKeyChange: (value: string) => void;
  onLlmBaseUrlChange: (value: string) => void;
  onLlmModelChange: (value: string) => void;
  onLlmApiKeyChange: (value: string) => void;
  onSynopsisChange: (value: string) => void;
  onGlossaryTextChange: (value: string) => void;
  onHonorificsChange: (mode: HonorificsMode) => void;
  onQaPassChange: (enabled: boolean) => void;
  onShotSnapChange: (enabled: boolean) => void;
  defaultOpen?: boolean;
  disabled?: boolean;
}

const fieldClassName =
  "w-full border border-border bg-background px-3 py-2 text-xs text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary disabled:cursor-not-allowed disabled:opacity-50";

export function SettingsPanel({
  beamSize,
  vadFilter,
  initialPrompt,
  translationEngine,
  deeplApiKey,
  llmBaseUrl,
  llmModel,
  llmApiKey,
  synopsis,
  glossaryText,
  honorifics,
  qaPass,
  shotSnap,
  onBeamSizeChange,
  onVadFilterChange,
  onInitialPromptChange,
  onTranslationEngineChange,
  onDeeplApiKeyChange,
  onLlmBaseUrlChange,
  onLlmModelChange,
  onLlmApiKeyChange,
  onSynopsisChange,
  onGlossaryTextChange,
  onHonorificsChange,
  onQaPassChange,
  onShotSnapChange,
  defaultOpen = false,
  disabled,
}: SettingsPanelProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);

  return (
    <div className="border border-border">
      <button
        type="button"
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

          <SettingSwitch
            label="VAD Filter"
            description="Voice activity detection for cleaner segments"
            checked={vadFilter}
            onCheckedChange={onVadFilterChange}
            disabled={disabled}
          />

          <SettingSwitch
            label="Scene-cut snapping"
            description="Align cue boundaries to nearby shot changes"
            checked={shotSnap}
            onCheckedChange={onShotSnapChange}
            disabled={disabled}
          />

          <Separator />

          <Field label="Initial prompt">
            <input
              value={initialPrompt}
              onChange={(event) => onInitialPromptChange(event.target.value)}
              placeholder="Character names and domain vocabulary"
              disabled={disabled}
              className={fieldClassName}
            />
          </Field>

          <Field label="Translation engine">
            <select
              value={translationEngine}
              onChange={(event) =>
                onTranslationEngineChange(event.target.value as TranslationEngine)
              }
              disabled={disabled}
              className={fieldClassName}
            >
              <option value="backend">Selected built-in backend</option>
              <option value="auto">Auto</option>
              <option value="libretranslate">LibreTranslate</option>
              <option value="deepl">DeepL</option>
              <option value="llm">OpenAI-compatible LLM</option>
            </select>
          </Field>

          {translationEngine === "deepl" && (
            <Field label="DeepL API key">
              <input
                type="password"
                value={deeplApiKey}
                onChange={(event) => onDeeplApiKeyChange(event.target.value)}
                placeholder="DeepL API key"
                disabled={disabled}
                className={fieldClassName}
              />
            </Field>
          )}

          {translationEngine === "llm" && (
            <>
              <Field label="LLM base URL">
                <input
                  value={llmBaseUrl}
                  onChange={(event) => onLlmBaseUrlChange(event.target.value)}
                  placeholder="http://127.0.0.1:11434"
                  disabled={disabled}
                  className={fieldClassName}
                />
              </Field>
              <Field label="LLM model">
                <input
                  value={llmModel}
                  onChange={(event) => onLlmModelChange(event.target.value)}
                  placeholder="qwen2.5:7b-instruct"
                  disabled={disabled}
                  className={fieldClassName}
                />
              </Field>
              <Field label="LLM API key (optional)">
                <input
                  type="password"
                  value={llmApiKey}
                  onChange={(event) => onLlmApiKeyChange(event.target.value)}
                  placeholder="API key if required"
                  disabled={disabled}
                  className={fieldClassName}
                />
              </Field>
            </>
          )}

          <Field label="Synopsis">
            <textarea
              rows={2}
              value={synopsis}
              onChange={(event) => onSynopsisChange(event.target.value)}
              placeholder="Brief context for consistent translation"
              disabled={disabled}
              className={cn(fieldClassName, "resize-y")}
            />
          </Field>

          <Field label="Glossary">
            <textarea
              rows={3}
              value={glossaryText}
              onChange={(event) => onGlossaryTextChange(event.target.value)}
              placeholder={"One entry per line: source = target"}
              disabled={disabled}
              className={cn(fieldClassName, "resize-y font-mono")}
            />
          </Field>

          <Field label="Honorifics">
            <select
              value={honorifics}
              onChange={(event) =>
                onHonorificsChange(event.target.value as HonorificsMode)
              }
              disabled={disabled}
              className={fieldClassName}
            >
              <option value="keep">Keep</option>
              <option value="drop">Drop</option>
            </select>
          </Field>

          <SettingSwitch
            label="Translation QA pass"
            description="Review the translated cues with the selected contextual engine"
            checked={qaPass}
            onCheckedChange={onQaPassChange}
            disabled={disabled}
          />
        </div>
      )}
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      {children}
    </div>
  );
}

function SettingSwitch({
  label,
  description,
  checked,
  onCheckedChange,
  disabled,
}: {
  label: string;
  description: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="space-y-0.5">
        <Label className="text-xs">{label}</Label>
        <p className="text-[10px] text-muted-foreground">{description}</p>
      </div>
      <Switch
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
      />
    </div>
  );
}
