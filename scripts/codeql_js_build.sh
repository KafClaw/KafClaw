#!/usr/bin/env bash
set -euo pipefail

# Prefer Homebrew Node/npm in non-interactive bash sessions on macOS.
if [ -d /opt/homebrew/bin ]; then
  export PATH="/opt/homebrew/bin:$PATH"
fi

install_node_deps() {
  if [ -f package-lock.json ] || [ -f npm-shrinkwrap.json ]; then
    npm ci
  else
    npm install
  fi
}

if [ -f electron/package.json ]; then
  (cd electron && install_node_deps && npm run build)
fi

if [ -f web/package.json ]; then
  (cd web && install_node_deps && npm run build)
fi
