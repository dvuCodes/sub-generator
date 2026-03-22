import { describe, expect, it } from "bun:test";
import { pickDroppedVideoPath } from "./videoPath";

describe("pickDroppedVideoPath", () => {
  it("returns the first supported dropped path", () => {
    expect(
      pickDroppedVideoPath([
        "C:/Videos/clip.txt",
        "C:/Videos/episode.mkv",
        "C:/Videos/backup.mp4",
      ])
    ).toBe("C:/Videos/episode.mkv");
  });

  it("returns null when no supported file is present", () => {
    expect(
      pickDroppedVideoPath(["C:/Videos/readme.txt", "C:/Videos/cover.jpg"])
    ).toBeNull();
  });
});
