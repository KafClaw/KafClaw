---
title: Configuration Keys
parent: Reference
nav_order: 3
---

## Configuration Sources

KafClaw resolves config in this order (highest priority first):

1. Runtime settings in `~/.kafclaw/timeline.db` (`settings` table)
2. Environment variables (`KAFCLAW_*`, plus provider vars)
3. Config file `~/.kafclaw/config.json`
4. Built-in defaults

## Core Files

- `~/.kafclaw/config.json` - persistent config file
- `~/.kafclaw/timeline.db` - runtime settings, events, tasks, memory metadata
- `~/.kafclaw/whatsapp.db` - WhatsApp session/device state

## Runtime Setting Keys

Most-used keys stored in `timeline.db`:

| Key | Purpose |
|-----|---------|
| `daily_token_limit` | Daily LLM token cap (`0` or empty = unlimited) |
| `whatsapp_allowlist` | Newline-separated approved WhatsApp JIDs |
| `whatsapp_denylist` | Newline-separated blocked WhatsApp JIDs |
| `whatsapp_pending` | Newline-separated pending WhatsApp JIDs |
| `whatsapp_pair_token` | Pairing token for first-contact flow |
| `silent_mode` | Suppress outbound WhatsApp when `true` |
| `bot_repo_path` | Active system/identity repo path |
| `selected_repo_path` | Active repository selected in dashboard |
| `group_name` | Current collaboration group name |
| `group_active` | Group participation flag |
| `group_heartbeat_last_attempt_at` | Last heartbeat attempt timestamp (RFC3339 UTC) |
| `group_heartbeat_last_success_at` | Last successful heartbeat timestamp (RFC3339 UTC) |
| `group_heartbeat_seq` | Monotonic local heartbeat sequence counter |
| `runtime_reconcile_last_at` | Last startup reconciliation timestamp |
| `runtime_reconcile_pending_deliveries` | Pending completed task deliveries discovered at startup |
| `runtime_reconcile_open_tasks` | Open task count (`pending`/`processing`) discovered at startup |
| `runtime_reconcile_open_group_tasks` | Open group task count (`pending`/`accepted`) discovered at startup |
| `kafscale_lfs_proxy_url` | LFS proxy URL for shared artifacts |
| `knowledge_presence_last_at` | Last published knowledge presence announcement (RFC3339 UTC) |
| `knowledge_capabilities_last_at` | Last published knowledge capabilities announcement (RFC3339 UTC) |
| `memory_embedding_install_requested_at` | Last embedding install bootstrap request timestamp (RFC3339 UTC) |
| `memory_embedding_install_model` | Embedding model last requested for install/bootstrap |
| `memory_overflow_events_total` | Count of memory-context truncation events due to budget limits |
| `memory_overflow_events_<lane>` | Per-lane overflow counters (e.g. `rag`, `working`, `observation`) |

## Useful CLI Commands

Inspect and update config:

```bash
kafclaw config get gateway.host
kafclaw config set gateway.host 127.0.0.1
kafclaw config set providers.openai.apiBase https://openrouter.ai/api/v1
kafclaw config set providers.openai.apiKey <token>
```

Guided updates:

```bash
kafclaw configure
```

Diagnostics:

```bash
kafclaw status
kafclaw doctor
```

## Node Identity (Required for Shared Knowledge)

`node.clawId` and `node.instanceId` identify who published a knowledge envelope.
They are required for proposal/vote/decision/fact workflows.

```json
{
  "node": {
    "clawId": "claw-a",
    "instanceId": "inst-a"
  }
}
```

| Key | Type | Description |
|-----|------|-------------|
| `node.clawId` | string | Stable claw identity used for voting and attribution |
| `node.instanceId` | string | Runtime instance identity for a specific process |

Operational note:
- Missing `node.clawId`/`node.instanceId` blocks governed knowledge commands and apply paths.

## Memory Embedding Configuration

`memory.embedding` is treated as required for semantic memory operation.

```json
{
  "memory": {
    "embedding": {
      "enabled": true,
      "provider": "local-hf",
      "model": "BAAI/bge-small-en-v1.5",
      "dimension": 384,
      "normalize": true
    }
  }
}
```

| Key | Type | Description |
|-----|------|-------------|
| `memory.embedding.enabled` | bool | Must be `true` for normal memory mode |
| `memory.embedding.provider` | string | Embedding backend (`local-hf`, `openai`, etc.) |
| `memory.embedding.model` | string | Embedding model identifier |
| `memory.embedding.dimension` | int | Embedding vector dimension (`> 0`) |
| `memory.embedding.normalize` | bool | Apply vector normalization |

Safety behavior:
- Adding a first embedding later does not wipe existing text-only memory rows.
- Switching an already-used embedding fingerprint requires `--confirm-memory-wipe`; without it, `configure` aborts.
- When confirmed, `configure` wipes `memory_chunks` before saving the new embedding config.
- `kafclaw doctor --fix` restores default embedding settings if missing/disabled.

## Knowledge Envelope Contract (Kafka)

When `knowledge.enabled=true`, knowledge topics (`knowledge.topics.*`) consume/publish envelopes that must include:
- `schemaVersion`
- `type` (`capabilities|presence|proposal|vote|decision|fact`)
- `traceId`
- `timestamp`
- `idempotencyKey`
- `clawId`
- `instanceId`

`clawId`/`instanceId` are enforced at validation time, and `idempotencyKey` is persisted in `knowledge_idempotency` to prevent duplicate apply.

See [Knowledge Contracts](/reference/knowledge-contracts/) for payload examples.

Governance feature flag:

| Key | Type | Description |
|-----|------|-------------|
| `knowledge.governanceEnabled` | bool | Enables proposal/vote/decision/fact apply paths (CLI + Kafka handler) |

## Knowledge Voting Policy

`knowledge.voting` controls quorum-based governance for shared decisions.

| Key | Type | Description |
|-----|------|-------------|
| `knowledge.voting.enabled` | bool | Enable voting workflow |
| `knowledge.voting.minPoolSize` | int | Activate voting when active pool size is at least this value |
| `knowledge.voting.quorumYes` | int | Minimum yes votes required for approval |
| `knowledge.voting.quorumNo` | int | Minimum no votes required for rejection |
| `knowledge.voting.timeoutSec` | int | Proposal timeout window (seconds) |
| `knowledge.voting.allowSelfVote` | bool | Allow proposer to cast a vote on own proposal |

Shared fact version policy:
- New `factId` must start at `version=1`.
- Existing facts accept only strictly sequential updates (`currentVersion + 1`).
- Same/lower versions with identical content are treated as `stale` (ignored safely).
- Same/lower versions with different content are `conflict`.
- Version gaps (`incoming > currentVersion + 1`) are `conflict` (out-of-order).

## Model Configuration

```json
{
  "model": {
    "name": "claude/claude-sonnet-4-5",
    "maxTokens": 8192,
    "temperature": 0.7,
    "maxToolIterations": 20,
    "taskRouting": {
      "security": "claude/claude-opus-4-6",
      "coding": "openai-codex/gpt-5.3-codex"
    }
  }
}
```

| Key | Type | Description |
|-----|------|-------------|
| `model.name` | string | Global default model in `provider/model` format |
| `model.maxTokens` | int | Max output tokens per LLM call |
| `model.temperature` | float | Sampling temperature (0.0 - 1.0) |
| `model.maxToolIterations` | int | Max tool-call rounds per request |
| `model.taskRouting` | map | Category to model string overrides (`security`, `coding`, `tool-heavy`, `creative`) |

## Provider Configuration

```json
{
  "providers": {
    "anthropic": { "apiKey": "sk-ant-...", "apiBase": "" },
    "openai": { "apiKey": "sk-...", "apiBase": "" },
    "gemini": { "apiKey": "AIza..." },
    "xai": { "apiKey": "xai-..." },
    "openrouter": { "apiKey": "sk-or-...", "apiBase": "https://openrouter.ai/api/v1" },
    "deepseek": { "apiKey": "sk-...", "apiBase": "https://api.deepseek.com/v1" },
    "groq": { "apiKey": "gsk_...", "apiBase": "https://api.groq.com/openai/v1" },
    "vllm": { "apiKey": "", "apiBase": "http://localhost:8000/v1" },
    "scalyticsCopilot": { "apiKey": "<token>", "apiBase": "https://copilot.scalytics.io/v1" }
  }
}
```

Each provider entry accepts `apiKey` and `apiBase`. See [LLM Providers](/reference/providers/) for details.

## Per-Agent Model Configuration

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "model": {
          "primary": "claude/claude-opus-4-6",
          "fallbacks": ["openai/gpt-4o"]
        },
        "subagents": {
          "model": "groq/llama-3.3-70b"
        }
      }
    ]
  }
}
```

| Key | Type | Description |
|-----|------|-------------|
| `agents.list[].model.primary` | string | Primary model for this agent |
| `agents.list[].model.fallbacks` | []string | Fallback models tried on transient errors |
| `agents.list[].subagents.model` | string | Model for subagents spawned by this agent |

## Subagent Memory Share Mode

```json
{
  "tools": {
    "subagents": {
      "memoryShareMode": "handoff"
    }
  }
}
```

| Key | Type | Description |
|-----|------|-------------|
| `tools.subagents.memoryShareMode` | string | Subagent memory mode: `isolated`, `handoff` (default), `inherit-readonly` |

Mode behavior:
- `isolated`: child session is isolated; no automatic parent-session handoff write.
- `handoff`: child stays isolated; completion handoff is appended to parent session.
- `inherit-readonly`: child receives read-only parent snapshot and still writes completion handoff to parent session.

## Middleware Configuration

| Section | Reference |
|---------|-----------|
| `contentClassification` | [Content Classification](/reference/middleware/#content-classification) |
| `promptGuard` | [Prompt Guard](/reference/middleware/#prompt-guard) |
| `outputSanitization` | [Output Sanitizer](/reference/middleware/#output-sanitizer) |
| `finops` | [FinOps Cost Attribution](/reference/middleware/#finops-cost-attribution) |

## Common Environment Variables

- `OPENAI_API_KEY`
- `OPENROUTER_API_KEY`
- `KAFCLAW_MODEL` - global model (e.g. `claude/claude-sonnet-4-5`)
- `KAFCLAW_AGENTS_WORKSPACE`
- `KAFCLAW_AGENTS_WORK_REPO_PATH`
- `KAFCLAW_GATEWAY_HOST`
- `KAFCLAW_GATEWAY_PORT`
- `KAFCLAW_GATEWAY_AUTH_TOKEN`
- `KAFCLAW_GROUP_KAFKA_BROKERS`
- `KAFCLAW_GROUP_KAFKA_SECURITY_PROTOCOL` (`PLAINTEXT`, `SSL`, `SASL_PLAINTEXT`, `SASL_SSL`)
- `KAFCLAW_GROUP_KAFKA_SASL_MECHANISM` (`PLAIN`, `SCRAM-SHA-256`, `SCRAM-SHA-512`)
- `KAFCLAW_GROUP_KAFKA_SASL_USERNAME`
- `KAFCLAW_GROUP_KAFKA_SASL_PASSWORD`
- `KAFCLAW_GROUP_KAFKA_TLS_CA_FILE`
- `KAFCLAW_GROUP_KAFKA_TLS_CERT_FILE`
- `KAFCLAW_GROUP_KAFKA_TLS_KEY_FILE`

## Related Docs

- [LLM Providers](/reference/providers/)
- [Models CLI](/reference/models-cli/)
- [Chat Middleware](/reference/middleware/)
- [Getting Started Guide](/start-here/getting-started/)
- [KafClaw Administration Guide](/operations-admin/admin-guide/)
- [Workspace Policy](/architecture-security/workspace-policy/)
