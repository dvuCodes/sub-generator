import {
  DEFAULT_SOURCE_LANGUAGE_OPTIONS,
  DEFAULT_TARGET_LANGUAGE_OPTIONS,
  type LanguageOption,
} from "../lib/languages";

interface LanguageSelectorProps {
  sourceLang: string;
  targetLang: string;
  onSourceChange: (lang: string) => void;
  onTargetChange: (lang: string) => void;
  sourceLanguages?: LanguageOption[];
  targetLanguages?: LanguageOption[];
  translationStatus?: string;
  disabled?: boolean;
}

export function LanguageSelector({
  sourceLang,
  targetLang,
  onSourceChange,
  onTargetChange,
  sourceLanguages,
  targetLanguages,
  translationStatus,
  disabled,
}: LanguageSelectorProps) {
  const availableSourceLanguages = sourceLanguages?.length
    ? sourceLanguages
    : DEFAULT_SOURCE_LANGUAGE_OPTIONS;
  const availableTargetLanguages = targetLanguages?.length
    ? targetLanguages
    : DEFAULT_TARGET_LANGUAGE_OPTIONS;

  return (
    <div className="space-y-2">
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className="block text-sm text-gray-400 mb-1">
            Source Language
          </label>
          <select
            value={sourceLang}
            onChange={(e) => onSourceChange(e.target.value)}
            disabled={disabled}
            className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-gray-200 focus:border-blue-500 focus:outline-none disabled:opacity-50"
          >
            {availableSourceLanguages.map((lang) => (
              <option key={lang.code} value={lang.code}>
                {lang.name}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-sm text-gray-400 mb-1">
            Target Language
          </label>
          <select
            value={targetLang}
            onChange={(e) => onTargetChange(e.target.value)}
            disabled={disabled}
            className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-gray-200 focus:border-blue-500 focus:outline-none disabled:opacity-50"
          >
            {availableTargetLanguages.map((lang) => (
              <option key={lang.code} value={lang.code}>
                {lang.name}
              </option>
            ))}
          </select>
        </div>
      </div>

      {translationStatus ? (
        <p className="text-xs text-gray-500">{translationStatus}</p>
      ) : null}
    </div>
  );
}
