#!/usr/bin/env bash
set -euo pipefail

# KafClaw installer for GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/KafClaw/KafClaw/main/scripts/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/KafClaw/KafClaw/main/scripts/install.sh | bash -s -- --version v2.6.3 --yes

REPO="kafclaw/kafclaw"
BINARY="kafclaw"
VERSION=""
LATEST_REQUESTED=0
LIST_RELEASES=0
INSTALL_DIR=""
WITH_COMPLETION=1
SETUP_SERVICE=1
ASSUME_YES=0
UNATTENDED=0
VERIFY_SIGNATURE=1
SERVICE_USER="kafclaw"
SHELL_RELOAD_HINT=""

log() {
  printf '[kafclaw-install] %s\n' "$*"
}

warn() {
  printf '[kafclaw-install] warning: %s\n' "$*" >&2
}

fail() {
  local code="$1"
  local message="$2"
  local remediation="${3:-check the logs above and rerun the installer}"
  printf '[kafclaw-install] error [%s]: %s. remediation: %s\n' "$code" "$message" "$remediation" >&2
  exit 1
}

die() {
  fail "INSTALL_FAILED" "$*" "check the logs above and rerun the installer"
}

require_cmd() {
  local name="$1"
  local remediation="${2:-install '${name}' and rerun the installer}"
  command -v "$name" >/dev/null 2>&1 || fail "INSTALL_PREREQ_MISSING" "missing required command: ${name}" "${remediation}"
}

usage() {
  cat <<'EOF'
KafClaw installer

Options:
  --version <tag>          Install a specific release tag (for example: v2.6.3)
  --latest                 Install the latest release
  --list-releases          Print latest + recent release versions and exit
  --install-dir <dir>      Installation directory (default: /usr/local/bin for root, ~/.local/bin otherwise)
  --service-user <name>    Service user to create when running as root on Linux (default: kafclaw)
  --verify-signature       Verify release signature with cosign (default)
  --no-signature-verify    Skip cosign verification (not recommended)
  --with-completion        Install shell completion for detected shell (default)
  --no-completion          Skip shell completion install
  --setup-service          Print startup/service guidance for detected platform (default)
  --no-service             Skip startup/service guidance
  --yes, -y                Non-interactive; accept default prompts
  --unattended             Headless install mode (non-interactive)
  -h, --help               Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      [[ $# -ge 2 ]] || die "--version requires a value"
      VERSION="$2"
      shift 2
      ;;
    --latest)
      LATEST_REQUESTED=1
      shift
      ;;
    --list-releases)
      LIST_RELEASES=1
      shift
      ;;
    --install-dir)
      [[ $# -ge 2 ]] || die "--install-dir requires a value"
      INSTALL_DIR="$2"
      shift 2
      ;;
    --service-user)
      [[ $# -ge 2 ]] || die "--service-user requires a value"
      SERVICE_USER="$2"
      shift 2
      ;;
    --verify-signature)
      VERIFY_SIGNATURE=1
      shift
      ;;
    --no-signature-verify)
      VERIFY_SIGNATURE=0
      shift
      ;;
    --with-completion)
      WITH_COMPLETION=1
      shift
      ;;
    --no-completion)
      WITH_COMPLETION=0
      shift
      ;;
    --setup-service)
      SETUP_SERVICE=1
      shift
      ;;
    --no-service)
      SETUP_SERVICE=0
      shift
      ;;
    --yes|-y)
      ASSUME_YES=1
      shift
      ;;
    --unattended)
      UNATTENDED=1
      ASSUME_YES=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

if [[ -n "$VERSION" && "$LATEST_REQUESTED" -eq 1 ]]; then
  fail "INSTALL_ARGS_INVALID" "use either --version <tag> or --latest, not both" "rerun with a single release selector"
fi
if [[ "$UNATTENDED" -eq 1 && -z "$VERSION" && "$LATEST_REQUESTED" -eq 0 ]]; then
  fail "INSTALL_ARGS_INVALID" "unattended mode requires explicit release selection" "pass --latest or --version <tag>"
fi

OS_RAW="$(uname -s)"
OS="$(echo "$OS_RAW" | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux|darwin) ;;
  msys*|mingw*|cygwin*)
    fail "INSTALL_OS_UNSUPPORTED" "Windows detected in POSIX shell installer" "use the Windows .exe release artifact or a PowerShell installer path"
    ;;
  *)
    fail "INSTALL_OS_UNSUPPORTED" "unsupported OS: $OS_RAW" "use Linux or macOS release artifacts"
    ;;
esac

ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    fail "INSTALL_ARCH_UNSUPPORTED" "unsupported architecture: $ARCH_RAW" "use amd64 or arm64 host/release artifacts"
    ;;
esac

PLATFORM_FLAVOR="generic"
if [[ "$OS" == "linux" ]]; then
  MODEL=""
  if [[ -r /proc/device-tree/model ]]; then
    MODEL="$(tr -d '\000' </proc/device-tree/model || true)"
  elif [[ -r /sys/firmware/devicetree/base/model ]]; then
    MODEL="$(tr -d '\000' </sys/firmware/devicetree/base/model || true)"
  fi
  MODEL_LC="$(echo "$MODEL" | tr '[:upper:]' '[:lower:]')"
  if [[ "$MODEL_LC" == *jetson* ]]; then
    PLATFORM_FLAVOR="jetson"
  elif [[ "$MODEL_LC" == *raspberry* ]]; then
    PLATFORM_FLAVOR="raspberry-pi"
  fi
fi

if [[ -z "$INSTALL_DIR" ]]; then
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="${HOME}/.local/bin"
  fi
fi

preflight_install_prereqs() {
  require_cmd uname "install coreutils/OS base utilities and rerun"
  require_cmd curl "install curl (for release download) and rerun"
  require_cmd awk "install awk and rerun"
  require_cmd sed "install sed and rerun"
  require_cmd grep "install grep and rerun"
  require_cmd mktemp "install mktemp and rerun"
  require_cmd install "install coreutils 'install' command and rerun"
  require_cmd chmod "install coreutils chmod and rerun"
  require_cmd id "install coreutils id and rerun"
  if [[ "$VERIFY_SIGNATURE" -eq 1 ]]; then
    require_cmd cosign "install cosign, or rerun with --no-signature-verify if policy allows"
  fi
}

preflight_install_prereqs

detect_latest_version() {
  curl --fail --show-error --silent --location --retry 3 --connect-timeout 10 -I "https://github.com/${REPO}/releases/latest" \
    | awk -F'/' 'tolower($1) ~ /^location:/ {gsub(/\r/, "", $NF); print $NF; exit}'
}

list_releases() {
  local releases_api="https://api.github.com/repos/${REPO}/releases?per_page=20"
  local payload tags latest
  payload="$(curl --fail --show-error --silent --location --retry 3 --connect-timeout 10 "${releases_api}")"
  tags="$(printf '%s\n' "$payload" | grep -Eo '"tag_name":[[:space:]]*"[^"]+"' | sed -E 's/.*"([^"]+)".*/\1/' || true)"
  [[ -n "$tags" ]] || fail "INSTALL_RELEASE_LIST_FAILED" "failed to parse releases list from ${releases_api}" "check GitHub API/network access and retry"
  latest="$(printf '%s\n' "$tags" | head -n1)"
  echo "Latest: ${latest#v}"
  echo "Recent releases:"
  printf '%s\n' "$tags" | sed 's/^v//' | sed 's/^/  - /'
}

if [[ "$LIST_RELEASES" -eq 1 ]]; then
  list_releases
  exit 0
fi

if [[ "$LATEST_REQUESTED" -eq 1 || ( -z "$VERSION" && "$UNATTENDED" -eq 0 ) ]]; then
  log "detecting latest release version"
  VERSION="$(detect_latest_version)"
  [[ -n "$VERSION" ]] || fail "INSTALL_LATEST_DETECT_FAILED" "failed to detect latest release version" "check GitHub connectivity and retry"
fi

if [[ "$VERSION" != v* ]]; then
  VERSION="v${VERSION}"
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}-${OS}-${ARCH}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/SHA256SUMS"
SIG_URL="${DOWNLOAD_URL}.sig"
PEM_URL="${DOWNLOAD_URL}.pem"
TMP_DIR="$(mktemp -d)"
TMP_BIN="${TMP_DIR}/${BINARY}-${OS}-${ARCH}"
TMP_SUMS="${TMP_DIR}/SHA256SUMS"
TMP_SIG="${TMP_DIR}/$(basename "${TMP_BIN}").sig"
TMP_PEM="${TMP_DIR}/$(basename "${TMP_BIN}").pem"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

log "install target: ${OS}/${ARCH} (${PLATFORM_FLAVOR})"
if [[ "$UNATTENDED" -eq 1 ]]; then
  log "running in unattended mode"
fi
log "downloading ${DOWNLOAD_URL}"
curl --fail --show-error --silent --location --retry 3 --connect-timeout 10 -o "$TMP_BIN" "$DOWNLOAD_URL" || fail "INSTALL_DOWNLOAD_FAILED" "failed to download ${DOWNLOAD_URL}" "verify release/version/platform and network access"

log "downloading checksums"
curl --fail --show-error --silent --location --retry 3 --connect-timeout 10 -o "$TMP_SUMS" "$CHECKSUMS_URL" || fail "INSTALL_CHECKSUM_FETCH_FAILED" "failed to download ${CHECKSUMS_URL}" "verify release assets and network access"

EXPECTED_SHA="$(awk -v f="$(basename "$TMP_BIN")" '$2 == f {print $1; exit}' "$TMP_SUMS")"
[[ -n "$EXPECTED_SHA" ]] || fail "INSTALL_CHECKSUM_MISSING" "checksum for $(basename "$TMP_BIN") not found in SHA256SUMS" "use a valid release/version for this platform"

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL_SHA="$(sha256sum "$TMP_BIN" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL_SHA="$(shasum -a 256 "$TMP_BIN" | awk '{print $1}')"
else
  fail "INSTALL_CHECKSUM_TOOL_MISSING" "missing checksum tool (need sha256sum or shasum)" "install coreutils/OpenSSL checksum utilities and rerun"
fi

[[ "$ACTUAL_SHA" == "$EXPECTED_SHA" ]] || fail "INSTALL_CHECKSUM_MISMATCH" "checksum verification failed for $(basename "$TMP_BIN")" "do not use this artifact; retry download or choose a different release"

if [[ "$VERIFY_SIGNATURE" -eq 1 ]]; then
  log "downloading signature and certificate"
  curl --fail --show-error --silent --location --retry 3 --connect-timeout 10 -o "$TMP_SIG" "$SIG_URL" || fail "INSTALL_SIGNATURE_FETCH_FAILED" "failed to download ${SIG_URL}" "verify release signature asset availability and retry"
  curl --fail --show-error --silent --location --retry 3 --connect-timeout 10 -o "$TMP_PEM" "$PEM_URL" || fail "INSTALL_CERT_FETCH_FAILED" "failed to download ${PEM_URL}" "verify release certificate asset availability and retry"
  log "verifying signature with cosign"
  cosign verify-blob \
    --certificate "$TMP_PEM" \
    --signature "$TMP_SIG" \
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
    --certificate-identity-regexp '^https://github\.com/[Kk]af[Cc]law/[Kk]af[Cc]law/\.github/workflows/release\.yml@refs/tags/.*$' \
    "$TMP_BIN" >/dev/null || fail "INSTALL_SIGNATURE_VERIFY_FAILED" "cosign signature verification failed for $(basename "$TMP_BIN")" "confirm release provenance and retry; or use --no-signature-verify only if policy allows"
fi

chmod +x "$TMP_BIN"

mkdir -p "$INSTALL_DIR"
TARGET_BIN="${INSTALL_DIR}/${BINARY}"
install -m 0755 "$TMP_BIN" "$TARGET_BIN"
log "installed ${BINARY} to ${TARGET_BIN}"

create_service_user_linux() {
  local name="$1"
  if id -u "$name" >/dev/null 2>&1; then
    log "service user already exists: ${name}"
    return 0
  fi
  if command -v useradd >/dev/null 2>&1; then
    useradd --create-home --shell /bin/bash "$name"
    log "created service user: ${name}"
    return 0
  fi
  if command -v adduser >/dev/null 2>&1; then
    adduser --disabled-password --gecos "" "$name"
    log "created service user: ${name}"
    return 0
  fi
  fail "INSTALL_SERVICE_USER_CREATE_FAILED" "cannot create user ${name}: neither useradd nor adduser is available" "install user management tools or rerun with root runtime accepted"
}

run_as_user() {
  local user="$1"
  shift
  if command -v sudo >/dev/null 2>&1; then
    sudo -u "$user" "$@"
    return
  fi
  if command -v su >/dev/null 2>&1; then
    su - "$user" -c "$(printf "%q " "$@")"
    return
  fi
  fail "INSTALL_RUNAS_UNAVAILABLE" "cannot run command as ${user}: missing sudo/su" "install sudo/su utilities or run completion setup manually"
}

resolve_home_for_user() {
  local user="$1"
  if command -v getent >/dev/null 2>&1; then
    getent passwd "$user" | awk -F: '{print $6; exit}'
    return
  fi
  if [[ "$OS" == "darwin" ]] && command -v dscl >/dev/null 2>&1; then
    dscl . -read "/Users/${user}" NFSHomeDirectory 2>/dev/null | awk '{print $2; exit}'
    return
  fi
}

prompt_yes_no_default_yes() {
  local prompt="$1"
  if [[ "$ASSUME_YES" -eq 1 ]]; then
    return 0
  fi
  read -r -p "${prompt} [Y/n] " ans
  ans="${ans:-Y}"
  case "$ans" in
    y|Y|yes|YES) return 0 ;;
    n|N|no|NO) return 1 ;;
    *) return 0 ;;
  esac
}

SERVICE_RUNTIME_USER=""
if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
  if prompt_yes_no_default_yes "Installing KafClaw as root is a security risk. Create a non-root user '${SERVICE_USER}' for this install?"; then
    if [[ "$OS" == "linux" ]]; then
      create_service_user_linux "$SERVICE_USER"
      SERVICE_RUNTIME_USER="$SERVICE_USER"
    else
      warn "automatic user creation is currently supported on Linux only; continuing without creating a new user"
    fi
  else
    warn "Installing as root service."
    SERVICE_RUNTIME_USER="root"
  fi
fi

ensure_line() {
  local line="$1"
  local file="$2"
  mkdir -p "$(dirname "$file")"
  touch "$file"
  if ! grep -Fqx "$line" "$file"; then
    printf '%s\n' "$line" >>"$file"
  fi
}

install_completion_for_shell() {
  local user_name="$1"
  local home_dir="$2"
  local shell_name="$3"
  local shell_base
  shell_base="$(basename "$shell_name")"

  case "$shell_base" in
    zsh)
      local comp_dir="${home_dir}/.zsh/completions"
      local comp_file="${comp_dir}/_kafclaw"
      mkdir -p "$comp_dir"
      if [[ "${EUID:-$(id -u)}" -eq 0 && "$user_name" != "root" ]]; then
        run_as_user "$user_name" "$TARGET_BIN" completion zsh >"$comp_file"
      else
        "$TARGET_BIN" completion zsh >"$comp_file"
      fi
      local zshrc="${home_dir}/.zshrc"
      ensure_line 'fpath=("$HOME/.zsh/completions" $fpath)' "$zshrc"
      ensure_line 'autoload -Uz compinit && compinit' "$zshrc"
      if [[ "$INSTALL_DIR" != "/usr/local/bin" ]]; then
        ensure_line 'export PATH="$HOME/.local/bin:$PATH"' "$zshrc"
      fi
      SHELL_RELOAD_HINT="source ${zshrc}"
      log "installed zsh completion at ${comp_file}"
      ;;
    bash)
      local comp_dir="${home_dir}/.local/share/bash-completion/completions"
      local comp_file="${comp_dir}/kafclaw"
      mkdir -p "$comp_dir"
      if [[ "${EUID:-$(id -u)}" -eq 0 && "$user_name" != "root" ]]; then
        run_as_user "$user_name" "$TARGET_BIN" completion bash >"$comp_file"
      else
        "$TARGET_BIN" completion bash >"$comp_file"
      fi
      local bashrc="${home_dir}/.bashrc"
      if [[ "$INSTALL_DIR" != "/usr/local/bin" ]]; then
        ensure_line 'export PATH="$HOME/.local/bin:$PATH"' "$bashrc"
      fi
      SHELL_RELOAD_HINT="source ${bashrc}"
      log "installed bash completion at ${comp_file}"
      ;;
    fish)
      local comp_dir="${home_dir}/.config/fish/completions"
      local comp_file="${comp_dir}/kafclaw.fish"
      mkdir -p "$comp_dir"
      if [[ "${EUID:-$(id -u)}" -eq 0 && "$user_name" != "root" ]]; then
        run_as_user "$user_name" "$TARGET_BIN" completion fish >"$comp_file"
      else
        "$TARGET_BIN" completion fish >"$comp_file"
      fi
      local fish_cfg="${home_dir}/.config/fish/config.fish"
      if [[ "$INSTALL_DIR" != "/usr/local/bin" ]]; then
        ensure_line 'fish_add_path -m $HOME/.local/bin' "$fish_cfg"
      fi
      SHELL_RELOAD_HINT="source ${fish_cfg}"
      log "installed fish completion at ${comp_file}"
      ;;
    *)
      warn "shell '${shell_base}' is not auto-configured for completion; run '${TARGET_BIN} completion <shell>' manually"
      ;;
  esac
}

if [[ "$WITH_COMPLETION" -eq 1 ]]; then
  INSTALL_USER="${USER:-}"
  INSTALL_HOME="${HOME:-}"
  INSTALL_SHELL="${SHELL:-}"
  if [[ "${EUID:-$(id -u)}" -eq 0 && -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" ]]; then
    INSTALL_USER="${SUDO_USER}"
    INSTALL_HOME="$(resolve_home_for_user "$INSTALL_USER")"
  fi
  if [[ -z "$INSTALL_USER" || -z "$INSTALL_HOME" || -z "$INSTALL_SHELL" ]]; then
    warn "cannot resolve target user/home/shell for completion install; skipping"
  else
    install_completion_for_shell "$INSTALL_USER" "$INSTALL_HOME" "$INSTALL_SHELL"
  fi
fi

install_verify() {
  local version_output
  local status_target_home="${INSTALL_HOME:-${HOME:-}}"

  log "verification"
  if ! version_output="$("$TARGET_BIN" version 2>/dev/null)"; then
    if ! version_output="$("$TARGET_BIN" --version 2>/dev/null)"; then
      fail "INSTALL_VERIFY_VERSION_FAILED" "installed binary failed version check" "rerun installer and confirm executable permissions at ${TARGET_BIN}"
    fi
  fi
  echo "  - version check: ok (${version_output})"

  if command -v "$BINARY" >/dev/null 2>&1; then
    echo "  - PATH check: ok ($(command -v "$BINARY"))"
  else
    warn "PATH check: '${BINARY}' is not currently discoverable in PATH"
  fi

  if [[ -n "$status_target_home" && -f "${status_target_home}/.kafclaw/config.json" ]]; then
    if "$TARGET_BIN" status >/dev/null 2>&1; then
      echo "  - status check: ok"
    else
      warn "status check failed; run '${TARGET_BIN} doctor' for diagnostics"
    fi
  else
    echo "  - status check: skipped (no config yet; run '${TARGET_BIN} onboard' first)"
  fi
}

if [[ "$SETUP_SERVICE" -eq 1 ]]; then
  echo
  log "startup guidance"
  case "$OS" in
    linux)
      if command -v systemctl >/dev/null 2>&1; then
        echo "Detected Linux with systemd."
        if [[ -n "$SERVICE_RUNTIME_USER" ]]; then
          echo "Recommended runtime user: ${SERVICE_RUNTIME_USER}"
        else
          echo "Recommended runtime user: non-root account"
        fi
        echo "Next steps:"
        echo "  1) ${TARGET_BIN} onboard"
        echo "  2) sudo ${TARGET_BIN} onboard --systemd --service-user ${SERVICE_RUNTIME_USER:-${SERVICE_USER}} --service-binary ${TARGET_BIN}"
        echo "  3) sudo systemctl daemon-reload && sudo systemctl enable --now kafclaw-gateway.service"
      else
        echo "Linux detected without systemd. Run gateway under your process supervisor (supervisord/runit/s6)."
      fi
      ;;
    darwin)
      echo "Detected macOS (launchd)."
      echo "Next steps:"
      echo "  1) ${TARGET_BIN} onboard"
      echo "  2) Create ~/Library/LaunchAgents/io.kafclaw.gateway.plist for '${TARGET_BIN} gateway'"
      echo "  3) load with: launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/io.kafclaw.gateway.plist"
      ;;
  esac
fi

echo
install_verify
if [[ -n "$SHELL_RELOAD_HINT" ]]; then
  echo "Reload your shell to enable completion and PATH updates:"
  echo "  ${SHELL_RELOAD_HINT}"
  echo "Then run:"
  echo "  kafclaw --help"
elif [[ "$INSTALL_DIR" != "/usr/local/bin" ]]; then
  echo "If needed, add to PATH:"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
fi
