# Dependency Validation & Auto-Install

**Date:** 2026-03-25
**Status:** Approved
**Platform:** Windows-only (macOS/Linux out of scope for this iteration)

## Problem

When required DLLs are missing from service directories (whisper-server, llama-server), services fail to start and the app shows a generic timeout error after 30-120 seconds. The user has no idea what's wrong or how to fix it. Missing ffmpeg produces a similarly unhelpful error.

## Solution

Proactive dependency validation on app connect. The Go sidecar probes service binaries and checks ffmpeg availability, then reports a structured `setup_status` to the frontend. If issues are detected, the frontend shows a banner with clear descriptions and download actions. The user chooses GPU or CPU bundles with trade-off descriptions. Downloads are extracted atomically with the sidecar handling the full install lifecycle.

## IPC Contract

### check_setup (Request)

```json
{ "command": "check_setup" }
```

Sent by frontend on connect, alongside existing `system_info` and `list_languages`.

### setup_status (Response)

```json
{
  "type": "setup_status",
  "services": [
    {
      "id": "whisper",
      "display_name": "whisper-server",
      "required_for": "transcription",
      "state": "ready | action_required",
      "issues": [
        {
          "code": "binary_not_found | binary_not_runnable | not_in_path",
          "observed_error": "optional stderr from exec probe"
        }
      ],
      "actions": [
        {
          "id": "whisper/install_gpu_bundle",
          "label": "Install GPU (CUDA) bundle",
          "description": "Preferred for NVIDIA GPUs. Faster transcription.",
          "kind": "archive",
          "preferred": true
        },
        {
          "id": "whisper/install_cpu_bundle",
          "label": "Install CPU bundle",
          "description": "Works on all systems. Slower transcription.",
          "kind": "archive"
        }
      ]
    },
    {
      "id": "ffmpeg",
      "display_name": "ffmpeg",
      "required_for": "transcription",
      "state": "action_required",
      "issues": [{ "code": "not_in_path" }],
      "actions": [
        {
          "id": "ffmpeg/install_manual",
          "label": "Install ffmpeg",
          "kind": "manual",
          "guidance": "Download ffmpeg from https://ffmpeg.org/download.html and add it to your system PATH."
        }
      ]
    }
  ]
}
```

**`required_for`** is a strict union: `"transcription" | "translation"`.

**`state`**: `"ready"` (probe passed) or `"action_required"` (fixable issue detected).

**Issue codes:**
- `"binary_not_found"` — exe not found in search paths. Archive actions attached.
- `"binary_not_runnable"` — exec probe failed. Archive actions attached ONLY when stderr matches DLL/shared-library load patterns (Windows: "DLL not found", "not found", error 0xC000007B; MSYS2: "cannot open shared object file"). For permission, timeout, or unknown failures: no actions, just `observed_error`.
- `"not_in_path"` — ffmpeg not found in system PATH. Manual guidance action.

**Action `kind`:** `"archive"` (downloadable zip) or `"manual"` (text guidance only).

**Action IDs** are globally unique, scoped by service: `whisper/install_gpu_bundle`, `llama/install_gpu_bundle`, etc.

**`preferred: true`** on GPU action when NVIDIA GPU detected via `detectGPU()`. Label says "Preferred for NVIDIA GPUs" — not "Recommended" since CUDA runtime compatibility isn't fully verified.

**Models are NOT checked** by `check_setup`. Whisper and Gemma models are auto-downloaded during generate — surfacing them on connect would create false warnings for normal first runs.

**Frontend `ServiceAction` is a subset of the Go-internal `Action` struct.** The Go `Action` struct includes additional internal fields (`URL`, `InstallDir`, `StripComponents`, `ExpectedBinary`, `ServiceID`) that are never sent to the frontend. The frontend only sees `id`, `label`, `description`, `kind`, `preferred`, and `guidance`.

### install_dependency (Request)

```json
{ "command": "install_dependency", "action_id": "whisper/install_gpu_bundle" }
```

Only `action_id` is sent. The sidecar resolves url, install_dir, and strip_components from its internal action registry built during the last `CheckSetup()`. Unknown action_ids are rejected.

The sidecar streams progress via `sendProgress()` with stages: `"downloading_dependency"`, `"extracting"`, `"validating"`. On completion, it re-runs `CheckSetup()` and sends a fresh `setup_status`.

### Download URLs

**whisper-server (v1.8.4):**
- GPU (CUDA 12): `https://github.com/ggml-org/whisper.cpp/releases/download/v1.8.4/whisper-cublas-12.4.0-bin-x64.zip` (~436 MB, `strip_components: 1`)
- CPU: `https://github.com/ggml-org/whisper.cpp/releases/download/v1.8.4/whisper-bin-x64.zip` (~5 MB, `strip_components: 1`)

**llama-server (latest):**
- GPU (CUDA 12): `https://github.com/ggml-org/llama.cpp/releases/latest/download/llama-cublas-12.4.0-bin-x64.zip` (~TBD, `strip_components: 1`)
- CPU: `https://github.com/ggml-org/llama.cpp/releases/latest/download/llama-bin-x64.zip` (~TBD, `strip_components: 1`)

Note: GitHub release zips typically have a top-level folder (e.g., `whisper-cublas-12.4.0-bin-x64/`). `strip_components: 1` removes this so files extract directly into `install_dir`. If a zip has no top-level folder, `strip_components: 0` should be used. The `extractZip` function must fail clearly if the expected binary is not found after extraction regardless of strip value.

## Go Sidecar — Dependency Checker

### New file: `go-sidecar/setup.go`

**`CheckSetup(config ServiceConfig) SetupStatusResponse`** — main entry point. Probes each service and returns structured status. Stores all discovered actions in the action registry.

**`probeService(binaryPath string) (issueCode, stderr string)`** — runs `<binary> --help` with 5-second timeout, captures stderr. A probe **succeeds** if the process starts and exits (regardless of exit code — many programs return non-zero for `--help`). A probe **fails** if the process cannot start at all (DLL load error, permission denied, not found) or if the 5-second timeout expires. The `--help` flag is stateless and does not conflict with an already-running instance of the same binary. Returns empty strings if probe succeeds.

**`classifyProbeError(stderr string) (code string, attachActions bool)`** — pattern matches stderr:
- Contains "cannot open shared object", "DLL not found", "not found", Windows error 0xC000007B → `"binary_not_runnable"`, `attachActions = true`
- Other errors (permission denied, timeout, etc.) → `"binary_not_runnable"`, `attachActions = false`

**`checkFFmpeg() ServiceStatus`** — reuses existing `validateCommandAvailability("ffmpeg")`. Returns manual guidance action if missing.

**`whisperDownloadActions(hasGPU bool) []Action`** / **`llamaDownloadActions(hasGPU bool) []Action`** — build action lists with download URLs (see Download URLs section) and `preferred: true` on GPU when NVIDIA detected. Each action includes `StripComponents`, `InstallDir`, `ExpectedBinary`, and `ServiceID` as internal fields.

**Action registry:** after `CheckSetup()` runs, all actions are stored in a map keyed by action_id. `resolveAction(actionID string) (*Action, error)` looks up from this map. Unknown IDs return error.

### New file: `go-sidecar/archive.go`

**`DownloadAndExtractArchive(action Action, progress func(downloaded, total int64)) error`**

1. Stop any managed service process for the target service
2. Download zip to temp file using existing `DownloadModel()` (genuine reuse — single file to temp path)
3. Extract zip to a temp sibling directory under the same parent as `install_dir` (e.g., `services/.whisper-server-install-tmp`) — same volume guarantees `os.Rename` works on Windows
4. Validate extracted layout — confirm expected binary exists after `strip_components` applied
5. Atomic swap via injectable `swapDirs` function:
   - Step A: Rename `install_dir` → `install_dir.bak`
   - Step B: Rename temp dir → `install_dir`
   - On both succeed: delete `.bak`
   - On Step A failure: return `"install_locked"` error, no cleanup needed (original dir untouched)
   - On Step B failure: restore Step A (rename `.bak` back to `install_dir`), return `"install_locked"` error with guidance: "Close any programs using the service directory and try again."
6. Clean up temp zip
7. Re-run `CheckSetup()` and send fresh `setup_status`

**`extractZip(zipPath, destDir string, stripComponents int) error`** — standard `archive/zip` extraction. `strip_components` stored per action, defaults to 0. Validates expected binary exists after extraction. Fails with clear error if binary not found (catches wrong `strip_components` or unexpected archive layout).

**Serialization:** Both `install_dependency` and `generate` are dispatched as goroutines in `main.go`. A `sync.Mutex` is acquired inside each goroutine (not in the command handler, to avoid blocking the stdin read loop). Use `TryLock()` — if the mutex is already held, return an error immediately: "Cannot install while processing" or "Cannot generate while installing."

### Modified: `go-sidecar/config.go`

Add `ActionID string` field to the `Command` struct with `json:"action_id,omitempty"` tag.

### Modified: `go-sidecar/main.go`

- Add `check_setup` and `install_dependency` command routing
- Dispatch `install_dependency` as goroutine with `TryLock` serialization
- Add `sync.Mutex` shared between generate and install goroutines

## Frontend

### New types in `src/lib/types.ts`

```typescript
export type RequiredFor = "transcription" | "translation";

export interface ServiceIssue {
  code: string;
  observed_error?: string;
}

export interface ServiceAction {
  id: string;
  label: string;
  description: string;
  kind: "archive" | "manual";
  preferred?: boolean;
  guidance?: string;
}

export interface ServiceStatus {
  id: string;
  display_name: string;
  required_for: RequiredFor;
  state: "ready" | "action_required";
  issues: ServiceIssue[];
  actions: ServiceAction[];
}

export interface SetupStatusResponse {
  type: "setup_status";
  services: ServiceStatus[];
}

export interface CheckSetupCommand {
  command: "check_setup";
}

export interface InstallDependencyCommand {
  command: "install_dependency";
  action_id: string;
}
```

Also update the `SidecarCommand` union to include `CheckSetupCommand | InstallDependencyCommand` and the `SidecarResponse` union to include `SetupStatusResponse`.

### New component: `src/components/SetupBanner.tsx`

Presentational component. Shows at top of main UI (above VideoDropzone) when any service has `state: "action_required"`. For each service with issues:
- Service name + what it's required for
- Issue description via `formatSetupIssue(issue)` (observed_error if present, human-readable fallback from code)
- Action buttons: `"archive"` actions get download buttons (preferred one highlighted), `"manual"` actions show guidance text
- Clicking download sends `install_dependency` and triggers install flow

### New: `src/lib/installState.ts`

Separate install reducer, not mixed into the generate processing state.

- `InstallState`: `{ stage, percent, message, pendingActionId, targetServiceId }`
- `advanceInstallState(current, response)` — handles `"downloading_dependency"`, `"extracting"`, `"validating"` stages
- Install completes when `setup_status` arrives AND the targeted service's state is `"ready"`. Unrelated `setup_status` responses during install are ignored.

### New: `src/lib/setupHelpers.ts`

Pure functions:
- `shouldDisableGenerate(setupStatus, targetLang)` — returns true when any `required_for: "transcription"` service is `action_required`, OR when targetLang is set and any `required_for: "translation"` service is `action_required`
- `formatSetupIssue(issue)` — maps issue codes to human-readable messages

### Modified: `src/App.tsx`

- New `"installing"` app state: `"idle" | "processing" | "complete" | "error" | "installing"`
- New `setupStatus` state holding `SetupStatusResponse`
- New `installState` state with separate reducer
- Send `check_setup` on connect
- Handle `setup_status` response in `onResponse` switch — store in state, check install completion against `pendingActionId`
- Handle install progress — route to `advanceInstallState` when `appState === "installing"`, route to `advanceProcessingState` when `appState === "processing"` (since the mutex prevents concurrency, only one is active)
- Render `SetupBanner` when idle and issues exist
- Render `ProcessingView` during installing (with configurable stage labels)
- Use `shouldDisableGenerate()` for Generate button disabled state

### Modified: `src/components/ProcessingView.tsx`

- Accept stage labels/order as props instead of using module-level `STAGE_ORDER` and `STAGE_LABELS` constants
- `src/lib/processingState.ts` keeps its own `STAGE_ORDER` for generate — not parameterized. The install flow uses `installState.ts` with its own stage set. `ProcessingView` just receives whatever stage labels are passed.
- No other changes — same progress bar, same layout

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Probe timeout (5s) | `binary_not_runnable`, no actions, `observed_error` set |
| DLL load error in probe | `binary_not_runnable`, archive actions attached |
| Permission error in probe | `binary_not_runnable`, no actions, `observed_error` set |
| Unknown probe failure | `binary_not_runnable`, no actions, `observed_error` set |
| Download fails | Error sent to frontend, return to idle |
| Extraction fails | Error sent, temp files cleaned up, return to idle |
| Locked install dir (Step A fails) | `"install_locked"` error, original dir untouched |
| Locked install dir (Step B fails) | `"install_locked"` error, `.bak` restored to original name |
| Unknown action_id | Error: "Unknown install action" |
| Install during generate | Error: "Cannot install while processing" |
| Generate during install | Error: "Cannot generate while installing" |

## Testing Strategy

### Go unit tests (`go-sidecar/setup_test.go`)

- `TestCheckSetup_AllReady` — all probes pass → all `"ready"`
- `TestCheckSetup_WhisperNotFound` — binary missing → `"binary_not_found"`, archive actions
- `TestCheckSetup_WhisperDLLError` — probe stderr matches DLL pattern → `"binary_not_runnable"`, archive actions
- `TestCheckSetup_WhisperPermissionError` — probe stderr doesn't match DLL → `"binary_not_runnable"`, no actions
- `TestCheckSetup_FFmpegMissing` — ffmpeg not in PATH → manual action
- `TestClassifyProbeError` — various stderr patterns correctly classified (DLL, permission, timeout, unknown)
- `TestResolveAction_Valid` — known action_id resolves with correct internal fields
- `TestResolveAction_Unknown` — unknown action_id returns error
- `TestProbeService_ExitCodeNonZero` — binary exits non-zero but without DLL error → probe succeeds (not a failure)

### Go unit tests (`go-sidecar/archive_test.go`)

- `TestExtractZip_StripComponents0` — nested folder preserved
- `TestExtractZip_StripComponents1` — top-level folder flattened
- `TestExtractZip_MissingBinary` — validation fails if expected binary not found after extraction
- `TestDownloadAndExtract_AtomicSwap` — old dir backed up, new dir in place
- `TestDownloadAndExtract_SwapStepAFails` — injected `swapDirs` fails on first rename → original untouched, `"install_locked"` error
- `TestDownloadAndExtract_SwapStepBFails` — injected `swapDirs` fails on second rename → `.bak` restored, `"install_locked"` error
- OS-specific lock test gated behind `//go:build windows`

### Go integration tests (`go-sidecar/setup_integration_test.go`)

- `TestProbeService_RealBinary` — probes actual binary, skips if not present

### Frontend tests

**`src/lib/installState.test.ts`:**
- Stage advancement through download → extract → validate
- Pending action tracking
- Exit on matching `setup_status` (targeted service now ready)
- Rejection of unrelated `setup_status` during install

**`src/lib/setupHelpers.test.ts`:**
- `shouldDisableGenerate`: transcription blocker → disabled
- `shouldDisableGenerate`: translation blocker + targetLang set → disabled
- `shouldDisableGenerate`: translation blocker + no targetLang → enabled
- `shouldDisableGenerate`: all ready → enabled
- `formatSetupIssue`: each issue code → correct message
- `formatSetupIssue`: with observed_error → includes it
