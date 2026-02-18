---
parent: Architecture and Security
---

# Subagents Threat Model

## Scope

This document defines security boundaries for subagent orchestration (`sessions_spawn`, `subagents`, `agents_list`) in KafClaw.

## Security Objectives

- Prevent privilege escalation from child to parent/root sessions.
- Enforce spawn-depth and concurrency limits.
- Keep subagent control scoped to the same root-session lineage.
- Ensure sensitive operations are auditable.
- Prevent duplicate completion announcements.

## Trust Boundaries

- Parent agent input is untrusted until policy-evaluated.
- Child subagents run with inherited-or-restricted tool policy.
- Cross-root run control is denied.
- Outbound announce delivery is best-effort with persisted retry state.

## Controls Implemented

- Spawn limits:
  - `tools.subagents.maxSpawnDepth`
  - `tools.subagents.maxChildrenPerAgent`
  - `tools.subagents.maxConcurrent`
- Tool policy guardrails:
  - depth-aware `sessions_spawn` denial at leaf depth
  - optional child allow/deny lists via `tools.subagents.tools.{allow,deny}`
- Root-scope session control:
  - run metadata includes `rootSession` and `requestedBy`
  - `kill`/`steer`/`list` operate within root-session scope
- Audit visibility:
  - timeline `SUBAGENT` events (`spawn_accepted`, `kill`, `steer`)
- Announce safety:
  - normalized `Status/Result/Notes` output
  - `ANNOUNCE_SKIP` suppression token
  - deterministic announce identity and in-process duplicate suppression
  - persisted retry/backoff state for deferred announces

## Known Limitations

- Duplicate suppression is deterministic at runtime/state level, but does not yet use a dedicated external idempotency cache across independent gateways.

## Operational Recommendations

- Keep `maxSpawnDepth=1` unless nested orchestration is explicitly needed.
- Keep strict child tool allow/deny policies for production.
- Monitor timeline for repeated `subagent` failures/timeouts.
- Prefer `cleanup=delete` only when downstream delivery guarantees are acceptable for your deployment.
