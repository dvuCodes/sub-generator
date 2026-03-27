#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ -x "${SCRIPT_DIR}/runtime/python" ]]; then
  exec "${SCRIPT_DIR}/runtime/python" "${SCRIPT_DIR}/service.py" "$@"
fi

exec python3 "${SCRIPT_DIR}/service.py" "$@"
