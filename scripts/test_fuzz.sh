#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

FUZZ_TIME="${FUZZ_TIME:-8s}"

echo "==> fuzz: shell guard traversal and destructive pattern checks"
go test -run=^$ -fuzz=FuzzGuardCommand_NoPanicAndTraversalBlocked -fuzztime="$FUZZ_TIME" ./internal/tools

echo ""
echo "==> fuzz: shell strict allow-list enforcement"
go test -run=^$ -fuzz=FuzzGuardCommand_StrictAllowList -fuzztime="$FUZZ_TIME" ./internal/tools

echo ""
echo "Fuzz suite passed."
