#!/usr/bin/env bash
set -euo pipefail

# GoMikroBot installer — downloads the correct binary from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/KafClaw/KafClaw/main/gomikrobot/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/KafClaw/KafClaw/main/gomikrobot/scripts/install.sh | bash -s -- v2.6.0

REPO="KafClaw/KafClaw"
BINARY="gomikrobot"
INSTALL_DIR="/usr/local/bin"

# --- Detect OS ---
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  darwin) ;;
  linux)  ;;
  *)
    echo "Unsupported OS: $OS" >&2
    echo "GoMikroBot supports macOS (darwin) and Linux." >&2
    exit 1
    ;;
esac

# --- Detect Architecture ---
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  amd64)   ARCH="amd64" ;;
  arm64)   ARCH="arm64" ;;
  aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# --- Determine version ---
VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  echo "Detecting latest version..."
  VERSION="$(curl -sI "https://github.com/${REPO}/releases/latest" \
    | grep -i '^location:' \
    | sed -E 's|.*/tag/([^ ]+).*|\1|' \
    | tr -d '\r')"
  if [[ -z "$VERSION" ]]; then
    echo "Failed to detect latest version." >&2
    echo "Specify a version manually: bash install.sh v2.6.0" >&2
    exit 1
  fi
fi

# Strip leading 'v' for display, keep for URL
VERSION_DISPLAY="${VERSION#v}"
echo "Installing gomikrobot ${VERSION_DISPLAY} (${OS}/${ARCH})..."

# --- Download ---
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}-${OS}-${ARCH}"
TMP_FILE="$(mktemp)"
trap 'rm -f "$TMP_FILE"' EXIT

HTTP_CODE="$(curl -fSL -o "$TMP_FILE" -w "%{http_code}" "$DOWNLOAD_URL" 2>/dev/null)" || true
if [[ "$HTTP_CODE" != "200" ]]; then
  echo "Download failed (HTTP ${HTTP_CODE})." >&2
  echo "URL: ${DOWNLOAD_URL}" >&2
  echo "Check that the version and platform are correct." >&2
  exit 1
fi

chmod +x "$TMP_FILE"

# --- Install ---
if cp "$TMP_FILE" "${INSTALL_DIR}/${BINARY}" 2>/dev/null; then
  echo "Installed to ${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} requires sudo..."
  sudo cp "$TMP_FILE" "${INSTALL_DIR}/${BINARY}"
  echo "Installed to ${INSTALL_DIR}/${BINARY}"
fi

# --- Verify ---
if command -v "$BINARY" &>/dev/null; then
  echo "Verification:"
  "$BINARY" version 2>/dev/null || "$BINARY" --version 2>/dev/null || echo "  ${BINARY} installed successfully"
else
  echo "Note: ${INSTALL_DIR} may not be in your PATH."
  echo "Run: export PATH=\"${INSTALL_DIR}:\$PATH\""
fi
