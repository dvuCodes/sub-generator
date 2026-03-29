package main

import (
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

	installDir := preferredWhisperInstallDir(p.svcManager.config.SearchRoots)
	if selectedASRBackend == "whisper_cpp" {
		modelFile := modelFilename(cmd.ModelSize)
		modelPath := filepath.Join(installDir, "models", modelFile)

		if _, err := os.Stat(modelPath); err != nil {
			if !os.IsNotExist(err) {
				sendError("Model check failed", fmt.Sprintf("cannot access model at %q: %v", modelPath, err))
				return
			}
			sendStage("downloading_model", fmt.Sprintf("Downloading %s model...", cmd.ModelSize))
			url := ModelDownloadURL(cmd.ModelSize)
			if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
				sendError("Model download failed", err.Error())
				return
			}
			if err := DownloadModel(url, modelPath, func(downloaded, total int64) {
				if total > 0 {
					pct := float64(downloaded) / float64(total) * 100
					sendProgress("downloading_model", pct, fmt.Sprintf("Downloading %s / %s", formatBytes(downloaded), formatBytes(total)))
				} else {
					sendProgress("downloading_model", 0, fmt.Sprintf("Downloading %s...", formatBytes(downloaded)))
				}
			}); err != nil {
				sendError("Model download failed", err.Error())
				return
			}
			sendProgress("downloading_model", 100, "Model downloaded")
		}

		if cmd.VADFilter {
			vadModelPath := filepath.Join(installDir, "models", vadModelFilename)
			if _, err := os.Stat(vadModelPath); err != nil && !os.IsNotExist(err) {
				sendError("VAD model check failed", fmt.Sprintf("cannot access VAD model at %q: %v", vadModelPath, err))
				return
			} else if os.IsNotExist(err) {
				sendStage("downloading_model", "Downloading VAD model...")
				if err := os.MkdirAll(filepath.Dir(vadModelPath), 0o755); err != nil {
					sendError("VAD model download failed", err.Error())
					return
				}
				if err := DownloadModel(VADModelDownloadURL(), vadModelPath, func(downloaded, total int64) {
					if total > 0 {
						pct := float64(downloaded) / float64(total) * 100
						sendProgress("downloading_model", pct, fmt.Sprintf("Downloading VAD model %s / %s", formatBytes(downloaded), formatBytes(total)))
					} else {
						sendProgress("downloading_model", 0, fmt.Sprintf("Downloading VAD model %s...", formatBytes(downloaded)))
					}
				}); err != nil {
					sendError("VAD model download failed", err.Error())
					return
				}
				sendProgress("downloading_model", 100, "VAD model downloaded")
			}
		}
	}

	// Step 3: Ensure services are running
	sendStage("starting_services", "Ensuring services are running...")
	if err := p.ensureServices(cmd); err != nil {
		sendError("Service startup failed", err.Error())
		return
	}

	// Step 4: Preprocess audio (if enabled)
	transcribePath := cmd.InputVideo
	audioConfig := cmd.AudioConfig
	if audioConfig == nil {
		ac := DefaultAudioConfig()
		audioConfig = &ac
	}

	if audioConfig.Enabled {
		sendStage("preprocessing", "Enhancing audio for transcription...")
		preprocessedPath, preprocessErr := PreprocessAudio(cmd.InputVideo, *audioConfig)
		if preprocessErr != nil {
			fmt.Fprintf(os.Stderr, "warning: audio preprocessing failed, falling back to raw upload: %v\n", preprocessErr)
			sendStage("preprocessing", "Audio enhancement skipped, using original audio")
		} else {
			transcribePath = preprocessedPath
			defer cleanupPreprocessed(preprocessedPath)
		}
	}

	// Step 5: Transcribe
	sendStage("transcribing", "Transcribing speech...")

	mediaDuration, probeErr := MediaDuration(cmd.InputVideo)
	if probeErr != nil {
		fmt.Fprintf(os.Stderr, "ffprobe duration probe failed (ETA unavailable): %v\n", probeErr)
	}

	hasGPU := detectGPU() != "none"
	estimatedSecs := EstimateTranscriptionSeconds(mediaDuration, cmd.ModelSize, hasGPU)
	if selectedASRBackend != "whisper_cpp" {
		estimatedSecs = 0
	}

	done := make(chan struct{})
	transcribeStart := time.Now()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed := time.Since(transcribeStart).Seconds()

				var percent float64
				var etaSecs float64
				var msg string

				if estimatedSecs > 0 {
					percent = (elapsed / estimatedSecs) * 100
					if percent > 99 {
						percent = 99
					}
					etaSecs = estimatedSecs - elapsed
					if etaSecs < 0 {
						etaSecs = 0
					}

					if elapsed > estimatedSecs*1.2 {
						msg = fmt.Sprintf("Transcribing... %s elapsed (taking longer than expected)", formatDuration(elapsed))
					} else {
						msg = fmt.Sprintf("Transcribing... %s elapsed / ~%s remaining", formatDuration(elapsed), formatDuration(etaSecs))
					}
				} else {
					msg = fmt.Sprintf("Transcribing... %s elapsed", formatDuration(elapsed))
				}

				sendTimerProgress("transcribing", percent, msg, elapsed, etaSecs)
			}
		}
	}()

	result, err := p.transcribe(cmd, selectedASRBackend, selectedASRModelID, transcribePath)
	close(done)

	if err != nil {
		sendError("Transcription failed", err.Error())
		return
	}

	sendProgress("transcribing", 100, fmt.Sprintf("Transcribed %d segments", len(result.Segments)))

	segments := result.Segments
	diarizationRan := false
	var speakerCount *int

	if diarizationRequested {
		sendStage("diarizing", "Labeling speakers...")
		annotatedSegments, count, annotateErr := p.annotateDiarization(transcribePath, segments)
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
		sourceLang := result.Language
		if cmd.SourceLang != nil && *cmd.SourceLang != "" && *cmd.SourceLang != "auto" {
			sourceLang = *cmd.SourceLang
		}
		if err := WriteTranscriptionLog(segments, logPath, sourceLang); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write transcription log to %q: %v\n", logPath, err)
		} else {
			transcriptionLogPath = logPath
		}
	}

	// Step 6: Translate (if target language specified)
	if cmd.TargetLang != nil && *cmd.TargetLang != "" && selectedTranslationBackend != "none" {
		sendStage("translating", fmt.Sprintf("Translating to %s...", *cmd.TargetLang))

		// Determine source language
		sourceLang := "auto"
		if cmd.SourceLang != nil && *cmd.SourceLang != "" && *cmd.SourceLang != "auto" {
			sourceLang = *cmd.SourceLang
		} else if result.Language != "" {
			sourceLang = result.Language
		}
		if selectedTranslationBackend == defaultTranslationBackend && sourceLang == "auto" {
			sendError("Translation failed", "Could not determine the source language for NLLB translation. Choose a source language explicitly or retry with clearer speech.")
			return
		}

		translated, err := p.translateSegments(
			selectedTranslationBackend,
			selectedTranslationModelID,
			segments,
			sourceLang,
			*cmd.TargetLang,
		)
		if err != nil {
			sendError("Translation failed", err.Error())
			return
		}

		segments = translated
	}

	// Step 7: Write subtitle file
	sendStage("writing", "Writing subtitle file...")

	outputFormat := cmd.OutputFormat
	if outputFormat == "" {
		outputFormat = "srt"
	}

	outputPath := ""
	if cmd.OutputPath != nil && *cmd.OutputPath != "" {
		outputPath = *cmd.OutputPath
	} else {
		outputPath = DeriveOutputPath(cmd.InputVideo, outputFormat, cmd.TargetLang)
	}

	writer := NewSubtitleWriter()
	if err := writer.Write(segments, outputPath, outputFormat, cmd.TargetLang); err != nil {
		sendError("Failed to write subtitle file", err.Error())
		return
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
	})
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
		return fmt.Errorf("unsupported video format '%s' (supported: %s)", ext, supportedExtsList())
	}

	return nil
}

func (p *Pipeline) transcribe(cmd Command, backend, modelID, transcribePath string) (*TranscriptionResult, error) {
	switch backend {
	case "whisper_cpp":
		transcriber := NewTranscriber(p.svcManager.config.WhisperPort)
		return transcribeWithOptionalVADRetry(
			cmd,
			transcribePath,
			cmd.InputVideo,
			func(path string, vadFilter bool) (*TranscriptionResult, error) {
				return transcriber.Transcribe(
					path,
					cmd.SourceLang,
					cmd.BeamSize,
					vadFilter,
				)
			},
			func(reason string) {
				fmt.Fprintf(
					os.Stderr,
					"warning: enhanced audio transcription returned no usable subtitle segments, retrying original input: %s\n",
					reason,
				)
				sendStage("transcribing", "Enhanced audio produced no usable transcript, retrying original audio...")
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
			transcribePath,
			cmd.InputVideo,
			func(path string, vadFilter bool) (*TranscriptionResult, error) {
				return client.Transcribe(path, cmd.SourceLang, modelID, cmd.BeamSize, vadFilter)
			},
			func(reason string) {
				fmt.Fprintf(
					os.Stderr,
					"warning: enhanced audio transcription returned unusable subtitle timings, retrying original input: %s\n",
					reason,
				)
				sendStage("transcribing", "Enhanced audio produced unusable subtitle timings, retrying original audio...")
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

func (p *Pipeline) ensureServices(cmd Command) error {
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
	sendProgress("starting_services", 100, "All services ready")

	return nil
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

func transcribeWithValidationFallback(
	primaryPath string,
	rawPath string,
	transcribe transcribeAttempt,
	onFallback func(reason string),
) (*TranscriptionResult, error) {
	result, _, err := transcribeWithValidationFallbackDetailed(
		primaryPath,
		rawPath,
		transcribe,
		onFallback,
	)
	return result, err
}

func transcribeWithValidationFallbackDetailed(
	primaryPath string,
	rawPath string,
	transcribe transcribeAttempt,
	onFallback func(reason string),
) (*TranscriptionResult, transcriptionValidation, error) {
	result, summary, err := transcribeValidated(primaryPath, transcribe)
	if err == nil {
		return result, summary, nil
	}

	if primaryPath == "" || rawPath == "" || primaryPath == rawPath {
		return nil, summary, err
	}

	if onFallback != nil {
		onFallback(err.Error())
	}

	result, summary, err = transcribeValidated(rawPath, transcribe)
	if err != nil {
		return nil, summary, err
	}

	return result, summary, nil
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
	primaryPath string,
	rawPath string,
	transcribe func(path string, vadFilter bool) (*TranscriptionResult, error),
	onInputFallback func(reason string),
	onVADRetry func(reason string),
) (*TranscriptionResult, error) {
	result, summary, err := transcribeWithValidationFallbackDetailed(
		primaryPath,
		rawPath,
		func(path string) (*TranscriptionResult, error) {
			return transcribe(path, cmd.VADFilter)
		},
		onInputFallback,
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
		primaryPath,
		func(path string) (*TranscriptionResult, error) {
			return transcribe(path, false)
		},
	); retryErr != nil {
		fmt.Fprintf(os.Stderr, "warning: VAD-disabled retry on enhanced input failed, keeping current transcription: %v\n", retryErr)
	} else {
		result, summary = considerRetryCandidate(result, summary, retryResult, retrySummary)
	}

	if primaryPath != "" && rawPath != "" && primaryPath != rawPath {
		if retryResult, retrySummary, retryErr := tryValidatedRetry(
			rawPath,
			func(path string) (*TranscriptionResult, error) {
				return transcribe(path, false)
			},
		); retryErr != nil {
			fmt.Fprintf(os.Stderr, "warning: VAD-disabled retry on original input failed, keeping current transcription: %v\n", retryErr)
		} else {
			result, summary = considerRetryCandidate(result, summary, retryResult, retrySummary)
		}
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
