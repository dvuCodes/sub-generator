import { useState } from "react";
import type { HonorificsMode, TranslationEngine } from "../lib/types";

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
  disabled?: boolean;
}

const inputClassName =
  "w-full rounded-lg border border-gray-700 bg-gray-900 px-3 py-2 text-sm text-gray-200 placeholder-gray-600 focus:border-blue-500 focus:outline-none disabled:opacity-50";

const labelClassName = "block text-sm text-gray-400 mb-1";

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
  disabled,
}: SettingsPanelProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [isAdvancedOpen, setIsAdvancedOpen] = useState(false);

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

          <div className="border border-gray-700 rounded-lg">
            <button
              onClick={() => setIsAdvancedOpen(!isAdvancedOpen)}
              disabled={disabled}
              className="w-full px-4 py-3 flex items-center justify-between text-gray-300 hover:text-gray-100 transition-colors"
            >
              <span className="text-sm font-medium">Advanced</span>
              <span
                className={`transform transition-transform ${isAdvancedOpen ? "rotate-180" : ""}`}
              >
                &#9660;
              </span>
            </button>

            {isAdvancedOpen && (
              <div className="px-4 pb-4 space-y-4 border-t border-gray-700 pt-4">
                <div>
                  <label className={labelClassName}>Initial Prompt</label>
                  <input
                    type="text"
                    value={initialPrompt}
                    onChange={(e) => onInitialPromptChange(e.target.value)}
                    placeholder="Domain terms e.g. character names"
                    disabled={disabled}
                    className={inputClassName}
                  />
                </div>

                <div>
                  <label className={labelClassName}>Translation Engine</label>
                  <select
                    value={translationEngine}
                    onChange={(e) =>
                      onTranslationEngineChange(
                        e.target.value as TranslationEngine
                      )
                    }
                    disabled={disabled}
                    className={inputClassName}
                  >
                    <option value="auto">Auto</option>
                    <option value="libretranslate">LibreTranslate</option>
                    <option value="deepl">DeepL</option>
                    <option value="llm">LLM (OpenAI-compatible)</option>
                  </select>
                </div>

                {translationEngine === "deepl" && (
                  <div>
                    <label className={labelClassName}>DeepL API Key</label>
                    <input
                      type="password"
                      value={deeplApiKey}
                      onChange={(e) => onDeeplApiKeyChange(e.target.value)}
                      placeholder="DeepL API key"
                      disabled={disabled}
                      className={inputClassName}
                    />
                  </div>
                )}

                {translationEngine === "llm" && (
                  <>
                    <div>
                      <label className={labelClassName}>LLM Base URL</label>
                      <input
                        type="text"
                        value={llmBaseUrl}
                        onChange={(e) => onLlmBaseUrlChange(e.target.value)}
                        placeholder="http://127.0.0.1:11434"
                        disabled={disabled}
                        className={inputClassName}
                      />
                    </div>
                    <div>
                      <label className={labelClassName}>LLM Model</label>
                      <input
                        type="text"
                        value={llmModel}
                        onChange={(e) => onLlmModelChange(e.target.value)}
                        placeholder="qwen2.5:7b-instruct"
                        disabled={disabled}
                        className={inputClassName}
                      />
                    </div>
                    <div>
                      <label className={labelClassName}>
                        LLM API Key (optional)
                      </label>
                      <input
                        type="password"
                        value={llmApiKey}
                        onChange={(e) => onLlmApiKeyChange(e.target.value)}
                        placeholder="API key if required"
                        disabled={disabled}
                        className={inputClassName}
                      />
                    </div>
                  </>
                )}

                <div>
                  <label className={labelClassName}>Synopsis</label>
                  <textarea
                    rows={2}
                    value={synopsis}
                    onChange={(e) => onSynopsisChange(e.target.value)}
                    placeholder="Brief context about the content"
                    disabled={disabled}
                    className={`${inputClassName} resize-y`}
                  />
                </div>

                <div>
                  <label className={labelClassName}>Glossary</label>
                  <textarea
                    rows={3}
                    value={glossaryText}
                    onChange={(e) => onGlossaryTextChange(e.target.value)}
                    placeholder={"One entry per line:\nsource = target"}
                    disabled={disabled}
                    className={`${inputClassName} resize-y font-mono`}
                  />
                  <p className="mt-1 text-xs text-gray-500">
                    One entry per line, format: source = target
                  </p>
                </div>

                <div>
                  <label className={labelClassName}>Honorifics</label>
                  <select
                    value={honorifics}
                    onChange={(e) =>
                      onHonorificsChange(e.target.value as HonorificsMode)
                    }
                    disabled={disabled}
                    className={inputClassName}
                  >
                    <option value="keep">Keep</option>
                    <option value="drop">Drop</option>
                  </select>
                </div>

                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={qaPass}
                    onChange={(e) => onQaPassChange(e.target.checked)}
                    disabled={disabled}
                    className="h-4 w-4 accent-blue-500"
                  />
                  <span className="text-sm text-gray-300">
                    QA review pass
                  </span>
                </label>

                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={shotSnap}
                    onChange={(e) => onShotSnapChange(e.target.checked)}
                    disabled={disabled}
                    className="h-4 w-4 accent-blue-500"
                  />
                  <span className="text-sm text-gray-300">
                    Snap subtitles to scene cuts
                  </span>
                </label>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
