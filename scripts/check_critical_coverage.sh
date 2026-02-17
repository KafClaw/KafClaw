#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

check_pkg_100() {
  local pkg="$1"
  local out
  out="$(go test -count=1 -cover "$pkg")"
  echo "$out"
  local pct
  pct="$(echo "$out" | sed -n 's/.*coverage: \([0-9.]*\)% of statements.*/\1/p' | tail -n 1)"
  if [[ -z "$pct" ]]; then
    echo "ERROR: failed to parse coverage for $pkg" >&2
    exit 1
  fi
  if [[ "$pct" != "100.0" ]]; then
    echo "ERROR: critical package $pkg coverage is $pct%, expected 100.0%" >&2
    exit 1
  fi
}

echo "==> critical package coverage (must be 100%)"
check_pkg_100 ./internal/policy
check_pkg_100 ./internal/approval
check_pkg_100 ./internal/bus
check_pkg_100 ./internal/session

echo ""
echo "==> critical function coverage (must be 100%)"
go test -count=1 -coverprofile=/tmp/tools_critical.cover.out ./internal/tools >/tmp/tools_critical.cover.log
cat /tmp/tools_critical.cover.log
func_line="$(go tool cover -func=/tmp/tools_critical.cover.out | grep 'internal/tools/shell.go:.*guardCommand' || true)"
if [[ -z "$func_line" ]]; then
  echo "ERROR: could not find guardCommand coverage line" >&2
  exit 1
fi
func_pct="$(echo "$func_line" | awk '{print $3}' | tr -d '%')"
if [[ "$func_pct" != "100.0" ]]; then
  echo "ERROR: guardCommand coverage is $func_pct%, expected 100.0%" >&2
  exit 1
fi

echo ""
echo "==> gateway critical function coverage"
go test -count=1 -coverprofile=/tmp/cli_critical.cover.out ./internal/cli >/tmp/cli_critical.cover.log
cat /tmp/cli_critical.cover.log

check_gateway_func_100() {
  local fn="$1"
  local pct
  pct="$(go tool cover -func=/tmp/cli_critical.cover.out | awk -v fn="$fn" '$1 ~ /internal\/cli\/gateway.go:/ && $2==fn {gsub("%","",$3); print $3}' | head -n 1)"
  if [[ -z "$pct" ]]; then
    echo "ERROR: could not find gateway function coverage for $fn" >&2
    exit 1
  fi
  if [[ "$pct" != "100.0" ]]; then
    echo "ERROR: gateway critical function $fn coverage is $pct%, expected 100.0%" >&2
    exit 1
  fi
}

check_gateway_func_100 newTraceID
check_gateway_func_100 normalizeWhatsAppJID
check_gateway_func_100 listRepoTree
check_gateway_func_100 runGit
check_gateway_func_100 runGh
check_gateway_func_100 orchDiscoveryHandler
check_gateway_func_100 Manager
check_gateway_func_100 Consumer
check_gateway_func_100 SetManager
check_gateway_func_100 SetConsumer
check_gateway_func_100 Clear
check_gateway_func_100 Active
check_gateway_func_100 maskSecret
check_gateway_func_100 PublishTrace
check_gateway_func_100 PublishAudit
check_gateway_func_100 inferTopicCategory

run_gateway_pct="$(go tool cover -func=/tmp/cli_critical.cover.out | awk '$1 ~ /internal\/cli\/gateway.go:/ && $2=="runGateway" {gsub("%","",$3); print $3}' | head -n 1)"
if [[ -z "$run_gateway_pct" ]]; then
  echo "ERROR: could not find runGateway coverage line" >&2
  exit 1
fi
# runGateway is a very large integration function with many network/provider branches.
# We enforce a non-trivial floor to guarantee smoke tests execute core runtime paths.
awk -v pct="$run_gateway_pct" 'BEGIN { if (pct+0 < 55.0) exit 1 }' || {
  echo "ERROR: runGateway coverage is $run_gateway_pct%, expected >=55.0%" >&2
  exit 1
}

echo "Critical coverage gate passed."
