import { describe, expect, it } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

function readJson(path: string) {
  return JSON.parse(readFileSync(path, "utf8"));
}

describe("Tauri shell open configuration", () => {
  it("allows the completion-state Open Folder action to open native local paths", () => {
    const repoRoot = process.cwd();
    const tauriConfig = readJson(join(repoRoot, "src-tauri", "tauri.conf.json"));
    const capability = readJson(
      join(repoRoot, "src-tauri", "capabilities", "default.json")
    );

    expect(typeof tauriConfig.plugins?.shell?.open).toBe("string");
    const openRegex = new RegExp(tauriConfig.plugins.shell.open);

    expect(openRegex.test("C:\\Users\\datvu\\SubGen")).toBe(true);
    expect(openRegex.test("C:\\")).toBe(true);
    expect(openRegex.test("\\\\server\\share\\SubGen")).toBe(true);
    expect(openRegex.test("/tmp/subgen")).toBe(true);
    expect(openRegex.test("/")).toBe(true);
    expect(openRegex.test("https://example.com")).toBe(false);
    expect(capability.permissions).toContain("shell:allow-open");
  });
});
