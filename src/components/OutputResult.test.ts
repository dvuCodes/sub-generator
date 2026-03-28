import { describe, expect, it } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { OutputResult } from "./OutputResult";

describe("OutputResult", () => {
  it("renders backend and diarization metadata", () => {
    const html = renderToStaticMarkup(
      createElement(OutputResult, {
        outputPath: "C:/tmp/out.srt",
        transcriptionLog: "C:/tmp/out.txt",
        segments: 42,
        durationSecs: 185,
        backendSummary: "faster_whisper + nllb + diarization",
        selectedASRBackend: "faster_whisper",
        diarizationRan: true,
        speakerCount: 3,
        onReset: () => {},
      })
    );

    expect(html).toContain("faster_whisper + nllb + diarization");
    expect(html).toContain("3");
    expect(html).toContain("Speaker Labels");
  });
});
