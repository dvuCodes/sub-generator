import { useCallback, useEffect, useRef, useState } from "react";
import { listen } from "@tauri-apps/api/event";
import { FormatSelector } from "./components/FormatSelector";
import { LanguageSelector } from "./components/LanguageSelector";
import { ModelSelector } from "./components/ModelSelector";
import { OutputResult } from "./components/OutputResult";
import { ProcessingView } from "./components/ProcessingView";
import { SettingsPanel } from "./components/SettingsPanel";
import { VideoDropzone } from "./components/VideoDropzone";
import { useSidecar } from "./hooks/useSidecar";
import { buildLanguageOptions } from "./lib/languages";
import type {
  CueView,
  GenerateCommand,
  GlossaryEntry,
  HonorificsMode,
  LanguagePair,
  ModelSize,
  OutputFormat,
  QCReport,
  SidecarResponse,
  TranslationEngine,
} from "./lib/types";

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
  qc: QCReport | null;
  preview: CueView[] | null;
}

interface SystemInfoState {
  whisperServer: boolean;
  libretranslate: boolean;
  gpu: string;
  ffmpeg: boolean;
  vadModel: boolean;
}

function parseGlossary(text: string): GlossaryEntry[] {
  return text
    .split("\n")
    .map((line) => {
      // Split on the first "=" only; values may legitimately contain "=".
      const idx = line.indexOf("=");
      if (idx === -1) return null;
      const source = line.slice(0, idx).trim();
      const target = line.slice(idx + 1).trim();
      if (!source || !target) return null;
      return { source, target };
    })
    .filter((entry): entry is GlossaryEntry => entry !== null);
}

function App() {
  const { connected, connecting, connect, sendCommand, onResponse } =
    useSidecar();

  const [videoPath, setVideoPath] = useState<string | null>(null);
  const [sourceLang, setSourceLang] = useState("auto");
  const [targetLang, setTargetLang] = useState("");
  const [model, setModel] = useState<ModelSize>("base");
  const [format, setFormat] = useState<OutputFormat>("srt");
  const [beamSize, setBeamSize] = useState(5);
  const [vadFilter, setVadFilter] = useState(true);
  const [initialPrompt, setInitialPrompt] = useState("");
  const [frameRate] = useState<number | undefined>(undefined);
  const [translationEngine, setTranslationEngine] =
    useState<TranslationEngine>("auto");
  const [deeplApiKey, setDeeplApiKey] = useState("");
  const [llmBaseUrl, setLlmBaseUrl] = useState("");
  const [llmModel, setLlmModel] = useState("");
  const [llmApiKey, setLlmApiKey] = useState("");
  const [synopsis, setSynopsis] = useState("");
  const [glossaryText, setGlossaryText] = useState("");
  const [honorifics, setHonorifics] = useState<HonorificsMode>("keep");
  const [qaPass, setQaPass] = useState(false);
  const [shotSnap, setShotSnap] = useState(true);

  const [appState, setAppState] = useState<AppState>("idle");
  const [processing, setProcessing] = useState<ProcessingState>({
    stage: "",
    percent: 0,
    message: "",
  });
  const [completion, setCompletion] = useState<CompletionState | null>(null);
  const [systemInfo, setSystemInfo] = useState<SystemInfoState | null>(null);
  const [availablePairs, setAvailablePairs] = useState<LanguagePair[]>([]);
  const [errorMsg, setErrorMsg] = useState("");
  const [infoMsg, setInfoMsg] = useState("");
  const [cancelRequested, setCancelRequested] = useState(false);

  useEffect(() => {
    connect().catch((err) => {
      setErrorMsg(`Failed to start backend: ${err}`);
      setAppState("error");
    });
  }, [connect]);

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
            qc: response.qc ?? null,
            preview: response.preview ?? null,
          });
          setAppState("complete");
          break;
        case "cancelled":
          setProcessing({ stage: "", percent: 0, message: "" });
          setInfoMsg(response.message || "Generation cancelled");
          setAppState("idle");
          break;
        case "languages":
          setAvailablePairs(response.installed);
          setSystemInfo((prev) =>
            prev
              ? { ...prev, libretranslate: true }
              : {
                  whisperServer: false,
                  libretranslate: true,
                  gpu: "unknown",
                  ffmpeg: false,
                  vadModel: false,
                }
          );
          break;
        case "system_info":
          setSystemInfo({
            whisperServer: response.whisper_server,
            libretranslate: response.libretranslate,
            gpu: response.gpu,
            ffmpeg: response.ffmpeg,
            vadModel: response.vad_model,
          });
          break;
        case "error":
          setErrorMsg(
            response.details
              ? `${response.message}: ${response.details}`
              : response.message
          );
          setAppState("error");
          break;
      }
    });
  }, [onResponse]);

  useEffect(() => {
    if (!connected) {
      return;
    }

    sendCommand({ command: "system_info" }).catch((err) => {
      console.error("Failed to request system info:", err);
    });
    sendCommand({ command: "list_languages" }).catch((err) => {
      console.error("Failed to request language list:", err);
    });
  }, [connected, sendCommand]);

  // A sidecar crash mid-job would otherwise leave the UI frozen on
  // "Processing..." forever - surface it immediately when the process dies.
  const isProcessingRef = useRef(false);
  useEffect(() => {
    isProcessingRef.current = appState === "processing";
  }, [appState]);

  useEffect(() => {
    const unlisten = listen("sidecar-terminated", () => {
      if (!isProcessingRef.current) return;
      setProcessing({ stage: "", percent: 0, message: "" });
      setErrorMsg(
        "Backend connection lost - the running job was interrupted. It restarts automatically; try again."
      );
      setAppState("error");
    });
    return () => {
      unlisten.then((fn) => fn()).catch(() => {});
    };
  }, []);

  const handleGenerate = useCallback(async () => {
    if (!videoPath || !connected) return;

    setAppState("processing");
    setProcessing({ stage: "validating", percent: 0, message: "Starting..." });
    setErrorMsg("");
    setInfoMsg("");
    setCancelRequested(false);

    try {
      const command: GenerateCommand = {
        command: "generate",
        input_video: videoPath,
        source_lang: sourceLang === "auto" ? null : sourceLang,
        target_lang: targetLang || null,
        output_format: format,
        output_path: null,
        model_size: model,
        beam_size: beamSize,
        vad_filter: vadFilter,
        frame_rate: frameRate,
        translation_engine: translationEngine,
        honorifics: honorifics,
        qa_pass: qaPass,
        shot_snap: shotSnap,
      };
      if (initialPrompt.trim()) {
        command.initial_prompt = initialPrompt.trim();
      }
      if (translationEngine === "deepl" && deeplApiKey.trim()) {
        command.deepl_api_key = deeplApiKey.trim();
      }
      if (translationEngine === "llm") {
        if (llmBaseUrl.trim()) {
          command.llm_base_url = llmBaseUrl.trim();
        }
        if (llmModel.trim()) {
          command.llm_model = llmModel.trim();
        }
        if (llmApiKey.trim()) {
          command.llm_api_key = llmApiKey.trim();
        }
      }
      if (synopsis.trim()) {
        command.synopsis = synopsis.trim();
      }
      const glossary = parseGlossary(glossaryText);
      if (glossary.length > 0) {
        command.glossary = glossary;
      }
      await sendCommand(command);
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
    initialPrompt,
    frameRate,
    translationEngine,
    deeplApiKey,
    llmBaseUrl,
    llmModel,
    llmApiKey,
    synopsis,
    glossaryText,
    honorifics,
    qaPass,
    shotSnap,
    sendCommand,
  ]);

  const handleCancel = useCallback(() => {
    if (cancelRequested) return;
    setCancelRequested(true);
    sendCommand({ command: "cancel" }).catch((err) => {
      console.error("Failed to send cancel command:", err);
      setCancelRequested(false);
    });
  }, [cancelRequested, sendCommand]);

  const handleReset = useCallback(() => {
    setAppState("idle");
    setVideoPath(null);
    setCompletion(null);
    setErrorMsg("");
    setInfoMsg("");
    setProcessing({ stage: "", percent: 0, message: "" });
    setCancelRequested(false);
  }, []);

  const isProcessing = appState === "processing";
  const languageOptions = buildLanguageOptions(availablePairs);
  const translationStatus = systemInfo?.libretranslate
    ? `LibreTranslate ready. ${languageOptions.target.length - 1} translation targets available.`
    : "LibreTranslate will start automatically when translation is needed.";

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <header className="border-b border-gray-800 px-6 py-4">
        <div className="max-w-2xl mx-auto flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">SubGen</h1>
            <p className="text-xs text-gray-500">
              Local Video Subtitle Generator
            </p>
            <p className="mt-1 text-xs text-gray-600">
              Whisper: {systemInfo?.whisperServer ? "ready" : "idle"} |
              Translation: {systemInfo?.libretranslate ? "ready" : "idle"} |
              GPU: {systemInfo?.gpu || "unknown"}
              {systemInfo && (
                <>
                  {" | "}
                  {systemInfo.ffmpeg ? "FFmpeg ready" : "FFmpeg missing"}
                  {" | "}VAD model:{" "}
                  {systemInfo.vadModel ? "available" : "missing"}
                </>
              )}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <div
              className={`h-2 w-2 rounded-full ${connected ? "bg-green-500" : connecting ? "animate-pulse bg-yellow-500" : "bg-red-500"}`}
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

      <main className="max-w-2xl mx-auto px-6 py-8 space-y-6">
        {appState === "complete" && completion ? (
          <OutputResult
            outputPath={completion.outputPath}
            segments={completion.segments}
            durationSecs={completion.durationSecs}
            qc={completion.qc}
            preview={completion.preview}
            onReset={handleReset}
          />
        ) : (
          <>
            <VideoDropzone
              selectedFile={videoPath}
              onFileSelect={setVideoPath}
              disabled={isProcessing}
            />

            <LanguageSelector
              sourceLang={sourceLang}
              targetLang={targetLang}
              onSourceChange={setSourceLang}
              onTargetChange={setTargetLang}
              sourceLanguages={languageOptions.source}
              targetLanguages={languageOptions.target}
              translationStatus={translationStatus}
              disabled={isProcessing}
            />

            <ModelSelector
              model={model}
              onChange={setModel}
              disabled={isProcessing}
            />

            <FormatSelector
              format={format}
              onChange={setFormat}
              disabled={isProcessing}
            />

            <SettingsPanel
              beamSize={beamSize}
              vadFilter={vadFilter}
              initialPrompt={initialPrompt}
              translationEngine={translationEngine}
              deeplApiKey={deeplApiKey}
              llmBaseUrl={llmBaseUrl}
              llmModel={llmModel}
              llmApiKey={llmApiKey}
              synopsis={synopsis}
              glossaryText={glossaryText}
              honorifics={honorifics}
              qaPass={qaPass}
              shotSnap={shotSnap}
              onBeamSizeChange={setBeamSize}
              onVadFilterChange={setVadFilter}
              onInitialPromptChange={setInitialPrompt}
              onTranslationEngineChange={setTranslationEngine}
              onDeeplApiKeyChange={setDeeplApiKey}
              onLlmBaseUrlChange={setLlmBaseUrl}
              onLlmModelChange={setLlmModel}
              onLlmApiKeyChange={setLlmApiKey}
              onSynopsisChange={setSynopsis}
              onGlossaryTextChange={setGlossaryText}
              onHonorificsChange={setHonorifics}
              onQaPassChange={setQaPass}
              onShotSnapChange={setShotSnap}
              disabled={isProcessing}
            />

            {isProcessing && (
              <ProcessingView
                stage={processing.stage}
                percent={processing.percent}
                message={processing.message}
                onCancel={handleCancel}
                cancelRequested={cancelRequested}
              />
            )}

            {appState === "error" && errorMsg && (
              <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4">
                <p className="whitespace-pre-line text-sm text-red-400">
                  {errorMsg}
                </p>
                <button
                  onClick={() => {
                    setErrorMsg("");
                    setAppState("idle");
                  }}
                  className="mt-2 text-xs text-red-400 underline hover:text-red-300"
                >
                  Dismiss
                </button>
              </div>
            )}

            {!isProcessing && infoMsg && (
              <div className="rounded-lg border border-gray-600/50 bg-gray-800/50 p-4">
                <p className="text-sm text-gray-300">{infoMsg}</p>
                <button
                  onClick={() => setInfoMsg("")}
                  className="mt-2 text-xs text-gray-400 underline hover:text-gray-200"
                >
                  Dismiss
                </button>
              </div>
            )}

            <button
              onClick={handleGenerate}
              disabled={!videoPath || !connected || isProcessing}
              className={`
                w-full rounded-lg py-3 text-lg font-medium transition-all
                ${
                  !videoPath || !connected || isProcessing
                    ? "cursor-not-allowed bg-gray-700 text-gray-400"
                    : "bg-blue-600 text-white hover:bg-blue-500"
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
