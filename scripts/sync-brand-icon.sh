#!/usr/bin/env bash
# Sync canonical brand icon (128x128 PNG) to all consumer paths.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${ROOT}/distributions/shared/media/icon.png"
if [[ ! -f "$SRC" ]]; then
  echo "missing canonical icon: $SRC" >&2
  exit 1
fi
cp "$SRC" "${ROOT}/distributions/vscode-extension/media/icon.png"
cp "$SRC" "${ROOT}/internal/dashboard/icon.png"
cp "$SRC" "${ROOT}/distributions/npm-cli/media/icon.png"
echo "Synced brand icon to vscode-extension, dashboard, and npm-cli"
