import * as assert from "node:assert/strict";

import { deriveOutputDirectory, explorerOpenTarget } from "./outputPath";

assert.equal(deriveOutputDirectory("/tmp/out/movie.srt"), "/tmp/out");
assert.equal(deriveOutputDirectory("C:\\movie.srt"), "C:\\");
assert.equal(explorerOpenTarget("/tmp/out/movie.srt"), "/tmp/out");
assert.equal(
  explorerOpenTarget("C:\\Users\\example\\movie.srt"),
  "C:\\Users\\example"
);
assert.equal(explorerOpenTarget("C:\\movie.srt"), "C:\\");
