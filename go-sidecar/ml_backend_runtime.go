package main

import (
	"os"
	"path/filepath"
)

const (
	defaultASRBackend         = "faster_whisper"
	defaultASRModelID         = "deepdml/faster-whisper-large-v3-turbo-ct2"
	defaultTranslationBackend = "nllb"
	defaultTranslationModelID = "JustFrederik/nllb-200-distilled-600M-ct2-int8"
	gemmaTranslationBackend   = "gemma_context"
	mlBackendServiceID        = "ml-backend"
)

func mlBackendInstallIsLaunchable(dir string) bool {
	if dir == "" {
		return false
	}

	candidates := []string{
		filepath.Join(dir, mlBackendLauncherName()),
		filepath.Join(dir, "subgen_ml_backend", "__main__.py"),
		filepath.Join(dir, "app", "service.py"),
		filepath.Join(dir, "service.py"),
		filepath.Join(dir, "backend.py"),
	}

	return firstExistingPath(candidates...) != ""
}

func preferredMLBackendInstallDir(searchRoots []string) string {
	var fallback string

	for _, root := range normalizeSearchRoots(searchRoots) {
		candidates := []string{
			filepath.Join(root, "services", "ml-backend"),
			filepath.Join(root, "python-backend"),
		}
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				if fallback == "" {
					fallback = candidate
				}
				if mlBackendInstallIsLaunchable(candidate) {
					return candidate
				}
			}
		}
	}

	if fallback != "" {
		return fallback
	}

	if len(searchRoots) > 0 {
		return filepath.Join(searchRoots[0], "services", "ml-backend")
	}

	return filepath.Join("services", "ml-backend")
}

func resolveMLBackendScriptPath(searchRoots []string) string {
	installDir := preferredMLBackendInstallDir(searchRoots)
	candidates := []string{
		filepath.Join(installDir, "subgen_ml_backend", "__main__.py"),
		filepath.Join(installDir, "app", "service.py"),
		filepath.Join(installDir, "service.py"),
		filepath.Join(installDir, "backend.py"),
	}
	return firstExistingPath(candidates...)
}

func resolveMLBackendRequirementsPath(searchRoots []string) string {
	installDir := preferredMLBackendInstallDir(searchRoots)
	candidates := []string{
		filepath.Join(installDir, "requirements.txt"),
		filepath.Join(filepath.Dir(resolveMLBackendScriptPath(searchRoots)), "requirements.txt"),
	}
	return firstExistingPath(candidates...)
}

func resolveMLBackendPython(searchRoots []string) string {
	installDir := preferredMLBackendInstallDir(searchRoots)
	candidates := []string{
		filepath.Join(installDir, "runtime", "python.exe"),
		filepath.Join(installDir, "runtime", "python"),
		filepath.Join(installDir, ".venv", "Scripts", "python.exe"),
		filepath.Join(installDir, ".venv", "bin", "python"),
	}

	if path := firstExistingPath(candidates...); path != "" {
		return path
	}

	if os.PathSeparator == '\\' {
		return "python"
	}
	return "python3"
}

func defaultCapabilities() CapabilitiesResponse {
	commonLanguages := commonLanguageOptions()
	return CapabilitiesResponse{
		Type: "capabilities",
		Defaults: BackendDefaults{
			ASRBackend:         defaultASRBackend,
			ASRModelID:         defaultASRModelID,
			TranslationBackend: defaultTranslationBackend,
			DiarizationEnabled: false,
		},
		Backends: BackendCapabilities{
			ASR: []ASRBackendCapability{
				{
					ID:              defaultASRBackend,
					DisplayName:     "Faster Whisper",
					Installed:       false,
					DefaultModelID:  defaultASRModelID,
					SourceLanguages: append([]LanguageOption{{Code: "auto", Name: "Auto-detect"}}, commonLanguages...),
				},
				{
					ID:              "whisper_cpp",
					DisplayName:     "whisper.cpp",
					Installed:       true,
					DefaultModelID:  "turbo",
					SourceLanguages: append([]LanguageOption{{Code: "auto", Name: "Auto-detect"}}, commonLanguages...),
				},
			},
			Translation: []TranslationBackendCapability{
				{
					ID:              defaultTranslationBackend,
					DisplayName:     "NLLB",
					Installed:       false,
					DefaultModelID:  defaultTranslationModelID,
					TargetLanguages: commonLanguages,
				},
				{
					ID:              gemmaTranslationBackend,
					DisplayName:     "Gemma Context",
					Installed:       true,
					DefaultModelID:  gemmaModelFilenameConst,
					TargetLanguages: commonLanguages,
				},
			},
		},
	}
}

func commonLanguageOptions() []LanguageOption {
	return []LanguageOption{
		{Code: "en", Name: "English"},
		{Code: "ja", Name: "Japanese"},
		{Code: "zh", Name: "Chinese"},
		{Code: "ko", Name: "Korean"},
		{Code: "es", Name: "Spanish"},
		{Code: "fr", Name: "French"},
		{Code: "de", Name: "German"},
		{Code: "pt", Name: "Portuguese"},
		{Code: "ru", Name: "Russian"},
		{Code: "ar", Name: "Arabic"},
		{Code: "hi", Name: "Hindi"},
		{Code: "vi", Name: "Vietnamese"},
		{Code: "th", Name: "Thai"},
		{Code: "it", Name: "Italian"},
		{Code: "nl", Name: "Dutch"},
		{Code: "pl", Name: "Polish"},
		{Code: "tr", Name: "Turkish"},
		{Code: "sv", Name: "Swedish"},
		{Code: "da", Name: "Danish"},
		{Code: "fi", Name: "Finnish"},
		{Code: "no", Name: "Norwegian"},
		{Code: "cs", Name: "Czech"},
		{Code: "el", Name: "Greek"},
		{Code: "he", Name: "Hebrew"},
		{Code: "hu", Name: "Hungarian"},
		{Code: "id", Name: "Indonesian"},
		{Code: "ms", Name: "Malay"},
		{Code: "ro", Name: "Romanian"},
		{Code: "sk", Name: "Slovak"},
		{Code: "uk", Name: "Ukrainian"},
		{Code: "bg", Name: "Bulgarian"},
		{Code: "hr", Name: "Croatian"},
		{Code: "lt", Name: "Lithuanian"},
		{Code: "lv", Name: "Latvian"},
		{Code: "et", Name: "Estonian"},
		{Code: "sl", Name: "Slovenian"},
		{Code: "sr", Name: "Serbian"},
		{Code: "ca", Name: "Catalan"},
		{Code: "gl", Name: "Galician"},
		{Code: "eu", Name: "Basque"},
		{Code: "mk", Name: "Macedonian"},
		{Code: "sq", Name: "Albanian"},
		{Code: "ka", Name: "Georgian"},
		{Code: "hy", Name: "Armenian"},
		{Code: "az", Name: "Azerbaijani"},
		{Code: "kk", Name: "Kazakh"},
		{Code: "uz", Name: "Uzbek"},
		{Code: "tl", Name: "Filipino"},
		{Code: "sw", Name: "Swahili"},
		{Code: "ta", Name: "Tamil"},
		{Code: "te", Name: "Telugu"},
		{Code: "bn", Name: "Bengali"},
		{Code: "ur", Name: "Urdu"},
		{Code: "fa", Name: "Persian"},
		{Code: "ne", Name: "Nepali"},
		{Code: "si", Name: "Sinhala"},
		{Code: "my", Name: "Myanmar"},
	}
}
