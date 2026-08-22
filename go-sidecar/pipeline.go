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

type Pipeline struct {
	svcManager *ServiceManager
}

func NewPipeline(svcManager *ServiceManager) *Pipeline {
	return &Pipeline{svcManager: svcManager}
}

func (p *Pipeline) Run(ctx context.Context, cmd Command) {
	startTime := time.Now()

	// Step 1: Validate input
	sendStage("validating", "Validating input file...")
	if err := p.validateInput(cmd.InputVideo); err != nil {
		sendError("Validation failed", err.Error())
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

	// Resolve translation engine before starting LibreTranslate-only services.
	var engine TranslateEngine
	translating := cmd.TargetLang != nil && strings.TrimSpace(*cmd.TargetLang) != ""
	targetLang := ""
	if translating {
		targetLang = *cmd.TargetLang
		var err error
		engine, err = selectTranslationEngine(cmd, p.svcManager.config.LibreTranslatePort)
		if err != nil {
			sendError("Translation setup failed", err.Error())
			return
		}
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
	transcriber := NewTranscriber(p.svcManager.config.WhisperPort)
	stopHeartbeat := startTranscribeHeartbeat(audioDuration)
	result, err := transcriber.Transcribe(ctx, wavPath, buildTranscribeOptions(cmd, p.svcManager.HasVADModel()))
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

	// Step 5: Pre-translation timing normalization on source text.
	sendStage("timing", "Normalizing cue timing...")
	preOpts := NewTimingOptions(sourceLangForTiming(sourceLang), cmd.FrameRate)
	preOpts.ShotTimes = shotTimes
	segments := NormalizeCues(result.Segments, preOpts, shotSnap)

	// Step 6: Translation with sliding-window context.
	if translating {
		sendStage("translating", fmt.Sprintf("Translating to %s via %s...", targetLang, engine.Name()))

		req := TranslateRequest{
			SourceLang: firstNonEmpty(sourceLang, "auto"),
			TargetLang: targetLang,
			Synopsis:   cmd.Synopsis,
			Glossary:   cmd.Glossary,
			Honorifics: firstNonEmpty(cmd.Honorifics, "keep"),
		}

		translated, err := ContextualTranslate(ctx, segments, engine, req, func(current, total int) {
			pct := float64(current) / float64(total) * 100
			sendProgress("translating", pct, fmt.Sprintf("Translated %d/%d lines", current, total))
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				sendCancelled()
				return
			}
			sendError("Translation failed", err.Error())
			return
		}
		segments = translated

		if cmd.QAPass {
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
	postOpts := NewTimingOptions(targetLang, cmd.FrameRate)
	postOpts.ShotTimes = shotTimes
	segments = NormalizeCues(segments, postOpts, shotSnap)

	FinalizeLineWrapping(segments, targetLang)

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
		Type:         "complete",
		OutputPath:   outputPath,
		Segments:     len(segments),
		DurationSecs: duration,
		QC:           report,
		Preview:      cuePreview(segments, 80),
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
	sendProgress("starting_services", 25, "Starting whisper-server...")
	if err := p.svcManager.StartWhisperServer(cmd.ModelSize); err != nil {
		return fmt.Errorf("whisper-server: %w", err)
	}
	sendProgress("starting_services", 60, "whisper-server ready")

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

func supportedExtsList() string {
	exts := make([]string, 0, len(supportedVideoExts))
	for ext := range supportedVideoExts {
		exts = append(exts, ext)
	}
	return strings.Join(exts, ", ")
}
