import * as assert from "node:assert/strict";

import {
  advanceProcessingState,
  createInitialProcessingState,
} from "./processingState";

const transcribing = advanceProcessingState(createInitialProcessingState(), {
  type: "progress",
  stage: "transcribing",
  percent: 65,
  message: "Transcribing speech...",
  elapsed_secs: 42,
});

const laterDownload = advanceProcessingState(transcribing, {
  type: "progress",
  stage: "downloading_model",
  percent: 15,
  message: "Downloading translation model...",
});

assert.equal(laterDownload.stage, "transcribing");
assert.equal(laterDownload.message, "Downloading translation model...");
assert.equal(laterDownload.percent, 15);
