#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

FUZZ_TIME="${FUZZ_TIME:-8s}"

run_fuzz() {
  local label="$1"
  local target="$2"
  echo "==> fuzz: ${label}"
  if go test -run=^$ -fuzz="${target}" -fuzztime="${FUZZ_TIME}" ./internal/tools; then
    return 0
  fi
  echo "fuzz target ${target} failed once; retrying..."
  go test -run=^$ -fuzz="${target}" -fuzztime="${FUZZ_TIME}" ./internal/tools
}

run_fuzz "shell guard traversal and destructive pattern checks" "FuzzGuardCommand_NoPanicAndTraversalBlocked"

echo ""
run_fuzz "shell strict allow-list enforcement" "FuzzGuardCommand_StrictAllowList"

echo ""
echo "Fuzz suite passed."
