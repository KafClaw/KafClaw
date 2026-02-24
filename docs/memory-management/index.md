---
title: Memory Management
nav_order: 4
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

- [Memory Notes](/agent-concepts/memory-notes/)
- [Architecture: Timeline and Memory](/architecture-security/architecture-timeline/)
- [Knowledge Contracts](/reference/knowledge-contracts/)
- [Memory Governance Operations](/memory-management/memory-governance-operations/)
- [Configuration Keys](/reference/config-keys/)

## Operational Endpoints

- `GET /api/v1/memory/status`
- `GET /api/v1/memory/metrics`
- `GET /api/v1/memory/embedding/status`
- `GET /api/v1/memory/embedding/healthz`
- `POST /api/v1/memory/embedding/install`
- `POST /api/v1/memory/embedding/reindex`
