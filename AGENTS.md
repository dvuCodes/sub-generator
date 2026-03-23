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
- Run project git commands from `C:\Users\datvu\projects\sub-generator`; do not use the parent `C:\Users\datvu\projects` repo for this project.
- If `rg.exe` is blocked in PowerShell, use `Select-String` and `Get-ChildItem` as the repository search fallback.
- If the Tauri app uses `externalBin` sidecars, verify the bundled sidecar executable is rebuilt when `go-sidecar/` changes.
- For `tauri dev`, avoid rebuilding or recopying the sidecar on unrelated Rust/UI changes; only refresh the dev copies when `go-sidecar/` is newer, otherwise Windows can fail on a locked `subgen-sidecar.exe`.
- If a dev sidecar copy is locked during `tauri dev`, warn and continue the rebuild; only require the user to close the active app when they need the newly built sidecar binary to replace the locked one.
- For `tauri dev`, keep `bundle.externalBin` enabled for packaging but override it out of `TAURI_CONFIG` in debug builds; otherwise `tauri-build` can panic trying to delete a locked sidecar in `src-tauri/target/dev-tauri/debug/`.
- For `tauri dev`, do not assume refreshing `src-tauri/subgen-sidecar-<target>.exe` is enough; also sync the built sidecar into `src-tauri/target/debug/` and `src-tauri/target/dev-tauri/debug/` so the running dev app does not reuse a stale sidecar.
- If a required runtime dependency is intentionally external, convert raw PATH/startup failures into actionable setup guidance before surfacing them in the UI.
- If backend language discovery is unavailable at startup, preserve the built-in language selector options and surface the translation setup issue as a non-fatal warning instead of blocking the transcription UI.
- Keep source transcription languages on the built-in Whisper list even when discovered translation pairs are sparse; only narrow translation targets to the backend-reported coverage.
- When the UI leaves source language on auto-detect, still send `language=auto` to `whisper-server`; omitting the field lets the bundled server default to English and can mis-transcribe foreign speech.
- For video transcription, start `whisper-server` with `--convert` and validate `ffmpeg` first; otherwise the server can reject `.mp4`/video uploads with a generic `400 Invalid request`.
- The bundled `whisper-server` returns subtitle-ready timestamps only with `response_format=verbose_json`; parse current `segments[].start`/`end` values as seconds, and keep legacy `t0`/`t1` millisecond parsing only as a fallback.
- If `whisper-server` returns zero segments, fail in the pipeline with explicit no-speech / missing-timestamps guidance before the subtitle writer runs; do not surface `astisub: no subtitles to write` directly.
- If `whisper-server` returns a mix of usable and unusable segment timings, drop only the unusable segments and continue; fail only when no subtitle-safe timed segments remain.
- For this desktop-only Tauri app, avoid `staticlib` in `src-tauri/Cargo.toml`; it produces a massive `app_lib.lib` on Windows and can fail rebuilds with archive rename access errors.
- If Windows dev builds lock `subgen.pdb`, disable dev debug info in `src-tauri/Cargo.toml` (`[profile.dev] debug = 0`) rather than fighting repeated PDB replace failures.
- If `tauri dev` is still blocked by stale Windows debug artifacts, isolate Rust outputs with `src-tauri/.cargo/config.toml` and a dedicated target dir instead of reusing `src-tauri/target/debug`.
- `tsconfig.app.json` excludes `src/**/*.test.ts` but not `src/**/*.test.tsx`; keep Bun/JSX regression tests on the excluded pattern or update the exclude list before relying on `.test.tsx`.
- Always mark tasks off when complete.
- After every correction to assumptions/process, update this `AGENTS.md`.
- When testing streamed Go HTTP request bodies, do not rely on `ContentLength`; assert the streaming mechanism itself (for example `io.PipeReader`) or read behavior instead.
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
