import { useCallback, useEffect, useState } from "react";
import { FormatSelector } from "./components/FormatSelector";
import { LanguageSelector } from "./components/LanguageSelector";
import { ModelSelector } from "./components/ModelSelector";
import { OutputResult } from "./components/OutputResult";
import { ProcessingView } from "./components/ProcessingView";
import { SettingsPanel } from "./components/SettingsPanel";
import { VideoDropzone } from "./components/VideoDropzone";
import { useSidecar } from "./hooks/useSidecar";
import { formatRuntimeError } from "./lib/runtimeError";
import type {
  GenerateCommand,
  LanguagePair,
  ModelSize,
  OutputFormat,
  SidecarResponse,
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
}

interface SystemInfoState {
  whisperServer: boolean;
  libretranslate: boolean;
  gpu: string;
}

const LANGUAGE_LABELS: Record<string, string> = {
  auto: "Auto-detect",
  ar: "Arabic",
  cs: "Czech",
  da: "Danish",
  de: "German",
  el: "Greek",
  en: "English",
  es: "Spanish",
  fi: "Finnish",
  fr: "French",
  hi: "Hindi",
  hu: "Hungarian",
  id: "Indonesian",
  it: "Italian",
  ja: "Japanese",
  ko: "Korean",
  ms: "Malay",
  nl: "Dutch",
  pl: "Polish",
  pt: "Portuguese",
  ro: "Romanian",
  ru: "Russian",
  sv: "Swedish",
  th: "Thai",
  tl: "Filipino",
  tr: "Turkish",
  uk: "Ukrainian",
  vi: "Vietnamese",
  zh: "Chinese",
};

function labelForLanguage(code: string) {
  return LANGUAGE_LABELS[code] ?? code.toUpperCase();
}

function buildLanguageOptions(pairs: LanguagePair[]) {
  if (pairs.length === 0) {
    return {
      source: [{ code: "auto", name: labelForLanguage("auto") }],
      target: [{ code: "", name: "No translation (transcribe only)" }],
    };
  }

  const sourceCodes = new Set<string>(["auto"]);
  const targetCodes = new Set<string>();

  for (const pair of pairs) {
    sourceCodes.add(pair.source);
    targetCodes.add(pair.target);
  }

  const toOptions = (codes: Set<string>) =>
    Array.from(codes)
      .sort((left, right) =>
        labelForLanguage(left).localeCompare(labelForLanguage(right))
      )
      .map((code) => ({ code, name: labelForLanguage(code) }));

  return {
    source: toOptions(sourceCodes),
    target: [
      { code: "", name: "No translation (transcribe only)" },
      ...toOptions(targetCodes),
    ],
  };
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
          });
          setAppState("complete");
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
                }
          );
          break;
        case "system_info":
          setSystemInfo({
            whisperServer: response.whisper_server,
            libretranslate: response.libretranslate,
            gpu: response.gpu,
          });
          break;
        case "error":
          setErrorMsg(formatRuntimeError(response.message, response.details));
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

  const handleGenerate = useCallback(async () => {
    if (!videoPath || !connected) return;

    setAppState("processing");
    setProcessing({ stage: "validating", percent: 0, message: "Starting..." });
    setErrorMsg("");

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
      };
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
              onBeamSizeChange={setBeamSize}
              onVadFilterChange={setVadFilter}
              disabled={isProcessing}
            />

            {isProcessing && (
              <ProcessingView
                stage={processing.stage}
                percent={processing.percent}
                message={processing.message}
              />
            )}

            {appState === "error" && errorMsg && (
              <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4">
                <p className="text-sm text-red-400">{errorMsg}</p>
                <button
                  onClick={() => setAppState("idle")}
                  className="mt-2 text-xs text-red-400 underline hover:text-red-300"
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
