import { describe, expect, it } from "bun:test";
import {
  advanceProcessingState,
  createInitialProcessingState,
} from "./processingState";

describe("advanceProcessingState", () => {
  it("does not regress to an earlier stage", () => {
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

    expect(laterDownload.stage).toBe("transcribing");
    expect(laterDownload.message).toBe("Downloading translation model...");
    expect(laterDownload.percent).toBe(15);
  });

  it("advances into the diarization stage", () => {
    const transcribing = advanceProcessingState(createInitialProcessingState(), {
      type: "stage",
      stage: "transcribing",
      message: "Running ASR...",
    });

    const diarizing = advanceProcessingState(transcribing, {
      type: "progress",
      stage: "diarizing",
      percent: 30,
      message: "Labeling speakers...",
      elapsed_secs: 12,
    });

    expect(diarizing.stage).toBe("diarizing");
    expect(diarizing.percent).toBe(30);
  });
});
