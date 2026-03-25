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

type Pipeline struct {
	svcManager       *ServiceManager
	cleanupServices func()
}

func NewPipeline(svcManager *ServiceManager) *Pipeline {
	return &Pipeline{
		svcManager:       svcManager,
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

	// Step 1: Validate input
	sendStage("validating", "Validating input file...")
	if err := p.validateInput(cmd.InputVideo); err != nil {
		sendError("Validation failed", err.Error())
		return
	}

	// Step 2: Download model if missing
	modelFile := modelFilename(cmd.ModelSize)
	installDir := preferredWhisperInstallDir(p.svcManager.config.SearchRoots)
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

	// Step 2b: Download VAD model if needed
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

	// Step 3: Ensure services are running
	sendStage("starting_services", "Ensuring services are running...")
	if err := p.ensureServices(cmd); err != nil {
		sendError("Service startup failed", err.Error())
		return
	}

	// Step 4: Transcribe
	sendStage("transcribing", "Transcribing speech...")

	mediaDuration, probeErr := MediaDuration(cmd.InputVideo)
	if probeErr != nil {
		fmt.Fprintf(os.Stderr, "ffprobe duration probe failed (ETA unavailable): %v\n", probeErr)
	}

	hasGPU := detectGPU() != "none"
	estimatedSecs := EstimateTranscriptionSeconds(mediaDuration, cmd.ModelSize, hasGPU)

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

	transcriber := NewTranscriber(p.svcManager.config.WhisperPort)
	result, err := transcriber.Transcribe(
		cmd.InputVideo,
		cmd.SourceLang,
		cmd.BeamSize,
		cmd.VADFilter,
	)
	close(done)

	if err != nil {
		sendError("Transcription failed", err.Error())
		return
	}

	if err := validateTranscriptionResult(result); err != nil {
		sendError("Transcription failed", err.Error())
		return
	}

	sendProgress("transcribing", 100, fmt.Sprintf("Transcribed %d segments", len(result.Segments)))

	segments := result.Segments

	// Step 4b: Write diagnostic transcription log (before translation overwrites segments)
	var transcriptionLogPath string
	if cmd.TargetLang != nil && *cmd.TargetLang != "" {
		logPath := DeriveTranscriptionLogPath(cmd.InputVideo)
		sourceLang := result.Language
		if cmd.SourceLang != nil && *cmd.SourceLang != "" {
			sourceLang = *cmd.SourceLang
		}
		if err := WriteTranscriptionLog(result.Segments, logPath, sourceLang); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write transcription log to %q: %v\n", logPath, err)
		} else {
			transcriptionLogPath = logPath
		}
	}

	// Step 5: Translate (if target language specified)
	if cmd.TargetLang != nil && *cmd.TargetLang != "" {
		sendStage("translating", fmt.Sprintf("Translating to %s...", *cmd.TargetLang))

		// Determine source language
		sourceLang := "auto"
		if cmd.SourceLang != nil && *cmd.SourceLang != "" {
			sourceLang = *cmd.SourceLang
		} else if result.Language != "" {
			sourceLang = result.Language
		}

		if !supportsTranslationPair(sourceLang, *cmd.TargetLang) {
			sendError(
				"Translation failed",
				fmt.Sprintf(
					"translation pair %s -> %s is not supported by GemmaTranslate",
					sourceLang,
					*cmd.TargetLang,
				),
			)
			return
		}

		llamaDir := preferredLlamaInstallDir(p.svcManager.config.SearchRoots)
		gemmaModelPath := filepath.Join(llamaDir, "models", gemmaModelFilenameConst)
		if _, err := os.Stat(gemmaModelPath); err != nil {
			if !os.IsNotExist(err) {
				sendError("Translation model check failed", fmt.Sprintf("cannot access model at %q: %v", gemmaModelPath, err))
				return
			}
			sendStage("downloading_model", "Downloading translation model (~7 GB)...")
			if err := os.MkdirAll(filepath.Dir(gemmaModelPath), 0o755); err != nil {
				sendError("Translation model download failed", err.Error())
				return
			}
			if err := DownloadModel(GemmaModelDownloadURL(), gemmaModelPath, func(downloaded, total int64) {
				if total > 0 {
					pct := float64(downloaded) / float64(total) * 100
					sendProgress("downloading_model", pct, fmt.Sprintf("Downloading translation model %s / %s", formatBytes(downloaded), formatBytes(total)))
				} else {
					sendProgress("downloading_model", 0, fmt.Sprintf("Downloading translation model %s...", formatBytes(downloaded)))
				}
			}); err != nil {
				sendError("Translation model download failed", err.Error())
				return
			}
			sendProgress("downloading_model", 100, "Translation model downloaded")
		}

		// Stop whisper-server to free GPU VRAM before starting llama-server
		p.svcManager.StopWhisperServer()

		sendProgress("translating", 0, "Starting translation engine...")
		if err := p.svcManager.StartLlamaServer(); err != nil {
			sendError("Translation engine startup failed", err.Error())
			return
		}

		translator := NewTranslator(p.svcManager.config.LlamaServerPort)

		translated, err := translator.TranslateSegments(
			segments,
			sourceLang,
			*cmd.TargetLang,
			func(current, total int) {
				pct := float64(current) / float64(total) * 100
				sendProgress("translating", pct, fmt.Sprintf("Translated %d/%d segments", current, total))
			},
		)
		if err != nil {
			sendError("Translation failed", err.Error())
			return
		}

		segments = translated
	}

	// Step 6: Write subtitle file
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
		Type:             "complete",
		OutputPath:       outputPath,
		TranscriptionLog: transcriptionLogPath,
		Segments:         len(segments),
		DurationSecs:     duration,
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

func (p *Pipeline) ensureServices(cmd Command) error {
	// Start whisper-server for transcription (llama-server starts later, just before translation)
	sendProgress("starting_services", 25, "Starting whisper-server...")
	if err := p.svcManager.StartWhisperServer(cmd.ModelSize); err != nil {
		return fmt.Errorf("whisper-server: %w", err)
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
	if result == nil {
		return fmt.Errorf("whisper-server returned no transcription result")
	}

	if len(result.Segments) > 0 {
		usable := result.Segments[:0]
		firstInvalidIndex := -1
		var firstInvalid Segment

		for i, segment := range result.Segments {
			if segment.Start >= 0 && segment.End > segment.Start {
				usable = append(usable, segment)
				continue
			}
			if firstInvalidIndex == -1 {
				firstInvalidIndex = i
				firstInvalid = segment
			}
		}

		if len(usable) > 0 {
			result.Segments = usable
			return nil
		}

		return fmt.Errorf(
			"whisper-server returned unusable segment timing data for every segment (first invalid segment %d start=%.3f end=%.3f), so subtitle timing could not be generated",
			firstInvalidIndex+1,
			firstInvalid.Start,
			firstInvalid.End,
		)
	}

	if strings.TrimSpace(result.Text) != "" {
		return fmt.Errorf("whisper-server returned transcript text but no timestamped segments, so subtitle timing could not be generated")
	}

	return fmt.Errorf("no speech was detected in the input video, so there are no subtitle segments to write")
}
