package main

import "fmt"

var gpuSpeedRatios = map[string]float64{
	"tiny":     30.0,
	"base":     15.0,
	"small":    8.0,
	"medium":   4.0,
	"large-v3": 2.0,
	"turbo":    10.0,
}

var cpuSpeedRatios = map[string]float64{
	"tiny":     8.0,
	"base":     4.0,
	"small":    2.0,
	"medium":   0.8,
	"large-v3": 0.3,
	"turbo":    3.0,
}

func ModelSpeedRatio(modelSize string, hasGPU bool) float64 {
	ratios := cpuSpeedRatios
	if hasGPU {
		ratios = gpuSpeedRatios
	}
	return ratios[modelSize]
}

func EstimateTranscriptionSeconds(audioDurationSecs float64, modelSize string, hasGPU bool) float64 {
	ratio := ModelSpeedRatio(modelSize, hasGPU)
	if ratio <= 0 || audioDurationSecs <= 0 {
		return 0
	}
	return audioDurationSecs / ratio
}

func formatDuration(secs float64) string {
	total := int(secs)
	if total < 0 {
		total = 0
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
