import type { LanguagePair } from "./types";

export interface LanguageOption {
  code: string;
  name: string;
}

const LANGUAGE_LABELS: Record<string, string> = {
  auto: "Auto-detect",
  ar: "Arabic",
  cs: "Czech",
  da: "Danish",
  de: "German",
  el: "Greek",
  en: "English",
  es: "Spanish",
  fi: "Finnish",
  fr: "French",
  hi: "Hindi",
  hu: "Hungarian",
  id: "Indonesian",
  it: "Italian",
  ja: "Japanese",
  ko: "Korean",
  ms: "Malay",
  nl: "Dutch",
  pl: "Polish",
  pt: "Portuguese",
  ro: "Romanian",
  ru: "Russian",
  sv: "Swedish",
  th: "Thai",
  tl: "Filipino",
  tr: "Turkish",
  uk: "Ukrainian",
  vi: "Vietnamese",
  zh: "Chinese",
};

function labelForLanguage(code: string) {
  return LANGUAGE_LABELS[code] ?? code.toUpperCase();
}

export function buildLanguageOptions(pairs: LanguagePair[]) {
  if (pairs.length === 0) {
    return {
      source: [],
      target: [],
    };
  }

  const sourceCodes = new Set<string>(Object.keys(LANGUAGE_LABELS));
  const targetCodes = new Set<string>();

  for (const pair of pairs) {
    sourceCodes.add(pair.source);
    targetCodes.add(pair.target);
  }

  const toOptions = (codes: Set<string>) =>
    Array.from(codes)
      .sort((left, right) =>
        labelForLanguage(left).localeCompare(labelForLanguage(right))
      )
      .map((code) => ({ code, name: labelForLanguage(code) }));

  return {
    source: toOptions(sourceCodes),
    target: toOptions(targetCodes),
  };
}
