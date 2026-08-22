package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var supportedVideoExts = map[string]bool{
	".mp4":  true,
	".mkv":  true,
	".avi":  true,
	".mov":  true,
	".webm": true,
	".flv":  true,
	".wmv":  true,
	".m4v":  true,
	".wav":  true,
	".mp3":  true,
}

const maxSubtitleSegmentDurationSeconds = 20.0
const overlongSegmentVADRetryThresholdSeconds = 60.0
const retryCoverageRegressionToleranceSeconds = 5.0

type Pipeline struct {
	svcManager      *ServiceManager
	cleanupServices func()
}

type transcribeAttempt func(path string) (*TranscriptionResult, error)

type transcriptionValidation struct {
	KeptSegments          int
	DroppedSegments       int
	LongestDroppedSeconds float64
	LastKeptEnd           float64
	UsableDuration        float64
}

type preparedTranscriptionInput struct {
	TranscriptionPath string
	DiarizationPath   string
	Cleanup           func()
}

func NewPipeline(svcManager *ServiceManager) *Pipeline {
	return &Pipeline{
		svcManager:      svcManager,
		cleanupServices: svcManager.StopAll,
	}
}

func (p *Pipeline) stopServices() {
	if p.cleanupServices != nil {
		p.cleanupServices()
		return
	}
	if p.svcManager != nil {
		p.svcManager.StopAll()
	}
}

func (p *Pipeline) Run(cmd Command) {
	p.RunContext(context.Background(), cmd)
}

func (p *Pipeline) RunContext(ctx context.Context, cmd Command) {
	startTime := time.Now()
	defer p.stopServices()

	selectedASRBackend := resolveASRBackend(cmd)
	selectedASRModelID := resolveASRModelID(cmd)
	selectedTranslationBackend := resolveTranslationBackend(cmd)
	selectedTranslationModelID := resolveTranslationModelID(cmd)
	diarizationRequested := cmd.DiarizationEnabled

	// Step 1: Validate input
	sendStage("validating", "Validating input file...")
	if err := p.validateInput(cmd.InputVideo); err != nil {
		sendError("Validation failed", err.Error())
		return
	}

	if err := ensureASRAssets(p.svcManager.config.SearchRoots, cmd, selectedASRBackend); err != nil {
		sendError("Model check failed", err.Error())
		return
	}

	ffmpegPath, err := LookupFFmpeg()
	if err != nil {
		sendError("Missing dependency", err.Error())
		return
	}

	// Step 2: Ensure services are running
	sendStage("starting_services", "Ensuring services are running...")
	if err := p.ensureServices(ctx, cmd); err != nil {
		sendError("Service startup failed", err.Error())
		return
	}

	// Resolve an explicitly configured contextual engine. If the advanced
	// engine fields are absent, preserve main's NLLB/Gemma backend selection.
	var engine TranslateEngine
	targetLang := derefString(cmd.TargetLang)
	translating := targetLang != "" && selectedTranslationBackend != "none"
	useContextualEngine := targetLang != "" && (strings.TrimSpace(cmd.TranslationEngine) != "" || hasLLMConfig(cmd) || hasDeepLConfig(cmd))
	if useContextualEngine {
		engine, err = selectTranslationEngine(cmd, p.svcManager.config.LibreTranslatePort)
		if err != nil {
			sendError("Translation setup failed", err.Error())
			return
		}
		translating = true
	}

	// Step 3: Extract canonical audio (16 kHz mono WAV) and detect scene cuts.
	sendStage("preparing", "Extracting audio track...")
	wavPath, audioDuration, cleanupAudio, err := p.extractAudio(ctx, ffmpegPath, cmd.InputVideo)
	if err != nil {
		sendError("Audio extraction failed", err.Error())
		return
	}
	defer cleanupAudio()

	shotSnap := cmd.ShotSnap == nil || *cmd.ShotSnap
	var shotTimes []float64
	if shotSnap {
		sendStage("preparing", "Detecting scene cuts...")
		shotTimes, _ = DetectShotChanges(ctx, ffmpegPath, cmd.InputVideo)
		if len(shotTimes) > 0 {
			sendProgress("preparing", 100, fmt.Sprintf("Found %d scene cuts", len(shotTimes)))
		}
	}

	// Step 4: Transcribe with heartbeats so the UI stays live.
	sendStage("transcribing", "Transcribing speech...")
	stopHeartbeat := startTranscribeHeartbeat(audioDuration)
	transcribeCmd := cmd
	transcribeCmd.InputVideo = wavPath
	result, err := p.transcribeContext(ctx, transcribeCmd, selectedASRBackend, selectedASRModelID)
	stopHeartbeat()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			sendCancelled()
			return
		}
		sendError("Transcription failed", err.Error())
		return
	}

	sourceLang := derefString(cmd.SourceLang)
	if sourceLang == "" || strings.EqualFold(sourceLang, "auto") {
		sourceLang = result.Language
	}

	sendProgress("transcribing", 100, fmt.Sprintf("Transcribed %d segments", len(result.Segments)))

	if len(result.Segments) == 0 {
		sendError("No speech detected", "The audio track contains no recognizable speech.")
		return
	}

	// Step 5: Normalize source timings before diarization and translation.
	sendStage("timing", "Normalizing cue timing...")
	preOpts := NewTimingOptions(sourceLangForTiming(sourceLang), cmd.FrameRate)
	preOpts.ShotTimes = shotTimes
	segments := NormalizeCues(result.Segments, preOpts, shotSnap)

	diarizationRan := false
	var speakerCount *int

	if diarizationRequested {
		sendStage("diarizing", "Labeling speakers...")
		annotatedSegments, count, annotateErr := p.annotateDiarization(wavPath, segments)
		if annotateErr != nil {
			fmt.Fprintf(os.Stderr, "warning: diarization failed, continuing without speaker labels: %v\n", annotateErr)
			sendStage("diarizing", "Speaker labeling unavailable, continuing without speaker labels")
		} else {
			segments = annotatedSegments
			diarizationRan = true
			speakerCount = &count
			sendProgress("diarizing", 100, fmt.Sprintf("Detected %d speaker(s)", count))
		}
	}

	// Step 5b: Write diagnostic transcription log (before translation overwrites segments)
	var transcriptionLogPath string
	if cmd.TargetLang != nil && *cmd.TargetLang != "" {
		logPath := DeriveTranscriptionLogPath(cmd.InputVideo)
		if err := WriteTranscriptionLog(segments, logPath, sourceLang); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write transcription log to %q: %v\n", logPath, err)
		} else {
			transcriptionLogPath = logPath
		}
	}

	// Step 6: Translation with sliding-window context.
	if translating {
		if selectedTranslationBackend == defaultTranslationBackend && sourceLang == "auto" {
			sendError("Translation failed", "Could not determine the source language for NLLB translation. Choose a source language explicitly or retry with clearer speech.")
			return
		}

		var translated []Segment
		if useContextualEngine {
			sendStage("translating", fmt.Sprintf("Translating to %s via %s...", targetLang, engine.Name()))
			req := TranslateRequest{
				SourceLang: firstNonEmpty(sourceLang, "auto"),
				TargetLang: targetLang,
				Synopsis:   cmd.Synopsis,
				Glossary:   cmd.Glossary,
				Honorifics: firstNonEmpty(cmd.Honorifics, "keep"),
			}
			translated, err = ContextualTranslate(ctx, segments, engine, req, func(current, total int) {
				pct := float64(current) / float64(total) * 100
				sendProgress("translating", pct, fmt.Sprintf("Translated %d/%d lines", current, total))
			})
		} else {
			sendStage("translating", fmt.Sprintf("Translating to %s...", targetLang))
			translated, err = p.translateSegments(
				selectedTranslationBackend,
				selectedTranslationModelID,
				segments,
				sourceLang,
				targetLang,
			)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				sendCancelled()
				return
			}
			sendError("Translation failed", err.Error())
			return
		}
		segments = translated

		if cmd.QAPass && useContextualEngine {
			req := TranslateRequest{
				SourceLang: firstNonEmpty(sourceLang, "auto"),
				TargetLang: targetLang,
				Synopsis:   cmd.Synopsis,
				Glossary:   cmd.Glossary,
				Honorifics: firstNonEmpty(cmd.Honorifics, "keep"),
			}
			sendStage("translating", "Running translation QA pass...")
			if qa, ok := engine.(interface {
				RefinePass(context.Context, []Segment, TranslateRequest, func(int, int)) ([]Segment, error)
			}); ok {
				refined, err := qa.RefinePass(ctx, segments, req, func(current, total int) {
					pct := float64(current) / float64(total) * 100
					sendProgress("translating", pct, fmt.Sprintf("Reviewed %d/%d lines", current, total))
				})
				if err == nil {
					segments = refined
				} else if !errors.Is(err, context.Canceled) {
					fmt.Fprintf(os.Stderr, "[subgen] QA pass skipped: %v\n", err)
				}
			}
		}
	}

	// Step 7: Post-translation normalization against target-language rules.
	sendStage("timing", "Applying subtitle timing rules...")
	outputLanguage := firstNonEmpty(targetLang, sourceLang)
	postOpts := NewTimingOptions(outputLanguage, cmd.FrameRate)
	postOpts.ShotTimes = shotTimes
	segments = NormalizeCues(segments, postOpts, shotSnap)

	FinalizeLineWrapping(segments, outputLanguage)

	// Step 8: Write subtitle file + QC report.
	sendStage("writing", "Writing subtitle file...")

	outputFormat := cmd.OutputFormat
	if outputFormat == "" {
		outputFormat = "srt"
	}

	outputPath := derefString(cmd.OutputPath)
	if outputPath == "" {
		outputPath = DeriveOutputPath(cmd.InputVideo, outputFormat, cmd.TargetLang)
	}

	writer := NewSubtitleWriter()
	if err := writer.Write(segments, outputPath, outputFormat, cmd.TargetLang); err != nil {
		sendError("Failed to write subtitle file", err.Error())
		return
	}

	report := ComputeQC(segments, postOpts)
	qcPath := outputPath + ".qc.json"
	if err := WriteQCReport(report, qcPath); err != nil {
		fmt.Fprintf(os.Stderr, "[subgen] QC report write failed: %v\n", err)
	}

	// Done
	duration := time.Since(startTime).Seconds()
	sendJSON(CompleteResponse{
		Type:               "complete",
		OutputPath:         outputPath,
		TranscriptionLog:   transcriptionLogPath,
		Segments:           len(segments),
		DurationSecs:       duration,
		BackendSummary:     buildBackendSummary(selectedASRBackend, selectedTranslationBackend, diarizationRan),
		SelectedASRBackend: selectedASRBackend,
		DiarizationRan:     diarizationRan,
		SpeakerCount:       speakerCount,
		QC:                 report,
		Preview:            cuePreview(segments, 80),
	})
}

func (p *Pipeline) extractAudio(ctx context.Context, ffmpegPath, input string) (string, float64, func(), error) {
	wavPath, duration, err := ExtractAudio(ctx, ffmpegPath, input)
	cleanup := func() {
		if wavPath != "" {
			_ = os.RemoveAll(filepath.Dir(wavPath))
		}
	}
	if err != nil {
		cleanup()
		return "", 0, cleanup, err
	}
	return wavPath, duration, cleanup, nil
}

// startTranscribeHeartbeat emits periodic progress while the long-running
// inference request is in flight. The whisper.cpp server serializes requests
// behind a mutex and exposes no progress API, so elapsed-time heartbeats keep
// the UI alive without fabricating percentages.
func startTranscribeHeartbeat(audioDuration float64) (stop func()) {
	started := time.Now()
	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed := time.Since(started).Truncate(time.Second)
				message := fmt.Sprintf("Transcribing... %s elapsed", elapsed)
				if audioDuration > 0 {
					message += fmt.Sprintf(" (%.0fs of audio)", audioDuration)
				}
				sendProgress("transcribing", -1, message)
			}
		}
	}()

	return func() { close(done); <-stopped }
}

func (p *Pipeline) ensureServices(ctx context.Context, cmd Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	switch resolveASRBackend(cmd) {
	case "whisper_cpp":
		sendProgress("starting_services", 25, "Starting whisper-server...")
		if err := p.svcManager.StartWhisperServer(cmd.ModelSize); err != nil {
			return fmt.Errorf("whisper-server: %w", err)
		}
	default:
		sendProgress("starting_services", 25, "Starting ml-backend...")
		if err := p.svcManager.StartMLBackend(); err != nil {
			return fmt.Errorf("ml-backend: %w", err)
		}
	}
	sendProgress("starting_services", 60, "Transcription service ready")

	if translatingWithLibre(cmd) {
		if !p.svcManager.IsLibreTranslateRunning() {
			sendProgress("starting_services", 80, "Starting LibreTranslate...")
			if err := p.svcManager.StartLibreTranslate(); err != nil {
				return fmt.Errorf("libretranslate: %w", err)
			}
		}
	}
	sendProgress("starting_services", 100, "All services ready")
	return nil
}

func translatingWithLibre(cmd Command) bool {
	if cmd.TargetLang == nil || strings.TrimSpace(*cmd.TargetLang) == "" {
		return false
	}
	if strings.TrimSpace(cmd.TranslationEngine) == "" && !hasLLMConfig(cmd) && !hasDeepLConfig(cmd) {
		return false
	}
	switch resolveEngineName(cmd) {
	case "libretranslate":
		return true
	case "auto":
		// auto falls back to LibreTranslate only when no other engine is configured
		return !hasLLMConfig(cmd) && !hasDeepLConfig(cmd)
	default:
		return false
	}
}

func resolveEngineName(cmd Command) string {
	name := strings.ToLower(strings.TrimSpace(cmd.TranslationEngine))
	if name == "" {
		return "auto"
	}
	return name
}

func hasLLMConfig(cmd Command) bool {
	return firstNonEmpty(cmd.LLMBaseURL, os.Getenv("SUBGEN_LLM_BASE_URL")) != ""
}

func hasDeepLConfig(cmd Command) bool {
	return firstNonEmpty(cmd.DeepLAPIKey, os.Getenv("SUBGEN_DEEPL_API_KEY")) != ""
}

// selectTranslationEngine picks the translation backend from explicit
// configuration, falling back through configured credentials to LibreTranslate.
func selectTranslationEngine(cmd Command, librePort int) (TranslateEngine, error) {
	switch resolveEngineName(cmd) {
	case "libretranslate":
		return NewLibreEngine(librePort), nil

	case "deepl":
		key := firstNonEmpty(cmd.DeepLAPIKey, os.Getenv("SUBGEN_DEEPL_API_KEY"))
		if key == "" {
			return nil, errors.New("DeepL engine selected but no API key provided")
		}
		return NewDeepLEngine(key), nil

	case "llm":
		base := firstNonEmpty(cmd.LLMBaseURL, os.Getenv("SUBGEN_LLM_BASE_URL"), "http://127.0.0.1:11434")
		model := firstNonEmpty(cmd.LLMModel, os.Getenv("SUBGEN_LLM_MODEL"))
		key := firstNonEmpty(cmd.LLMAPIKey, os.Getenv("SUBGEN_LLM_API_KEY"), os.Getenv("OPENAI_API_KEY"))
		return NewLLMEngine(base, model, key), nil

	default: // auto
		if base := firstNonEmpty(cmd.LLMBaseURL, os.Getenv("SUBGEN_LLM_BASE_URL")); base != "" {
			model := firstNonEmpty(cmd.LLMModel, os.Getenv("SUBGEN_LLM_MODEL"))
			key := firstNonEmpty(cmd.LLMAPIKey, os.Getenv("SUBGEN_LLM_API_KEY"), os.Getenv("OPENAI_API_KEY"))
			return NewLLMEngine(base, model, key), nil
		}
		if key := firstNonEmpty(cmd.DeepLAPIKey, os.Getenv("SUBGEN_DEEPL_API_KEY")); key != "" {
			return NewDeepLEngine(key), nil
		}
		return NewLibreEngine(librePort), nil
	}
}

func buildTranscribeOptions(cmd Command, vadModelAvailable bool) TranscribeOptions {
	opts := DefaultTranscribeOptions()

	sourceLang := derefString(cmd.SourceLang)
	if strings.EqualFold(sourceLang, "auto") {
		sourceLang = ""
	}
	opts.SourceLang = sourceLang
	opts.InitialPrompt = cmd.InitialPrompt
	opts.VADEnabled = cmd.VADFilter && vadModelAvailable

	if cmd.BeamSize > 0 {
		opts.BeamSize = cmd.BeamSize
	}

	// Subtitle-friendly initial segmentation; the timing engine refines further.
	if sourceLang != "" && isCJKLanguage(sourceLang) {
		opts.MaxLen = 28
	} else {
		opts.MaxLen = 42
	}

	return opts
}

func (p *Pipeline) validateInput(path string) error {
	if path == "" {
		return fmt.Errorf("no input video specified")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file")
	}

	ext := strings.ToLower(filepath.Ext(path))
	if !supportedVideoExts[ext] {
		return fmt.Errorf("unsupported media format '%s' (supported: %s)", ext, supportedExtsList())
	}

	return nil
}

func (p *Pipeline) transcribe(cmd Command, backend, modelID string) (*TranscriptionResult, error) {
	return p.transcribeContext(context.Background(), cmd, backend, modelID)
}

func (p *Pipeline) transcribeContext(ctx context.Context, cmd Command, backend, modelID string) (*TranscriptionResult, error) {
	switch backend {
	case "whisper_cpp":
		transcriber := NewTranscriber(p.svcManager.config.WhisperPort)
		return transcribeWithOptionalVADRetry(
			cmd,
			func(path string, vadFilter bool) (*TranscriptionResult, error) {
				opts := buildTranscribeOptions(cmd, p.svcManager.HasVADModel())
				opts.VADEnabled = vadFilter && p.svcManager.HasVADModel()
				return transcriber.Transcribe(ctx, path, opts)
			},
			func(reason string) {
				fmt.Fprintf(
					os.Stderr,
					"warning: transcription returned oversized VAD segments, retrying without VAD: %s\n",
					reason,
				)
				sendStage("transcribing", "Collapsed VAD segments detected, retrying without VAD...")
			},
		)
	default:
		client := NewMLBackendClient(p.svcManager.MLBackendURL())
		return transcribeWithOptionalVADRetry(
			cmd,
			func(path string, vadFilter bool) (*TranscriptionResult, error) {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				result, err := client.Transcribe(path, cmd.SourceLang, modelID, cmd.BeamSize, vadFilter)
				if err == nil && ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return result, err
			},
			func(reason string) {
				fmt.Fprintf(
					os.Stderr,
					"warning: transcription returned oversized VAD segments, retrying without VAD: %s\n",
					reason,
				)
				sendStage("transcribing", "Collapsed VAD segments detected, retrying without VAD...")
			},
		)
	}
}

func (p *Pipeline) annotateDiarization(audioPath string, segments []Segment) ([]Segment, int, error) {
	if err := p.svcManager.StartMLBackend(); err != nil {
		return nil, 0, err
	}
	client := NewMLBackendClient(p.svcManager.MLBackendURL())
	return client.AnnotateDiarization(audioPath, segments)
}

func (p *Pipeline) translateSegments(backend, modelID string, segments []Segment, sourceLang, targetLang string) ([]Segment, error) {
	switch backend {
	case gemmaTranslationBackend:
		if !supportsTranslationPair(sourceLang, targetLang) {
			return nil, fmt.Errorf("translation pair %s -> %s is not supported by GemmaTranslate", sourceLang, targetLang)
		}

		llamaDir := preferredLlamaInstallDir(p.svcManager.config.SearchRoots)
		gemmaModelPath := filepath.Join(llamaDir, "models", gemmaModelFilenameConst)
		if _, err := os.Stat(gemmaModelPath); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("cannot access model at %q: %v", gemmaModelPath, err)
			}
			if err := os.MkdirAll(filepath.Dir(gemmaModelPath), 0o755); err != nil {
				return nil, err
			}
			if err := DownloadModel(GemmaModelDownloadURL(), gemmaModelPath, nil); err != nil {
				return nil, err
			}
		}

		p.svcManager.StopWhisperServer()
		if err := p.svcManager.StartLlamaServer(); err != nil {
			return nil, err
		}

		translator := NewTranslator(p.svcManager.config.LlamaServerPort)
		return translator.TranslateSegments(segments, sourceLang, targetLang, nil)
	default:
		if err := p.svcManager.StartMLBackend(); err != nil {
			return nil, err
		}
		client := NewMLBackendClient(p.svcManager.MLBackendURL())
		return client.TranslateSegments(segments, sourceLang, targetLang, modelID)
	}
}

func cuePreview(segments []Segment, limit int) []CueView {
	if len(segments) > limit {
		segments = segments[:limit]
	}
	out := make([]CueView, 0, len(segments))
	for _, seg := range segments {
		text := seg.Text
		if len(seg.Lines) > 0 {
			text = strings.Join(seg.Lines, "\n")
		}
		out = append(out, CueView{Start: seg.Start, End: seg.End, Text: text})
	}
	return out
}

func sourceLangForTiming(lang string) string {
	if lang == "" {
		return "ja"
	}
	return lang
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func ensureASRAssets(searchRoots []string, cmd Command, backend string) error {
	if backend != "whisper_cpp" {
		return nil
	}

	installDir := preferredWhisperInstallDir(searchRoots)
	modelFile := modelFilename(cmd.ModelSize)
	modelPath := filepath.Join(installDir, "models", modelFile)

	if _, err := os.Stat(modelPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("cannot access model at %q: %v", modelPath, err)
		}
		sendStage("downloading_model", fmt.Sprintf("Downloading %s model...", cmd.ModelSize))
		url := ModelDownloadURL(cmd.ModelSize)
		if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
			return err
		}
		if err := DownloadModel(url, modelPath, func(downloaded, total int64) {
			if total > 0 {
				pct := float64(downloaded) / float64(total) * 100
				sendProgress("downloading_model", pct, fmt.Sprintf("Downloading %s / %s", formatBytes(downloaded), formatBytes(total)))
			} else {
				sendProgress("downloading_model", 0, fmt.Sprintf("Downloading %s...", formatBytes(downloaded)))
			}
		}); err != nil {
			return err
		}
		sendProgress("downloading_model", 100, "Model downloaded")
	}

	if !cmd.VADFilter {
		return nil
	}

	vadModelPath := filepath.Join(installDir, "models", vadModelFilename)
	if _, err := os.Stat(vadModelPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot access VAD model at %q: %v", vadModelPath, err)
	} else if os.IsNotExist(err) {
		sendStage("downloading_model", "Downloading VAD model...")
		if err := os.MkdirAll(filepath.Dir(vadModelPath), 0o755); err != nil {
			return err
		}
		if err := DownloadModel(VADModelDownloadURL(), vadModelPath, func(downloaded, total int64) {
			if total > 0 {
				pct := float64(downloaded) / float64(total) * 100
				sendProgress("downloading_model", pct, fmt.Sprintf("Downloading VAD model %s / %s", formatBytes(downloaded), formatBytes(total)))
			} else {
				sendProgress("downloading_model", 0, fmt.Sprintf("Downloading VAD model %s...", formatBytes(downloaded)))
			}
		}); err != nil {
			return err
		}
		sendProgress("downloading_model", 100, "VAD model downloaded")
	}

	return nil
}

func prepareTranscriptionInput(cmd Command, backend string) preparedTranscriptionInput {
	prepared := preparedTranscriptionInput{
		TranscriptionPath: cmd.InputVideo,
		DiarizationPath:   cmd.InputVideo,
		Cleanup:           func() {},
	}

	if backend != defaultASRBackend {
		return prepared
	}

	stagedPath, cleanup, err := stageNeutralAudio(cmd.InputVideo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: neutral audio staging failed, falling back to original input: %v\n", err)
		return prepared
	}

	prepared.TranscriptionPath = stagedPath
	prepared.DiarizationPath = stagedPath
	if cleanup != nil {
		prepared.Cleanup = cleanup
	}

	return prepared
}

func supportedExtsList() string {
	exts := make([]string, 0, len(supportedVideoExts))
	for ext := range supportedVideoExts {
		exts = append(exts, ext)
	}
	return strings.Join(exts, ", ")
}

func validateTranscriptionResult(result *TranscriptionResult) error {
	_, err := validateTranscriptionResultDetailed(result)
	return err
}

func validateTranscriptionResultDetailed(result *TranscriptionResult) (transcriptionValidation, error) {
	var summary transcriptionValidation

	if result == nil {
		return summary, fmt.Errorf("whisper-server returned no transcription result")
	}

	if len(result.Segments) > 0 {
		usable := result.Segments[:0]
		firstInvalidIndex := -1
		var firstInvalid Segment

		for i, segment := range result.Segments {
			if segmentIsSubtitleSafe(segment) {
				usable = append(usable, segment)
				summary.KeptSegments++
				summary.LastKeptEnd = segment.End
				summary.UsableDuration += segment.End - segment.Start
				continue
			}
			summary.DroppedSegments++
			if duration := segment.End - segment.Start; duration > summary.LongestDroppedSeconds {
				summary.LongestDroppedSeconds = duration
			}
			if firstInvalidIndex == -1 {
				firstInvalidIndex = i
				firstInvalid = segment
			}
		}

		if len(usable) > 0 {
			result.Segments = usable
			return summary, nil
		}

		return summary, fmt.Errorf(
			"whisper-server returned unusable subtitle timings for every segment (first invalid segment %d start=%.3f end=%.3f duration=%.3fs), so subtitle timing could not be generated",
			firstInvalidIndex+1,
			firstInvalid.Start,
			firstInvalid.End,
			firstInvalid.End-firstInvalid.Start,
		)
	}

	if strings.TrimSpace(result.Text) != "" {
		return summary, fmt.Errorf("whisper-server returned transcript text but no timestamped segments, so subtitle timing could not be generated")
	}

	return summary, fmt.Errorf("no speech was detected in the input video, so there are no subtitle segments to write")
}

func segmentIsSubtitleSafe(segment Segment) bool {
	if segment.Start < 0 || segment.End <= segment.Start {
		return false
	}

	return segment.End-segment.Start <= maxSubtitleSegmentDurationSeconds
}

func transcribeValidated(
	path string,
	transcribe transcribeAttempt,
) (*TranscriptionResult, transcriptionValidation, error) {
	var summary transcriptionValidation

	result, err := transcribe(path)
	if err != nil {
		return nil, summary, err
	}

	detailed, err := validateTranscriptionResultDetailed(result)
	if err != nil {
		return nil, detailed, err
	}

	return result, detailed, nil
}

func considerRetryCandidate(
	currentResult *TranscriptionResult,
	currentSummary transcriptionValidation,
	candidateResult *TranscriptionResult,
	candidateSummary transcriptionValidation,
) (*TranscriptionResult, transcriptionValidation) {
	if !retryImprovesTranscription(candidateSummary, currentSummary) {
		return currentResult, currentSummary
	}

	return candidateResult, candidateSummary
}

func tryValidatedRetry(
	path string,
	transcribe transcribeAttempt,
) (*TranscriptionResult, transcriptionValidation, error) {
	if path == "" {
		return nil, transcriptionValidation{}, fmt.Errorf("empty retry path")
	}

	return transcribeValidated(path, transcribe)
}

func transcribeWithOptionalVADRetry(
	cmd Command,
	transcribe func(path string, vadFilter bool) (*TranscriptionResult, error),
	onVADRetry func(reason string),
) (*TranscriptionResult, error) {
	result, summary, err := transcribeValidated(
		cmd.InputVideo,
		func(path string) (*TranscriptionResult, error) {
			return transcribe(path, cmd.VADFilter)
		},
	)
	if err != nil {
		return nil, err
	}

	if !shouldRetryWithoutVAD(cmd, summary) {
		return result, nil
	}

	if onVADRetry != nil {
		onVADRetry(
			fmt.Sprintf(
				"dropped %d oversized segment(s), longest %.3fs",
				summary.DroppedSegments,
				summary.LongestDroppedSeconds,
			),
		)
	}

	if retryResult, retrySummary, retryErr := tryValidatedRetry(
		cmd.InputVideo,
		func(path string) (*TranscriptionResult, error) {
			return transcribe(path, false)
		},
	); retryErr != nil {
		fmt.Fprintf(os.Stderr, "warning: VAD-disabled retry on original input failed, keeping current transcription: %v\n", retryErr)
	} else {
		result, summary = considerRetryCandidate(result, summary, retryResult, retrySummary)
	}

	return result, nil
}

func shouldRetryWithoutVAD(cmd Command, summary transcriptionValidation) bool {
	return cmd.VADFilter && summary.LongestDroppedSeconds >= overlongSegmentVADRetryThresholdSeconds
}

func retryImprovesTranscription(candidate transcriptionValidation, baseline transcriptionValidation) bool {
	if candidate.LastKeptEnd > baseline.LastKeptEnd+1 {
		return true
	}
	if candidate.LastKeptEnd+retryCoverageRegressionToleranceSeconds < baseline.LastKeptEnd {
		return false
	}
	if candidate.UsableDuration > baseline.UsableDuration+1 {
		return true
	}
	return candidate.KeptSegments > baseline.KeptSegments
}
