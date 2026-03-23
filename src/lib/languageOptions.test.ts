import { describe, expect, it } from "bun:test";
import { buildLanguageOptions } from "./languageOptions";

describe("buildLanguageOptions", () => {
  it("returns source and target language lists", () => {
    const opts = buildLanguageOptions();
    expect(opts.source).toEqual([
      { code: "auto", name: "Auto-detect" },
      { code: "en", name: "English" },
      { code: "ja", name: "Japanese" },
    ]);
    expect(opts.target).toEqual([
      { code: "en", name: "English" },
      { code: "ja", name: "Japanese" },
    ]);
  });
});
