import { describe, expect, it } from "bun:test";
import {
  advanceProcessingState,
  createInitialProcessingState,
} from "./processingState";

describe("advanceProcessingState", () => {
  it("clears the previous percent when a new stage starts", () => {
    const next = advanceProcessingState(
      {
        stage: "starting_services",
        percent: 100,
        message: "All services ready",
      },
      {
        type: "stage",
        stage: "transcribing",
        message: "Transcribing speech...",
      }
    );

    expect(next).toEqual({
      stage: "transcribing",
      percent: null,
      message: "Transcribing speech...",
    });
  });

  it("keeps the current percent when the same stage message updates", () => {
    const next = advanceProcessingState(
      {
        stage: "transcribing",
        percent: 42,
        message: "Chunk 2/5",
      },
      {
        type: "stage",
        stage: "transcribing",
        message: "Still transcribing...",
      }
    );

    expect(next).toEqual({
      stage: "transcribing",
      percent: 42,
      message: "Still transcribing...",
    });
  });

  it("stores determinate progress updates for the active stage", () => {
    const next = advanceProcessingState(createInitialProcessingState(), {
      type: "progress",
      stage: "translating",
      percent: 61.7,
      message: "Translated 8/13 segments",
    });

    expect(next).toEqual({
      stage: "translating",
      percent: 61.7,
      message: "Translated 8/13 segments",
    });
  });
});
