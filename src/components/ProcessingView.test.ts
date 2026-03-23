import { describe, expect, it } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { ProcessingView } from "./ProcessingView";

describe("ProcessingView", () => {
  it("renders a stop button while processing can be cancelled", () => {
    const html = renderToStaticMarkup(
      createElement(ProcessingView, {
        stage: "transcribing",
        percent: null,
        message: "Transcribing speech...",
        onStop: () => {},
        stopLabel: "Stop Processing",
      } as never)
    );

    expect(html).toContain("Stop Processing");
  });
});
