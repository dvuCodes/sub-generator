import type { LanguagePair } from "./types";

export interface LanguageOption {
  code: string;
  name: string;
}

const NO_TRANSLATION_OPTION: LanguageOption = {
  code: "",
  name: "No translation (transcribe only)",
};

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

function sortLanguageCodes(codes: Iterable<string>) {
  return Array.from(codes).sort((left, right) =>
    labelForLanguage(left).localeCompare(labelForLanguage(right))
  );
}

function buildSourceOptions(codes: Set<string>) {
  const sourceCodes = new Set(codes);
  sourceCodes.delete("");
  sourceCodes.add("auto");

  const sortedCodes = sortLanguageCodes(sourceCodes).filter(
    (code) => code !== "auto"
  );

  return [
    { code: "auto", name: labelForLanguage("auto") },
    ...sortedCodes.map((code) => ({
      code,
      name: labelForLanguage(code),
    })),
  ];
}

function buildTargetOptions(codes: Set<string>) {
  const targetCodes = new Set(codes);
  targetCodes.delete("");
  targetCodes.delete("auto");

  return [
    NO_TRANSLATION_OPTION,
    ...sortLanguageCodes(targetCodes).map((code) => ({
      code,
      name: labelForLanguage(code),
    })),
  ];
}

const DEFAULT_LANGUAGE_CODES = new Set(Object.keys(LANGUAGE_LABELS));

export const DEFAULT_SOURCE_LANGUAGE_OPTIONS = buildSourceOptions(
  DEFAULT_LANGUAGE_CODES
);

export const DEFAULT_TARGET_LANGUAGE_OPTIONS = buildTargetOptions(
  DEFAULT_LANGUAGE_CODES
);

export function buildLanguageOptions(pairs: LanguagePair[]) {
  const sourceCodes = new Set(DEFAULT_LANGUAGE_CODES);
  const targetCodes = new Set(DEFAULT_LANGUAGE_CODES);

  for (const pair of pairs) {
    sourceCodes.add(pair.source);
    targetCodes.add(pair.target);
  }

  return {
    source: buildSourceOptions(sourceCodes),
    target: buildTargetOptions(targetCodes),
  };
}
