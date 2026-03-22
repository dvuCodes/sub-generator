import { useState, useEffect, useCallback } from "react";
import { useSidecar } from "./hooks/useSidecar";
import { VideoDropzone } from "./components/VideoDropzone";
import { LanguageSelector } from "./components/LanguageSelector";
import { ModelSelector } from "./components/ModelSelector";
import { FormatSelector } from "./components/FormatSelector";
import { SettingsPanel } from "./components/SettingsPanel";
import { ProcessingView } from "./components/ProcessingView";
import { OutputResult } from "./components/OutputResult";
import type { SidecarResponse } from "./lib/types";

type AppState = "idle" | "processing" | "complete" | "error";

interface ProcessingState {
  stage: string;
  percent: number;
  message: string;
}

interface CompletionState {
  outputPath: string;
  segments: number;
  durationSecs: number;
}

function App() {
  const { connected, connecting, connect, sendCommand, onResponse } =
    useSidecar();

  // Form state
  const [videoPath, setVideoPath] = useState<string | null>(null);
  const [sourceLang, setSourceLang] = useState("auto");
  const [targetLang, setTargetLang] = useState("");
  const [model, setModel] = useState("base");
  const [format, setFormat] = useState("srt");
  const [beamSize, setBeamSize] = useState(5);
  const [vadFilter, setVadFilter] = useState(true);

  // App state
  const [appState, setAppState] = useState<AppState>("idle");
  const [processing, setProcessing] = useState<ProcessingState>({
    stage: "",
    percent: 0,
    message: "",
  });
  const [completion, setCompletion] = useState<CompletionState | null>(null);
  const [errorMsg, setErrorMsg] = useState("");

  // Auto-connect sidecar on mount
  useEffect(() => {
    connect().catch((err) => {
      setErrorMsg(`Failed to start backend: ${err}`);
      setAppState("error");
    });
  }, [connect]);

  // Handle sidecar responses
  useEffect(() => {
    onResponse((response: SidecarResponse) => {
      switch (response.type) {
        case "progress":
          setProcessing({
            stage: response.stage,
            percent: response.percent,
            message: response.message,
          });
          break;
        case "stage":
          setProcessing((prev) => ({
            ...prev,
            stage: response.stage,
            message: response.message,
          }));
          break;
        case "complete":
          setCompletion({
            outputPath: response.output_path,
            segments: response.segments,
            durationSecs: response.duration_secs,
          });
          setAppState("complete");
          break;
        case "error":
          setErrorMsg(response.message);
          setAppState("error");
          break;
      }
    });
  }, [onResponse]);

  const handleGenerate = useCallback(async () => {
    if (!videoPath || !connected) return;

    setAppState("processing");
    setProcessing({ stage: "validating", percent: 0, message: "Starting..." });
    setErrorMsg("");

    try {
      await sendCommand({
        command: "generate",
        input_video: videoPath,
        source_lang: sourceLang === "auto" ? null : sourceLang,
        target_lang: targetLang || null,
        output_format: format as "srt" | "ass" | "vtt",
        output_path: null,
        model_size: model as any,
        beam_size: beamSize,
        vad_filter: vadFilter,
      });
    } catch (err) {
      setErrorMsg(`Failed to send command: ${err}`);
      setAppState("error");
    }
  }, [
    videoPath,
    connected,
    sourceLang,
    targetLang,
    format,
    model,
    beamSize,
    vadFilter,
    sendCommand,
  ]);

  const handleReset = useCallback(() => {
    setAppState("idle");
    setVideoPath(null);
    setCompletion(null);
    setErrorMsg("");
    setProcessing({ stage: "", percent: 0, message: "" });
  }, []);

  const isProcessing = appState === "processing";

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      {/* Header */}
      <header className="border-b border-gray-800 px-6 py-4">
        <div className="max-w-2xl mx-auto flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">SubGen</h1>
            <p className="text-xs text-gray-500">
              Local Video Subtitle Generator
            </p>
          </div>
          <div className="flex items-center gap-2">
            <div
              className={`w-2 h-2 rounded-full ${connected ? "bg-green-500" : connecting ? "bg-yellow-500 animate-pulse" : "bg-red-500"}`}
            />
            <span className="text-xs text-gray-500">
              {connected
                ? "Connected"
                : connecting
                  ? "Connecting..."
                  : "Disconnected"}
            </span>
          </div>
        </div>
      </header>

      {/* Main content */}
      <main className="max-w-2xl mx-auto px-6 py-8 space-y-6">
        {appState === "complete" && completion ? (
          <OutputResult
            outputPath={completion.outputPath}
            segments={completion.segments}
            durationSecs={completion.durationSecs}
            onReset={handleReset}
          />
        ) : (
          <>
            {/* Video selection */}
            <VideoDropzone
              selectedFile={videoPath}
              onFileSelect={setVideoPath}
              disabled={isProcessing}
            />

            {/* Language selection */}
            <LanguageSelector
              sourceLang={sourceLang}
              targetLang={targetLang}
              onSourceChange={setSourceLang}
              onTargetChange={setTargetLang}
              disabled={isProcessing}
            />

            {/* Model selection */}
            <ModelSelector
              model={model}
              onChange={setModel}
              disabled={isProcessing}
            />

            {/* Format selection */}
            <FormatSelector
              format={format}
              onChange={setFormat}
              disabled={isProcessing}
            />

            {/* Advanced settings */}
            <SettingsPanel
              beamSize={beamSize}
              vadFilter={vadFilter}
              onBeamSizeChange={setBeamSize}
              onVadFilterChange={setVadFilter}
              disabled={isProcessing}
            />

            {/* Processing view */}
            {isProcessing && (
              <ProcessingView
                stage={processing.stage}
                percent={processing.percent}
                message={processing.message}
              />
            )}

            {/* Error display */}
            {appState === "error" && errorMsg && (
              <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-4">
                <p className="text-red-400 text-sm">{errorMsg}</p>
                <button
                  onClick={() => setAppState("idle")}
                  className="mt-2 text-xs text-red-400 hover:text-red-300 underline"
                >
                  Dismiss
                </button>
              </div>
            )}

            {/* Generate button */}
            <button
              onClick={handleGenerate}
              disabled={!videoPath || !connected || isProcessing}
              className={`
                w-full py-3 rounded-lg font-medium text-lg transition-all
                ${
                  !videoPath || !connected || isProcessing
                    ? "bg-gray-700 text-gray-400 cursor-not-allowed"
                    : "bg-blue-600 hover:bg-blue-500 text-white"
                }
              `}
            >
              {isProcessing
                ? "Processing..."
                : !connected
                  ? "Waiting for backend..."
                  : "Generate Subtitles"}
            </button>
          </>
        )}
      </main>
    </div>
  );
}

export default App;
