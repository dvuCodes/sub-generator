package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// --- IPC Command Types (received from Tauri via stdin) ---

// GlossaryEntry enforces a preferred translation for a source term.
type GlossaryEntry struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type Command struct {
	Command            string  `json:"command"`
	InputVideo         string  `json:"input_video,omitempty"`
	SourceLang         *string `json:"source_lang,omitempty"`
	TargetLang         *string `json:"target_lang,omitempty"`
	OutputFormat       string  `json:"output_format,omitempty"`
	OutputPath         *string `json:"output_path,omitempty"`
	ASRBackend         string  `json:"asr_backend,omitempty"`
	ASRModelID         string  `json:"asr_model_id,omitempty"`
	ModelSize          string  `json:"model_size,omitempty"`
	TranslationBackend string  `json:"translation_backend,omitempty"`
	DiarizationEnabled bool    `json:"diarization_enabled,omitempty"`
	BeamSize           int     `json:"beam_size,omitempty"`
	VADFilter          bool    `json:"vad_filter,omitempty"`
	ActionID           string  `json:"action_id,omitempty"`
	// install_language fields
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`

	// Transcription tuning.
	InitialPrompt string  `json:"initial_prompt,omitempty"`
	FrameRate     float64 `json:"frame_rate,omitempty"` // 0 = auto/none

	// Translation engine selection and credentials.
	TranslationEngine string          `json:"translation_engine,omitempty"` // "" | libretranslate | deepl | llm
	DeepLAPIKey       string          `json:"deepl_api_key,omitempty"`
	LLMBaseURL        string          `json:"llm_base_url,omitempty"`
	LLMModel          string          `json:"llm_model,omitempty"`
	LLMAPIKey         string          `json:"llm_api_key,omitempty"`
	Synopsis          string          `json:"synopsis,omitempty"`
	Glossary          []GlossaryEntry `json:"glossary,omitempty"`
	Honorifics        string          `json:"honorifics,omitempty"` // "" | keep | drop
	QAPass            bool            `json:"qa_pass,omitempty"`

	// Subtitle timing/formatting.
	ShotSnap *bool `json:"shot_snap,omitempty"`
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
	Type               string    `json:"type"`
	OutputPath         string    `json:"output_path"`
	TranscriptionLog   string    `json:"transcription_log,omitempty"`
	Segments           int       `json:"segments"`
	DurationSecs       float64   `json:"duration_secs"`
	BackendSummary     string    `json:"backend_summary,omitempty"`
	SelectedASRBackend string    `json:"selected_asr_backend,omitempty"`
	DiarizationRan     bool      `json:"diarization_ran,omitempty"`
	SpeakerCount       *int      `json:"speaker_count,omitempty"`
	QC                 *QCReport `json:"qc,omitempty"`
	Preview            []CueView `json:"preview,omitempty"`
}

// CancelledResponse tells the frontend the active job was cancelled.
type CancelledResponse struct {
	Type    string `json:"type"`
	Message string `json:"message"`
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
	LibreTranslate    bool   `json:"libretranslate"`
	FFmpeg            bool   `json:"ffmpeg"`
	VADModel          bool   `json:"vad_model"`
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

// Word is a token-level timestamp used for precise cue splitting.
type Word struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type Segment struct {
	Start        float64  `json:"start"`
	End          float64  `json:"end"`
	Text         string   `json:"text"`
	Lines        []string `json:"lines,omitempty"` // wrapped display lines
	Words        []Word   `json:"words,omitempty"`
	NoSpeechProb float64  `json:"no_speech_prob,omitempty"`
	AvgLogprob   float64  `json:"avg_logprob,omitempty"`
	SpeakerID    string   `json:"speaker_id,omitempty"`
	SpeakerLabel string   `json:"speaker_label,omitempty"`
}

// CueView is a compact cue representation for UI previews and review.
type CueView struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// QCIssue flags a cue that violates subtitle quality guidelines.
type QCIssue struct {
	Index  int      `json:"index"`
	Start  float64  `json:"start"`
	End    float64  `json:"end"`
	Issues []string `json:"issues"`
}

// QCReport summarizes subtitle timing/formatting quality.
type QCReport struct {
	CueCount int            `json:"cue_count"`
	AvgCPS   float64        `json:"avg_cps"`
	MaxCPS   float64        `json:"max_cps"`
	Summary  map[string]int `json:"summary"`
	Issues   []QCIssue      `json:"issues,omitempty"`
}

func (q *QCReport) count(issue string) {
	if q.Summary == nil {
		q.Summary = map[string]int{}
	}
	q.Summary[issue]++
}

type TranscriptionResult struct {
	Text     string    `json:"text"`
	Segments []Segment `json:"segments"`
	Language string    `json:"language,omitempty"` // ISO 639-1
	Duration float64   `json:"duration,omitempty"` // seconds of audio
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
	SearchRoots        []string
	WhisperServerPath  string
	WhisperModelPath   string
	WhisperPort        int
	LlamaServerPort    int
	MLBackendPort      int
	LibreTranslatePort int
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
	whisperPort, llamaPort, mlBackendPort := allocateManagedServicePorts()
	libreTranslatePort := 5000
	if listener, err := reserveLoopbackListener(libreTranslatePort); err == nil {
		libreTranslatePort = listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
	}

	return ServiceConfig{
		SearchRoots:        searchRoots,
		WhisperServerPath:  whisperBinary,
		WhisperModelPath:   whisperModel,
		WhisperPort:        whisperPort,
		LlamaServerPort:    llamaPort,
		MLBackendPort:      mlBackendPort,
		LibreTranslatePort: libreTranslatePort,
	}
}

func allocateManagedServicePorts() (int, int, int) {
	const (
		defaultWhisperPort = 8080
		defaultLlamaPort   = 8081
		defaultMLPort      = 8082
	)

	preferred := []int{defaultWhisperPort, defaultLlamaPort, defaultMLPort}
	listeners := make([]net.Listener, 0, len(preferred))
	ports := make([]int, 0, len(preferred))

	for _, port := range preferred {
		listener, err := reserveLoopbackListener(port)
		if err != nil {
			return defaultWhisperPort, defaultLlamaPort, defaultMLPort
		}
		listeners = append(listeners, listener)
		ports = append(ports, listener.Addr().(*net.TCPAddr).Port)
	}

	for _, listener := range listeners {
		_ = listener.Close()
	}

	return ports[0], ports[1], ports[2]
}

func reserveLoopbackListener(preferredPort int) (net.Listener, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", loopbackHost, preferredPort))
	if err == nil {
		return listener, nil
	}

	return net.Listen("tcp", fmt.Sprintf("%s:0", loopbackHost))
}

func resolveWhisperAssets(roots []string, modelSize string) (string, string) {
	searchRoots := normalizeSearchRoots(roots)
	binaryName := "whisper-server"

	explicitModel := strings.TrimSpace(modelSize) != ""
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
		if !explicitModel && requestedModel != defaultModel {
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
	if modelPath == "" && !explicitModel {
		defaultCandidates := make([]string, 0, len(searchRoots)*2)
		for _, root := range searchRoots {
			defaultCandidates = append(defaultCandidates,
				filepath.Join(root, "services", "whisper-server", "models", defaultModel),
				filepath.Join(root, "models", defaultModel),
			)
		}
		modelPath = firstExistingPath(defaultCandidates...)
	}
	if modelPath == "" {
		if len(searchRoots) > 0 {
			modelPath = filepath.Join(
				searchRoots[0], "services", "whisper-server", "models", requestedModel,
			)
		} else {
			modelPath = filepath.Join("models", requestedModel)
		}
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

// ResolveVADModel finds the Silero VAD GGML model next to the whisper models.
func ResolveVADModel(roots []string) string {
	for _, root := range normalizeSearchRoots(roots) {
		modelsDirs := []string{
			filepath.Join(root, "services", "whisper-server", "models"),
			filepath.Join(root, "models"),
		}
		for _, dir := range modelsDirs {
			matches, err := filepath.Glob(filepath.Join(dir, vadModelPattern))
			if err == nil && len(matches) > 0 {
				sort.Strings(matches)
				return matches[len(matches)-1]
			}
		}
	}
	return ""
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
	case "turbo", "large-v3-turbo":
		return "ggml-large-v3-turbo.bin"
	case "large-v3-q5_0":
		return "ggml-large-v3-q5_0.bin"
	case "turbo-q5_0", "large-v3-turbo-q5_0":
		return "ggml-large-v3-turbo-q5_0.bin"
	case "turbo-q8_0", "large-v3-turbo-q8_0":
		return "ggml-large-v3-turbo-q8_0.bin"
	default:
		return "ggml-base.bin"
	}
}

// dtwPresetForModel maps a model filename to the whisper.cpp --dtw alignment
// preset used for token-level timestamps. Returns "" when no preset exists.
func dtwPresetForModel(modelFilename string) string {
	switch {
	case strings.Contains(modelFilename, "large-v3-turbo"):
		return "large.v3.turbo"
	case strings.Contains(modelFilename, "large-v3"):
		return "large.v3"
	case strings.Contains(modelFilename, "medium.en"):
		return "medium.en"
	case strings.Contains(modelFilename, "medium"):
		return "medium"
	case strings.Contains(modelFilename, "small.en"):
		return "small.en"
	case strings.Contains(modelFilename, "small"):
		return "small"
	case strings.Contains(modelFilename, "base.en"):
		return "base.en"
	case strings.Contains(modelFilename, "base"):
		return "base"
	case strings.Contains(modelFilename, "tiny.en"):
		return "tiny.en"
	case strings.Contains(modelFilename, "tiny"):
		return "tiny"
	default:
		return ""
	}
}

// vadModelPattern matches the Silero VAD GGML models shipped with whisper.cpp.
const vadModelPattern = "ggml-silero*.bin"

// whisperLanguageCodes maps whisper's full language names to ISO 639-1 codes.
var whisperLanguageCodes = map[string]string{
	"afrikaans": "af", "albanian": "sq", "amharic": "am", "arabic": "ar",
	"armenian": "hy", "assamese": "as", "azerbaijani": "az", "bashkir": "ba",
	"basque": "eu", "belarusian": "be", "bengali": "bn", "bosnian": "bs",
	"breton": "br", "bulgarian": "bg", "burmese": "my", "cantonese": "zh",
	"catalan": "ca", "chinese": "zh", "croatian": "hr", "czech": "cs",
	"danish": "da", "dutch": "nl", "english": "en", "estonian": "et",
	"faroese": "fo", "finnish": "fi", "french": "fr", "galician": "gl",
	"georgian": "ka", "german": "de", "greek": "el", "gujarati": "gu",
	"haitian": "ht", "hausa": "ha", "hawaiian": "haw", "hebrew": "he",
	"hindi": "hi", "hungarian": "hu", "icelandic": "is", "indonesian": "id",
	"italian": "it", "japanese": "ja", "javanese": "jv", "kannada": "kn",
	"kazakh": "kk", "khmer": "km", "korean": "ko", "lao": "lo",
	"latin": "la", "latvian": "lv", "lingala": "ln", "lithuanian": "lt",
	"luxembourgish": "lb", "macedonian": "mk", "malay": "ms",
	"malayalam": "ml", "maltese": "mt", "maori": "mi", "marathi": "mr",
	"mongolian": "mn", "nepali": "ne", "norwegian": "no", "occitan": "oc",
	"panjabi": "pa", "pashto": "ps", "persian": "fa", "polish": "pl",
	"portuguese": "pt", "punjabi": "pa", "romanian": "ro", "russian": "ru",
	"sanskrit": "sa", "serbian": "sr", "shona": "sn", "sindhi": "sd",
	"sinhala": "si", "slovak": "sk", "slovenian": "sl", "somali": "so",
	"spanish": "es", "sundanese": "su", "swahili": "sw", "swedish": "sv",
	"tajik": "tg", "tamil": "ta", "tatar": "tt", "telugu": "te",
	"thai": "th", "tibetan": "bo", "turkish": "tr", "turkmen": "tk",
	"ukrainian": "uk", "urdu": "ur", "uzbek": "uz", "vietnamese": "vi",
	"welsh": "cy", "yiddish": "yi", "yoruba": "yo",
}

// WhisperLangToCode converts a whisper full language name ("japanese")
// or an already-ISO code into an ISO 639-1 code.
func WhisperLangToCode(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if code, ok := whisperLanguageCodes[lower]; ok {
		return code
	}
	if len(lower) == 2 || len(lower) == 3 {
		return lower
	}
	return ""
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
