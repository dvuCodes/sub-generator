package main

import (
	"os"
	"path/filepath"
	"strings"
)

// --- IPC Command Types (received from Tauri via stdin) ---

type Command struct {
	Command            string       `json:"command"`
	InputVideo         string       `json:"input_video,omitempty"`
	SourceLang         *string      `json:"source_lang,omitempty"`
	TargetLang         *string      `json:"target_lang,omitempty"`
	OutputFormat       string       `json:"output_format,omitempty"`
	OutputPath         *string      `json:"output_path,omitempty"`
	ASRBackend         string       `json:"asr_backend,omitempty"`
	ASRModelID         string       `json:"asr_model_id,omitempty"`
	ModelSize          string       `json:"model_size,omitempty"`
	TranslationBackend string       `json:"translation_backend,omitempty"`
	DiarizationEnabled bool         `json:"diarization_enabled,omitempty"`
	BeamSize           int          `json:"beam_size,omitempty"`
	VADFilter          bool         `json:"vad_filter,omitempty"`
	ActionID           string       `json:"action_id,omitempty"`
	// install_language fields
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
}

// --- IPC Response Types (sent to Tauri via stdout) ---

type ProgressResponse struct {
	Type        string   `json:"type"`
	Stage       string   `json:"stage"`
	Percent     float64  `json:"percent"`
	Message     string   `json:"message"`
	ElapsedSecs *float64 `json:"elapsed_secs,omitempty"`
	ETASecs     *float64 `json:"eta_secs,omitempty"`
}

type StageResponse struct {
	Type    string `json:"type"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type CompleteResponse struct {
	Type               string  `json:"type"`
	OutputPath         string  `json:"output_path"`
	TranscriptionLog   string  `json:"transcription_log,omitempty"`
	Segments           int     `json:"segments"`
	DurationSecs       float64 `json:"duration_secs"`
	BackendSummary     string  `json:"backend_summary,omitempty"`
	SelectedASRBackend string  `json:"selected_asr_backend,omitempty"`
	DiarizationRan     bool    `json:"diarization_ran,omitempty"`
	SpeakerCount       *int    `json:"speaker_count,omitempty"`
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
	Type              string `json:"type"`
	WhisperServer     bool   `json:"whisper_server"`
	TranslationEngine bool   `json:"translation_engine"`
	MLBackend         bool   `json:"ml_backend"`
	GPU               string `json:"gpu"`
}

type VRAMInfo struct {
	TotalMiB int `json:"total_mib"`
	UsedMiB  int `json:"used_mib"`
	FreeMiB  int `json:"free_mib"`
}

type VramInfoResponse struct {
	Type string    `json:"type"`
	Vram *VRAMInfo `json:"vram"`
}

// --- Transcription Types ---

type Segment struct {
	Start        float64 `json:"start"`
	End          float64 `json:"end"`
	Text         string  `json:"text"`
	SpeakerID    string  `json:"speaker_id,omitempty"`
	SpeakerLabel string  `json:"speaker_label,omitempty"`
}

type TranscriptionResult struct {
	Text     string    `json:"text"`
	Segments []Segment `json:"segments"`
	Language string    `json:"language,omitempty"`
}

type LanguageOption struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type BackendDefaults struct {
	ASRBackend         string `json:"asr_backend"`
	ASRModelID         string `json:"asr_model_id"`
	TranslationBackend string `json:"translation_backend"`
	DiarizationEnabled bool   `json:"diarization_enabled"`
}

type ASRBackendCapability struct {
	ID              string           `json:"id"`
	DisplayName     string           `json:"display_name"`
	Installed       bool             `json:"installed"`
	DefaultModelID  string           `json:"default_model_id,omitempty"`
	SourceLanguages []LanguageOption `json:"source_languages,omitempty"`
}

type TranslationBackendCapability struct {
	ID              string           `json:"id"`
	DisplayName     string           `json:"display_name"`
	Installed       bool             `json:"installed"`
	DefaultModelID  string           `json:"default_model_id,omitempty"`
	TargetLanguages []LanguageOption `json:"target_languages,omitempty"`
}

type BackendCapabilities struct {
	ASR         []ASRBackendCapability         `json:"asr"`
	Translation []TranslationBackendCapability `json:"translation"`
}

type CapabilitiesResponse struct {
	Type     string              `json:"type"`
	Defaults BackendDefaults     `json:"defaults"`
	Backends BackendCapabilities `json:"backends"`
}

// --- Service Configuration ---

type ServiceConfig struct {
	SearchRoots       []string
	WhisperServerPath string
	WhisperModelPath  string
	WhisperPort       int
	LlamaServerPort   int
	MLBackendPort     int
}

func DefaultServiceConfig() ServiceConfig {
	roots := make([]string, 0, 8)
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, ancestorRoots(cwd, 3)...)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		roots = append(roots, ancestorRoots(exeDir, 4)...)
		roots = append(
			roots,
			filepath.Join(exeDir, "resources"),
			filepath.Join(exeDir, "..", "resources"),
			filepath.Join(exeDir, "..", "Resources"),
		)
	}
	return resolveServiceConfig(roots...)
}

func resolveServiceConfig(roots ...string) ServiceConfig {
	searchRoots := normalizeSearchRoots(roots)
	whisperBinary, whisperModel := resolveWhisperAssets(searchRoots, "base")

	return ServiceConfig{
		SearchRoots:       searchRoots,
		WhisperServerPath: whisperBinary,
		WhisperModelPath:  whisperModel,
		WhisperPort:       8080,
		LlamaServerPort:   8081,
		MLBackendPort:     8082,
	}
}

func resolveWhisperAssets(roots []string, modelSize string) (string, string) {
	searchRoots := normalizeSearchRoots(roots)
	binaryName := "whisper-server"

	requestedModel := modelFilename(modelSize)
	defaultModel := modelFilename("base")

	executableNames := whisperExecutableCandidates()
	binaryCandidates := make([]string, 0, len(searchRoots)*len(executableNames)*2)
	modelCandidates := make([]string, 0, len(searchRoots)*4)

	for _, root := range searchRoots {
		for _, executableName := range executableNames {
			binaryCandidates = append(binaryCandidates,
				filepath.Join(root, "services", "whisper-server", executableName),
				filepath.Join(root, executableName),
			)
		}
		modelCandidates = append(modelCandidates,
			filepath.Join(root, "services", "whisper-server", "models", requestedModel),
			filepath.Join(root, "models", requestedModel),
		)
		if requestedModel != defaultModel {
			modelCandidates = append(modelCandidates,
				filepath.Join(root, "services", "whisper-server", "models", defaultModel),
				filepath.Join(root, "models", defaultModel),
			)
		}
	}

	binaryPath := firstExistingPath(binaryCandidates...)
	if binaryPath == "" {
		binaryPath = binaryName
	}

	modelPath := firstExistingPath(modelCandidates...)
	if modelPath == "" {
		modelPath = filepath.Join("models", defaultModel)
	}

	return binaryPath, modelPath
}

func whisperExecutableCandidates() []string {
	primary := whisperExecutableName()
	alternate := "whisper-server"
	if primary == alternate {
		alternate = "whisper-server.exe"
	}

	return []string{primary, alternate}
}

func modelFilename(modelSize string) string {
	switch strings.ToLower(modelSize) {
	case "tiny":
		return "ggml-tiny.bin"
	case "base", "":
		return "ggml-base.bin"
	case "small":
		return "ggml-small.bin"
	case "medium":
		return "ggml-medium.bin"
	case "large-v3":
		return "ggml-large-v3.bin"
	case "turbo":
		return "ggml-large-v3-turbo.bin"
	default:
		return "ggml-base.bin"
	}
}

func ancestorRoots(path string, levels int) []string {
	roots := []string{path}
	current := path

	for i := 0; i < levels; i++ {
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		roots = append(roots, parent)
		current = parent
	}

	return roots
}

func normalizeSearchRoots(roots []string) []string {
	seen := make(map[string]struct{}, len(roots))
	normalized := make([]string, 0, len(roots))

	for _, root := range roots {
		if root == "" {
			continue
		}
		cleaned := filepath.Clean(root)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}

	return normalized
}

func resolveVADModelPath(searchRoots []string) string {
	roots := normalizeSearchRoots(searchRoots)
	candidates := make([]string, 0, len(roots)*2)
	for _, root := range roots {
		candidates = append(candidates,
			filepath.Join(root, "services", "whisper-server", "models", vadModelFilename),
			filepath.Join(root, "models", vadModelFilename),
		)
	}
	return firstExistingPath(candidates...)
}

func firstExistingPath(paths ...string) string {
	for _, candidate := range paths {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// --- llama-server resolution (mirrors whisper-server pattern) ---

const gemmaModelFilenameConst = "GemmaTranslate-v3-12B.i1-Q4_K_S.gguf"

func llamaExecutableName() string {
	if os.PathSeparator == '\\' {
		return "llama-server.exe"
	}
	return "llama-server"
}

func llamaExecutableCandidates() []string {
	primary := llamaExecutableName()
	alternate := "llama-server"
	if primary == alternate {
		alternate = "llama-server.exe"
	}
	return []string{primary, alternate}
}

func resolveLlamaServerBinary(searchRoots []string) string {
	roots := normalizeSearchRoots(searchRoots)
	executableNames := llamaExecutableCandidates()
	candidates := make([]string, 0, len(roots)*len(executableNames)*2)

	for _, root := range roots {
		for _, name := range executableNames {
			candidates = append(candidates,
				filepath.Join(root, "services", "llama-server", name),
				filepath.Join(root, name),
			)
		}
	}

	path := firstExistingPath(candidates...)
	if path == "" {
		return "llama-server"
	}
	return path
}

func resolveGemmaModelPath(searchRoots []string) string {
	roots := normalizeSearchRoots(searchRoots)
	candidates := make([]string, 0, len(roots)*2)
	for _, root := range roots {
		candidates = append(candidates,
			filepath.Join(root, "services", "llama-server", "models", gemmaModelFilenameConst),
			filepath.Join(root, "models", gemmaModelFilenameConst),
		)
	}
	return firstExistingPath(candidates...)
}

func preferredLlamaInstallDir(searchRoots []string) string {
	for _, root := range normalizeSearchRoots(searchRoots) {
		candidate := filepath.Join(root, "services", "llama-server")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	if len(searchRoots) > 0 {
		return filepath.Join(searchRoots[0], "services", "llama-server")
	}

	return filepath.Join("services", "llama-server")
}

func mlBackendLauncherName() string {
	if os.PathSeparator == '\\' {
		return "run.bat"
	}
	return "run.sh"
}

func resolveMLBackendLauncher(searchRoots []string) string {
	roots := normalizeSearchRoots(searchRoots)
	candidates := make([]string, 0, len(roots)*2)
	launcher := mlBackendLauncherName()
	for _, root := range roots {
		installDir := preferredMLBackendInstallDir([]string{root})
		candidates = append(candidates,
			filepath.Join(installDir, launcher),
			filepath.Join(root, launcher),
		)
	}

	if path := firstExistingPath(candidates...); path != "" {
		return path
	}

	return filepath.Join("services", "ml-backend", launcher)
}
