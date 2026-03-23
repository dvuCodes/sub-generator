interface LanguageOption {
  code: string;
  name: string;
}

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

const LANGUAGES: LanguageOption[] = [
  { code: "auto", name: "Auto-detect" },
  { code: "en", name: "English" },
  { code: "ja", name: "Japanese" },
];

const TARGET_LANGUAGES = [
  { code: "", name: "No translation (transcribe only)" },
  { code: "en", name: "English" },
  { code: "ja", name: "Japanese" },
];

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
    : LANGUAGES;
  const availableTargetLanguages = targetLanguages?.length
    ? [{ code: "", name: "No translation (transcribe only)" }, ...targetLanguages]
    : TARGET_LANGUAGES;

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
