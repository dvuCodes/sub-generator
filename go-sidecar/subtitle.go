package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	astisub "github.com/asticode/go-astisub"
)

type SubtitleWriter struct{}

func NewSubtitleWriter() *SubtitleWriter {
	return &SubtitleWriter{}
}

// Write persists normalized cues in the requested format. Line wrapping is
// applied here (when not already present) so SRT, VTT and ASS output share
// identical line breaks.
func (sw *SubtitleWriter) Write(segments []Segment, outputPath string, format string, targetLang *string) error {
	subs := astisub.NewSubtitles()

	subs.Metadata = &astisub.Metadata{
		Title: "SubGen Generated Subtitles",
		// Emit modern ASS (v4.00+) instead of legacy SSA v4.00 so players
		// apply numpad alignment semantics and our style block correctly.
		SSAScriptType: "v4.00+",
		SSAPlayResX:   astiInt(384),
		SSAPlayResY:   astiInt(288),
		SSAWrapStyle:  "0", // smart wrapping, top line wider
	}

	if format == "ass" {
		subs.Styles = map[string]*astisub.Style{
			"Default": buildDefaultStyle(targetLang),
		}
	}

	for _, seg := range segments {
		lines := seg.Lines
		displayText := formatSegmentText(seg)
		if len(lines) == 0 || seg.SpeakerLabel != "" {
			lines = WrapCueLines(displayText, targetLang)
		}
		if len(lines) == 0 {
			continue
		}

		lines2 := make([]astisub.Line, 0, len(lines))
		for _, line := range lines {
			lines2 = append(lines2, astisub.Line{
				Items: []astisub.LineItem{{Text: line}},
			})
		}

		subs.Items = append(subs.Items, &astisub.Item{
			StartAt: time.Duration(seg.Start * float64(time.Second)),
			EndAt:   time.Duration(seg.End * float64(time.Second)),
			Lines:   lines2,
		})
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	switch format {
	case "srt":
		return subs.Write(outputPath)
	case "ass":
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
		defer f.Close()
		return subs.WriteToSSA(f)
	case "vtt":
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
		defer f.Close()
		return subs.WriteToWebVTT(f)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func buildDefaultStyle(targetLang *string) *astisub.Style {
	fontName := "Arial"
	if targetLang != nil && isCJKLanguage(*targetLang) {
		fontName = "Arial Unicode MS"
	}

	return &astisub.Style{
		ID: "Default",
		InlineStyle: &astisub.StyleAttributes{
			SSAFontName:       fontName,
			SSAFontSize:       astiFloat(24),
			SSAPrimaryColour:  astiColor(255, 255, 255),
			SSAOutlineColour:  astiColor(0, 0, 0),
			SSABackColour:     astiColor(0, 0, 0),
			SSABold:           astiBool(false),
			SSAOutline:        astiFloat(2),
			SSAShadow:         astiFloat(1),
			SSAAlignment:      astiInt(2), // bottom center
			SSAMarginLeft:     astiInt(10),
			SSAMarginRight:    astiInt(10),
			SSAMarginVertical: astiInt(20),
		},
	}
}

// WriteQCReport persists the QC summary next to the subtitle file as JSON.
func WriteQCReport(report *QCReport, outputPath string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal QC report: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create QC report directory: %w", err)
	}
	return os.WriteFile(outputPath, data, 0o644)
}

// DeriveOutputPath generates an output path from the input video path.
// e.g., "movie.mp4" with target "ja" and format "srt" → "movie.ja.srt"
func DeriveOutputPath(inputVideo string, format string, targetLang *string) string {
	ext := filepath.Ext(inputVideo)
	base := strings.TrimSuffix(inputVideo, ext)

	if targetLang != nil && *targetLang != "" {
		return fmt.Sprintf("%s.%s.%s", base, *targetLang, format)
	}
	return fmt.Sprintf("%s.%s", base, format)
}

func isCJKLanguage(lang string) bool {
	switch lang {
	case "ja", "zh", "ko":
		return true
	default:
		return false
	}
}

// Helper functions for go-astisub style attributes
func astiFloat(v float64) *float64 { return &v }
func astiBool(v bool) *bool        { return &v }
func astiInt(v int) *int           { return &v }

func astiColor(r, g, b int) *astisub.Color {
	return &astisub.Color{Red: uint8(r), Green: uint8(g), Blue: uint8(b), Alpha: 0}
}

// DeriveTranscriptionLogPath generates a .transcription.txt path from the input video path.
// e.g., "movie.mp4" → "movie.transcription.txt"
func DeriveTranscriptionLogPath(inputVideo string) string {
	ext := filepath.Ext(inputVideo)
	base := strings.TrimSuffix(inputVideo, ext)
	return base + ".transcription.txt"
}

// WriteTranscriptionLog writes the raw transcription segments (with their start/end
// timestamps) to a .txt file for diagnostic purposes (root-cause analysis of
// transcription vs. translation quality issues).
func WriteTranscriptionLog(segments []Segment, outputPath string, sourceLang string) error {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create transcription log: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "# Transcription (source language: %s)\n\n", sourceLang)

	for _, seg := range segments {
		start := formatTimestamp(seg.Start)
		end := formatTimestamp(seg.End)
		fmt.Fprintf(f, "[%s --> %s] %s\n", start, end, formatSegmentText(seg))
	}

	return nil
}

func formatSegmentText(seg Segment) string {
	text := strings.TrimSpace(seg.Text)
	if seg.SpeakerLabel == "" {
		return text
	}
	return fmt.Sprintf("[%s] %s", seg.SpeakerLabel, text)
}

func formatTimestamp(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}
