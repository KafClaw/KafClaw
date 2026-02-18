---
title: User Manual
parent: Getting Started
nav_order: 2
---

# KafClaw User Manual

A comprehensive guide to installing, configuring, and using KafClaw — a personal AI assistant framework written in Go with an Electron desktop frontend.

---

## Table of Contents

1. [Getting Started](#1-getting-started)
2. [Quick Start](#2-quick-start)
3. [CLI Reference](#3-cli-reference)
4. [Web Dashboard](#4-web-dashboard)
5. [WhatsApp, Slack, and Teams Integration](#5-whatsapp-slack-and-teams-integration)
6. [Memory System](#6-memory-system)
7. [Day2Day Task Tracker](#7-day2day-task-tracker)
8. [Soul Files and Workspace](#8-soul-files-and-workspace)
9. [FAQ / Troubleshooting](#9-faq--troubleshooting)

---

## 1. Getting Started

### Prerequisites

- **Go 1.24.0+** (toolchain 1.24.13)
- **OpenAI API key** (or OpenRouter API key)
- **Operating System:** macOS / Linux / Windows

### Installation

Build from source:

```bash
cd KafClaw
go build ./cmd/kafclaw
```

Or use the Makefile:

```bash
cd KafClaw
make build
```

Install system-wide to `/usr/local/bin`:

```bash
kafclaw install
# May require sudo
```

### First-Time Setup

```bash
kafclaw onboard
```

Creates `~/.kafclaw/config.json` with defaults. Add your API key:

```bash
export OPENAI_API_KEY="sk-..."
# Or edit ~/.kafclaw/config.json directly
```

Verify:

```bash
kafclaw status
```

---

## 2. Quick Start

```bash
# 1. Build
cd KafClaw && make build

# 2. Initialize
kafclaw onboard

# 3. Add API key
export OPENAI_API_KEY="sk-..."

# 4. Test
kafclaw agent -m "hello"

# 5. Start full gateway
kafclaw gateway
```

Once the gateway is running:
- **API server** — `http://localhost:18790`
- **Web dashboard** — `http://localhost:18791`
- **WhatsApp** — connects automatically if configured

### Logic Flow

1. **Input** — A message arrives (WhatsApp, CLI, Web UI, scheduler).
2. **Bus** — Published to the message bus as an InboundMessage.
3. **Dedup** — Idempotency key checked to prevent reprocessing.
4. **Context** — Context builder assembles system prompt from soul files, working memory, observations, skills, and RAG context.
5. **Processing** — LLM decides if tools are needed.
6. **Tool Loop** — If a tool is called, policy engine evaluates access, tool executes, result feeds back to LLM. Repeats up to 20 iterations.
7. **Post-Processing** — Response saved to session, indexed into memory, observer enqueued.
8. **Delivery** — Response published to message bus, channel delivers to user.

---

## 3. CLI Reference

KafClaw provides the following CLI commands. Run `kafclaw --help` for the full list.
Core startup commands: `onboard`, `doctor`, `status`, `gateway`, `agent`, `config`.

### 3.1 `gateway`

Start the agent gateway daemon.

```bash
kafclaw gateway
```

- Ports: 18790 (API), 18791 (dashboard)
- Runs until Ctrl+C. Handles graceful shutdown of all subsystems.
- If `gateway.authToken` is set, dashboard API routes require bearer auth (except `/api/v1/status` and CORS preflight), and `POST /chat` on port `18790` also requires bearer auth.

### 3.2 `agent`

Single-message mode for quick interactions.

```bash
kafclaw agent -m "hello"
kafclaw agent -m "what did we discuss?" -s "cli:project-x"
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--message` | `-m` | *(required)* | Message to send |
| `--session` | `-s` | `cli:default` | Session ID |

### 3.3 `onboard`

Initialize configuration.

```bash
kafclaw onboard
kafclaw onboard --force   # reset to defaults
kafclaw onboard --non-interactive --profile local --llm skip
kafclaw onboard --non-interactive --profile remote --llm openai-compatible --llm-api-base http://localhost:11434/v1 --llm-model llama3.1:8b
```

Common onboarding profiles:
- `local`
- `local-kafka`
- `remote`

Useful onboarding flags:
- `--systemd` to install service/override/env (Linux)
- `--subagents-max-spawn-depth`, `--subagents-max-children`, `--subagents-max-concurrent`
- `--subagents-archive-minutes`, `--subagents-model`, `--subagents-thinking`

Subagent runtime notes:
- `sessions_spawn` accepts `runTimeoutSeconds` for per-run hard timeout
- `subagents` supports selectors (`last`, numeric index, runId prefix, label prefix, child session key)
- `subagents(action=kill_all)` stops all active children for the current parent session

### 3.4 `status`

Show system status: version, config, API keys, channel enablement, Slack/Teams per-account capability details, isolation scope details, account configuration diagnostics, pairing queue, and unsafe group policy warnings.

```bash
kafclaw status
```

### 3.5 `install`

Install binary to `/usr/local/bin`.

```bash
kafclaw install
```

### 3.6 `doctor`

Run diagnostics for config, env, and runtime safety defaults.

```bash
kafclaw doctor
kafclaw doctor --fix
kafclaw doctor --generate-gateway-token
```

Includes Slack/Teams account configuration diagnostics checks.

### 3.7 `config`

Read and update config values from CLI.

```bash
kafclaw config get gateway.host
kafclaw config set gateway.host 127.0.0.1
```

### 3.8 `configure`

Guided config updates (higher-level than raw key/value `config set`).

```bash
kafclaw configure
kafclaw configure --subagents-allow-agents agent-main,agent-research --non-interactive
kafclaw configure --clear-subagents-allow-agents --non-interactive
```

### 3.9 `kshark`

Kafka diagnostics helper.

```bash
kafclaw kshark --auto --yes
kafclaw kshark --props ./client.properties --topic group.mygroup.requests --group mygroup-workers --yes
```

Use either:
- `--auto` (derive Kafka settings from current KafClaw group config), or
- `--props` (explicit Kafka client properties file).

### 3.10 `whatsapp-setup`

Interactive WhatsApp configuration wizard.

```bash
kafclaw whatsapp-setup
```

Prompts for: enable, pairing token, allowlist, denylist.

### 3.11 `whatsapp-auth`

Manage WhatsApp JID authorization.

```bash
kafclaw whatsapp-auth --list
kafclaw whatsapp-auth --approve "+1234567890@s.whatsapp.net"
kafclaw whatsapp-auth --deny "+0987654321@s.whatsapp.net"
```

### 3.12 `group`

Group collaboration management (requires Kafka).

```bash
kafclaw group status
kafclaw group join mygroup
kafclaw group leave
kafclaw group members
```

### 3.13 `pairing`

Manage pending Slack/Teams sender pairings.

```bash
kafclaw pairing pending
kafclaw pairing approve slack ABC123
kafclaw pairing deny msteams XYZ999
```

---

## 4. Web Dashboard

### Access

```
http://localhost:18791
```

### Features

- **Timeline View** (`/timeline`) — Full conversation history with trace IDs, sender info, event types, classifications. Auto-refreshes every 5 seconds.

- **Trace Viewer** — Drill into individual request flows: inbound, outbound, LLM, and tool execution spans. Task metadata (token counts, delivery status) and policy decisions.

- **Memory Dashboard** — Layer stats, observer status, ER1 sync status, expertise tracker, working memory preview.

- **Repository Browser** — File tree, content viewing, Git diff, commit, pull, push, branch checkout, PR creation via `gh`.

- **Web Chat** — Send messages from the browser. Supports web user management and WhatsApp JID linking.

- **Settings Panel** — Runtime settings including silent mode. Changes take effect immediately.

### Electron App

The Electron desktop app wraps the dashboard with additional capabilities:

| Mode | Sidecar | Group | Network |
|------|---------|-------|---------|
| Standalone | Local binary | No | localhost only |
| Full | Local binary | Kafka | localhost + Kafka |
| Remote | None | N/A | Remote API URL |

Header status indicators: mode badge, memory LED, sidecar/connection status.

---

## 5. WhatsApp, Slack, and Teams Integration

> See also: [whatsapp-setup.md](./whatsapp-setup/) for full details

KafClaw uses `whatsmeow` for native Go WhatsApp connectivity. No Node.js bridge required.

### Setup Flow

1. `kafclaw whatsapp-setup` — Enable and configure
2. `kafclaw gateway` — Start the daemon
3. Scan QR code at `~/.kafclaw/whatsapp-qr.png` with WhatsApp (Settings > Linked Devices)
4. Session persists in `~/.kafclaw/whatsapp.db` — auto-reconnects on restart

### Authorization Model

Three-tier JID system (default-deny):

- **Allowlist** — Authorized to interact. Messages processed normally.
- **Denylist** — Explicitly blocked. Messages silently dropped.
- **Pending** — Unknown senders held until admin approves/denies.

### Silent Mode

Default on. When enabled, outbound WhatsApp messages are suppressed (logged as `suppressed`). Force-send override available per web user.

### Slack and Teams bridge

Slack and Teams run through the channel bridge (`cmd/channelbridge`) and pair with KafClaw via inbound/outbound HTTP.

Required bridge inputs:

- Slack: bot token, optional app token (socket mode), signing secret, inbound token
- Teams: app id/password, inbound bearer, inbound token, OpenID config/JWKS

Core behavior:

- Unknown sender in DM policy `pairing` gets a pairing code.
- Approve pairing from CLI:
  - `kafclaw pairing approve slack <code>`
  - `kafclaw pairing approve msteams <code>`
- Group policy supports `allowlist`, `open`, `disabled`, plus mention gating in groups/channels.

Multi-account and isolation:

- Slack and Teams support named accounts (`channels.<provider>.accounts[]`).
- Message routing is account-aware (`account_id`).
- Session isolation mode is configurable per provider/account:
  - `channel`, `account`, `room` (default), `thread`, `user`
- Default behavior isolates by provider + account + room so user A and user B do not leak sessions across different chats/accounts.

Reply behavior:

- Outbound supports `reply_mode=off|first|all`.
- `off`: never thread reply.
- `first`: only first reply in a thread context.
- `all`: thread reply whenever thread id is present.

Known limits:

- Slack normalization/chunking is functional but not full OpenClaw variant parity.
- Teams runs on custom Go Bot Framework HTTP/JWT flow (no direct Microsoft Agents Hosting runtime parity in Go).

---

## 6. Memory System

> See also: [architecture-timeline.md](./architecture-timeline/) for the full memory architecture

### Overview

The 6-layer memory system initializes automatically when the LLM provider supports embeddings. On startup the gateway logs whether memory is active.

### Agent Tools

**`remember`** — Store information in long-term memory.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | Yes | Information to remember |
| `tags` | string | No | Comma-separated tags |

**`recall`** — Search memory for relevant information.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | Search query |
| `limit` | integer | No | Max results (default: 5) |

**`update_working_memory`** — Update the per-user scratchpad.

### RAG Context Injection

On every message, KafClaw searches semantic memory:
- Top 5 results retrieved
- Filtered by relevance score >= 30%
- Injected into system prompt as a `# Relevant Memory` section

### Memory Layers

| Layer | TTL | Source |
|-------|-----|--------|
| Soul | Permanent | Identity files indexed at startup |
| Conversation | 30 days | Auto-indexed Q&A pairs |
| Tool | 14 days | Tool execution outputs |
| Group | 60 days | Shared via Kafka/LFS |
| ER1 | Permanent | Personal memory sync |
| Observation | Permanent | LLM-compressed conversation history |

### Lifecycle

- Daily TTL pruning (per-source retention)
- Observer triggers at message threshold (default 50)
- Reflector consolidates when observations exceed max (default 200)

---

## 7. Day2Day Task Tracker

Built-in daily task management. Commands work via any channel (CLI, WhatsApp, Web UI).

### Commands

| Command | Description |
|---------|-------------|
| `dtu [text]` | Update — add task or enter capture mode |
| `dtp [text]` | Progress — log progress or enter capture mode |
| `dts` | Summarize — consolidate today's tasks |
| `dtn` | Next — suggest next task to work on |
| `dta` | All — list all open tasks as prioritized plan |
| `dtc` | Close — submit buffered content from capture mode |

### Capture Mode

```
User:  dtu
Bot:   Day2Day: dtu capture started. Send dtc to close.
User:  Fix the login page CSS
User:  Update the API rate limiter
User:  dtc
Bot:   Updated. Next step: Fix the login page CSS
```

### Task Files

Stored as markdown in the system repo:

```
{system-repo}/operations/day2day/tasks/YYYY-MM-DD.md
```

Format: `- [ ]` for open, `- [x]` for completed. Includes progress log, consolidated state, and next step.

---

## 8. Soul Files and Workspace

### Workspace Structure

Default: `~/.kafclaw/workspace/` — the agent's state home.

### Bootstrap Files

Loaded at startup and assembled into the system prompt:

| File | Purpose |
|------|---------|
| `AGENTS.md` | Governance rules and operational constraints |
| `SOUL.md` | Core personality and behavioral guidelines |
| `USER.md` | User-specific preferences and context |
| `TOOLS.md` | Tool usage guidelines and restrictions |
| `IDENTITY.md` | Agent identity and naming |

### Work Repo

The agent's exclusive write target. Default: `~/.kafclaw/work-repo/`.

Artifact directories:
- `memory/` — Memory files (MEMORY.md, daily notes)
- Specification docs — Behavior specifications
- `tasks/` — Plans and milestones
- `docs/` — Explanations and summaries

### Skills

Custom skills in `{workspace}/skills/{skill-name}/SKILL.md` and `{system-repo}/skills/{skill-name}/SKILL.md` are loaded into the system prompt at startup.

### Configuration Hierarchy

Resolved in this precedence (highest wins):

1. **Runtime settings** (`~/.kafclaw/timeline.db`, modifiable via dashboard)
2. **Environment variables** (prefix: `KAFCLAW_`)
3. **Config file** (`~/.kafclaw/config.json`)
4. **Default values** (hardcoded in `DefaultConfig()`)

Key environment variables:

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | OpenAI API key |
| `OPENROUTER_API_KEY` | OpenRouter API key |
| `KAFCLAW_AGENTS_MODEL` | Model (default: `anthropic/claude-sonnet-4-5`) |
| `KAFCLAW_AGENTS_WORKSPACE` | Workspace directory |
| `KAFCLAW_AGENTS_WORK_REPO_PATH` | Work repo directory |

---

## 9. FAQ / Troubleshooting

### "Config not found"

Run `kafclaw onboard` to create `~/.kafclaw/config.json`.

### "API Key not found"

Set via environment variable or config file:

```bash
export OPENAI_API_KEY="sk-..."
```

Fallback chain: `KAFCLAW_OPENAI_API_KEY` > `OPENAI_API_KEY` > `OPENROUTER_API_KEY` > config.json.

### Port already in use

```bash
make rerun   # kills existing processes on 18790/18791, rebuilds, starts
```

### WhatsApp QR code not showing

QR code saved as image file: `~/.kafclaw/whatsapp-qr.png`. Open with an image viewer.

### WhatsApp session already linked

Session stored in `~/.kafclaw/whatsapp.db`. To re-link, delete this file and restart.

### Daily token quota exceeded

Options: wait until tomorrow (resets daily), increase `daily_token_limit` via dashboard settings, or clear the setting for unlimited.

### Messages not being delivered

Check if silent mode is enabled in the dashboard Settings panel. Disable it, or enable `force_send` for specific web users.

For Slack/Teams also check:

- Bridge process is running and reachable from KafClaw outbound URL.
- Inbound token matches between bridge and `channels.<provider>.inboundToken`.
- Provider auth is valid:
  - Slack signing secret/token
  - Teams bearer + app credentials/JWKS validation
- `kafclaw status` for per-account capabilities/diagnostics and policy warnings.

### Slack request rejected (401/403)

Most common causes:

- `SLACK_SIGNING_SECRET` mismatch
- stale request timestamp
- invalid Slack bot token/app token

Use bridge `/slack/probe` and `kafclaw status` to verify credentials and account diagnostics.

### Teams request rejected (401/403)

Most common causes:

- invalid `MSTEAMS_INBOUND_BEARER`
- invalid Bot Framework JWT claims (`aud`, `iss`, `exp`, `nbf`)
- untrusted `serviceUrl` host

Use bridge `/teams/probe` to inspect token claims, permission coverage, and graph capability checks.

### "Max iterations reached"

Agent hit the tool call limit (default: 20). Simplify your request or increase `maxToolIterations` in config.json.

### Docker deployment

```bash
make docker-up      # build and start
make docker-logs    # view logs
make docker-down    # stop
```
