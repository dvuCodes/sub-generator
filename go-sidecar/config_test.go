package main

import (
	"encoding/json"
	"strings"
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
		"asr_backend": "faster_whisper",
		"asr_model_id": "deepdml/faster-whisper-large-v3-turbo-ct2",
		"translation_backend": "nllb",
		"diarization_enabled": true,
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
	if cmd.ASRBackend != "faster_whisper" {
		t.Errorf("ASRBackend = %q, want 'faster_whisper'", cmd.ASRBackend)
	}
	if cmd.ASRModelID != "deepdml/faster-whisper-large-v3-turbo-ct2" {
		t.Errorf("ASRModelID = %q, want faster-whisper model", cmd.ASRModelID)
	}
	if cmd.TranslationBackend != "nllb" {
		t.Errorf("TranslationBackend = %q, want 'nllb'", cmd.TranslationBackend)
	}
	if !cmd.DiarizationEnabled {
		t.Error("expected DiarizationEnabled to be true")
	}
	if cmd.ModelSize != "large-v3" {
		t.Errorf("ModelSize = %q, want 'large-v3'", cmd.ModelSize)
	}
}

func TestResponseSerialization(t *testing.T) {
	resp := CompleteResponse{
		Type:               "complete",
		OutputPath:         "C:/Videos/movie.en.srt",
		Segments:           42,
		DurationSecs:       123.4,
		BackendSummary:     "faster_whisper + nllb",
		DiarizationRan:     true,
		SpeakerCount:       intPtr(2),
		SelectedASRBackend: "faster_whisper",
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
	if decoded.BackendSummary != "faster_whisper + nllb" {
		t.Errorf("BackendSummary = %q, want backend summary", decoded.BackendSummary)
	}
	if !decoded.DiarizationRan {
		t.Error("expected DiarizationRan to be true")
	}
	if decoded.SpeakerCount == nil || *decoded.SpeakerCount != 2 {
		t.Errorf("SpeakerCount = %v, want 2", decoded.SpeakerCount)
	}
	if decoded.SelectedASRBackend != "faster_whisper" {
		t.Errorf("SelectedASRBackend = %q, want faster_whisper", decoded.SelectedASRBackend)
	}
}

func TestVramInfoResponseSerialization(t *testing.T) {
	resp := VramInfoResponse{
		Type: "vram_info",
		Vram: &VRAMInfo{TotalMiB: 8192, UsedMiB: 2048, FreeMiB: 6144},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded VramInfoResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.Type != "vram_info" {
		t.Errorf("Type = %q, want vram_info", decoded.Type)
	}
	if decoded.Vram == nil {
		t.Fatal("Vram is nil, want non-nil")
	}
	if decoded.Vram.TotalMiB != 8192 {
		t.Errorf("TotalMiB = %d, want 8192", decoded.Vram.TotalMiB)
	}
	if decoded.Vram.UsedMiB != 2048 {
		t.Errorf("UsedMiB = %d, want 2048", decoded.Vram.UsedMiB)
	}
	if decoded.Vram.FreeMiB != 6144 {
		t.Errorf("FreeMiB = %d, want 6144", decoded.Vram.FreeMiB)
	}
}

func TestVramInfoResponseNilVram(t *testing.T) {
	resp := VramInfoResponse{Type: "vram_info", Vram: nil}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if !strings.Contains(string(data), `"vram":null`) {
		t.Errorf("expected null vram in JSON, got %s", data)
	}
}

func TestDefaultServiceConfig(t *testing.T) {
	config := DefaultServiceConfig()

	if config.WhisperPort != 8080 {
		t.Errorf("WhisperPort = %d, want 8080", config.WhisperPort)
	}
	if config.LlamaServerPort != 8081 {
		t.Errorf("LlamaServerPort = %d, want 8081", config.LlamaServerPort)
	}
}

func TestAudioConfig_Defaults(t *testing.T) {
	cfg := DefaultAudioConfig()

	if !cfg.Enabled {
		t.Error("expected Enabled to be true")
	}
	if cfg.VocalBoostDB != 3 {
		t.Errorf("expected VocalBoostDB=3, got %g", cfg.VocalBoostDB)
	}
	if !cfg.NoiseGate {
		t.Error("expected NoiseGate to be true")
	}
	if !cfg.Normalize {
		t.Error("expected Normalize to be true")
	}
}

func TestAudioConfig_Deserialize(t *testing.T) {
	input := `{
		"command": "generate",
		"input_video": "test.mkv",
		"model_size": "base",
		"beam_size": 5,
		"vad_filter": true,
		"audio_config": {
			"enabled": true,
			"vocal_boost_db": 4,
			"noise_gate": false,
			"normalize": true
		}
	}`

	var cmd Command
	if err := json.Unmarshal([]byte(input), &cmd); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if !cmd.AudioConfig.Enabled {
		t.Error("expected AudioConfig.Enabled=true")
	}
	if cmd.AudioConfig.VocalBoostDB != 4 {
		t.Errorf("expected VocalBoostDB=4, got %g", cmd.AudioConfig.VocalBoostDB)
	}
	if cmd.AudioConfig.NoiseGate {
		t.Error("expected AudioConfig.NoiseGate=false")
	}
	if !cmd.AudioConfig.Normalize {
		t.Error("expected AudioConfig.Normalize=true")
	}
}

func TestAudioConfig_DeserializeMissing(t *testing.T) {
	input := `{
		"command": "generate",
		"input_video": "test.mkv"
	}`

	var cmd Command
	if err := json.Unmarshal([]byte(input), &cmd); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if cmd.AudioConfig != nil {
		t.Error("expected AudioConfig to be nil when missing from JSON")
	}
}

func TestSystemInfoResponseSerializationIncludesMLBackend(t *testing.T) {
	resp := SystemInfoResponse{
		Type:              "system_info",
		WhisperServer:     true,
		TranslationEngine: false,
		MLBackend:         true,
		GPU:               "NVIDIA",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded SystemInfoResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !decoded.MLBackend {
		t.Fatal("expected MLBackend to round-trip as true")
	}
}

func TestSegmentSerializationIncludesSpeakerMetadata(t *testing.T) {
	segment := Segment{
		Start:        0.5,
		End:          1.5,
		Text:         "Hello",
		SpeakerID:    "spk_1",
		SpeakerLabel: "Speaker 1",
	}

	data, err := json.Marshal(segment)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Segment
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.SpeakerID != "spk_1" {
		t.Errorf("SpeakerID = %q, want spk_1", decoded.SpeakerID)
	}
	if decoded.SpeakerLabel != "Speaker 1" {
		t.Errorf("SpeakerLabel = %q, want Speaker 1", decoded.SpeakerLabel)
	}
}

func TestCapabilitiesResponseSerialization(t *testing.T) {
	resp := CapabilitiesResponse{
		Type: "capabilities",
		Defaults: BackendDefaults{
			ASRBackend:         "faster_whisper",
			ASRModelID:         "deepdml/faster-whisper-large-v3-turbo-ct2",
			TranslationBackend: "nllb",
			DiarizationEnabled: false,
		},
		Backends: BackendCapabilities{
			ASR: []ASRBackendCapability{
				{
					ID:              "faster_whisper",
					DisplayName:     "Faster Whisper",
					Installed:       true,
					DefaultModelID:  "deepdml/faster-whisper-large-v3-turbo-ct2",
					SourceLanguages: []LanguageOption{{Code: "auto", Name: "Auto-detect"}},
				},
			},
			Translation: []TranslationBackendCapability{
				{
					ID:              "nllb",
					DisplayName:     "NLLB",
					Installed:       true,
					DefaultModelID:  "JustFrederik/nllb-200-distilled-600M-ct2-int8",
					TargetLanguages: []LanguageOption{{Code: "eng_Latn", Name: "English"}},
				},
			},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CapabilitiesResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Type != "capabilities" {
		t.Errorf("Type = %q, want capabilities", decoded.Type)
	}
	if decoded.Defaults.ASRBackend != "faster_whisper" {
		t.Errorf("Defaults.ASRBackend = %q, want faster_whisper", decoded.Defaults.ASRBackend)
	}
	if len(decoded.Backends.ASR) != 1 || decoded.Backends.ASR[0].ID != "faster_whisper" {
		t.Fatalf("unexpected ASR backends: %#v", decoded.Backends.ASR)
	}
	if len(decoded.Backends.Translation) != 1 || decoded.Backends.Translation[0].ID != "nllb" {
		t.Fatalf("unexpected translation backends: %#v", decoded.Backends.Translation)
	}
}

func intPtr(value int) *int {
	return &value
}
