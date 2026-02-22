---
title: API Endpoints
parent: Reference
nav_order: 2
---

Runtime HTTP surfaces:

- Gateway API (default `:18790`)
  - `POST /chat`
- Dashboard/API server (default `:18791`)
  - status/auth: `/api/v1/status`, `/api/v1/auth/verify`
  - timeline/traces: `/api/v1/timeline`, `/api/v1/trace/{traceID}`, `/api/v1/trace-graph/{traceID}`
  - memory: `/api/v1/memory/status`, `/api/v1/memory/reset`, `/api/v1/memory/config`, `/api/v1/memory/prune`
  - settings: `/api/v1/settings`, `/api/v1/workrepo`
  - approvals/tasks: `/api/v1/approvals/*`, `/api/v1/tasks`
  - web users/chat: `/api/v1/webusers`, `/api/v1/weblinks`, `/api/v1/webchat/send`
  - repo/orchestrator/group endpoints under `/api/v1/*`

Detailed API docs:
- [KafClaw Operations Guide](/operations-admin/operations-guide/)
- [KafClaw Administration Guide](/operations-admin/admin-guide/)
- [Detailed Architecture](/architecture-security/architecture-detailed/)
