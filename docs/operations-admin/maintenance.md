---
parent: Operations and Admin
title: Operations and Maintenance
---

# Operations and Maintenance

Operational runbook for keeping KafClaw healthy in dev and production-like setups.

## Daily Checks

```bash
./kafclaw status
./kafclaw doctor
```

For env hygiene and permissions:

```bash
./kafclaw doctor --fix
```

Memory + knowledge quick checks:

```bash
./kafclaw knowledge status --json
curl -s http://127.0.0.1:18791/api/v1/memory/embedding/healthz
curl -s http://127.0.0.1:18791/api/v1/memory/embedding/status
```

## Runtime Modes and Expectations

- `local`
  - bind: `127.0.0.1`
  - group/orchestrator: off
- `local-kafka`
  - bind: `127.0.0.1`
  - group/orchestrator: on
- `remote`
  - bind: typically `0.0.0.0` or specific LAN IP
  - dashboard auth token: strongly recommended
  - with auth token configured, both dashboard API and `POST /chat` require bearer auth

If you intentionally run remote, do not force loopback defaults.

## Updating Config Safely

Use CLI config commands:

```bash
./kafclaw config get gateway.host
./kafclaw config set gateway.host "127.0.0.1"
./kafclaw config unset channels.telegram.token
```

Bracket paths are supported:

```bash
./kafclaw config set channels.telegram.allowFrom[0] "alice"
./kafclaw config get channels.telegram.allowFrom[0]
```

Subagent limits can be tuned via config:

```bash
./kafclaw config get tools.subagents.maxSpawnDepth
./kafclaw config set tools.subagents.maxSpawnDepth 1
./kafclaw config set tools.subagents.maxChildrenPerAgent 5
./kafclaw config set tools.subagents.maxConcurrent 8
./kafclaw config set tools.subagents.archiveAfterMinutes 60
./kafclaw config set tools.subagents.allowAgents '["agent-main","agent-research"]'
./kafclaw config set tools.subagents.model "anthropic/claude-sonnet-4-5"
./kafclaw config set tools.subagents.thinking "medium"
```

Guided alternative:

```bash
./kafclaw configure --subagents-allow-agents agent-main,agent-research --non-interactive
```

## Service Operation (Linux systemd)

Preferred (CLI-managed):

```bash
sudo ./kafclaw daemon status
sudo ./kafclaw daemon restart
```

Direct `systemctl` fallback:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now kafclaw-gateway.service
sudo systemctl status kafclaw-gateway.service
```

Restart after config changes:

```bash
sudo systemctl restart kafclaw-gateway.service
```

## Backup

Minimum backup set:

- `~/.kafclaw/config.json`
- `~/.config/kafclaw/env`
- `~/.kafclaw/timeline.db`
- workspace and work-repo dirs used by your config

## Upgrade Flow

```bash
git pull
make build
go test ./...
```

Then restart runtime/service.

## Troubleshooting Quick Map

- Gateway unreachable from network:
  - check `gateway.host`
  - check firewall ports `18790`, `18791`
- Remote mode blocked:
  - ensure dashboard auth token exists
  - verify token is sent to both dashboard API and `/chat` calls
- Kafka group issues:
  - verify brokers and group config
  - use `kafclaw kshark` for diagnostics
- LLM errors:
  - verify API base, model, token in config/env
- Embedding runtime unhealthy / memory recall degraded:
  - run `kafclaw doctor` and confirm `memory_embedding_configured` passes
  - trigger bootstrap: `curl -X POST http://127.0.0.1:18791/api/v1/memory/embedding/install -d '{"model":"BAAI/bge-small-en-v1.5"}' -H 'Content-Type: application/json'`
  - verify `/api/v1/memory/embedding/healthz` reports `ready=true`
- Embedding model switched and recall quality dropped:
  - expected behavior: vector index must be reset when fingerprint changes
  - reindex explicitly (destructive): `curl -X POST http://127.0.0.1:18791/api/v1/memory/embedding/reindex -d '{"confirmWipe":true,"reason":"embedding_switch"}' -H 'Content-Type: application/json'`
  - monitor `memory_chunks` refill and `knowledge facts` consistency
- Shared knowledge not converging across claws:
  - confirm presence/capability heartbeats by checking runtime settings:
    - `kafclaw config get knowledge.enabled`
    - `kafclaw config get knowledge.topics.presence`
    - `kafclaw config get knowledge.topics.capabilities`
  - validate envelope dedup and voting outcomes with:
    - `kafclaw knowledge decisions --status approved --json`
    - `kafclaw knowledge facts --json`
- Subagent spawn denied:
  - check `tools.subagents.maxSpawnDepth`
  - check `tools.subagents.maxChildrenPerAgent`
  - check `tools.subagents.maxConcurrent`
- Subagent steer did not keep old run:
  - expected in phase 2: steer replaces execution by killing old run and spawning a new steered run

- Subagent target selection failed:
  - use `target=last`, `target=<numeric index>`, `target=<runId prefix>`, `target=<label prefix>`, or full child session key
- Subagent controls cannot see target run:
  - verify the run belongs to the same root session scope (cross-root controls are denied)

## Subagent Audit Events

When trace IDs are active, subagent operations append timeline events:

- `subagent spawn_accepted`
- `subagent kill` (includes `killed=true|false` in metadata)
- `subagent steer`

Runtime resilience:

- subagent runs are persisted in `~/.kafclaw/subagents/runs-<workspace-hash>.json`
- in-flight runs at restart are marked failed with restart reason
- ended runs are cleaned up after `tools.subagents.archiveAfterMinutes`
- announce retries/backoff are persisted and resumed on restart
- subagent completion announces are normalized as `Status/Result/Notes`; return `ANNOUNCE_SKIP` to suppress

Use trace views in dashboard to inspect these lifecycle events.

## Deep References

- [Operations Guide](/operations-admin/operations-guide/)
- [Admin Guide](/operations-admin/admin-guide/)
- [Security Risks](/architecture-security/security-risks/)
