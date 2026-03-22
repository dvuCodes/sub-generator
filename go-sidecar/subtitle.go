package main

import (
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

func (sw *SubtitleWriter) Write(segments []Segment, outputPath string, format string, targetLang *string) error {
	subs := astisub.NewSubtitles()

	// Set metadata
	subs.Metadata = &astisub.Metadata{
		Title: "SubGen Generated Subtitles",
	}

	// Apply CJK-friendly style for ASS format
	if format == "ass" && targetLang != nil && isCJKLanguage(*targetLang) {
		subs.Styles = map[string]*astisub.Style{
			"Default": {
				ID: "Default",
				InlineStyle: &astisub.StyleAttributes{
					SSAFontName:       "Arial Unicode MS",
					SSAFontSize:       astiFloat(24),
					SSAPrimaryColour:  astiColor(255, 255, 255), // white
					SSAOutlineColour:  astiColor(0, 0, 0),       // black outline
					SSABackColour:     astiColor(0, 0, 0),       // black shadow
					SSABold:           astiBool(false),
					SSAOutline:        astiFloat(2),
					SSAShadow:         astiFloat(1),
					SSAAlignment:      astiInt(2), // bottom center
					SSAMarginLeft:     astiInt(10),
					SSAMarginRight:    astiInt(10),
					SSAMarginVertical: astiInt(20),
				},
			},
		}
	}

	// Convert segments to subtitle items
	for _, seg := range segments {
		item := &astisub.Item{
			StartAt: time.Duration(seg.Start * float64(time.Second)),
			EndAt:   time.Duration(seg.End * float64(time.Second)),
			Lines: []astisub.Line{
				{
					Items: []astisub.LineItem{
						{Text: strings.TrimSpace(seg.Text)},
					},
				},
			},
		}
		subs.Items = append(subs.Items, item)
	}

	// Ensure output directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write file based on format
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
