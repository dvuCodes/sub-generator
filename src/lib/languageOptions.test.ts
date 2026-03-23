import { describe, expect, it } from "bun:test";
import { buildLanguageOptions } from "./languageOptions";

describe("buildLanguageOptions", () => {
  it("returns empty option lists when the backend has no pairs", () => {
    expect(buildLanguageOptions([])).toEqual({
      source: [],
      target: [],
    });
  });

  it("keeps the full built-in source list when translation pairs are sparse", () => {
    expect(
      buildLanguageOptions([
        { source: "ja", target: "en" },
        { source: "en", target: "fr" },
      ])
    ).toEqual({
      source: [
        { code: "ar", name: "Arabic" },
        { code: "auto", name: "Auto-detect" },
        { code: "zh", name: "Chinese" },
        { code: "cs", name: "Czech" },
        { code: "da", name: "Danish" },
        { code: "nl", name: "Dutch" },
        { code: "en", name: "English" },
        { code: "tl", name: "Filipino" },
        { code: "fi", name: "Finnish" },
        { code: "fr", name: "French" },
        { code: "de", name: "German" },
        { code: "el", name: "Greek" },
        { code: "hi", name: "Hindi" },
        { code: "hu", name: "Hungarian" },
        { code: "id", name: "Indonesian" },
        { code: "it", name: "Italian" },
        { code: "ja", name: "Japanese" },
        { code: "ko", name: "Korean" },
        { code: "ms", name: "Malay" },
        { code: "pl", name: "Polish" },
        { code: "pt", name: "Portuguese" },
        { code: "ro", name: "Romanian" },
        { code: "ru", name: "Russian" },
        { code: "es", name: "Spanish" },
        { code: "sv", name: "Swedish" },
        { code: "th", name: "Thai" },
        { code: "tr", name: "Turkish" },
        { code: "uk", name: "Ukrainian" },
        { code: "vi", name: "Vietnamese" },
      ],
      target: [
        { code: "en", name: "English" },
        { code: "fr", name: "French" },
      ],
    });
  });
});
