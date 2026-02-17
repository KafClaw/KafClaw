#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v codeql >/dev/null 2>&1; then
  echo "ERROR: codeql CLI not found in PATH"
  echo "Install from: https://docs.github.com/en/code-security/codeql-cli/getting-started-with-the-codeql-cli"
  exit 1
fi

mkdir -p .tmp/codeql

echo "==> Ensuring CodeQL standard query packs are available"
codeql pack download codeql/go-queries codeql/javascript-queries

run_go() {
  echo "==> CodeQL (Go)"
  rm -rf .tmp/codeql/go-db
  codeql database create .tmp/codeql/go-db \
    --language=go \
    --command="go build ./..."

  codeql database analyze .tmp/codeql/go-db \
    codeql/go-queries:codeql-suites/go-security-and-quality.qls \
    --download \
    --format=sarifv2.1.0 \
    --output .tmp/codeql/go.sarif
}

run_js() {
  echo "==> CodeQL (JavaScript/TypeScript)"
  rm -rf .tmp/codeql/js-db
  chmod +x scripts/codeql_js_build.sh
  codeql database create .tmp/codeql/js-db \
    --language=javascript-typescript \
    --command="./scripts/codeql_js_build.sh"

  codeql database analyze .tmp/codeql/js-db \
    codeql/javascript-queries:codeql-suites/javascript-security-and-quality.qls \
    --download \
    --format=sarifv2.1.0 \
    --output .tmp/codeql/javascript.sarif
}

run_go
run_js

echo ""
echo "CodeQL local run complete."
echo "SARIF outputs:"
echo "  .tmp/codeql/go.sarif"
echo "  .tmp/codeql/javascript.sarif"
