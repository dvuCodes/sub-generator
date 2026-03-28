# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Add a bundled Python ML backend for Faster Whisper ASR, NLLB translation, and optional speaker diarization.
- Add backend-aware subtitle generation controls for ASR provider, ASR model, translation backend, and diarization.
- Add backend capability, setup, and completion metadata across the Go sidecar and desktop UI.
- Add frontend and backend regression tests for backend selection, capability-driven languages, speaker labeling, and ML backend HTTP clients.

### Changed
- Make the ML backend the default local subtitle pipeline while keeping `whisper.cpp` and Gemma translation as manual fallback paths.
- Stage `python-backend/` into the runtime mirror for dev and inject packaged ML-backend resources from `build.rs` for non-debug builds.
- Drive source and target language options from backend capabilities instead of static translation assumptions.
- Keep the full built-in source transcription language list available while still narrowing translation targets to backend-reported coverage.

### Fixed
- Fix auto-detect translation handoff so detected ASR language is used for NLLB instead of sending `source_lang="auto"`.
- Fix subtitle stitching and output formatting to preserve speaker boundaries and speaker labels when diarization is enabled.
- Fix `tauri dev` rebuild loops caused by watching generated `src-tauri/resources/ml-backend` files during debug builds.
- Fix capability merging so installed `whisper.cpp` and Gemma fallback backends remain selectable when the ML backend is available.
- Fix setup validation so broken ML backend startups surface actionable setup errors instead of failing later during generation.
- Fix Windows Faster Whisper CUDA DLL failures by retrying transcription on CPU when CUDA libraries are unavailable at load or runtime.
