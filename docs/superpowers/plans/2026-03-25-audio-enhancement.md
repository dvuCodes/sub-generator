# Audio Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add FFmpeg audio preprocessing and Whisper temperature fallback to improve transcription accuracy for anime/TV/movie content.

**Architecture:** A new `audio.go` file in the Go sidecar handles audio preprocessing via FFmpeg filters (high-pass, low-pass, vocal EQ, loudnorm, noise gate) producing a 16kHz mono WAV. The pipeline orchestrates preprocessing before transcription with graceful fallback to raw upload on failure. The frontend exposes an Audio Enhancement settings panel. Whisper temperature changes from fixed `0` to fallback list.

**Tech Stack:** Go (sidecar), FFmpeg (preprocessing), React/TypeScript (frontend), shadcn/ui (UI components)

**Spec:** `docs/superpowers/specs/2026-03-24-audio-enhancement-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `go-sidecar/audio.go` | Create | FFmpeg preprocessing: `PreprocessAudio()`, `buildFilterChain()`, `cleanupPreprocessed()` |
| `go-sidecar/audio_test.go` | Create | Unit tests for filter chain building, integration tests for preprocessing |
| `go-sidecar/config.go` | Modify (lines 11-24) | Add `AudioConfig` struct, `DefaultAudioConfig()`, and field on `Command` |
| `go-sidecar/config_test.go` | Modify (append) | Tests for AudioConfig defaults and JSON deserialization |
| `go-sidecar/transcriber.go` | Modify (line 183) | Change temperature from `"0"` to `"0.0,0.2,0.4,0.6,0.8,1.0"` |
| `go-sidecar/pipeline.go` | Modify (lines 119-178) | Orchestrate `PreprocessAudio()` before transcription with fallback |
| `src/lib/types.ts` | Modify (lines 12-22) | Add `AudioConfig` interface and field on `GenerateCommand` |
| `src/components/SettingsPanel.tsx` | Modify | Add Audio Enhancement section (master toggle, vocal boost slider, noise gate toggle, normalization toggle) |
| `src/App.tsx` | Modify (lines 46-52, 158-198) | Add `audioConfig` state, wire to `GenerateCommand` and `SettingsPanel` |

---

### Task 0: Create feature branch

- [ ] **Step 1: Create and switch to feature branch**

```bash
git checkout -b feat/audio-enhancement main
```

---

### Task 1: Add AudioConfig to Go config with tests (TDD)

**Files:**
- Modify: `go-sidecar/config.go:11-24`
- Modify: `go-sidecar/config_test.go` (append — file already exists with 6 tests and imports `encoding/json` and `testing`)

- [ ] **Step 1: Write failing tests for AudioConfig**

Append the following test functions to the existing `go-sidecar/config_test.go` (no import changes needed — `encoding/json` and `testing` are already imported):

```go

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

	// When audio_config is missing from JSON, Go zero-values apply
	// Pipeline is responsible for applying defaults via DefaultAudioConfig()
	if cmd.AudioConfig.Enabled {
		t.Error("expected zero-value Enabled=false when missing from JSON")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go-sidecar && go test -run TestAudioConfig -v`
Expected: FAIL — `DefaultAudioConfig` undefined.

- [ ] **Step 3: Add AudioConfig struct, DefaultAudioConfig, and field to Command**

In `go-sidecar/config.go`, replace lines 11-24 (the `Command` struct):

Old:
```go
type Command struct {
	Command      string  `json:"command"`
	InputVideo   string  `json:"input_video,omitempty"`
	SourceLang   *string `json:"source_lang,omitempty"`
	TargetLang   *string `json:"target_lang,omitempty"`
	OutputFormat string  `json:"output_format,omitempty"`
	OutputPath   *string `json:"output_path,omitempty"`
	ModelSize    string  `json:"model_size,omitempty"`
	BeamSize     int     `json:"beam_size,omitempty"`
	VADFilter    bool    `json:"vad_filter,omitempty"`
	// install_language fields
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
}
```

New:
```go
type Command struct {
	Command      string      `json:"command"`
	InputVideo   string      `json:"input_video,omitempty"`
	SourceLang   *string     `json:"source_lang,omitempty"`
	TargetLang   *string     `json:"target_lang,omitempty"`
	OutputFormat string      `json:"output_format,omitempty"`
	OutputPath   *string     `json:"output_path,omitempty"`
	ModelSize    string      `json:"model_size,omitempty"`
	BeamSize     int         `json:"beam_size,omitempty"`
	VADFilter    bool        `json:"vad_filter,omitempty"`
	AudioConfig  AudioConfig `json:"audio_config"`
	// install_language fields
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
}

type AudioConfig struct {
	Enabled      bool    `json:"enabled"`
	VocalBoostDB float64 `json:"vocal_boost_db"`
	NoiseGate    bool    `json:"noise_gate"`
	Normalize    bool    `json:"normalize"`
}

func DefaultAudioConfig() AudioConfig {
	return AudioConfig{
		Enabled:      true,
		VocalBoostDB: 3,
		NoiseGate:    true,
		Normalize:    true,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-sidecar && go test -run TestAudioConfig -v`
Expected: All 3 tests PASS.

- [ ] **Step 5: Verify full suite still passes**

Run: `cd go-sidecar && go test -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add go-sidecar/config.go go-sidecar/config_test.go
git commit -m "feat(audio): add AudioConfig struct, defaults, and deserialization tests"
```

---

### Task 2: Build filter chain logic (TDD)

**Files:**
- Create: `go-sidecar/audio.go`
- Create: `go-sidecar/audio_test.go`

- [ ] **Step 1: Write failing tests for buildFilterChain**

Create `go-sidecar/audio_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFilterChain_Defaults(t *testing.T) {
	cfg := DefaultAudioConfig()
	chain := buildFilterChain(cfg)

	expected := "highpass=f=200,lowpass=f=8000,equalizer=f=1000:t=h:w=2000:g=3,loudnorm=I=-16:TP=-1.5:LRA=11,agate=threshold=0.01:attack=5:release=50"
	if chain != expected {
		t.Errorf("expected %q, got %q", expected, chain)
	}
}

func TestBuildFilterChain_NoiseGateOff(t *testing.T) {
	cfg := DefaultAudioConfig()
	cfg.NoiseGate = false
	chain := buildFilterChain(cfg)

	if strings.Contains(chain, "agate") {
		t.Errorf("expected no agate filter, got %q", chain)
	}
	if !strings.Contains(chain, "loudnorm") {
		t.Errorf("expected loudnorm filter present, got %q", chain)
	}
}

func TestBuildFilterChain_NormalizeOff(t *testing.T) {
	cfg := DefaultAudioConfig()
	cfg.Normalize = false
	chain := buildFilterChain(cfg)

	if strings.Contains(chain, "loudnorm") {
		t.Errorf("expected no loudnorm filter, got %q", chain)
	}
	if !strings.Contains(chain, "agate") {
		t.Errorf("expected agate filter present, got %q", chain)
	}
}

func TestBuildFilterChain_VocalBoostZero(t *testing.T) {
	cfg := DefaultAudioConfig()
	cfg.VocalBoostDB = 0
	chain := buildFilterChain(cfg)

	if strings.Contains(chain, "equalizer") {
		t.Errorf("expected no equalizer filter when boost is 0, got %q", chain)
	}
}

func TestBuildFilterChain_VocalBoostMax(t *testing.T) {
	cfg := DefaultAudioConfig()
	cfg.VocalBoostDB = 6
	chain := buildFilterChain(cfg)

	if !strings.Contains(chain, "g=6") {
		t.Errorf("expected g=6 in equalizer filter, got %q", chain)
	}
}

func TestBuildFilterChain_SubTogglesOff(t *testing.T) {
	cfg := AudioConfig{
		Enabled:      true,
		VocalBoostDB: 0,
		NoiseGate:    false,
		Normalize:    false,
	}
	chain := buildFilterChain(cfg)

	expected := "highpass=f=200,lowpass=f=8000"
	if chain != expected {
		t.Errorf("expected %q, got %q", expected, chain)
	}
}
```

Note: imports include `"os"` and `"path/filepath"` upfront — they are needed by later tests added in Task 3.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go-sidecar && go test -run TestBuildFilterChain -v`
Expected: FAIL — `buildFilterChain` undefined.

- [ ] **Step 3: Implement buildFilterChain**

Create `go-sidecar/audio.go`:

```go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func buildFilterChain(cfg AudioConfig) string {
	filters := []string{
		"highpass=f=200",
		"lowpass=f=8000",
	}

	if cfg.VocalBoostDB > 0 {
		filters = append(filters, fmt.Sprintf(
			"equalizer=f=1000:t=h:w=2000:g=%g",
			cfg.VocalBoostDB,
		))
	}

	if cfg.Normalize {
		filters = append(filters, "loudnorm=I=-16:TP=-1.5:LRA=11")
	}

	if cfg.NoiseGate {
		filters = append(filters, "agate=threshold=0.01:attack=5:release=50")
	}

	return strings.Join(filters, ",")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-sidecar && go test -run TestBuildFilterChain -v`
Expected: All 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add go-sidecar/audio.go go-sidecar/audio_test.go
git commit -m "feat(audio): implement buildFilterChain with unit tests"
```

---

### Task 3: Implement PreprocessAudio and cleanup (TDD)

**Files:**
- Modify: `go-sidecar/audio.go`
- Modify: `go-sidecar/audio_test.go`

- [ ] **Step 1: Write tests for PreprocessAudio**

Append these tests to `go-sidecar/audio_test.go` (the `os` and `filepath` imports are already present from Task 2):

```go
func TestPreprocessAudio_InvalidFile(t *testing.T) {
	cfg := DefaultAudioConfig()
	_, err := PreprocessAudio("/nonexistent/file.mp4", cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestPreprocessAudio_ValidVideo(t *testing.T) {
	// Generate a short test video with FFmpeg (2 seconds of sine wave)
	tmpDir := t.TempDir()
	testVideo := filepath.Join(tmpDir, "test.mkv")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:d=2",
		"-shortest", "-y", testVideo,
	)
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg not available or failed to create test video: %v", err)
	}

	cfg := DefaultAudioConfig()
	wavPath, err := PreprocessAudio(testVideo, cfg)
	if err != nil {
		t.Fatalf("PreprocessAudio failed: %v", err)
	}
	defer cleanupPreprocessed(wavPath)

	// Verify output exists and is non-empty
	info, err := os.Stat(wavPath)
	if err != nil {
		t.Fatalf("output WAV not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("output WAV is empty")
	}
	if !strings.HasSuffix(wavPath, "-enhanced.wav") {
		t.Errorf("expected WAV filename to end with -enhanced.wav, got %q", wavPath)
	}
}

func TestPreprocessAudio_NoAudioStream(t *testing.T) {
	// Generate a video with no audio
	tmpDir := t.TempDir()
	testVideo := filepath.Join(tmpDir, "no-audio.mkv")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:d=2",
		"-an", "-y", testVideo,
	)
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg not available or failed to create test video: %v", err)
	}

	cfg := DefaultAudioConfig()
	_, err := PreprocessAudio(testVideo, cfg)
	if err == nil {
		t.Fatal("expected error for video with no audio stream")
	}
}

func TestPreprocessAudio_Cleanup(t *testing.T) {
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, "sub-generator-audio", "test-cleanup.wav")
	os.MkdirAll(filepath.Dir(testFile), 0o755)
	os.WriteFile(testFile, []byte("dummy"), 0o644)

	cleanupPreprocessed(testFile)

	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, but it still exists")
	}
}
```

Also add `"os/exec"` to the import block at the top of `audio_test.go`:

```go
import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go-sidecar && go test -run "TestPreprocessAudio" -v`
Expected: FAIL — `PreprocessAudio` and `cleanupPreprocessed` undefined.

- [ ] **Step 3: Implement PreprocessAudio and cleanupPreprocessed**

Add to `go-sidecar/audio.go` after `buildFilterChain`:

```go
const audioTempDir = "sub-generator-audio"

func PreprocessAudio(inputPath string, cfg AudioConfig) (string, error) {
	if _, err := os.Stat(inputPath); err != nil {
		return "", fmt.Errorf("input file not accessible: %w", err)
	}

	tempDir := filepath.Join(os.TempDir(), audioTempDir)
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath := filepath.Join(tempDir, baseName+"-enhanced.wav")

	filterChain := buildFilterChain(cfg)

	args := []string{
		"-i", inputPath,
		"-vn",
		"-af", filterChain,
		"-ar", "16000",
		"-ac", "1",
		"-f", "wav",
		"-y",
		outputPath,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg preprocessing failed: %w", err)
	}

	if info, err := os.Stat(outputPath); err != nil || info.Size() == 0 {
		return "", fmt.Errorf("ffmpeg produced no output for %q", inputPath)
	}

	return outputPath, nil
}

func cleanupPreprocessed(wavPath string) {
	if wavPath == "" {
		return
	}
	if err := os.Remove(wavPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: failed to clean up preprocessed audio %q: %v\n", wavPath, err)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-sidecar && go test -run "TestPreprocessAudio" -v`
Expected: All 4 tests PASS (InvalidFile, ValidVideo, NoAudioStream, Cleanup). ValidVideo and NoAudioStream will be skipped if FFmpeg is not installed.

- [ ] **Step 5: Verify full test suite still passes**

Run: `cd go-sidecar && go test -v`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add go-sidecar/audio.go go-sidecar/audio_test.go
git commit -m "feat(audio): implement PreprocessAudio and cleanup with integration tests"
```

---

### Task 4: Change Whisper temperature to fallback list

**Files:**
- Modify: `go-sidecar/transcriber.go:183`

- [ ] **Step 1: Update temperature parameter**

In `go-sidecar/transcriber.go`, change line 183 from:

```go
	if err := writer.WriteField("temperature", "0"); err != nil {
```

to:

```go
	if err := writer.WriteField("temperature", "0.0,0.2,0.4,0.6,0.8,1.0"); err != nil {
```

- [ ] **Step 2: Verify it compiles**

Run: `cd go-sidecar && go build ./...`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add go-sidecar/transcriber.go
git commit -m "feat(audio): enable Whisper temperature fallback for hallucination reduction"
```

---

### Task 5: Wire preprocessing into pipeline

**Files:**
- Modify: `go-sidecar/pipeline.go:119-178`

- [ ] **Step 1: Add preprocessing step before transcription**

In `go-sidecar/pipeline.go`, insert the following block **before** the existing `// Step 4: Transcribe` comment (line 119). Then update the transcriber call to use `transcribePath`:

Insert before line 119:

```go
	// Step 4: Preprocess audio (if enabled)
	transcribePath := cmd.InputVideo
	audioConfig := cmd.AudioConfig
	if !audioConfig.Enabled && audioConfig.VocalBoostDB == 0 && !audioConfig.NoiseGate && !audioConfig.Normalize {
		// AudioConfig is all zero-values (missing from frontend JSON) — apply defaults
		audioConfig = DefaultAudioConfig()
	}

	if audioConfig.Enabled {
		sendStage("preprocessing", "Enhancing audio for transcription...")
		preprocessedPath, preprocessErr := PreprocessAudio(cmd.InputVideo, audioConfig)
		if preprocessErr != nil {
			fmt.Fprintf(os.Stderr, "warning: audio preprocessing failed, falling back to raw upload: %v\n", preprocessErr)
			sendStage("preprocessing", "Audio enhancement skipped, using original audio")
		} else {
			transcribePath = preprocessedPath
			defer cleanupPreprocessed(preprocessedPath)
		}
	}

```

Then change the `transcriber.Transcribe()` call (originally line 173-174, shifted down after the insertion above) from:

```go
	result, err := transcriber.Transcribe(
		cmd.InputVideo,
```

to:

```go
	result, err := transcriber.Transcribe(
		transcribePath,
```

- [ ] **Step 2: Update subsequent step comment numbers**

Make these exact replacements in `go-sidecar/pipeline.go`:
- `// Step 4: Transcribe` → `// Step 5: Transcribe`
- `// Step 4b: Write diagnostic` → `// Step 5b: Write diagnostic`
- `// Step 5: Translate` → `// Step 6: Translate`
- `// Step 6: Write subtitle file` → `// Step 7: Write subtitle file`

- [ ] **Step 3: Verify it compiles**

Run: `cd go-sidecar && go build ./...`
Expected: No errors.

- [ ] **Step 4: Run full test suite**

Run: `cd go-sidecar && go test -v`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add go-sidecar/pipeline.go
git commit -m "feat(audio): wire preprocessing into pipeline with graceful fallback"
```

---

### Task 6: Add AudioConfig to frontend and wire everything

This task combines types, UI, and state wiring into a single commit so the frontend never enters a broken state.

**Files:**
- Modify: `src/lib/types.ts:12-22`
- Modify: `src/components/SettingsPanel.tsx`
- Modify: `src/App.tsx:46-52,158-198`

- [ ] **Step 1: Add AudioConfig interface and field to types.ts**

In `src/lib/types.ts`, add the `AudioConfig` interface before the `GenerateCommand` interface (before line 12), and add the field to `GenerateCommand`:

```typescript
export interface AudioConfig {
  enabled: boolean;
  vocal_boost_db: number;
  noise_gate: boolean;
  normalize: boolean;
}

export interface GenerateCommand {
  command: "generate";
  input_video: string;
  source_lang: string | null;
  target_lang: string | null;
  output_format: OutputFormat;
  output_path: string | null;
  model_size: ModelSize;
  beam_size: number;
  vad_filter: boolean;
  audio_config: AudioConfig;
}
```

- [ ] **Step 2: Update SettingsPanel with Audio Enhancement section**

In `src/components/SettingsPanel.tsx`:

**2a.** Add these imports at the top (alongside existing imports):

```typescript
import { Separator } from "@/components/ui/separator";
import type { AudioConfig } from "@/lib/types";
```

**2b.** Replace the `SettingsPanelProps` interface (lines 9-15) with:

```typescript
interface SettingsPanelProps {
  beamSize: number;
  vadFilter: boolean;
  audioConfig: AudioConfig;
  onBeamSizeChange: (size: number) => void;
  onVadFilterChange: (enabled: boolean) => void;
  onAudioConfigChange: (config: AudioConfig) => void;
  disabled?: boolean;
}
```

**2c.** Update the component destructuring (line 17-23) to include the new props:

```typescript
export function SettingsPanel({
  beamSize,
  vadFilter,
  audioConfig,
  onBeamSizeChange,
  onVadFilterChange,
  onAudioConfigChange,
  disabled,
}: SettingsPanelProps) {
```

**2d.** Insert the Audio Enhancement section after line 79 (the closing `</div>` of the VAD Filter block) and before line 81 (the closing `</div>` of the `{isOpen && ...}` content wrapper):

```tsx
          <Separator className="my-1" />

          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label className="text-xs">Audio Enhancement</Label>
                <p className="text-[10px] text-muted-foreground">
                  Preprocess audio to improve transcription
                </p>
              </div>
              <Switch
                checked={audioConfig.enabled}
                onCheckedChange={(enabled) =>
                  onAudioConfigChange({ ...audioConfig, enabled })
                }
                disabled={disabled}
              />
            </div>

            {audioConfig.enabled && (
              <div className="space-y-4 pl-1">
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <Label className="text-xs text-muted-foreground">
                      Vocal Boost
                    </Label>
                    <span className="font-mono text-xs text-foreground">
                      {audioConfig.vocal_boost_db} dB
                    </span>
                  </div>
                  <Slider
                    min={0}
                    max={6}
                    step={1}
                    value={[audioConfig.vocal_boost_db]}
                    onValueChange={([val]) =>
                      onAudioConfigChange({
                        ...audioConfig,
                        vocal_boost_db: val,
                      })
                    }
                    disabled={disabled}
                  />
                  <div className="flex justify-between text-[10px] text-muted-foreground">
                    <span>Off</span>
                    <span>Max boost</span>
                  </div>
                </div>

                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <Label className="text-xs text-muted-foreground">
                      Noise Gate
                    </Label>
                    <p className="text-[10px] text-muted-foreground">
                      Suppress non-speech segments
                    </p>
                  </div>
                  <Switch
                    checked={audioConfig.noise_gate}
                    onCheckedChange={(noise_gate) =>
                      onAudioConfigChange({ ...audioConfig, noise_gate })
                    }
                    disabled={disabled}
                  />
                </div>

                <div className="flex items-center justify-between">
                  <div className="space-y-0.5">
                    <Label className="text-xs text-muted-foreground">
                      Normalization
                    </Label>
                    <p className="text-[10px] text-muted-foreground">
                      Even out volume levels
                    </p>
                  </div>
                  <Switch
                    checked={audioConfig.normalize}
                    onCheckedChange={(normalize) =>
                      onAudioConfigChange({ ...audioConfig, normalize })
                    }
                    disabled={disabled}
                  />
                </div>
              </div>
            )}
          </div>
```

- [ ] **Step 3: Wire AudioConfig state in App.tsx**

**3a.** Add `AudioConfig` to the existing type import (line 25-28 of `src/App.tsx`):

```typescript
import type {
  AudioConfig,
  GenerateCommand,
  ModelSize,
  OutputFormat,
  SidecarResponse,
} from "./lib/types";
```

**3b.** Add state after the `vadFilter` state (after line 52):

```typescript
  const [audioConfig, setAudioConfig] = useState<AudioConfig>({
    enabled: true,
    vocal_boost_db: 3,
    noise_gate: true,
    normalize: true,
  });
```

**3c.** In `handleGenerate` (around line 172), add `audio_config` to the command object:

```typescript
      const command: GenerateCommand = {
        command: "generate",
        input_video: videoPath,
        source_lang: sourceLang === "auto" ? null : sourceLang,
        target_lang: targetLang || null,
        output_format: format,
        output_path: null,
        model_size: model,
        beam_size: beamSize,
        vad_filter: vadFilter,
        audio_config: audioConfig,
      };
```

**3d.** Update the `useCallback` dependency array (around line 188) to include `audioConfig`:

```typescript
  }, [
    videoPath,
    connected,
    sourceLang,
    targetLang,
    format,
    model,
    beamSize,
    vadFilter,
    audioConfig,
    sendCommand,
  ]);
```

**3e.** Update the `<SettingsPanel>` JSX (around line 347) to pass the new props:

```tsx
            <SettingsPanel
              beamSize={beamSize}
              vadFilter={vadFilter}
              audioConfig={audioConfig}
              onBeamSizeChange={setBeamSize}
              onVadFilterChange={setVadFilter}
              onAudioConfigChange={setAudioConfig}
            />
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `npm run build`
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add src/lib/types.ts src/components/SettingsPanel.tsx src/App.tsx
git commit -m "feat(audio): add AudioConfig to frontend types, SettingsPanel UI, and App state"
```

---

### Task 7: End-to-end verification

- [ ] **Step 1: Run Go test suite**

Run: `cd go-sidecar && go test -v`
Expected: All tests pass.

- [ ] **Step 2: Run frontend build**

Run: `npm run build`
Expected: No errors.

- [ ] **Step 3: Verify FFmpeg filter chain manually**

Run:

```bash
ffmpeg -f lavfi -i "sine=frequency=1000:duration=2" -af "highpass=f=200,lowpass=f=8000,equalizer=f=1000:t=h:w=2000:g=3,loudnorm=I=-16:TP=-1.5:LRA=11,agate=threshold=0.01:attack=5:release=50" -ar 16000 -ac 1 -f wav -y /tmp/test-filter-chain.wav
```

Expected: Creates a valid WAV file without FFmpeg errors.

- [ ] **Step 4: Review all changes**

Run: `git diff main --stat`

Expected files:
- `go-sidecar/audio.go` (new)
- `go-sidecar/audio_test.go` (new)
- `go-sidecar/config.go` (modified)
- `go-sidecar/config_test.go` (new)
- `go-sidecar/transcriber.go` (modified)
- `go-sidecar/pipeline.go` (modified)
- `src/lib/types.ts` (modified)
- `src/components/SettingsPanel.tsx` (modified)
- `src/App.tsx` (modified)
