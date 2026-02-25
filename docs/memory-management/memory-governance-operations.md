---
title: Memory Governance Operations
parent: Memory Management
nav_order: 3
---

# Memory Governance Operations

Operator runbook for durable memory, embedding lifecycle, and shared knowledge governance.

## Scope

This runbook covers:

- Durable memory behavior across restarts and model switches
- Embedding install and switching workflow
- Knowledge governance and quorum voting operations

For architecture context, see:
- [Memory Architecture and Notes](/agent-concepts/memory-notes/)
- [Architecture: Timeline and Memory](/architecture-security/architecture-timeline/)
- [Knowledge Contracts](/reference/knowledge-contracts/)

## Durable Memory Concept

Durable memory means runtime state survives process restarts and host reboots unless an explicit wipe path is used.

What persists:
- Timeline and operational state in `~/.kafclaw/timeline.db`
- Knowledge dedup ledger (`knowledge_idempotency`)
- Knowledge facts/proposals/votes tables
- Existing text-only memory rows before first embedding enable

What can be wiped intentionally:
- Vector memory (`memory_chunks`) on confirmed embedding switch

Durability guarantees:
- Restart does not drop open tasks, heartbeats, or knowledge governance history.
- First-time embedding enable does not wipe existing memory rows.
- Embedding model switch requires explicit confirmation and then wipes vectors to prevent mixed embedding spaces.

### Durability Matrix

| Event | Open tasks and heartbeats | Knowledge history | Vector memory (`memory_chunks`) | Notes |
|-----|-----|-----|-----|-----|
| Process restart | Preserved | Preserved | Preserved | State reloaded from `~/.kafclaw/timeline.db` |
| Host reboot | Preserved | Preserved | Preserved | Same as restart after service comes back |
| First embedding enable | Preserved | Preserved | Preserved (new vectors added) | No wipe of existing text-only rows |
| Embedding model switch (`--confirm-memory-wipe`) | Preserved | Preserved | Wiped then reindexed | Required to avoid mixed embedding spaces |
| Governance disabled (`knowledge.governanceEnabled=false`) | Preserved | Preserved | Unchanged | Governed apply paths stop; presence/capabilities can still publish |

## Embedding Lifecycle Operations

Initial or corrective setup:

```bash
./kafclaw configure
./kafclaw configure --non-interactive --memory-embedding-enabled-set --memory-embedding-enabled=true --memory-embedding-provider local-hf --memory-embedding-model BAAI/bge-small-en-v1.5 --memory-embedding-dimension 384
./kafclaw doctor
./kafclaw doctor --fix
```

Switch embedding model (destructive for vectors only):

```bash
./kafclaw configure --non-interactive --memory-embedding-model BAAI/bge-base-en-v1.5 --confirm-memory-wipe
```

Embedding switch policy:

- Configure blocks a switch when embedded memory already exists unless `--confirm-memory-wipe` is provided.
- First-time embedding enable (from disabled to configured) does not wipe existing text-only memory rows.
- On confirmed switch, `memory_chunks` is wiped so old vectors do not mix with new embedding space.

Runtime endpoints:

```bash
curl -s http://127.0.0.1:18791/api/v1/memory/status
curl -s http://127.0.0.1:18791/api/v1/memory/metrics
curl -s http://127.0.0.1:18791/api/v1/memory/embedding/status
curl -s http://127.0.0.1:18791/api/v1/memory/embedding/healthz
curl -X POST http://127.0.0.1:18791/api/v1/memory/embedding/install \
  -H 'Content-Type: application/json' \
  -d '{"model":"BAAI/bge-small-en-v1.5"}'
curl -X POST http://127.0.0.1:18791/api/v1/memory/embedding/reindex \
  -H 'Content-Type: application/json' \
  -d '{"confirmWipe":true,"reason":"embedding_switch"}'
```

## Knowledge Governance Operations

Identity prerequisites for governance:

```bash
./kafclaw config set node.clawId "claw-a"
./kafclaw config set node.instanceId "inst-a"
```

- Governance commands and apply paths require both identity keys.

Status and proposal/vote flow:

```bash
./kafclaw knowledge status --json
./kafclaw knowledge propose --proposal-id p1 --group mygroup --statement "Adopt runbook v2"
./kafclaw knowledge vote --proposal-id p1 --vote yes
./kafclaw knowledge decisions --status approved --json
./kafclaw knowledge facts --group mygroup --json
```

Governance behavior:

- Envelope dedup is persisted in `knowledge_idempotency`.
- Quorum policy is controlled by `knowledge.voting.*`.
- Shared facts apply sequential version policy (`accepted|stale|conflict`).
- Apply paths are feature-gated by `knowledge.governanceEnabled`.

## Cascade Failure Triage Snippet

Use this quick flow when a cascading task is stuck or failed.

1. Pull the full state machine for the trace:

```bash
kafclaw task status --trace <trace-id> --json
```

2. Check transition order for the failing task:
- expected happy path: `pending -> running -> self_test -> validated -> committed -> released_next`
- expected retry path: `self_test -> pending` when retries remain
- terminal paths: `failed` on runtime, validation budget, or commit errors

3. Identify deterministic failure reason in JSON:
- `missing_input`
- `missing_output`
- `invalid_rules`
- runtime or commit error metadata
- escalation marker (when retry threshold exceeded): `artifact.escalation` with fallback model/tooling hints

4. Apply targeted fix:
- contract issue: update `required_input` or `produced_output`
- validation issue: tighten or correct `validation_rules`
- runtime issue: fix tool or execution precondition

5. Re-run task and verify transition recovery:

```bash
kafclaw task status --trace <trace-id> --json
```
