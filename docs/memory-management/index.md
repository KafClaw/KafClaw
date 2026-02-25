---
title: Memory Management
nav_order: 7
has_children: true
---

Private memory, durability, embeddings, and shared knowledge governance in one place.

## What Lives Here

- Private memory lanes and context shaping
- Embedding runtime health/install/reindex lifecycle
- Restart/model-switch durability guarantees
- Shared knowledge governance (proposal/vote/decision/fact)
- Conflict/stale/version policy

## Core Pages

- [Memory Architecture and Notes](/agent-concepts/memory-notes/)
- [CoT Cascading Protocol](/agent-concepts/CoT/)
- [Durable Memory Concept (Runbook)](/memory-management/memory-governance-operations/#durable-memory-concept)
- [Memory Governance Operations](/memory-management/memory-governance-operations/)

## Related References

- [Knowledge Contracts](/reference/knowledge-contracts/)
- [Configuration Keys](/reference/config-keys/)
- [Architecture: Timeline and Memory](/architecture-security/architecture-timeline/)

## Operational Endpoints

- `GET /api/v1/memory/status`
- `GET /api/v1/memory/metrics`
- `GET /api/v1/memory/embedding/status`
- `GET /api/v1/memory/embedding/healthz`
- `POST /api/v1/memory/embedding/install`
- `POST /api/v1/memory/embedding/reindex`
