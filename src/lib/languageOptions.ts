import {
  findASRBackendCapability,
  findTranslationBackendCapability,
} from "./backendOptions";
import type {
  ASRBackend,
  CapabilitiesResponse,
  LanguageOption,
  TranslationBackend,
} from "./types";

const ALL_LANGUAGES: LanguageOption[] = [
  { code: "en", name: "English" },
  { code: "ja", name: "Japanese" },
  { code: "zh", name: "Chinese" },
  { code: "ko", name: "Korean" },
  { code: "es", name: "Spanish" },
  { code: "fr", name: "French" },
  { code: "de", name: "German" },
  { code: "pt", name: "Portuguese" },
  { code: "ru", name: "Russian" },
  { code: "ar", name: "Arabic" },
  { code: "hi", name: "Hindi" },
  { code: "vi", name: "Vietnamese" },
  { code: "th", name: "Thai" },
  { code: "it", name: "Italian" },
  { code: "nl", name: "Dutch" },
  { code: "pl", name: "Polish" },
  { code: "tr", name: "Turkish" },
  { code: "sv", name: "Swedish" },
  { code: "da", name: "Danish" },
  { code: "fi", name: "Finnish" },
  { code: "no", name: "Norwegian" },
  { code: "cs", name: "Czech" },
  { code: "el", name: "Greek" },
  { code: "he", name: "Hebrew" },
  { code: "hu", name: "Hungarian" },
  { code: "id", name: "Indonesian" },
  { code: "ms", name: "Malay" },
  { code: "ro", name: "Romanian" },
  { code: "sk", name: "Slovak" },
  { code: "uk", name: "Ukrainian" },
  { code: "bg", name: "Bulgarian" },
  { code: "hr", name: "Croatian" },
  { code: "lt", name: "Lithuanian" },
  { code: "lv", name: "Latvian" },
  { code: "et", name: "Estonian" },
  { code: "sl", name: "Slovenian" },
  { code: "sr", name: "Serbian" },
  { code: "ca", name: "Catalan" },
  { code: "gl", name: "Galician" },
  { code: "eu", name: "Basque" },
  { code: "mk", name: "Macedonian" },
  { code: "sq", name: "Albanian" },
  { code: "ka", name: "Georgian" },
  { code: "hy", name: "Armenian" },
  { code: "az", name: "Azerbaijani" },
  { code: "kk", name: "Kazakh" },
  { code: "uz", name: "Uzbek" },
  { code: "tl", name: "Filipino" },
  { code: "sw", name: "Swahili" },
  { code: "ta", name: "Tamil" },
  { code: "te", name: "Telugu" },
  { code: "bn", name: "Bengali" },
  { code: "ur", name: "Urdu" },
  { code: "fa", name: "Persian" },
  { code: "ne", name: "Nepali" },
  { code: "si", name: "Sinhala" },
  { code: "my", name: "Myanmar" },
];

const SOURCE_LANGUAGES: LanguageOption[] = [
  { code: "auto", name: "Auto-detect" },
  ...ALL_LANGUAGES,
];

function ensureSourceLanguages(
  languages: LanguageOption[] | undefined
): LanguageOption[] {
  if (!languages?.length) {
    return SOURCE_LANGUAGES;
  }

  const merged = new Map<string, LanguageOption>();
  for (const language of SOURCE_LANGUAGES) {
    merged.set(language.code, language);
  }
  for (const language of languages) {
    if (language.code !== "auto" && !merged.has(language.code)) {
      merged.set(language.code, language);
    }
  }

  return Array.from(merged.values());
}

function ensureTargetLanguages(
  languages: LanguageOption[] | undefined
): LanguageOption[] {
  return languages?.length
    ? languages.filter((language) => language.code !== "auto")
    : ALL_LANGUAGES;
}

export function buildLanguageOptions(
  capabilities: CapabilitiesResponse | null,
  asrBackend: ASRBackend,
  translationBackend: TranslationBackend
): { source: LanguageOption[]; target: LanguageOption[] } {
  const asr = findASRBackendCapability(capabilities, asrBackend);
  const translation =
    translationBackend === "none"
      ? undefined
      : findTranslationBackendCapability(capabilities, translationBackend);

  return {
    source: ensureSourceLanguages(asr?.source_languages),
    target: ensureTargetLanguages(translation?.target_languages),
  };
}
