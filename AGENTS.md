# Repository Guidelines
## Baseline Workflow

- Start every task by determining:
  1. goal + acceptance criteria,
  2. constraints (time, safety, scope),
  3. what must be inspected (files, commands, tests).
- If requirements are ambiguous, ask targeted clarifying questions before making irreversible changes.
- When changes are required:
  - propose a short plan (2-6 bullets), then execute.
- Always check for relevant skills before building.
- For JavaScript/TypeScript package management in this repo, use Bun whenever it is applicable; prefer `bun install`, `bun run`, `bun test`, and `bunx` over `npm`, `npx`, `yarn`, or `pnpm` unless a tool explicitly requires a different package manager.
- Run project git commands from `C:\Users\datvu\projects\sub-generator`; do not use the parent `C:\Users\datvu\projects` repo for this project.
- Run Go commands and tests from `C:\Users\datvu\projects\sub-generator\go-sidecar`; it is a nested Go module and `go test ./...` will not resolve correctly from the repo root.
- If `rg.exe` is blocked in PowerShell, use `Select-String` and `Get-ChildItem` as the repository search fallback.
- If the Tauri app uses `externalBin` sidecars, verify the bundled sidecar executable is rebuilt when `go-sidecar/` changes.
- For `tauri dev`, avoid rebuilding or recopying the sidecar on unrelated Rust/UI changes; only refresh the dev copies when `go-sidecar/` is newer, otherwise Windows can fail on a locked `subgen-sidecar.exe`.
- In WSL/Linux dev shells, ensure `go` is installed and on `PATH` before running `tauri dev`; the sidecar is built from `go-sidecar/` during the Tauri build and otherwise fails before the app starts.
- If WSL package installs are blocked by missing `sudo` access, a user-local Go install under `$HOME/.local/go/bin` is sufficient for `go test ./...` and `tauri dev` as long as that path is exported before running the toolchain.
- If `bun` is not on the WSL `PATH`, use the existing Windows install at `/mnt/c/Users/datvu/.bun/bin/bun.exe` for frontend tests such as `bun test`.
- If a dev sidecar copy is locked during `tauri dev`, warn and continue the rebuild; only require the user to close the active app when they need the newly built sidecar binary to replace the locked one.
- For `tauri dev`, keep `bundle.externalBin` enabled for packaging but override it out of `TAURI_CONFIG` in debug builds; otherwise `tauri-build` can panic trying to delete a locked sidecar in `src-tauri/target/dev-tauri/debug/`.
- For `tauri dev`, do not assume refreshing `src-tauri/subgen-sidecar-<target>.exe` is enough; also sync the built sidecar into `src-tauri/target/debug/` and `src-tauri/target/dev-tauri/debug/` so the running dev app does not reuse a stale sidecar.
- The current Tauri desktop dependency graph resolves to crates that require Rust `1.88.0` (for example `time 0.3.47`), so keep `rust-toolchain.toml` and `src-tauri/Cargo.toml` aligned at `1.88.0` or newer; otherwise older Cargo versions can fail while parsing transitive manifests before the app even builds.
- In WSL/Linux `tauri dev`, a blank WebKit window with `libEGL` / `MESA` / `ZINK` renderer errors is usually a host rendering issue, not a React crash; set Linux WebKit fallbacks before Tauri startup and keep the workaround scoped to WSL so Windows behavior stays unchanged.
- If a required runtime dependency is intentionally external, convert raw PATH/startup failures into actionable setup guidance before surfacing them in the UI.
- When resolving repo-local `services/whisper-server/` assets, accept both `whisper-server` and `whisper-server.exe`; mixed Windows/WSL-style search roots can surface the non-native suffix even when the documented install is present.
- If backend language discovery is unavailable at startup, preserve the built-in language selector options and surface the translation setup issue as a non-fatal warning instead of blocking the transcription UI.
- Keep source transcription languages on the built-in Whisper list even when discovered translation pairs are sparse; only narrow translation targets to the backend-reported coverage.
- In the frontend, do not auto-switch the selected ASR or translation backend just because capabilities say it needs setup; preserve the current choice unless that backend is absent entirely, so slow capability probes do not flip the UI to source-only or whisper.cpp.
- For the completion-state `Open Folder` action, pass the native directory path to `@tauri-apps/plugin-shell` `open()`; do not convert local folders into `file://` URLs or Windows Explorer can fail to open them.
- When the UI leaves source language on auto-detect, still send `language=auto` to `whisper-server`; omitting the field lets the bundled server default to English and can mis-transcribe foreign speech.
- For video transcription, start `whisper-server` with `--convert` and validate `ffmpeg` first; otherwise the server can reject `.mp4`/video uploads with a generic `400 Invalid request`.
- The bundled `whisper-server` returns subtitle-ready timestamps only with `response_format=verbose_json`; parse current `segments[].start`/`end` values as seconds, and keep legacy `t0`/`t1` millisecond parsing only as a fallback.
- If `whisper-server` returns zero segments, fail in the pipeline with explicit no-speech / missing-timestamps guidance before the subtitle writer runs; do not surface `astisub: no subtitles to write` directly.
- If `whisper-server` returns a mix of usable and unusable segment timings, drop only the unusable segments and continue; fail only when no subtitle-safe timed segments remain.
- Treat ASR segments longer than subtitle-safe bounds (currently 20s) as unusable timings; retry once on the original input with `vad_filter=false` instead of translating the hallucinated span.
- If Faster Whisper returns pathological overlong VAD segments (for example, a single segment spanning a large stretch of runtime), retry transcription once with `vad_filter=false` before accepting the filtered result; otherwise validation can silently drop most of the tail while leaving only a few short segments.
- When comparing transcription retries, do not accept a retry that materially regresses the last kept subtitle timestamp just because it has more short segments; prefer the candidate that preserves or improves tail coverage on the original input.
- The Go sidecar pipeline must stop managed `whisper-server` / `llama-server` processes on every terminal path itself; do not rely on the frontend to send `stop_services` after `complete` or `error`, or VRAM can remain allocated if that round-trip is missed.
- For this desktop-only Tauri app, avoid `staticlib` in `src-tauri/Cargo.toml`; it produces a massive `app_lib.lib` on Windows and can fail rebuilds with archive rename access errors.
- If Windows dev builds lock `subgen.pdb`, disable dev debug info in `src-tauri/Cargo.toml` (`[profile.dev] debug = 0`) rather than fighting repeated PDB replace failures.
- If `tauri dev` is still blocked by stale Windows debug artifacts, isolate Rust outputs with `src-tauri/.cargo/config.toml` and a dedicated target dir instead of reusing `src-tauri/target/debug`.
- `tsconfig.app.json` excludes `src/**/*.test.ts` but not `src/**/*.test.tsx`; keep Bun/JSX regression tests on the excluded pattern or update the exclude list before relying on `.test.tsx`.
- Always mark tasks off when complete.
- After every correction to assumptions/process, update this `AGENTS.md`.
- Track every substantial feature, bug, investigation, or reliability task in Linear under the `SubGen` project on team `Rio31`; add or link the issue before major work starts, and post progress plus verification notes to the issue before closing it.
- For loopback `whisper-server` HTTP calls, stage multipart uploads in a temp file and send them as fixed-length request bodies; `io.Pipe` streaming uploads can still fail with `use of closed network connection` on real transcription files even when a small probe file succeeds.
- Treat `list_languages` as static capability metadata only; do not use it as proof that the translation engine is installed or running.
- When testing streamed Go HTTP request bodies, do not rely on `ContentLength`; assert the streaming mechanism itself (for example `io.PipeReader`) or read behavior instead.
- Treat `python-backend/` as the canonical ML backend source tree and `services/ml-backend/` as a staged runtime mirror/cache root; do not maintain a second Python implementation under `services/ml-backend/`.
- Managed sidecar services should not rely on fixed loopback ports staying free; if 8080-8082 are occupied, select open loopback ports for the current sidecar session so local tools or stale processes do not masquerade as dependency-install failures.
- Keep the Python ML backend launcher scripts pointed at the staged root `service.py`; the build copies `python-backend/` as-is and does not wrap it under an extra `app/` directory.
- Do not list the staged `src-tauri/resources/ml-backend` tree in `tauri.conf.json` for dev builds; Tauri will watch it and can loop forever if `build.rs` refreshes those files. Inject packaged ML-backend resources from `build.rs` only for non-debug builds.
- Treat Faster Whisper CUDA DLL failures on Windows as both startup and runtime fallback cases; `cublas64_12.dll` issues can surface lazily during segment generation, so retry on CPU instead of only guarding model construction.
- When making file edits, use the Codex `apply_patch` tool (do not embed `apply_patch` inside shell commands).
- Do not propose follow-up tasks or enhancements at the end of your final answer.
- When working on frontend design, use playwright to test and confirm desired feature implemention.

## Context7 MCP (library docs)

Use Context7 to fetch accurate, version-matched documentation during coding tasks.

- Add `use context7` when you need library/API docs.
- If known, pin the library with slash syntax (e.g., `use library /supabase/supabase`).
- Mention the target version.
- Fetch minimal targeted docs; summarize (no large dumps).

## Editing files

- Make the smallest safe change that solves the issue.
- Preserve existing style and conventions.
- Prefer patch-style edits (small, reviewable diffs) over full-file rewrites.
- After making changes, run the project's standard checks when feasible (format/lint, unit tests, build/typecheck).
- For frontend/UI changes, when possible, do a quick smoke test using Playwright MCP (navigate key routes, click primary flows, check console errors, and capture a screenshot if helpful).
