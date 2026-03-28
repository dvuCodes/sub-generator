import { describe, expect, it } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { SetupBanner } from "./SetupBanner";

describe("SetupBanner", () => {
  it("renders actionable install buttons for command actions", () => {
    const html = renderToStaticMarkup(
      createElement(SetupBanner, {
        setupStatus: {
          type: "setup_status",
          services: [
            {
              id: "ml-backend",
              display_name: "ml-backend",
              required_for: "transcription",
              state: "action_required",
              issues: [{ code: "binary_not_runnable" }],
              actions: [
                {
                  id: "ml-backend/install_python_dependencies",
                  label: "Install Python dependencies",
                  description: "Install missing ML backend Python packages.",
                  kind: "command",
                  preferred: true,
                },
              ],
            },
          ],
        },
        onInstall: () => {},
      } as never)
    );

    expect(html).toContain("Install Python dependencies");
  });

  it("renders manual action labels alongside setup guidance", () => {
    const html = renderToStaticMarkup(
      createElement(SetupBanner, {
        setupStatus: {
          type: "setup_status",
          services: [
            {
              id: "ml-backend",
              display_name: "ml-backend",
              required_for: "transcription",
              state: "action_required",
              issues: [{ code: "binary_not_found" }],
              actions: [
                {
                  id: "ml-backend/install_bundle",
                  label: "Install ML backend bundle",
                  description: "Use the bundled ML backend files.",
                  kind: "manual",
                  guidance:
                    "In dev, use python-backend/service.py or stage the tree into services/ml-backend/.",
                },
              ],
            },
          ],
        },
        onInstall: () => {},
      } as never)
    );

    expect(html).toContain("Install ML backend bundle");
    expect(html).toContain("python-backend/service.py");
  });
});
