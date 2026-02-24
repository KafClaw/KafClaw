---
title: Memory Governance Operations
parent: Memory Management
nav_order: 5
---

# Memory Governance Operations

Operator runbook for embedding lifecycle and shared knowledge governance.

## Config Management

Low-level edits:

```bash
./kafclaw config get gateway.host
./kafclaw config set gateway.host 127.0.0.1
./kafclaw config unset channels.slack.accounts
```

Guided updates:

```bash
./kafclaw configure
./kafclaw configure --non-interactive --memory-embedding-enabled-set --memory-embedding-enabled=true --memory-embedding-provider local-hf --memory-embedding-model BAAI/bge-small-en-v1.5 --memory-embedding-dimension 384
./kafclaw configure --non-interactive --memory-embedding-model BAAI/bge-base-en-v1.5 --confirm-memory-wipe
```

Memory embedding switch policy:

- Configure blocks a switch when embedded memory already exists unless `--confirm-memory-wipe` is provided.
- First-time embedding enable (from disabled to configured) does not wipe existing text-only memory rows.
- On confirmed switch, `memory_chunks` is wiped so old vectors do not mix with new embedding space.

## Knowledge Governance

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

## Runtime Endpoints

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
