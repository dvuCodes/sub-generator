package main

import "strings"

func resolveASRBackend(cmd Command) string {
	backend := strings.TrimSpace(cmd.ASRBackend)
	if backend == "" {
		return defaultASRBackend
	}
	return backend
}

func resolveASRModelID(cmd Command) string {
	modelID := strings.TrimSpace(cmd.ASRModelID)
	if modelID == "" {
		if resolveASRBackend(cmd) == "whisper_cpp" {
			if cmd.ModelSize != "" {
				return cmd.ModelSize
			}
			return "turbo"
		}
		return defaultASRModelID
	}
	return modelID
}

func resolveTranslationBackend(cmd Command) string {
	backend := strings.TrimSpace(cmd.TranslationBackend)
	if backend == "" {
		if cmd.TargetLang == nil || strings.TrimSpace(*cmd.TargetLang) == "" {
			return "none"
		}
		return defaultTranslationBackend
	}
	return backend
}

func resolveTranslationModelID(cmd Command) string {
	switch resolveTranslationBackend(cmd) {
	case gemmaTranslationBackend:
		return gemmaModelFilenameConst
	case defaultTranslationBackend:
		return defaultTranslationModelID
	default:
		return ""
	}
}
