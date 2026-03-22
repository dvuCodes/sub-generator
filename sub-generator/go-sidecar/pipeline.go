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
	svcManager *ServiceManager
}

func NewPipeline(svcManager *ServiceManager) *Pipeline {
	return &Pipeline{svcManager: svcManager}
}

func (p *Pipeline) Run(cmd Command) {
	startTime := time.Now()

	// Step 1: Validate input
	sendStage("validating", "Validating input file...")
	if err := p.validateInput(cmd.InputVideo); err != nil {
		sendError("Validation failed", err.Error())
		return
	}

	// Step 2: Ensure services are running
	sendStage("starting_services", "Ensuring services are running...")
	if err := p.ensureServices(cmd); err != nil {
		sendError("Service startup failed", err.Error())
		return
	}

	// Step 3: Transcribe
	sendStage("transcribing", "Transcribing speech...")
	transcriber := NewTranscriber(p.svcManager.config.WhisperPort)
	result, err := transcriber.Transcribe(
		cmd.InputVideo,
		cmd.SourceLang,
		cmd.ModelSize,
		cmd.BeamSize,
		cmd.VADFilter,
	)
	if err != nil {
		sendError("Transcription failed", err.Error())
		return
	}

	sendProgress("transcribing", 100, fmt.Sprintf("Transcribed %d segments", len(result.Segments)))

	segments := result.Segments

	// Step 4: Translate (if target language specified)
	if cmd.TargetLang != nil && *cmd.TargetLang != "" {
		sendStage("translating", fmt.Sprintf("Translating to %s...", *cmd.TargetLang))

		translator := NewTranslator(p.svcManager.config.LibreTranslatePort)

		// Determine source language
		sourceLang := "auto"
		if cmd.SourceLang != nil && *cmd.SourceLang != "" {
			sourceLang = *cmd.SourceLang
		} else if result.Language != "" {
			sourceLang = result.Language
		}

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

	// Step 5: Write subtitle file
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
		Type:         "complete",
		OutputPath:   outputPath,
		Segments:     len(segments),
		DurationSecs: duration,
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
	// Always need whisper-server for transcription
	if !p.svcManager.IsWhisperRunning() {
		sendProgress("starting_services", 25, "Starting whisper-server...")
		if err := p.svcManager.StartWhisperServer(); err != nil {
			return fmt.Errorf("whisper-server: %w", err)
		}
	}
	sendProgress("starting_services", 50, "whisper-server ready")

	// Only need LibreTranslate if translating
	if cmd.TargetLang != nil && *cmd.TargetLang != "" {
		if !p.svcManager.IsLibreTranslateRunning() {
			sendProgress("starting_services", 75, "Starting LibreTranslate...")
			if err := p.svcManager.StartLibreTranslate(); err != nil {
				return fmt.Errorf("libretranslate: %w", err)
			}
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
