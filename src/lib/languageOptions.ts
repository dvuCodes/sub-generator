export interface LanguageOption {
  code: string;
  name: string;
}

const SOURCE_LANGUAGES: LanguageOption[] = [
  { code: "auto", name: "Auto-detect" },
  { code: "en", name: "English" },
  { code: "ja", name: "Japanese" },
];

const TARGET_LANGUAGES: LanguageOption[] = [
  { code: "en", name: "English" },
  { code: "ja", name: "Japanese" },
];

export function buildLanguageOptions() {
  return {
    source: SOURCE_LANGUAGES,
    target: TARGET_LANGUAGES,
  };
}
