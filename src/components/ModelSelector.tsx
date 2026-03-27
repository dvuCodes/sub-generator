import { cn } from "@/lib/utils";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import type {
  ASRBackend,
  ASRBackendCapability,
  ModelSize,
  TranslationBackend,
  TranslationBackendCapability,
} from "@/lib/types";

interface BackendOption {
  id: TranslationBackend;
  label: string;
  description: string;
  installed: boolean;
}

interface ModelOption {
  id: string;
  name: string;
  meta: string;
  desc: string;
}

interface ModelSelectorProps {
  asrBackend: ASRBackend;
  asrOptions: ASRBackendCapability[];
  asrModelId: string;
  translationBackend: TranslationBackend;
  translationOptions: TranslationBackendCapability[];
  whisperModel: ModelSize;
  diarizationEnabled: boolean;
  onAsrBackendChange: (backend: ASRBackend) => void;
  onAsrModelChange: (modelId: string) => void;
  onTranslationBackendChange: (backend: TranslationBackend) => void;
  onWhisperModelChange: (model: ModelSize) => void;
  onDiarizationChange: (enabled: boolean) => void;
  disabled?: boolean;
}

const WHISPER_MODELS: {
  id: ModelSize;
  name: string;
  size: string;
  desc: string;
}[] = [
  { id: "tiny", name: "Tiny", size: "75 MB", desc: "Quick draft, lower accuracy" },
  { id: "base", name: "Base", size: "142 MB", desc: "Balanced local fallback" },
  { id: "small", name: "Small", size: "466 MB", desc: "Higher accuracy fallback" },
  { id: "medium", name: "Medium", size: "1.5 GB", desc: "Slow, strong accuracy" },
  { id: "large-v3", name: "Large v3", size: "3.1 GB", desc: "Best local Whisper accuracy" },
  { id: "turbo", name: "Turbo", size: "809 MB", desc: "Recommended whisper.cpp fallback" },
];

const FASTER_WHISPER_MODELS: ModelOption[] = [
  {
    id: "deepdml/faster-whisper-large-v3-turbo-ct2",
    name: "Large v3 Turbo",
    meta: "Recommended",
    desc: "Fast multilingual default with CTranslate2 weights",
  },
  {
    id: "Systran/faster-whisper-large-v3",
    name: "Large v3",
    meta: "Highest accuracy",
    desc: "Stronger quality, heavier runtime cost",
  },
];

function backendLabel(name: string, installed: boolean) {
  return installed ? name : `${name} (setup required)`;
}

function modelOptionsForBackend(
  backend: string,
  defaultModelId: string | undefined
): ModelOption[] {
  if (backend !== "faster_whisper") {
    return [];
  }

  if (!defaultModelId || FASTER_WHISPER_MODELS.some((model) => model.id === defaultModelId)) {
    return FASTER_WHISPER_MODELS;
  }

  return [
    {
      id: defaultModelId,
      name: defaultModelId.split("/").pop() ?? defaultModelId,
      meta: "Default",
      desc: "Backend-reported default model",
    },
    ...FASTER_WHISPER_MODELS,
  ];
}

export function ModelSelector({
  asrBackend,
  asrOptions,
  asrModelId,
  translationBackend,
  translationOptions,
  whisperModel,
  diarizationEnabled,
  onAsrBackendChange,
  onAsrModelChange,
  onTranslationBackendChange,
  onWhisperModelChange,
  onDiarizationChange,
  disabled,
}: ModelSelectorProps) {
  const translationChoices: BackendOption[] = [
    {
      id: "none",
      label: "No translation",
      description: "Generate subtitles in the source language only",
      installed: true,
    },
    ...translationOptions.map((backend) => ({
      id: backend.id,
      label: backendLabel(backend.display_name, backend.installed),
      description:
        backend.installed
          ? backend.id === "nllb"
            ? "Deterministic multilingual subtitle translation"
            : "Context-aware rewriting with Gemma"
          : backend.id === "nllb"
            ? "Requires the ML backend. See setup guidance above."
            : "Requires the ML backend and llama.cpp. See setup guidance above.",
      installed: backend.installed,
    })),
  ];
  const currentASR = asrOptions.find((backend) => backend.id === asrBackend);
  const asrModelOptions = modelOptionsForBackend(asrBackend, currentASR?.default_model_id);

  return (
    <div className="space-y-5 border border-border p-4">
      <div className="space-y-2">
        <Label className="text-xs text-muted-foreground uppercase tracking-wider">
          ASR Backend
        </Label>
        <div className="grid gap-2 sm:grid-cols-2">
          {asrOptions.map((backend) => (
            <button
              key={backend.id}
              type="button"
              disabled={disabled || !backend.installed}
              onClick={() => onAsrBackendChange(backend.id)}
              className={cn(
                "space-y-1 border px-3 py-3 text-left text-xs transition-colors",
                asrBackend === backend.id
                  ? "border-primary bg-primary/10 text-foreground"
                  : "border-border text-muted-foreground hover:border-muted-foreground hover:text-foreground",
                (!backend.installed || disabled) && "cursor-not-allowed opacity-50"
              )}
            >
              <div className="font-medium">{backendLabel(backend.display_name, backend.installed)}</div>
              <div className="text-[10px] leading-4">
                {backend.id === "faster_whisper"
                  ? "Bundled Python runtime for default multilingual ASR"
                  : "Legacy whisper.cpp fallback path"}
              </div>
            </button>
          ))}
        </div>
      </div>

      {asrBackend === "whisper_cpp" ? (
        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground uppercase tracking-wider">
            Whisper Model
          </Label>
          <div className="grid grid-cols-3 gap-1.5">
            {WHISPER_MODELS.map((model) => (
              <button
                key={model.id}
                type="button"
                onClick={() => onWhisperModelChange(model.id)}
                disabled={disabled}
                className={cn(
                  "space-y-1 border px-3 py-2.5 text-left text-xs transition-colors",
                  whisperModel === model.id
                    ? "border-primary bg-primary/10 text-foreground"
                    : "border-border text-muted-foreground hover:border-muted-foreground hover:text-foreground",
                  disabled && "cursor-not-allowed opacity-50"
                )}
              >
                <div className="font-medium">{model.name}</div>
                <div className="text-[10px] opacity-70">{model.size}</div>
              </button>
            ))}
          </div>
          <p className="text-[11px] text-muted-foreground">
            {WHISPER_MODELS.find((model) => model.id === whisperModel)?.desc ??
              "Select a fallback model"}
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          <Label className="text-xs text-muted-foreground uppercase tracking-wider">
            ASR Model
          </Label>
          <div className="grid gap-2 sm:grid-cols-2">
            {asrModelOptions.map((model) => (
              <button
                key={model.id}
                type="button"
                disabled={disabled}
                onClick={() => onAsrModelChange(model.id)}
                className={cn(
                  "space-y-1 border px-3 py-3 text-left text-xs transition-colors",
                  asrModelId === model.id
                    ? "border-primary bg-primary/10 text-foreground"
                    : "border-border text-muted-foreground hover:border-muted-foreground hover:text-foreground",
                  disabled && "cursor-not-allowed opacity-50"
                )}
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="font-medium">{model.name}</span>
                  <span className="text-[10px] uppercase tracking-wider text-muted-foreground">
                    {model.meta}
                  </span>
                </div>
                <div className="text-[10px] leading-4">{model.desc}</div>
              </button>
            ))}
          </div>
        </div>
      )}

      <div className="space-y-2">
        <Label className="text-xs text-muted-foreground uppercase tracking-wider">
          Translation Backend
        </Label>
        <div className="grid gap-2 sm:grid-cols-3">
          {translationChoices.map((backend) => (
            <button
              key={backend.id}
              type="button"
              disabled={disabled || !backend.installed}
              onClick={() => onTranslationBackendChange(backend.id)}
              className={cn(
                "space-y-1 border px-3 py-3 text-left text-xs transition-colors",
                translationBackend === backend.id
                  ? "border-primary bg-primary/10 text-foreground"
                  : "border-border text-muted-foreground hover:border-muted-foreground hover:text-foreground",
                (!backend.installed || disabled) && "cursor-not-allowed opacity-50"
              )}
            >
              <div className="font-medium">{backend.label}</div>
              <div className="text-[10px] leading-4">{backend.description}</div>
            </button>
          ))}
        </div>
      </div>

      <div className="flex items-center justify-between border border-border px-3 py-3">
        <div className="space-y-0.5">
          <Label className="text-xs uppercase tracking-wider text-muted-foreground">
            Speaker Diarization
          </Label>
          <p className="text-[11px] text-muted-foreground">
            Add speaker labels and break translation blocks on speaker changes
          </p>
        </div>
        <Switch
          checked={diarizationEnabled}
          onCheckedChange={onDiarizationChange}
          disabled={disabled}
        />
      </div>
    </div>
  );
}
