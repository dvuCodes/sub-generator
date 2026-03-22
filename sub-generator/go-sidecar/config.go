package main

// --- IPC Command Types (received from Tauri via stdin) ---

type Command struct {
	Command     string  `json:"command"`
	InputVideo  string  `json:"input_video,omitempty"`
	SourceLang  *string `json:"source_lang,omitempty"`
	TargetLang  *string `json:"target_lang,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
	OutputPath  *string `json:"output_path,omitempty"`
	ModelSize   string  `json:"model_size,omitempty"`
	BeamSize    int     `json:"beam_size,omitempty"`
	VADFilter   bool    `json:"vad_filter,omitempty"`
	// install_language fields
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
}

// --- IPC Response Types (sent to Tauri via stdout) ---

type ProgressResponse struct {
	Type    string  `json:"type"`
	Stage   string  `json:"stage"`
	Percent float64 `json:"percent"`
	Message string  `json:"message"`
}

type StageResponse struct {
	Type    string `json:"type"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type CompleteResponse struct {
	Type         string  `json:"type"`
	OutputPath   string  `json:"output_path"`
	Segments     int     `json:"segments"`
	DurationSecs float64 `json:"duration_secs"`
}

type ErrorResponse struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type LanguagePair struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type LanguagesResponse struct {
	Type      string         `json:"type"`
	Installed []LanguagePair `json:"installed"`
}

type SystemInfoResponse struct {
	Type           string `json:"type"`
	WhisperServer  bool   `json:"whisper_server"`
	LibreTranslate bool   `json:"libretranslate"`
	GPU            string `json:"gpu"`
}

// --- Transcription Types ---

type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type TranscriptionResult struct {
	Text     string    `json:"text"`
	Segments []Segment `json:"segments"`
	Language string    `json:"language,omitempty"`
}

// --- Service Configuration ---

type ServiceConfig struct {
	WhisperServerPath string
	WhisperModelPath  string
	WhisperPort       int
	LibreTranslatePort int
}

func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		WhisperServerPath: "whisper-server",
		WhisperModelPath:  "models/ggml-base.bin",
		WhisperPort:       8080,
		LibreTranslatePort: 5000,
	}
}
