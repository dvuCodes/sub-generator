import * as assert from "node:assert/strict";

import { deriveOutputDirectory, explorerOpenTarget } from "./outputPath";

assert.equal(deriveOutputDirectory("/tmp/out/movie.srt"), "/tmp/out");
assert.equal(deriveOutputDirectory("C:\\movie.srt"), "C:\\");
assert.equal(explorerOpenTarget("/tmp/out/movie.srt"), "file:///tmp/out");
assert.equal(
  explorerOpenTarget("C:\\Users\\datvu\\movie.srt"),
  "file:///C:/Users/datvu"
);
assert.equal(explorerOpenTarget("C:\\movie.srt"), "file:///C:/");
