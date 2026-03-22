package main

import (
	"encoding/json"
	"testing"
)

func TestCommandParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantCmd string
	}{
		{
			name:    "generate command",
			input:   `{"command":"generate","input_video":"movie.mp4","model_size":"base","beam_size":5,"vad_filter":true}`,
			wantCmd: "generate",
		},
		{
			name:    "list_languages",
			input:   `{"command":"list_languages"}`,
			wantCmd: "list_languages",
		},
		{
			name:    "system_info",
			input:   `{"command":"system_info"}`,
			wantCmd: "system_info",
		},
		{
			name:    "start_services",
			input:   `{"command":"start_services"}`,
			wantCmd: "start_services",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd Command
			err := json.Unmarshal([]byte(tt.input), &cmd)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if cmd.Command != tt.wantCmd {
				t.Errorf("Command = %q, want %q", cmd.Command, tt.wantCmd)
			}
		})
	}
}

func TestGenerateCommandWithLangs(t *testing.T) {
	input := `{
		"command": "generate",
		"input_video": "C:/Videos/anime.mkv",
		"source_lang": "ja",
		"target_lang": "en",
		"output_format": "srt",
		"model_size": "large-v3",
		"beam_size": 5,
		"vad_filter": true
	}`

	var cmd Command
	err := json.Unmarshal([]byte(input), &cmd)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if cmd.Command != "generate" {
		t.Errorf("Command = %q, want 'generate'", cmd.Command)
	}
	if cmd.InputVideo != "C:/Videos/anime.mkv" {
		t.Errorf("InputVideo = %q, want 'C:/Videos/anime.mkv'", cmd.InputVideo)
	}
	if cmd.SourceLang == nil || *cmd.SourceLang != "ja" {
		t.Errorf("SourceLang = %v, want 'ja'", cmd.SourceLang)
	}
	if cmd.TargetLang == nil || *cmd.TargetLang != "en" {
		t.Errorf("TargetLang = %v, want 'en'", cmd.TargetLang)
	}
	if cmd.ModelSize != "large-v3" {
		t.Errorf("ModelSize = %q, want 'large-v3'", cmd.ModelSize)
	}
}

func TestResponseSerialization(t *testing.T) {
	resp := CompleteResponse{
		Type:         "complete",
		OutputPath:   "C:/Videos/movie.en.srt",
		Segments:     42,
		DurationSecs: 123.4,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CompleteResponse
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Type != "complete" {
		t.Errorf("Type = %q, want 'complete'", decoded.Type)
	}
	if decoded.Segments != 42 {
		t.Errorf("Segments = %d, want 42", decoded.Segments)
	}
}

func TestDefaultServiceConfig(t *testing.T) {
	config := DefaultServiceConfig()

	if config.WhisperPort != 8080 {
		t.Errorf("WhisperPort = %d, want 8080", config.WhisperPort)
	}
	if config.LibreTranslatePort != 5000 {
		t.Errorf("LibreTranslatePort = %d, want 5000", config.LibreTranslatePort)
	}
}
