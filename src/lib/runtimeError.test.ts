import { describe, expect, it } from "bun:test";
import { formatRuntimeError } from "./runtimeError";

describe("formatRuntimeError", () => {
  it("turns a whisper-server PATH failure into setup guidance", () => {
    const error = formatRuntimeError(
      "Service startup failed",
      'whisper-server: whisper-server executable "whisper-server" not found in PATH'
    );

    expect(error).toContain("Service startup failed:");
    expect(error).toContain("whisper-server is not installed");
    expect(error).toContain("services\\whisper-server\\whisper-server.exe");
    expect(error).toContain("services\\whisper-server\\models\\ggml-base.bin");
  });

  it("keeps unrelated errors unchanged", () => {
    expect(formatRuntimeError("Validation failed", "unsupported video format"))
      .toBe("Validation failed: unsupported video format");
  });

  it("turns a llama-server PATH failure into setup guidance", () => {
    const error = formatRuntimeError(
      "Translation engine startup failed",
      'llama-server executable "llama-server" not found in PATH'
    );

    expect(error).toContain("Translation engine startup failed:");
    expect(error).toContain("llama-server is required for translation");
    expect(error).toContain("services\\llama-server\\llama-server.exe");
  });
});
