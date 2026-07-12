#!/usr/bin/env bash
set -e

# Kotro Proxy Installation Script
# This script detects the OS and Architecture and downloads the latest release from GitHub.

GITHUB_REPO="kotro-labs/kotro-proxy-engine"
BIN_DIR="/usr/local/bin"

echo "Installing Kotro Proxy Engine..."

# Detect OS
OS="$(uname -s)"
case "${OS}" in
    Linux*)     OS_TARGET="unknown-linux-gnu";;
    Darwin*)    OS_TARGET="apple-darwin";;
    *)          echo "Unsupported OS: ${OS}" && exit 1;;
esac

# Detect Architecture
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)     ARCH_TARGET="x86_64";;
    arm64|aarch64) ARCH_TARGET="aarch64";;
    *)          echo "Unsupported architecture: ${ARCH}" && exit 1;;
esac

TARGET="${ARCH_TARGET}-${OS_TARGET}"
ARTIFACT_NAME="kotro-proxy-${TARGET}"

# Fetch latest release data
echo "Fetching latest release information..."
LATEST_RELEASE_URL="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
# NOTE: The release asset name format is usually kotro-proxy-<target>.tar.gz
DOWNLOAD_URL=$(curl -sL "${LATEST_RELEASE_URL}" | grep "browser_download_url.*${ARTIFACT_NAME}.tar.gz" | cut -d : -f 2,3 | tr -d \" | xargs)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Could not find a release for ${TARGET}."
    echo "Please compile from source using 'cargo build --release'."
    exit 1
fi

# Download and extract
TMP_DIR=$(mktemp -d)
cd "${TMP_DIR}"

echo "Downloading ${ARTIFACT_NAME}..."
curl -sL "$DOWNLOAD_URL" -o "${ARTIFACT_NAME}.tar.gz"

echo "Extracting..."
tar -xzf "${ARTIFACT_NAME}.tar.gz"

echo "Installing to ${BIN_DIR} (requires sudo)..."
sudo mv "${ARTIFACT_NAME}" "${BIN_DIR}/kotro"
sudo chmod +x "${BIN_DIR}/kotro"

# Cleanup
rm -rf "${TMP_DIR}"

echo ""
echo "========================================="
echo "✅ Kotro Proxy Engine installed successfully!"
echo ""
echo "To start the proxy, simply run:"
echo "  $ kotro"
echo ""
echo "Then navigate to http://localhost:3000 to view the Dashboard!"
echo "========================================="
