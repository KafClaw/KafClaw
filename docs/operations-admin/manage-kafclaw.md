---
parent: Operations and Admin
---

# KafClaw Management Guide

Operator-focused guide for managing KafClaw from CLI and runtime endpoints.

## 1. Core Command Surface

| Command | Purpose |
|--------|---------|
| `kafclaw onboard` | Initialize config and scaffold workspace identity files |
| `kafclaw gateway` | Run full gateway (API, dashboard, channels, memory, group/orchestrator when enabled) |
| `kafclaw status` | Quick operational status: config, providers, channels, pairing, policy diagnostics |
| `kafclaw doctor` | Run setup and config diagnostics |
| `kafclaw config` | Low-level dotted-path config read/write/unset |
| `kafclaw configure` | Guided updates for selected settings (currently subagent allowlist) |
| `kafclaw group` | Join/leave/status/members for Kafka collaboration group |
| `kafclaw kshark` | Kafka connectivity and protocol diagnostics |
| `kafclaw agent -m` | Single-shot direct CLI interaction with agent loop |
| `kafclaw pairing` | Approve/deny pending Slack/Teams sender pairings |
| `kafclaw whatsapp-setup` | Configure WhatsApp auth and initial lists |
| `kafclaw whatsapp-auth` | Approve/deny/list WhatsApp JIDs |
| `kafclaw install` | Install binary to `/usr/local/bin` |
| `kafclaw version` | Print build version |

## 2. First-Time Operator Runbook

```bash
./kafclaw onboard
./kafclaw status
./kafclaw doctor
./kafclaw gateway
```

Then verify:

- API: `http://127.0.0.1:18790`
- Dashboard: `http://127.0.0.1:18791`

## 3. Onboarding and Modes

### Interactive

```bash
./kafclaw onboard
```

### Non-interactive examples

```bash
./kafclaw onboard --non-interactive --profile local --llm skip
./kafclaw onboard --non-interactive --profile local-kafka --kafka-brokers localhost:9092 --group-name kafclaw --agent-id agent-local --role worker --llm skip
./kafclaw onboard --non-interactive --profile remote --llm openai-compatible --llm-api-base http://localhost:11434/v1 --llm-model llama3.1:8b
```

Mode effects applied by onboarding:

| Mode | Gateway Host | Group | Orchestrator | Auth Token |
|------|--------------|-------|--------------|------------|
| `local` | `127.0.0.1` | off | off | none |
| `local-kafka` | `127.0.0.1` | on | on | none |
| `remote` | `0.0.0.0` | off | off | generated/required |

Onboarding also scaffolds workspace files:

- `AGENTS.md`
- `SOUL.md`
- `USER.md`
- `TOOLS.md`
- `IDENTITY.md`

Use `--force` to overwrite existing config and scaffold files.

## 4. Daily Health Checks

### Status snapshot

```bash
./kafclaw status
```

Highlights include:

- config + API key presence
- WhatsApp session/QR state
- Slack/Teams account diagnostics and policy warnings
- pending pairing counts from timeline

### Doctor checks

```bash
./kafclaw doctor
./kafclaw doctor --fix
./kafclaw doctor --generate-gateway-token
```

`doctor` returns non-zero when failing checks exist.

## 5. Config Management

### Low-level config edits

```bash
./kafclaw config get gateway.host
./kafclaw config set gateway.host 127.0.0.1
./kafclaw config unset channels.slack.accounts
```

### Guided update path

```bash
./kafclaw configure
./kafclaw configure --subagents-allow-agents agent-main,agent-research --non-interactive
./kafclaw configure --clear-subagents-allow-agents --non-interactive
```

### LLM provider and token management

Interactive (recommended):

```bash
./kafclaw onboard
```

Provider setup options in onboarding:

- `cli-token` (prompts for token; OpenRouter-style OpenAI-compatible flow)
- `openai-compatible` (prompts for API base, optional token, model)
- `skip` (retain current settings)

Direct config edits:

```bash
./kafclaw config get providers.openai.apiBase
./kafclaw config set providers.openai.apiBase "https://openrouter.ai/api/v1"
./kafclaw config set providers.openai.apiKey "<token>"
./kafclaw config set model.name "anthropic/claude-sonnet-4-5"
```

## 6. Group Collaboration Operations

```bash
./kafclaw group join mygroup
./kafclaw group status
./kafclaw group members
./kafclaw group leave
```

Notes:

- Group state is persisted in timeline settings (`group_name`, `group_active`)
- `group status` also prints resolved topic names and LFS health
- `group members` reads roster snapshots from timeline DB

### Configure Kafka broker connection (examples)

Using onboarding profile:

```bash
./kafclaw onboard --non-interactive --profile local-kafka --kafka-brokers "broker1:9092,broker2:9092" --group-name kafclaw --agent-id agent-ops --role worker --llm skip
```

Using direct config commands:

```bash
./kafclaw config set group.enabled true
./kafclaw config set group.groupName "kafclaw"
./kafclaw config set group.kafkaBrokers "broker1:9092,broker2:9092"
./kafclaw config set group.consumerGroup "kafclaw-workers"
./kafclaw config set group.agentId "agent-ops"
```

Using KafScale proxy style settings:

```bash
./kafclaw config set group.lfsProxyUrl "https://your-kafscale-endpoint"
./kafclaw config set group.lfsProxyApiKey "<kafscale-api-key>"
```

Verification:

```bash
./kafclaw group join kafclaw
./kafclaw group status
./kafclaw kshark --auto --yes
```

## 7. Kafka Diagnostics with KShark

Auto-config from current KafClaw group config:

```bash
./kafclaw kshark --auto --yes
```

Using explicit properties:

```bash
./kafclaw kshark --props ./client.properties --topic group.mygroup.requests --group mygroup-workers --yes
```

Useful options:

- `--json <file>` export report
- `--diag` include traceroute/MTU diagnostics
- `--preset` for predefined connection templates

## 8. Channel Auth and Pairing

### Pairing queue (Slack/Teams)

```bash
./kafclaw pairing pending
./kafclaw pairing approve slack ABC123
./kafclaw pairing deny msteams XYZ999
```

### WhatsApp auth flow

```bash
./kafclaw whatsapp-setup
./kafclaw whatsapp-auth --list
./kafclaw whatsapp-auth --approve "+123456789@s.whatsapp.net"
./kafclaw whatsapp-auth --deny "+123456789@s.whatsapp.net"
```

## 9. Channel Bridge (`cmd/channelbridge`)

Build and run:

```bash
go build -o /tmp/channelbridge ./cmd/channelbridge
/tmp/channelbridge
```

Default bind: `:18888` (set via `CHANNEL_BRIDGE_ADDR`).

Key endpoints:

- `GET /healthz`
- `GET /status`
- `POST /slack/events`
- `POST /slack/commands`
- `POST /slack/interactions`
- `POST /teams/messages`

Core env vars:

- `KAFCLAW_BASE_URL` (default `http://127.0.0.1:18791`)
- `KAFCLAW_SLACK_INBOUND_TOKEN`
- `KAFCLAW_MSTEAMS_INBOUND_TOKEN`
- `SLACK_SIGNING_SECRET`
- `MSTEAMS_INBOUND_BEARER`
- `CHANNEL_BRIDGE_STATE` (bridge state file path)

## 10. State and Paths

Core runtime files:

- Config: `~/.kafclaw/config.json`
- Env: `~/.config/kafclaw/env`
- Timeline DB: `~/.kafclaw/timeline.db`
- WhatsApp session: `~/.kafclaw/whatsapp.db`
- WhatsApp QR image: `~/.kafclaw/whatsapp-qr.png`
- Subagent state: `~/.kafclaw/subagents/`

## 11. Client Auth Token Distribution

This section applies to **direct HTTP clients** that call KafClaw API endpoints.
For Slack/Teams/WhatsApp users, authentication is handled by provider bridge + pairing/allowlist controls, not by manually passing the gateway bearer token.

When `KAFCLAW_GATEWAY_AUTH_TOKEN` (or `gateway.authToken`) is set, direct clients do not auto-receive tokens.
Operators must distribute tokens out-of-band (secure chat, secret manager, deployment env injection, etc.).

Operator token sources:

- `~/.kafclaw/config.json` (`gateway.authToken`)
- `~/.config/kafclaw/env` (if managed there)
- `./kafclaw doctor --generate-gateway-token` (rotate/create)

Client request header:

```http
Authorization: Bearer <token>
```

Validation endpoint:

- `POST /api/v1/auth/verify` checks a provided bearer token
- it validates tokens; it does not mint or return a token

## 12. Incident Shortcuts

### Gateway will not start

1. `./kafclaw doctor`
2. Confirm API key/provider config
3. Check timeline DB permissions and disk
4. Re-run `./kafclaw onboard` (or `--force` if needed)

### Kafka/group issues

1. `./kafclaw group status`
2. `./kafclaw kshark --auto --yes`
3. Verify brokers/auth and LFS proxy settings

### Channel ingress issues

1. `./kafclaw status` for account diagnostics
2. Check `pairing pending`
3. If using bridge, check `GET /status` on channel bridge
