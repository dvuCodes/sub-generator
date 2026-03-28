package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListAvailableLanguagesReturnsStaticPairs(t *testing.T) {
	langs, err := listAvailableLanguages()
	if err != nil {
		t.Fatalf("listAvailableLanguages() error = %v", err)
	}
	if len(langs) == 0 {
		t.Fatal("listAvailableLanguages() returned empty list")
	}

	// Verify a known pair exists
	found := false
	for _, pair := range langs {
		if pair.Source == "en" && pair.Target == "ja" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("listAvailableLanguages() missing en->ja pair")
	}
}

func TestListCapabilitiesUsesNewDefaults(t *testing.T) {
	capabilities := listCapabilities()

	if capabilities.Type != "capabilities" {
		t.Fatalf("Type = %q, want capabilities", capabilities.Type)
	}
	if capabilities.Defaults.ASRBackend != "faster_whisper" {
		t.Fatalf("Defaults.ASRBackend = %q, want faster_whisper", capabilities.Defaults.ASRBackend)
	}
	if capabilities.Defaults.TranslationBackend != "nllb" {
		t.Fatalf("Defaults.TranslationBackend = %q, want nllb", capabilities.Defaults.TranslationBackend)
	}
	if len(capabilities.Backends.ASR) < 2 {
		t.Fatalf("expected at least 2 ASR backends, got %d", len(capabilities.Backends.ASR))
	}
	if len(capabilities.Backends.Translation) < 2 {
		t.Fatalf("expected at least 2 translation backends, got %d", len(capabilities.Backends.Translation))
	}
}

func TestMergeCapabilitiesPreservesLocalBackendAvailability(t *testing.T) {
	base := defaultCapabilities()
	for i := range base.Backends.ASR {
		if base.Backends.ASR[i].ID == "whisper_cpp" {
			base.Backends.ASR[i].Installed = true
			base.Backends.ASR[i].DefaultModelID = "turbo"
		}
	}
	for i := range base.Backends.Translation {
		if base.Backends.Translation[i].ID == gemmaTranslationBackend {
			base.Backends.Translation[i].Installed = true
			base.Backends.Translation[i].DefaultModelID = gemmaModelFilenameConst
		}
	}

	remote := &CapabilitiesResponse{
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
					Installed:       true,
					DefaultModelID:  defaultASRModelID,
					SourceLanguages: []LanguageOption{{Code: "auto", Name: "Auto-detect"}},
				},
				{
					ID:              "whisper_cpp",
					DisplayName:     "whisper.cpp",
					Installed:       false,
					DefaultModelID:  "",
					SourceLanguages: []LanguageOption{{Code: "auto", Name: "Auto-detect"}},
				},
			},
			Translation: []TranslationBackendCapability{
				{
					ID:              defaultTranslationBackend,
					DisplayName:     "NLLB",
					Installed:       true,
					DefaultModelID:  defaultTranslationModelID,
					TargetLanguages: []LanguageOption{{Code: "en", Name: "English"}},
				},
				{
					ID:              gemmaTranslationBackend,
					DisplayName:     "Gemma Context",
					Installed:       false,
					DefaultModelID:  "",
					TargetLanguages: []LanguageOption{{Code: "en", Name: "English"}},
				},
			},
		},
	}

	mergeCapabilities(&base, remote)

	for _, backend := range base.Backends.ASR {
		if backend.ID == "whisper_cpp" {
			if !backend.Installed {
				t.Fatal("expected whisper_cpp to remain installed when local probe succeeded")
			}
			if backend.DefaultModelID != "turbo" {
				t.Fatalf("expected whisper_cpp default model to be preserved, got %q", backend.DefaultModelID)
			}
		}
	}

	for _, backend := range base.Backends.Translation {
		if backend.ID == gemmaTranslationBackend {
			if !backend.Installed {
				t.Fatal("expected gemma_context to remain installed when local probe succeeded")
			}
			if backend.DefaultModelID != gemmaModelFilenameConst {
				t.Fatalf("expected gemma default model to be preserved, got %q", backend.DefaultModelID)
			}
		}
	}
}

func TestBuildCapabilitiesMarksWhisperCPPInstalledWhenAnyModelExists(t *testing.T) {
	root := t.TempDir()

	whisperDir := filepath.Join(root, "services", "whisper-server")
	if err := os.MkdirAll(filepath.Join(whisperDir, "models"), 0o755); err != nil {
		t.Fatalf("mkdir whisper dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(whisperDir, whisperExecutableName()), []byte(""), 0o644); err != nil {
		t.Fatalf("write whisper binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(whisperDir, "models", modelFilename("turbo")), []byte(""), 0o644); err != nil {
		t.Fatalf("write whisper turbo model: %v", err)
	}

	capabilities := buildCapabilitiesResponse(NewServiceManager(resolveServiceConfig(root)))
	backend := findASRBackendByID(t, capabilities, "whisper_cpp")
	if !backend.Installed {
		t.Fatal("expected whisper_cpp to be available when the binary exists and at least one supported model is installed")
	}
}

func TestBuildCapabilitiesMarksGemmaInstalledWhenLlamaBinaryExists(t *testing.T) {
	root := t.TempDir()

	llamaDir := filepath.Join(root, "services", "llama-server")
	if err := os.MkdirAll(llamaDir, 0o755); err != nil {
		t.Fatalf("mkdir llama dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(llamaDir, llamaExecutableName()), []byte(""), 0o644); err != nil {
		t.Fatalf("write llama binary: %v", err)
	}

	capabilities := buildCapabilitiesResponse(NewServiceManager(resolveServiceConfig(root)))
	backend := findTranslationBackendByID(t, capabilities, gemmaTranslationBackend)
	if !backend.Installed {
		t.Fatal("expected gemma_context to stay selectable when llama-server is installed and the model can be downloaded on demand")
	}
}

func TestLocalServiceBaseURLUsesIPv4Loopback(t *testing.T) {
	if got := localServiceBaseURL(5000); got != "http://127.0.0.1:5000" {
		t.Fatalf("localServiceBaseURL() = %q, want %q", got, "http://127.0.0.1:5000")
	}
}

func TestHandleCommandGenerateRunsAsynchronously(t *testing.T) {
	originalRunGenerate := runGenerate
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	runGenerate = func(_ *Pipeline, _ Command) {
		close(started)
		<-release
		close(done)
	}
	t.Cleanup(func() {
		runGenerate = originalRunGenerate
	})

	returned := make(chan struct{})
	go func() {
		handleCommand(Command{Command: "generate"}, &Pipeline{}, nil)
		close(returned)
	}()

	<-started

	select {
	case <-returned:
		// handleCommand should return before generate finishes.
	default:
		t.Fatal("handleCommand(generate) blocked until the pipeline finished")
	}

	select {
	case <-done:
		t.Fatal("generate runner finished before the test released it")
	default:
	}

	close(release)
	<-done
}

func findASRBackendByID(t *testing.T, capabilities CapabilitiesResponse, id string) ASRBackendCapability {
	t.Helper()

	for _, backend := range capabilities.Backends.ASR {
		if backend.ID == id {
			return backend
		}
	}

	t.Fatalf("ASR backend %q not found in %+v", id, capabilities.Backends.ASR)
	return ASRBackendCapability{}
}

func findTranslationBackendByID(t *testing.T, capabilities CapabilitiesResponse, id string) TranslationBackendCapability {
	t.Helper()

	for _, backend := range capabilities.Backends.Translation {
		if backend.ID == id {
			return backend
		}
	}

	t.Fatalf("translation backend %q not found in %+v", id, capabilities.Backends.Translation)
	return TranslationBackendCapability{}
}
