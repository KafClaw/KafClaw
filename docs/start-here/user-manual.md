---
title: User Manual
parent: Getting Started
nav_order: 2
---

# KafClaw User Manual

A comprehensive guide to installing, configuring, and using KafClaw - a personal AI assistant framework written in Go with an Electron desktop frontend.

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

Release installer (recommended):

```bash
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --latest
```

Headless/unattended:

```bash
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --unattended --latest
```

Pinned version:

```bash
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --version v2.6.3
```

Source build path:

```bash
cd KafClaw
make build
```

For complete install options (`--list-releases`, signature verification defaults, root/runtime behavior), see [KafClaw Management Guide](/operations-admin/manage-kafclaw/).

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
- **API server** - `http://localhost:18790`
- **Web dashboard** - `http://localhost:18791`
- **WhatsApp** - connects automatically if configured

### Logic Flow

<style>
  .kc-logic-wrap {
    margin: 16px 0 8px;
    border: 1px solid #d9e2f0;
    border-radius: 14px;
    background: linear-gradient(160deg, #f7fbff 0%, #eef4ff 100%);
    padding: 16px;
  }
  .kc-logic-intro {
    margin: 0 0 12px;
    color: #27405f;
    font-size: 0.96rem;
  }
  .kc-logic-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 10px;
  }
  .kc-logic-step {
    position: relative;
    min-height: 124px;
    border-radius: 12px;
    border: 1px solid #c9d8ef;
    background: #ffffff;
    padding: 10px 10px 12px;
    box-shadow: 0 4px 14px rgba(24, 58, 106, 0.08);
  }
  .kc-logic-step strong {
    display: block;
    color: #103f7a;
    margin-bottom: 5px;
    font-size: 0.92rem;
  }
  .kc-logic-step p {
    margin: 0;
    color: #314e72;
    font-size: 0.86rem;
    line-height: 1.38;
  }
  .kc-logic-badge {
    display: inline-block;
    margin-bottom: 6px;
    border-radius: 999px;
    padding: 2px 8px;
    background: #e8f0ff;
    color: #174487;
    font-size: 0.75rem;
    font-weight: 700;
  }
  @media (max-width: 1050px) {
    .kc-logic-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }
  @media (max-width: 640px) {
    .kc-logic-wrap {
      padding: 12px;
    }
    .kc-logic-grid {
      grid-template-columns: 1fr;
      gap: 9px;
    }
    .kc-logic-step {
      min-height: 0;
    }
  }
</style>

<div class="kc-logic-wrap" role="img" aria-label="KafClaw request processing logic flow infographic">
  <p class="kc-logic-intro">
    The runtime always follows this order, with tool usage looping until the model is ready to answer.
  </p>
  <div class="kc-logic-grid">
    <article class="kc-logic-step">
      <span class="kc-logic-badge">Step 1</span>
      <strong>Input</strong>
      <p>A message enters from WhatsApp, CLI, Web UI, or scheduler.</p>
    </article>
    <article class="kc-logic-step">
      <span class="kc-logic-badge">Step 2</span>
      <strong>Bus</strong>
      <p>The inbound envelope is published on the message bus.</p>
    </article>
    <article class="kc-logic-step">
      <span class="kc-logic-badge">Step 3</span>
      <strong>Dedup</strong>
      <p>Idempotency keys block duplicate processing.</p>
    </article>
    <article class="kc-logic-step">
      <span class="kc-logic-badge">Step 4</span>
      <strong>Context</strong>
      <p>System prompt is built from soul files, memory layers, skills, and RAG context.</p>
    </article>
    <article class="kc-logic-step">
      <span class="kc-logic-badge">Step 5</span>
      <strong>Processing</strong>
      <p>The LLM plans response strategy and decides whether tools are needed.</p>
    </article>
    <article class="kc-logic-step">
      <span class="kc-logic-badge">Step 6</span>
      <strong>Tool Loop</strong>
      <p>Policy checks run, tools execute, and results return to the model, up to 20 iterations.</p>
    </article>
    <article class="kc-logic-step">
      <span class="kc-logic-badge">Step 7</span>
      <strong>Post-Processing</strong>
      <p>Session is updated, memory indexing runs, and observer jobs are queued.</p>
    </article>
    <article class="kc-logic-step">
      <span class="kc-logic-badge">Step 8</span>
      <strong>Delivery</strong>
      <p>Response is published outbound and delivered by the active channel.</p>
    </article>
  </div>
</div>

---

## 3. CLI Reference

KafClaw provides the following CLI commands. Run `kafclaw --help` for the full list.
Core startup commands: `onboard`, `doctor`, `status`, `gateway`, `daemon`, `agent`, `config`.

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
- `--reset-scope` (`none|config|full`) for deterministic reset behavior
- `--wait-for-gateway` and `--health-timeout` for post-onboard health gating
- `--skip-healthcheck` to bypass readiness checks in constrained automation
- `--daemon-runtime` to persist daemon runtime label in config
- `--subagents-max-spawn-depth`, `--subagents-max-children`, `--subagents-max-concurrent`
- `--subagents-archive-minutes`, `--subagents-model`, `--subagents-thinking`

Service lifecycle commands:

```bash
sudo kafclaw daemon install --activate
sudo kafclaw daemon status
sudo kafclaw daemon restart
```

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

Install the current local binary:

- root: `/usr/local/bin`
- non-root: `~/.local/bin`

```bash
kafclaw install
```

Generate shell completion:

```bash
kafclaw completion zsh
kafclaw completion bash
```

### 3.6 `doctor`

Run diagnostics for config, env, and runtime safety defaults.

```bash
kafclaw doctor
kafclaw doctor --fix
kafclaw doctor --generate-gateway-token
```

Includes Slack/Teams account configuration diagnostics checks.
Also validates memory embedding configuration. If missing/disabled, `doctor` fails and
`kafclaw doctor --fix` applies default embedding settings.

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
kafclaw configure --subagents-memory-share-mode handoff --non-interactive
kafclaw configure --non-interactive --kafka-brokers "broker1:9092,broker2:9092" --kafka-security-protocol SASL_SSL --kafka-sasl-mechanism SCRAM-SHA-512 --kafka-sasl-username "<username>" --kafka-sasl-password "<password>" --kafka-tls-ca-file "/path/to/ca.pem"
kafclaw configure --non-interactive --memory-embedding-enabled-set --memory-embedding-enabled=true --memory-embedding-provider local-hf --memory-embedding-model BAAI/bge-small-en-v1.5 --memory-embedding-dimension 384
kafclaw configure --non-interactive --memory-embedding-model BAAI/bge-base-en-v1.5 --confirm-memory-wipe
```

Subagent memory modes:
- `isolated`: child session is isolated and no automatic parent handoff is written
- `handoff` (default): child session stays isolated and completion handoff is appended to parent session
- `inherit-readonly`: child gets read-only parent snapshot and still writes handoff to parent

Memory embedding safety rules:
- Embedding config is mandatory in normal mode (`configure` rejects disabling/empty provider/model/dimension).
- First-time embedding enable does not wipe existing text-only memory rows.
- Switching an already-used embedding requires `--confirm-memory-wipe`; then memory chunks are wiped before saving.

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

- **Timeline View** (`/timeline`) - Full conversation history with trace IDs, sender info, event types, classifications. Auto-refreshes every 5 seconds.

- **Trace Viewer** - Drill into individual request flows: inbound, outbound, LLM, and tool execution spans. Task metadata (token counts, delivery status) and policy decisions.

- **Memory Dashboard** - Layer stats, observer status, ER1 sync status, expertise tracker, working memory preview.

- **Repository Browser** - File tree, content viewing, Git diff, commit, pull, push, branch checkout, PR creation via `gh`.

- **Web Chat** - Send messages from the browser. Supports web user management and WhatsApp JID linking.

- **Settings Panel** - Runtime settings including silent mode. Changes take effect immediately.

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

> See also: [WhatsApp Setup](/integrations/whatsapp-setup/) for full details

KafClaw uses `whatsmeow` for native Go WhatsApp connectivity. No Node.js bridge required.

### Setup Flow

1. `kafclaw whatsapp-setup` - Enable and configure
2. `kafclaw gateway` - Start the daemon
3. Scan QR code at `~/.kafclaw/whatsapp-qr.png` with WhatsApp (Settings > Linked Devices)
4. Session persists in `~/.kafclaw/whatsapp.db` - auto-reconnects on restart

### Authorization Model

Three-tier JID system (default-deny):

- **Allowlist** - Authorized to interact. Messages processed normally.
- **Denylist** - Explicitly blocked. Messages silently dropped.
- **Pending** - Unknown senders held until admin approves/denies.

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

> See also: [Timeline Architecture](/architecture-security/architecture-timeline/) for the full memory architecture
> Visual: [Memory Architecture and Notes](/agent-concepts/memory-notes/)

### Overview

The 6-layer memory system initializes automatically when the LLM provider supports embeddings. On startup the gateway logs whether memory is active.

### Agent Tools

**`remember`** - Store information in long-term memory.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | Yes | Information to remember |
| `tags` | string | No | Comma-separated tags |

**`recall`** - Search memory for relevant information.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | Yes | Search query |
| `limit` | integer | No | Max results (default: 5) |

**`update_working_memory`** - Update the per-user scratchpad.

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
| `dtu [text]` | Update - add task or enter capture mode |
| `dtp [text]` | Progress - log progress or enter capture mode |
| `dts` | Summarize - consolidate today's tasks |
| `dtn` | Next - suggest next task to work on |
| `dta` | All - list all open tasks as prioritized plan |
| `dtc` | Close - submit buffered content from capture mode |

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

Default: `~/.kafclaw/workspace/` - the agent's state home.

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
- `memory/` - Memory files (MEMORY.md, daily notes)
- Specification docs - Behavior specifications
- `tasks/` - Plans and milestones
- `docs/` - Explanations and summaries

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
