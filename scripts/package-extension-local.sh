#!/usr/bin/env bash
# Build a production .vsix from locally staged native binaries.
#
# Usage:
#   scripts/package-extension-local.sh
#   scripts/package-extension-local.sh ~/Downloads/artifacts
#
# Expects binaries in distributions/vscode-extension/bin/ OR pass a folder containing:
#   kotro-proxy-aarch64-apple-darwin
#   kotro-proxy-x86_64-apple-darwin
#   kotro-proxy-x86_64-unknown-linux-gnu
#   kotro-proxy-x86_64-pc-windows-msvc.exe
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EXT_DIR="${ROOT}/distributions/vscode-extension"
BIN_DIR="${EXT_DIR}/bin"
SRC_DIR="${1:-}"

copy_bin() {
  local name="$1"
  if [[ -n "$SRC_DIR" && -f "${SRC_DIR}/${name}" ]]; then
    cp "${SRC_DIR}/${name}" "${BIN_DIR}/${name}"
    chmod +x "${BIN_DIR}/${name}" 2>/dev/null || true
    echo "Staged ${name} from ${SRC_DIR}"
  elif [[ -f "${BIN_DIR}/${name}" ]]; then
    echo "Using existing ${BIN_DIR}/${name}"
  else
    echo "Missing binary: ${name}" >&2
    echo "Download CI artifacts and pass the folder, or copy into ${BIN_DIR}/" >&2
    exit 1
  fi
}

mkdir -p "$BIN_DIR"

if [[ -x "${ROOT}/bin/kotro-proxy" ]]; then
  case "$(uname -s)/$(uname -m)" in
    Darwin/arm64)  cp "${ROOT}/bin/kotro-proxy" "${BIN_DIR}/kotro-proxy-aarch64-apple-darwin" ;;
    Darwin/x86_64) cp "${ROOT}/bin/kotro-proxy" "${BIN_DIR}/kotro-proxy-x86_64-apple-darwin" ;;
    Linux/x86_64)  cp "${ROOT}/bin/kotro-proxy" "${BIN_DIR}/kotro-proxy-x86_64-unknown-linux-gnu" ;;
  esac
  echo "Staged local Go proxy into ${BIN_DIR} for $(uname -s)/$(uname -m)"
fi

copy_bin "kotro-proxy-aarch64-apple-darwin"
copy_bin "kotro-proxy-x86_64-apple-darwin"
copy_bin "kotro-proxy-x86_64-unknown-linux-gnu"
copy_bin "kotro-proxy-x86_64-pc-windows-msvc.exe"

cd "$EXT_DIR"
npm ci
npm run compile
npx @vscode/vsce package --no-dependencies -o "${ROOT}/kotro-proxy-engine.vsix"

echo ""
echo "Built: ${ROOT}/kotro-proxy-engine.vsix"
echo ""
echo "Upload manually (no VSCE_PAT required):"
echo "  https://marketplace.visualstudio.com/manage/publishers/kotrolabs"
echo "  → Upload extension → drag kotro-proxy-engine.vsix"
