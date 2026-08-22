import { describe, expect, it } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import App from "../App";

describe("App language selector", () => {
  it("keeps common source and target languages selectable before discovery completes", () => {
    const html = renderToStaticMarkup(createElement(App));

    expect(html).toContain(">English<");
    expect(html).toContain(">Japanese<");
    expect(html).toContain(">No translation (transcribe only)<");
  });
});
