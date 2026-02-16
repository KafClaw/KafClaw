#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# bootstrap-macos.sh — Install KafClaw build prerequisites on macOS
# =============================================================================

GO_VERSION="1.24.13"
GO_MIN="1.24"

echo ""
echo "KafClaw — macOS Bootstrap"
echo "========================="
echo ""

# ---------------------------------------------------------------------------
# Xcode Command Line Tools (provides git, make)
# ---------------------------------------------------------------------------
if ! xcode-select -p >/dev/null 2>&1; then
    echo "Installing Xcode Command Line Tools..."
    xcode-select --install
    echo "  Please complete the Xcode CLT installation dialog, then re-run this script."
    exit 0
else
    echo "  Xcode CLT — OK"
fi

# ---------------------------------------------------------------------------
# Homebrew (optional but recommended)
# ---------------------------------------------------------------------------
if command -v brew >/dev/null 2>&1; then
    echo "  Homebrew  — OK ($( brew --version | head -1 ))"
    HAS_BREW=true
else
    echo "  Homebrew  — not installed (optional, skipping)"
    HAS_BREW=false
fi

# ---------------------------------------------------------------------------
# Go
# ---------------------------------------------------------------------------
install_go() {
    echo ""
    echo "Installing Go ${GO_VERSION}..."

    ARCH=$(uname -m)
    case "$ARCH" in
        arm64|aarch64) GOARCH="arm64" ;;
        x86_64)        GOARCH="amd64" ;;
        *)             echo "Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    TARBALL="go${GO_VERSION}.darwin-${GOARCH}.tar.gz"
    URL="https://go.dev/dl/${TARBALL}"

    echo "  Downloading ${URL}..."
    curl -sLO "$URL"

    echo "  Installing to /usr/local/go..."
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "$TARBALL"
    rm "$TARBALL"

    # Ensure PATH is set
    GO_PATH_LINE='export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH'
    SHELL_RC="$HOME/.zshrc"
    if ! grep -qF '/usr/local/go/bin' "$SHELL_RC" 2>/dev/null; then
        echo "" >> "$SHELL_RC"
        echo "# Go (added by KafClaw bootstrap)" >> "$SHELL_RC"
        echo "$GO_PATH_LINE" >> "$SHELL_RC"
        echo "  Added Go to PATH in ${SHELL_RC}"
    fi

    export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH
}

if command -v go >/dev/null 2>&1; then
    GO_VER=$(go version | sed -E 's/.*go([0-9]+\.[0-9]+).*/\1/')
    GO_MAJ=$(echo "$GO_VER" | cut -d. -f1)
    GO_MIN_CUR=$(echo "$GO_VER" | cut -d. -f2)
    REQ_MAJ=$(echo "$GO_MIN" | cut -d. -f1)
    REQ_MIN=$(echo "$GO_MIN" | cut -d. -f2)

    if [ "$GO_MAJ" -lt "$REQ_MAJ" ] || { [ "$GO_MAJ" -eq "$REQ_MAJ" ] && [ "$GO_MIN_CUR" -lt "$REQ_MIN" ]; }; then
        echo "  Go ${GO_VER} found but too old (need >= ${GO_MIN})"
        install_go
    else
        echo "  Go         — OK ($(go version | sed -E 's/.*go([0-9]+\.[0-9]+\.[0-9]+).*/\1/'))"
    fi
else
    install_go
fi

# ---------------------------------------------------------------------------
# Git
# ---------------------------------------------------------------------------
if command -v git >/dev/null 2>&1; then
    echo "  Git        — OK ($(git --version | sed 's/git version //'))"
else
    echo "  Git not found — install Xcode CLT or run: brew install git"
    exit 1
fi

# ---------------------------------------------------------------------------
# Node.js (optional — for Electron)
# ---------------------------------------------------------------------------
if command -v node >/dev/null 2>&1; then
    echo "  Node.js    — OK ($(node --version))"
else
    echo "  Node.js    — not installed (optional, needed for Electron desktop app)"
    if $HAS_BREW; then
        echo "               Install: brew install node"
    fi
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "All required prerequisites are installed."
echo ""
echo "Next steps:"
echo "  cd gomikrobot"
echo "  make check     # verify prerequisites"
echo "  make build     # compile"
echo "  make run       # start gateway"
echo ""
