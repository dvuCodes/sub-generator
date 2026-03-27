package main

import "testing"

func TestResolveASRBackendDefaultsToFasterWhisper(t *testing.T) {
	got := resolveASRBackend(Command{})
	if got != "faster_whisper" {
		t.Fatalf("resolveASRBackend() = %q, want faster_whisper", got)
	}
}

func TestResolveASRBackendHonorsExplicitWhisperCpp(t *testing.T) {
	got := resolveASRBackend(Command{ASRBackend: "whisper_cpp"})
	if got != "whisper_cpp" {
		t.Fatalf("resolveASRBackend() = %q, want whisper_cpp", got)
	}
}

func TestResolveTranslationBackendDefaultsToNoneWithoutTarget(t *testing.T) {
	got := resolveTranslationBackend(Command{})
	if got != "none" {
		t.Fatalf("resolveTranslationBackend() = %q, want none", got)
	}
}

func TestResolveTranslationBackendDefaultsToNLLBWithTarget(t *testing.T) {
	target := "eng_Latn"
	got := resolveTranslationBackend(Command{TargetLang: &target})
	if got != "nllb" {
		t.Fatalf("resolveTranslationBackend() = %q, want nllb", got)
	}
}

func TestResolveTranslationBackendHonorsExplicitGemmaContext(t *testing.T) {
	target := "eng_Latn"
	got := resolveTranslationBackend(Command{
		TargetLang:         &target,
		TranslationBackend: "gemma_context",
	})
	if got != "gemma_context" {
		t.Fatalf("resolveTranslationBackend() = %q, want gemma_context", got)
	}
}

func TestBuildBackendSummaryIncludesDiarization(t *testing.T) {
	got := buildBackendSummary("faster_whisper", "nllb", true)
	if got != "faster_whisper + nllb + diarization" {
		t.Fatalf("buildBackendSummary() = %q", got)
	}
}
