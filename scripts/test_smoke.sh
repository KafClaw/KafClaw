#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

run() {
  local label="$1"
  shift
  echo ""
  echo "==> $label"
  "$@"
}

# Critical foundations: policy, approvals, message transport, session persistence.
run "core policy + approvals + transport" \
  go test -count=1 ./internal/policy ./internal/approval ./internal/bus ./internal/session

# Hard gate: critical logic must stay fully covered.
run "critical coverage gate" \
  bash scripts/check_critical_coverage.sh

# Agent core behaviors: message classification, approval interception, delivery reliability.
run "agent core flows" \
  go test -count=1 -run "TestInternalExternalClassificationE2E|TestPolicyTierGatingByMessageType|TestApprovalFlowApproved|TestApprovalFlowDenied|TestDeliveryWorkerPollPicksPendingTasks|TestDeliveryWorkerMaxRetryMarksFailed" ./internal/agent

# Tooling safety and execution reliability.
run "tool execution + safety" \
  go test -count=1 -run "TestExecTool_Basic|TestExecTool_Timeout|TestExecTool_DenyPatterns|TestExecTool_PathTraversal|TestRegistry" ./internal/tools

# Timeline persistence and task/audit access.
run "timeline persistence" \
  go test -count=1 -run "TestCreateAndGetTask|TestUpdateTaskStatus|TestListPendingDeliveries|TestTraceGraphCoverage" ./internal/timeline

# Gateway boot/shutdown and API smoke.
run "gateway boot + API smoke" \
  go test -count=1 -run "TestRunGatewayBootAndShutdown|TestRunGatewayServesDashboardEndpoints" ./internal/cli

echo ""
echo "Smoke suite passed."
