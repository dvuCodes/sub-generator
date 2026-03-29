import { describe, expect, it } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { SettingsPanel } from "./SettingsPanel";

describe("SettingsPanel", () => {
  it("keeps beam and VAD controls without audio enhancement options", () => {
    const html = renderToStaticMarkup(
      createElement(SettingsPanel, {
        beamSize: 5,
        vadFilter: true,
        onBeamSizeChange: () => {},
        onVadFilterChange: () => {},
        defaultOpen: true,
      })
    );

    expect(html).toContain("Beam Size");
    expect(html).toContain("VAD Filter");
    expect(html).not.toContain("Audio Enhancement");
    expect(html).not.toContain("Preprocess audio to improve transcription");
  });
});
