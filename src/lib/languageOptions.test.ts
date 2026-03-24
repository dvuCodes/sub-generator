import { describe, expect, it } from "bun:test";
import { buildLanguageOptions } from "./languageOptions";

describe("buildLanguageOptions", () => {
  it("returns source and target language lists with 55+ languages", () => {
    const opts = buildLanguageOptions();

    // Source should have Auto-detect first, then all languages
    expect(opts.source[0]).toEqual({ code: "auto", name: "Auto-detect" });
    expect(opts.source.length).toBeGreaterThan(55);

    // Target should have all languages (no Auto-detect)
    expect(opts.target.length).toBeGreaterThan(54);
    expect(opts.target.find((l) => l.code === "auto")).toBeUndefined();

    // Known languages should be present
    expect(opts.source.find((l) => l.code === "en")).toEqual({ code: "en", name: "English" });
    expect(opts.source.find((l) => l.code === "ja")).toEqual({ code: "ja", name: "Japanese" });
    expect(opts.target.find((l) => l.code === "ko")).toEqual({ code: "ko", name: "Korean" });
  });
});
