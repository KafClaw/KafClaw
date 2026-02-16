#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# bootstrap-ubuntu.sh — Install KafClaw build prerequisites on Ubuntu/Debian
#
# Works on: Ubuntu 18.04+ (Jetson Nano L4T), Ubuntu 20.04+, Debian 10+
# Architectures: amd64, arm64
# =============================================================================

GO_VERSION="1.24.13"
GO_MIN="1.24"

echo ""
echo "KafClaw — Ubuntu/Debian Bootstrap"
echo "=================================="
echo ""

# ---------------------------------------------------------------------------
# Detect architecture
# ---------------------------------------------------------------------------
ARCH=$(uname -m)
case "$ARCH" in
    aarch64|arm64) GOARCH="arm64" ;;
    x86_64)        GOARCH="amd64" ;;
    armv7l)        GOARCH="armv6l" ;;
    *)             echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
echo "  Platform: $(uname -s) ${ARCH} (Go arch: ${GOARCH})"

# ---------------------------------------------------------------------------
# System packages (git, make, build-essential)
# ---------------------------------------------------------------------------
echo ""
echo "Checking system packages..."

NEED_INSTALL=()
for pkg in git make; do
    if command -v "$pkg" >/dev/null 2>&1; then
        echo "  $pkg — OK"
    else
        NEED_INSTALL+=("$pkg")
    fi
done

if [ ${#NEED_INSTALL[@]} -gt 0 ]; then
    echo "  Installing: ${NEED_INSTALL[*]}"
    sudo apt-get update -qq
    sudo apt-get install -y -qq "${NEED_INSTALL[@]}"
fi

# ---------------------------------------------------------------------------
# Go — install or upgrade from official tarball
# ---------------------------------------------------------------------------
echo ""
echo "Checking Go..."

install_go() {
    TARBALL="go${GO_VERSION}.linux-${GOARCH}.tar.gz"
    URL="https://go.dev/dl/${TARBALL}"

    echo "  Downloading ${URL}..."
    wget -q "$URL" -O "/tmp/${TARBALL}"

    echo "  Installing to /usr/local/go..."
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/${TARBALL}"
    rm "/tmp/${TARBALL}"

    # Ensure PATH is set
    GO_PATH_LINE='export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH'
    SHELL_RC="$HOME/.bashrc"
    if ! grep -qF '/usr/local/go/bin' "$SHELL_RC" 2>/dev/null; then
        echo "" >> "$SHELL_RC"
        echo "# Go (added by KafClaw bootstrap)" >> "$SHELL_RC"
        echo "$GO_PATH_LINE" >> "$SHELL_RC"
        echo "  Added Go to PATH in ${SHELL_RC}"
    fi

    export PATH=/usr/local/go/bin:$HOME/go/bin:$PATH
}

# Remove old system Go if present (Ubuntu 18.04 ships Go 1.10)
if dpkg -l golang-go >/dev/null 2>&1 || dpkg -l 'golang-1.*-go' >/dev/null 2>&1; then
    SYS_GO_VER=$(go version 2>/dev/null | sed -E 's/.*go([0-9]+\.[0-9]+).*/\1/' || echo "0.0")
    SYS_MAJ=$(echo "$SYS_GO_VER" | cut -d. -f1)
    SYS_MIN=$(echo "$SYS_GO_VER" | cut -d. -f2)
    REQ_MAJ=$(echo "$GO_MIN" | cut -d. -f1)
    REQ_MIN=$(echo "$GO_MIN" | cut -d. -f2)

    if [ "$SYS_MAJ" -lt "$REQ_MAJ" ] || { [ "$SYS_MAJ" -eq "$REQ_MAJ" ] && [ "$SYS_MIN" -lt "$REQ_MIN" ]; }; then
        echo "  System Go ${SYS_GO_VER} is too old (need >= ${GO_MIN}), removing..."
        sudo apt-get remove -y -qq golang-go golang-1.*-go 2>/dev/null || true
    fi
fi

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
        echo "  Go — OK ($(go version | sed -E 's/.*go([0-9]+\.[0-9]+\.[0-9]+).*/\1/'))"
    fi
else
    echo "  Go not found, installing ${GO_VERSION}..."
    install_go
fi

# Verify
echo "  Installed: $(go version)"

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
echo "NOTE: If Go was just installed, you may need to run:"
echo "  source ~/.bashrc"
echo "before the 'go' command is available in this shell."
echo ""
