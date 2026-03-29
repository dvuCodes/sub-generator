# Contributing to SubGen

Thanks for considering a contribution.

## Before you start

- Read the root `README.md` for the current architecture and setup flow.
- Keep changes focused. This repo is still stabilizing its local-first transcription pipeline.
- Use Bun for JavaScript and TypeScript workflows in this repo.

## Development workflow

1. Install frontend dependencies with `bun install`.
2. Install Python backend dependencies with `python -m pip install -r python-backend/requirements.txt`.
3. Run the checks relevant to your change before opening a PR:
   - `bun run lint`
   - `bun run test`
   - `bun run build`
   - `go test ./...` from `go-sidecar/`
   - `python -m unittest discover -s python-backend -p "test_*.py"`
   - `cargo check` from `src-tauri/`

## Pull requests

- Explain the user-facing problem you are solving.
- Include verification notes and any important setup assumptions.
- Keep refactors tied to the work at hand; avoid unrelated cleanup in the same change.

## Scope guidance

Contributions are especially helpful when they improve:

- subtitle correctness and segment timing
- local setup clarity and reliability
- backend capability detection and error messaging
- desktop workflow polish without adding hosted dependencies
