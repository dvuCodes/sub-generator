import { useCallback, useEffect, useRef, useState } from "react";
import { FormatSelector } from "./components/FormatSelector";
import { LanguageSelector } from "./components/LanguageSelector";
import { ModelSelector } from "./components/ModelSelector";
import { OutputResult } from "./components/OutputResult";
import { ProcessingView } from "./components/ProcessingView";
import {
  ProcessingDrawer,
  type ProcessingLogEntry,
} from "./components/ProcessingDrawer";
import { SettingsPanel } from "./components/SettingsPanel";
import { VideoDropzone } from "./components/VideoDropzone";
import { VramIndicator } from "./components/VramIndicator";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { useSidecar } from "./hooks/useSidecar";
import { useVramPolling } from "./hooks/useVramPolling";
import { buildLanguageOptions } from "./lib/languageOptions";
import {
  findTranslationBackendCapability,
  resolveASRModelId,
  resolvePreferredASRBackend,
  resolvePreferredTranslationBackend,
} from "./lib/backendOptions";
import { reduceSystemInfo, type SystemInfoState } from "./lib/systemInfo";
import {
  advanceProcessingState,
  createInitialProcessingState,
  type ProcessingState,
} from "./lib/processingState";
import { formatRuntimeError } from "./lib/runtimeError";
import { SetupBanner } from "./components/SetupBanner";
import {
  findPromptableInstallAction,
  isServiceReady,
  shouldDisableGenerate,
} from "./lib/setupHelpers";
import {
  advanceInstallState,
  createInitialInstallState,
  isInstallComplete,
  type InstallState,
} from "./lib/installState";
import type {
  ASRBackend,
  CapabilitiesResponse,
  GenerateCommand,
  ModelSize,
  OutputFormat,
  SetupStatusResponse,
  SidecarResponse,
  TranslationBackend,
} from "./lib/types";
import { cn } from "@/lib/utils";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  Cancel01Icon,
  SparklesIcon,
  SubtitleIcon,
} from "@hugeicons/core-free-icons";

type AppState = "idle" | "processing" | "complete" | "error" | "installing";

interface CompletionState {
  outputPath: string;
  transcriptionLog?: string;
  segments: number;
  durationSecs: number;
  backendSummary?: string;
  selectedASRBackend?: string;
  diarizationRan?: boolean;
  speakerCount?: number;
}

const STAGE_LABELS: Record<string, string> = {
  validating: "Validate",
  downloading_model: "Download",
  starting_services: "Services",
  transcribing: "Transcribe",
  diarizing: "Speakers",
  translating: "Translate",
  writing: "Write",
};

function formatLogTime(date: Date = new Date()) {
  return date.toLocaleTimeString("en-AU", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
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
  const [capabilities, setCapabilities] = useState<CapabilitiesResponse | null>(null);
  const [asrBackend, setAsrBackend] = useState<ASRBackend>("faster_whisper");
  const [asrModelId, setAsrModelId] = useState("");
  const [translationBackend, setTranslationBackend] =
    useState<TranslationBackend>("nllb");
  const [diarizationEnabled, setDiarizationEnabled] = useState(false);
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
  const [processingLog, setProcessingLog] = useState<ProcessingLogEntry[]>([]);

  const selectedASRBackend = resolvePreferredASRBackend(capabilities, asrBackend);
  const selectedTranslationBackend = resolvePreferredTranslationBackend(
    capabilities,
    translationBackend
  );
  const selectedTranslationCapability = findTranslationBackendCapability(
    capabilities,
    selectedTranslationBackend
  );
  const selectedASRModelId = resolveASRModelId(
    capabilities,
    selectedASRBackend,
    model,
    asrModelId
  );
  const effectiveTranslationBackend: TranslationBackend =
    targetLang && selectedTranslationBackend !== "none"
      ? selectedTranslationBackend
      : "none";
  const languageOptions = buildLanguageOptions(
    capabilities,
    selectedASRBackend,
    selectedTranslationBackend
  );
  const mlBackendReady = isServiceReady(setupStatus, "ml-backend");
  const llamaReady = isServiceReady(setupStatus, "llama");
  const asrOptions =
    capabilities?.backends.asr ?? [
      {
        id: "faster_whisper",
        display_name: "Faster Whisper",
        installed: mlBackendReady,
        default_model_id: capabilities?.defaults.asr_model_id ?? "",
        source_languages: [],
      },
      {
        id: "whisper_cpp",
        display_name: "whisper.cpp",
        installed: true,
        default_model_id: "turbo",
        source_languages: [],
      },
    ];
  const translationOptions =
    capabilities?.backends.translation ?? [
      {
        id: "nllb",
        display_name: "NLLB",
        installed: mlBackendReady,
        default_model_id: "",
        target_languages: [],
      },
      {
        id: "gemma_context",
        display_name: "Gemma Context",
        installed: llamaReady,
        default_model_id: "",
        target_languages: [],
      },
    ];

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
  const promptedInstallActionsRef = useRef(new Set<string>());
  const logCounterRef = useRef(0);
  const logMetaRef = useRef({
    lastStage: "",
    lastPercent: -1,
    lastMessage: "",
  });

  const appendLog = useCallback(
    (entry: Omit<ProcessingLogEntry, "id" | "time">) => {
      const id = `log-${logCounterRef.current++}`;
      const time = formatLogTime();
      setProcessingLog((current) => [
        ...current.slice(-199),
        { id, time, ...entry },
      ]);
    },
    []
  );

  const resetProcessingLog = useCallback((message?: string) => {
    logCounterRef.current = 0;
    logMetaRef.current = { lastStage: "", lastPercent: -1, lastMessage: "" };
    if (message) {
      const time = formatLogTime();
      setProcessingLog([
        { id: "log-0", time, level: "info", label: "init", message },
      ]);
      logCounterRef.current = 1;
    } else {
      setProcessingLog([]);
    }
  }, []);

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
          setSetupStatus(response);
          if (appStateRef.current === "installing" && installStateRef.current) {
            if (isInstallComplete(installStateRef.current, response)) {
              setAppState("idle");
              setInstallState(null);
              sendCommandRef.current({ command: "capabilities" }).catch((err) => {
                console.error("Failed to refresh capabilities:", err);
              });
              sendCommandRef.current({ command: "system_info" }).catch((err) => {
                console.error("Failed to refresh system info:", err);
              });
            }
          }
          break;
        case "capabilities":
          setCapabilities(response);
          setTranslationWarning("");
          break;
        case "progress":
          if (appStateRef.current === "installing") {
            setInstallState((current) =>
              current ? advanceInstallState(current, response) : current
            );
          } else {
            setProcessing((current) => advanceProcessingState(current, response));
            const percent = Math.round(response.percent);
            const label = STAGE_LABELS[response.stage] ?? response.stage;
            const shouldLogStage = response.stage !== logMetaRef.current.lastStage;
            const shouldLogProgress =
              shouldLogStage ||
              percent === 0 ||
              percent === 100 ||
              percent - logMetaRef.current.lastPercent >= 5 ||
              response.message !== logMetaRef.current.lastMessage;

            if (shouldLogStage) {
              appendLog({
                level: "info",
                label: "stage",
                message: `${label} started`,
              });
              logMetaRef.current.lastStage = response.stage;
              logMetaRef.current.lastPercent = -1;
            }

            if (shouldLogProgress) {
              const progressMessage = response.message
                ? `${label} ${percent}% — ${response.message}`
                : `${label} ${percent}%`;
              appendLog({
                level: "info",
                label: "progress",
                message: progressMessage,
              });
            }

            logMetaRef.current.lastPercent = percent;
            logMetaRef.current.lastMessage = response.message;
          }
          break;
        case "stage":
          if (appStateRef.current === "installing") {
            setInstallState((current) =>
              current ? advanceInstallState(current, response) : current
            );
          } else {
            setProcessing((current) => advanceProcessingState(current, response));
            const label = STAGE_LABELS[response.stage] ?? response.stage;
            if (response.stage !== logMetaRef.current.lastStage) {
              appendLog({
                level: "info",
                label: "stage",
                message: response.message
                  ? `${label} — ${response.message}`
                  : `${label} started`,
              });
              logMetaRef.current.lastStage = response.stage;
              logMetaRef.current.lastPercent = -1;
              logMetaRef.current.lastMessage = response.message;
            } else if (response.message !== logMetaRef.current.lastMessage) {
              appendLog({
                level: "info",
                label: "update",
                message: response.message || `${label} updated`,
              });
              logMetaRef.current.lastMessage = response.message;
            }
          }
          break;
        case "complete":
          setCompletion({
            outputPath: response.output_path,
            transcriptionLog: response.transcription_log,
            segments: response.segments,
            durationSecs: response.duration_secs,
            backendSummary: response.backend_summary,
            selectedASRBackend: response.selected_asr_backend,
            diarizationRan: response.diarization_ran,
            speakerCount: response.speaker_count,
          });
          setAppState("complete");
          appendLog({
            level: "success",
            label: "complete",
            message: "Subtitle generation finished.",
          });
          sendCommandRef.current({ command: "stop_services" }).catch((err) => {
            console.error("Failed to stop services:", err);
          });
          break;
        case "languages":
          setTranslationWarning("");
          break;
        case "system_info":
          setSystemInfo((prev) => reduceSystemInfo(prev, response));
          if (response.translation_engine || response.ml_backend) {
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

          appendLog({
            level: "error",
            label: "error",
            message: formattedError,
          });
          setErrorMsg(formattedError);
          setAppState("error");
          sendCommandRef.current({ command: "stop_services" }).catch((err) => {
            console.error("Failed to stop services:", err);
          });
          break;
        }
      }
    });
  }, [appendLog, onResponse]);

  useEffect(() => {
    if (!connected) {
      return;
    }

    sendCommand({ command: "system_info" }).catch((err) => {
      console.error("Failed to request system info:", err);
    });
    sendCommand({ command: "capabilities" }).catch((err) => {
      console.error("Failed to request capabilities:", err);
      setTranslationWarning(`Failed to load backend capabilities: ${err}`);
    });
    sendCommand({ command: "check_setup" }).catch((err) => {
      console.error("Failed to check setup:", err);
    });
  }, [connected, sendCommand]);

  useEffect(() => {
    if (!capabilities) {
      return;
    }

    setAsrBackend((current) => {
      return resolvePreferredASRBackend(capabilities, current);
    });
    setTranslationBackend((current) => {
      return resolvePreferredTranslationBackend(capabilities, current);
    });
    setDiarizationEnabled((current) =>
      current || capabilities.defaults.diarization_enabled
    );
  }, [capabilities]);

  useEffect(() => {
    setAsrModelId((current) =>
      resolveASRModelId(capabilities, selectedASRBackend, model, current)
    );
  }, [capabilities, model, selectedASRBackend]);

  useEffect(() => {
    if (
      selectedTranslationBackend === "none" &&
      targetLang !== ""
    ) {
      setTargetLang("");
      return;
    }

    if (
      sourceLang !== "auto" &&
      !languageOptions.source.some((lang) => lang.code === sourceLang)
    ) {
      setSourceLang("auto");
    }

    if (
      targetLang &&
      !languageOptions.target.some((lang) => lang.code === targetLang)
    ) {
      setTargetLang("");
    }
  }, [languageOptions, selectedTranslationBackend, sourceLang, targetLang]);

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
    resetProcessingLog("Session initialized.");
    setErrorMsg("");

    try {
      const command: GenerateCommand = {
        command: "generate",
        input_video: videoPath,
        source_lang: sourceLang,
        target_lang: effectiveTranslationBackend === "none" ? null : targetLang,
        output_format: format,
        output_path: null,
        asr_backend: selectedASRBackend,
        asr_model_id: selectedASRModelId,
        model_size: model,
        translation_backend: effectiveTranslationBackend,
        diarization_enabled: diarizationEnabled,
        beam_size: beamSize,
        vad_filter: vadFilter,
      };
      await sendCommand(command);
    } catch (err) {
      setErrorMsg(`Failed to send command: ${err}`);
      setAppState("error");
    }
  }, [
    beamSize,
    connected,
    diarizationEnabled,
    effectiveTranslationBackend,
    format,
    model,
    resetProcessingLog,
    selectedASRBackend,
    selectedASRModelId,
    sendCommand,
    sourceLang,
    targetLang,
    vadFilter,
    videoPath,
  ]);

  const handleReset = useCallback(() => {
    setAppState("idle");
    setVideoPath(null);
    setCompletion(null);
    setErrorMsg("");
    setProcessing(createInitialProcessingState());
    resetProcessingLog();
  }, [resetProcessingLog]);

  const handleStopProcessing = useCallback(async () => {
    if (appState !== "processing" || isStopping) {
      return;
    }

    setIsStopping(true);
    setProcessing((current) => ({
      ...current,
      message: "Stopping processing...",
    }));
    appendLog({
      level: "warn",
      label: "stop",
      message: "Stop requested by user. Waiting for shutdown...",
    });

    try {
      await disconnect();
      setAppState("idle");
      setCompletion(null);
      setErrorMsg("");
      setProcessing(createInitialProcessingState());
      resetProcessingLog();
      await connect();
    } catch (err) {
      setErrorMsg(`Failed to stop processing: ${err}`);
      setAppState("error");
    } finally {
      setIsStopping(false);
    }
  }, [appState, appendLog, connect, disconnect, isStopping, resetProcessingLog]);

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

  const requestInstall = useCallback(
    async (actionId: string, options?: { prompt?: boolean }) => {
      try {
        const action = setupStatus?.services
          .flatMap((service) => service.actions)
          .find((candidate) => candidate.id === actionId);

        if (options?.prompt !== false && action?.kind === "command") {
          const { confirm } = await import("@tauri-apps/plugin-dialog");
          const accepted = await confirm(
            [
              action.description,
              action.guidance ?? "SubGen will install the missing dependencies automatically.",
              "Do you want SubGen to install them now?",
            ].join("\n\n"),
            {
              title: "Install Missing Dependencies",
              kind: "warning",
              okLabel: "Install",
              cancelLabel: "Not now",
            }
          );

          if (!accepted) {
            return;
          }
        }

        await handleInstall(actionId);
      } catch (err) {
        setErrorMsg(`Failed to start install: ${err}`);
        setAppState("error");
      }
    },
    [handleInstall, setupStatus]
  );

  const handleASRBackendChange = useCallback(
    (backend: ASRBackend) => {
      setAsrBackend(backend);
      setAsrModelId(resolveASRModelId(capabilities, backend, model, ""));
    },
    [capabilities, model]
  );

  const handleTranslationBackendChange = useCallback((backend: TranslationBackend) => {
    setTranslationBackend(backend);
    if (backend === "none") {
      setTargetLang("");
    }
  }, []);

  const handleWhisperModelChange = useCallback(
    (nextModel: ModelSize) => {
      setModel(nextModel);
      if (selectedASRBackend === "whisper_cpp") {
        setAsrModelId(nextModel);
      }
    },
    [selectedASRBackend]
  );

  const isProcessing = appState === "processing";

  useEffect(() => {
    if (appState !== "idle") {
      return;
    }

    const action = findPromptableInstallAction(setupStatus);
    if (!action || promptedInstallActionsRef.current.has(action.id)) {
      return;
    }

    promptedInstallActionsRef.current.add(action.id);
    void requestInstall(action.id);
  }, [appState, requestInstall, setupStatus]);

  const translationStatus =
    selectedTranslationBackend === "none"
      ? "Source-only subtitles. Translation is disabled."
      : !targetLang
        ? `Choose a target language to run ${selectedTranslationCapability?.display_name ?? "translation"}.`
      : selectedTranslationCapability?.installed
        ? `${selectedTranslationCapability.display_name} ready. ${languageOptions.target.length} translation targets available.`
        : translationWarning ||
          `${selectedTranslationCapability?.display_name ?? "Translation backend"} requires setup before translation can run.`;

  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      <header className="border-b border-border px-6 py-3">
        <div className="mx-auto flex max-w-2xl items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-8 items-center justify-center bg-primary">
              <HugeiconsIcon
                icon={SubtitleIcon}
                className="size-4 text-primary-foreground"
                strokeWidth={2}
              />
            </div>
            <div>
              <h1 className="text-sm font-medium tracking-wide">SUBGEN</h1>
              <p className="text-[10px] text-muted-foreground">
                {systemInfo?.gpu &&
                systemInfo.gpu !== "unknown" &&
                systemInfo.gpu !== "none"
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
              <div className="mr-2 hidden items-center gap-1.5 sm:flex">
                <Badge
                  variant={systemInfo.whisperServer ? "default" : "outline"}
                  className="text-[10px]"
                >
                  Whisper
                </Badge>
                <Badge
                  variant={systemInfo.mlBackend ? "default" : "outline"}
                  className="text-[10px]"
                >
                  ML
                </Badge>
                <Badge
                  variant={systemInfo.translationEngine ? "default" : "outline"}
                  className="text-[10px]"
                >
                  Gemma
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
              {connected ? "Ready" : connecting ? "Connecting" : "Offline"}
            </span>
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-2xl flex-1 space-y-5 px-6 py-6">
        {appState === "complete" && completion ? (
          <OutputResult
            outputPath={completion.outputPath}
            transcriptionLog={completion.transcriptionLog}
            segments={completion.segments}
            durationSecs={completion.durationSecs}
            backendSummary={completion.backendSummary}
            selectedASRBackend={completion.selectedASRBackend}
            diarizationRan={completion.diarizationRan}
            speakerCount={completion.speakerCount}
            onReset={handleReset}
          />
        ) : isProcessing ? (
          <>
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
            <ProcessingDrawer
              stage={processing.stage}
              percent={processing.percent}
              message={processing.message}
              elapsedSecs={processing.elapsedSecs}
              etaSecs={processing.etaSecs}
              logEntries={processingLog}
            />
          </>
        ) : appState === "installing" && installState ? (
          <ProcessingView
            stage={installState.stage}
            percent={installState.percent}
            message={installState.message}
            elapsedSecs={null}
            etaSecs={null}
            stageOrder={[
              "downloading_dependency",
              "extracting",
              "installing_dependency",
              "validating",
            ]}
            stageLabels={{
              downloading_dependency: "Download",
              extracting: "Extract",
              installing_dependency: "Install",
              validating: "Validate",
            }}
          />
        ) : (
          <>
            {setupStatus && (
              <SetupBanner
                setupStatus={setupStatus}
                onInstall={requestInstall}
                disabled={appState === "installing"}
              />
            )}

            <VideoDropzone selectedFile={videoPath} onFileSelect={setVideoPath} />

            <Separator />

            <LanguageSelector
              sourceLang={sourceLang}
              targetLang={targetLang}
              onSourceChange={setSourceLang}
              onTargetChange={setTargetLang}
              sourceLanguages={languageOptions.source}
              targetLanguages={languageOptions.target}
              translationStatus={translationStatus}
              disabled={isProcessing}
              targetDisabled={selectedTranslationBackend === "none"}
            />

            <ModelSelector
              asrBackend={selectedASRBackend}
              asrOptions={asrOptions}
              asrModelId={selectedASRModelId}
              translationBackend={selectedTranslationBackend}
              translationOptions={translationOptions}
              whisperModel={model}
              diarizationEnabled={diarizationEnabled}
              onAsrBackendChange={handleASRBackendChange}
              onAsrModelChange={setAsrModelId}
              onTranslationBackendChange={handleTranslationBackendChange}
              onWhisperModelChange={handleWhisperModelChange}
              onDiarizationChange={setDiarizationEnabled}
              disabled={isProcessing}
            />

            <FormatSelector format={format} onChange={setFormat} />

            <SettingsPanel
              beamSize={beamSize}
              vadFilter={vadFilter}
              onBeamSizeChange={setBeamSize}
              onVadFilterChange={setVadFilter}
              disabled={isProcessing}
            />

            {appState === "error" && errorMsg && (
              <div className="flex items-start gap-3 border border-destructive/30 bg-destructive/5 p-4">
                <HugeiconsIcon
                  icon={Cancel01Icon}
                  className="mt-0.5 size-4 shrink-0 text-destructive"
                  strokeWidth={2}
                />
                <div className="flex-1">
                  <p className="text-xs text-destructive">{errorMsg}</p>
                  <button
                    type="button"
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
              disabled={
                !videoPath ||
                !connected ||
                isProcessing ||
                shouldDisableGenerate(
                  setupStatus,
                  {
                    asrBackend: selectedASRBackend,
                    translationBackend: effectiveTranslationBackend,
                    diarizationEnabled,
                  }
                )
              }
            >
              <HugeiconsIcon icon={SparklesIcon} className="size-4" strokeWidth={1.5} />
              {!connected ? "Waiting for backend..." : "Generate Subtitles"}
            </Button>
          </>
        )}
      </main>
    </div>
  );
}

export default App;
