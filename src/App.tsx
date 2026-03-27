import { useCallback, useEffect, useRef, useState } from "react";
import { FormatSelector } from "./components/FormatSelector";
import { LanguageSelector } from "./components/LanguageSelector";
import { ModelSelector } from "./components/ModelSelector";
import { OutputResult } from "./components/OutputResult";
import { ProcessingView } from "./components/ProcessingView";
import { SettingsPanel } from "./components/SettingsPanel";
import { VideoDropzone } from "./components/VideoDropzone";
import { VramIndicator } from "./components/VramIndicator";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { useSidecar } from "./hooks/useSidecar";
import { useVramPolling } from "./hooks/useVramPolling";
import { buildLanguageOptions } from "./lib/languageOptions";
import { reduceSystemInfo, type SystemInfoState } from "./lib/systemInfo";
import {
  advanceProcessingState,
  createInitialProcessingState,
  type ProcessingState,
} from "./lib/processingState";
import { formatRuntimeError } from "./lib/runtimeError";
import { SetupBanner } from "./components/SetupBanner";
import { shouldDisableGenerate } from "./lib/setupHelpers";
import {
  advanceInstallState,
  createInitialInstallState,
  isInstallComplete,
  type InstallState,
} from "./lib/installState";
import type {
  AudioConfig,
  GenerateCommand,
  ModelSize,
  OutputFormat,
  SidecarResponse,
  SetupStatusResponse,
} from "./lib/types";
import { cn } from "@/lib/utils";
import { HugeiconsIcon } from "@hugeicons/react";
import { SubtitleIcon, SparklesIcon, Cancel01Icon } from "@hugeicons/core-free-icons";

type AppState = "idle" | "processing" | "complete" | "error" | "installing";

interface CompletionState {
  outputPath: string;
  transcriptionLog?: string;
  segments: number;
  durationSecs: number;
}

function App() {
  const { connected, connecting, connect, disconnect, sendCommand, onResponse } =
    useSidecar();

  const [videoPath, setVideoPath] = useState<string | null>(null);
  const [sourceLang, setSourceLang] = useState("auto");
  const [targetLang, setTargetLang] = useState("");
  const [model, setModel] = useState<ModelSize>("base");
  const [format, setFormat] = useState<OutputFormat>("srt");
  const [beamSize, setBeamSize] = useState(5);
  const [vadFilter, setVadFilter] = useState(true);
  const [audioConfig, setAudioConfig] = useState<AudioConfig>({
    enabled: true,
    vocal_boost_db: 3,
    noise_gate: true,
    normalize: true,
  });

  const [appState, setAppState] = useState<AppState>("idle");
  const [processing, setProcessing] = useState<ProcessingState>(
    createInitialProcessingState
  );
  const [completion, setCompletion] = useState<CompletionState | null>(null);
  const [systemInfo, setSystemInfo] = useState<SystemInfoState | null>(null);
  const [errorMsg, setErrorMsg] = useState("");
  const [translationWarning, setTranslationWarning] = useState("");
  const [isStopping, setIsStopping] = useState(false);
  const [setupStatus, setSetupStatus] = useState<SetupStatusResponse | null>(null);
  const [installState, setInstallState] = useState<InstallState | null>(null);

  const hasNvidiaGpu =
    systemInfo !== null &&
    systemInfo.gpu !== "none" &&
    systemInfo.gpu !== "unknown" &&
    systemInfo.gpu !== "";

  const { vram, handleVramResponse } = useVramPolling({
    enabled: connected && hasNvidiaGpu,
    sendCommand,
  });

  const handleVramResponseRef = useRef(handleVramResponse);
  handleVramResponseRef.current = handleVramResponse;

  const sendCommandRef = useRef(sendCommand);
  sendCommandRef.current = sendCommand;

  const appStateRef = useRef(appState);
  appStateRef.current = appState;

  const installStateRef = useRef(installState);
  installStateRef.current = installState;

  useEffect(() => {
    connect().catch((err) => {
      setErrorMsg(`Failed to start backend: ${err}`);
      setAppState("error");
    });
  }, [connect]);

  useEffect(() => {
    onResponse((response: SidecarResponse) => {
      switch (response.type) {
        case "setup_status":
          setSetupStatus(response as SetupStatusResponse);
          if (appStateRef.current === "installing" && installStateRef.current) {
            if (isInstallComplete(installStateRef.current, response as SetupStatusResponse)) {
              setAppState("idle");
              setInstallState(null);
            }
          }
          break;
        case "progress":
          if (appStateRef.current === "installing") {
            setInstallState((current) =>
              current ? advanceInstallState(current, response) : current
            );
          } else {
            setProcessing((current) => advanceProcessingState(current, response));
          }
          break;
        case "stage":
          if (appStateRef.current === "installing") {
            setInstallState((current) =>
              current ? advanceInstallState(current, response) : current
            );
          } else {
            setProcessing((current) => advanceProcessingState(current, response));
          }
          break;
        case "complete":
          setCompletion({
            outputPath: response.output_path,
            transcriptionLog: response.transcription_log,
            segments: response.segments,
            durationSecs: response.duration_secs,
          });
          setAppState("complete");
          sendCommandRef.current({ command: "stop_services" }).catch((err) => {
            console.error("Failed to stop services:", err);
          });
          break;
        case "languages":
          setTranslationWarning("");
          break;
        case "system_info":
          setSystemInfo((prev) => reduceSystemInfo(prev, response));
          if (response.translation_engine) {
            setTranslationWarning("");
          }
          break;
        case "vram_info":
          handleVramResponseRef.current(response.vram);
          break;
        case "error": {
          const formattedError = formatRuntimeError(
            response.message,
            response.details
          );

          if (response.message === "Failed to list languages") {
            setTranslationWarning(formattedError);
            break;
          }

          setErrorMsg(formattedError);
          setAppState("error");
          sendCommandRef.current({ command: "stop_services" }).catch((err) => {
            console.error("Failed to stop services:", err);
          });
          break;
        }
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
    sendCommand({ command: "check_setup" }).catch((err) => {
      console.error("Failed to check setup:", err);
    });
  }, [connected, sendCommand]);

  const languageOptions = buildLanguageOptions();

  const handleGenerate = useCallback(async () => {
    if (!videoPath || !connected) return;

    setAppState("processing");
    setProcessing({
      stage: "validating",
      percent: null,
      message: "Starting...",
      elapsedSecs: null,
      etaSecs: null,
    });
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
        audio_config: audioConfig,
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
    audioConfig,
    sendCommand,
  ]);

  const handleReset = useCallback(() => {
    setAppState("idle");
    setVideoPath(null);
    setCompletion(null);
    setErrorMsg("");
    setProcessing(createInitialProcessingState());
  }, []);

  const handleStopProcessing = useCallback(async () => {
    if (appState !== "processing" || isStopping) {
      return;
    }

    setIsStopping(true);
    setProcessing((current) => ({
      ...current,
      message: "Stopping processing...",
    }));

    try {
      await disconnect();
      setAppState("idle");
      setCompletion(null);
      setErrorMsg("");
      setProcessing(createInitialProcessingState());
      await connect();
    } catch (err) {
      setErrorMsg(`Failed to stop processing: ${err}`);
      setAppState("error");
    } finally {
      setIsStopping(false);
    }
  }, [appState, connect, disconnect, isStopping]);

  const handleInstall = useCallback(
    async (actionId: string) => {
      const serviceId = actionId.split("/")[0];
      setAppState("installing");
      setInstallState(createInitialInstallState(actionId, serviceId));
      setErrorMsg("");

      try {
        await sendCommand({ command: "install_dependency", action_id: actionId });
      } catch (err) {
        setErrorMsg(`Failed to start install: ${err}`);
        setAppState("error");
        setInstallState(null);
      }
    },
    [sendCommand]
  );

  const isProcessing = appState === "processing";
  const translationStatus = systemInfo?.translationEngine
    ? `GemmaTranslate ready. ${languageOptions.target.length} translation targets available.`
    : translationWarning ||
      "Translation engine will start automatically when needed.";

  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      {/* Header */}
      <header className="border-b border-border px-6 py-3">
        <div className="mx-auto flex max-w-2xl items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-8 items-center justify-center bg-primary">
              <HugeiconsIcon icon={SubtitleIcon} className="size-4 text-primary-foreground" strokeWidth={2} />
            </div>
            <div>
              <h1 className="text-sm font-medium tracking-wide">SUBGEN</h1>
              <p className="text-[10px] text-muted-foreground">
                {systemInfo?.gpu && systemInfo.gpu !== "unknown" && systemInfo.gpu !== "none"
                  ? systemInfo.gpu
                  : "Local subtitle generator"}
              </p>
              {vram && (
                <VramIndicator
                  totalMiB={vram.total_mib}
                  usedMiB={vram.used_mib}
                  freeMiB={vram.free_mib}
                />
              )}
            </div>
          </div>
          <div className="flex items-center gap-2">
            {systemInfo && (
              <div className="hidden sm:flex items-center gap-1.5 mr-2">
                <Badge variant={systemInfo.whisperServer ? "default" : "outline"} className="text-[10px]">
                  Whisper
                </Badge>
                <Badge variant={systemInfo.translationEngine ? "default" : "outline"} className="text-[10px]">
                  Translate
                </Badge>
              </div>
            )}
            <div
              className={cn(
                "size-2",
                connected
                  ? "bg-chart-1"
                  : connecting
                    ? "animate-pulse bg-chart-4"
                    : "bg-destructive"
              )}
            />
            <span className="text-[10px] text-muted-foreground">
              {connected
                ? "Ready"
                : connecting
                  ? "Connecting"
                  : "Offline"}
            </span>
          </div>
        </div>
      </header>

      {/* Main */}
      <main className="mx-auto w-full max-w-2xl flex-1 px-6 py-6 space-y-5">
        {appState === "complete" && completion ? (
          <OutputResult
            outputPath={completion.outputPath}
            transcriptionLog={completion.transcriptionLog}
            segments={completion.segments}
            durationSecs={completion.durationSecs}
            onReset={handleReset}
          />
        ) : isProcessing ? (
          <ProcessingView
            stage={processing.stage}
            percent={processing.percent}
            message={processing.message}
            elapsedSecs={processing.elapsedSecs}
            etaSecs={processing.etaSecs}
            onStop={handleStopProcessing}
            stopDisabled={isStopping}
            stopLabel={isStopping ? "Stopping..." : "Stop Processing"}
          />
        ) : appState === "installing" && installState ? (
          <ProcessingView
            stage={installState.stage}
            percent={installState.percent}
            message={installState.message}
            elapsedSecs={null}
            etaSecs={null}
            stageOrder={["downloading_dependency", "extracting", "validating"]}
            stageLabels={{
              downloading_dependency: "Download",
              extracting: "Extract",
              validating: "Validate",
            }}
          />
        ) : (
          <>
            {setupStatus && (
              <SetupBanner
                setupStatus={setupStatus}
                onInstall={handleInstall}
              />
            )}

            <VideoDropzone
              selectedFile={videoPath}
              onFileSelect={setVideoPath}
            />

            <Separator />

            <LanguageSelector
              sourceLang={sourceLang}
              targetLang={targetLang}
              onSourceChange={setSourceLang}
              onTargetChange={setTargetLang}
              sourceLanguages={languageOptions.source}
              targetLanguages={languageOptions.target}
              translationStatus={translationStatus}
            />

            <ModelSelector
              model={model}
              onChange={setModel}
            />

            <FormatSelector
              format={format}
              onChange={setFormat}
            />

            <SettingsPanel
              beamSize={beamSize}
              vadFilter={vadFilter}
              audioConfig={audioConfig}
              onBeamSizeChange={setBeamSize}
              onVadFilterChange={setVadFilter}
              onAudioConfigChange={setAudioConfig}
              disabled={isProcessing}
            />

            {appState === "error" && errorMsg && (
              <div className="flex items-start gap-3 border border-destructive/30 bg-destructive/5 p-4">
                <HugeiconsIcon icon={Cancel01Icon} className="mt-0.5 size-4 shrink-0 text-destructive" strokeWidth={2} />
                <div className="flex-1">
                  <p className="text-xs text-destructive">{errorMsg}</p>
                  <button
                    onClick={() => setAppState("idle")}
                    className="mt-2 text-[10px] text-destructive/70 underline underline-offset-2 hover:text-destructive"
                  >
                    Dismiss
                  </button>
                </div>
              </div>
            )}

            <Button
              size="lg"
              className="w-full py-6 text-xs font-medium uppercase tracking-widest"
              onClick={handleGenerate}
              disabled={!videoPath || !connected || shouldDisableGenerate(setupStatus, targetLang)}
            >
              <HugeiconsIcon icon={SparklesIcon} className="size-4" strokeWidth={1.5} />
              {!connected
                ? "Waiting for backend..."
                : "Generate Subtitles"}
            </Button>
          </>
        )}
      </main>
    </div>
  );
}

export default App;
