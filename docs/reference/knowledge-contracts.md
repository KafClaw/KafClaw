---
title: Knowledge Contracts
parent: Reference
nav_order: 7
---

# Knowledge Contracts

Versioned Kafka envelope for shared knowledge topics.

## Envelope

All knowledge messages use this base envelope (`schemaVersion: "v1"`):

```json
{
  "schemaVersion": "v1",
  "type": "proposal|vote|decision|fact|presence|capabilities",
  "traceId": "trace-...",
  "timestamp": "2026-02-24T00:00:00Z",
  "idempotencyKey": "knowledge:...",
  "clawId": "claw-a",
  "instanceId": "inst-a",
  "payload": {}
}
```

Required fields are validated before apply. Duplicate `idempotencyKey` values are deduplicated via `knowledge_idempotency`.

## Governed Payloads

`proposal`:

```json
{
  "proposalId": "p1",
  "group": "prod",
  "title": "Adopt runbook v2",
  "statement": "Use v2 for incident handling",
  "tags": ["ops", "runbook"]
}
```

`vote`:

```json
{
  "proposalId": "p1",
  "vote": "yes|no",
  "reason": "optional"
}
```

`decision`:

```json
{
  "proposalId": "p1",
  "outcome": "approved|rejected|expired",
  "yes": 3,
  "no": 1,
  "reason": "optional"
}
```

`fact`:

```json
{
  "factId": "svc.runbook",
  "group": "prod",
  "subject": "service-x",
  "predicate": "runbook",
  "object": "v2",
  "version": 2,
  "source": "decision:p1",
  "proposalId": "p1",
  "decisionId": "decision:p1",
  "tags": ["ops"]
}
```

## Feature Flag

Governed apply paths are controlled by:

- `knowledge.enabled`
- `knowledge.governanceEnabled`

When governance is disabled, proposal/vote/decision/fact write/apply paths are skipped/denied while non-governed presence/capabilities announcements may still publish.
