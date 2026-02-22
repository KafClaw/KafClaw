---
title: Getting Started Guide
parent: Getting Started
nav_order: 1
---

# Getting Started

This guide gets KafClaw from zero to a working setup, with focus on onboarding and workspace identity files.

## 1. Quick Install (Release Binary)

Fast path (downloads latest matching binary from GitHub Releases, verifies checksum + signature, installs completion):

```bash
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --latest
```

Reload shell config so `kafclaw` and completion are available in the current terminal:

```bash
source ~/.zshrc   # or: source ~/.bashrc
```

Quick verify:

```bash
kafclaw version
# If you already onboarded:
kafclaw status
```

Headless/unattended:

```bash
# Latest
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --unattended --latest

# Pinned version
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --unattended --version v2.6.3
```

List available releases first:

```bash
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --list-releases
```

For full install flags and operator-focused behavior (root handling, signature controls, unattended modes), see [KafClaw Management Guide](/operations-admin/manage-kafclaw/).

Desktop app distribution:

- The Electron desktop app is shipped as prebuilt release artifacts (for example `.dmg`, `.exe`, Linux package formats).
- Download/install it separately from GitHub Releases when you want the desktop experience.
- Default gateway/headless onboarding/install does not auto-install Electron dependencies.

## 2. Prerequisites (Source Build Path)

- Go `1.24+`
- Git
- Optional for group mode: Kafka reachable from your machine
- Optional for desktop: Electron dependencies

## 3. Installing from Source

This path is for developers who want to build KafClaw locally from source (for code changes, debugging, or running unreleased updates).

```bash
git clone https://github.com/KafClaw/KafClaw.git
cd KafClaw
make check
make build
```

Binary target: `./kafclaw`

`make check` enforces Go `>=1.24` before build.

## 4. Onboard (Mode + LLM)

Run onboarding wizard:

```bash
./kafclaw onboard
```

Onboarding does three things:

1. Creates or updates `~/.kafclaw/config.json`
2. Applies runtime profile and provider settings
3. Scaffolds workspace identity files:
   - `AGENTS.md`
   - `SOUL.md`
   - `USER.md`
   - `TOOLS.md`
   - `IDENTITY.md`

You will be asked for:

- runtime mode: `local`, `local-kafka`, or `remote`
- LLM setup: `claude`, `openai`, `gemini`, `gemini-cli`, `openai-codex`, `xai`, `scalytics-copilot`, `openrouter`, `deepseek`, `groq`, `openai-compatible`, `cli-token`, or `skip`

Before writing config, onboarding shows a summary and asks for confirmation.

### Non-interactive examples

Local:

```bash
./kafclaw onboard --non-interactive --profile local --llm skip
```

Local + Kafka:

```bash
./kafclaw onboard --non-interactive --profile local-kafka --kafka-brokers localhost:9092 --group-name kafclaw --agent-id agent-local --role worker --llm skip
```

Local + Kafka + SASL/SSL:

```bash
./kafclaw onboard --non-interactive --profile local-kafka --llm skip \
  --kafka-brokers "broker1:9092,broker2:9092" \
  --kafka-security-protocol SASL_SSL \
  --kafka-sasl-mechanism SCRAM-SHA-512 \
  --kafka-sasl-username "<username>" \
  --kafka-sasl-password "<password>" \
  --kafka-tls-ca-file "/path/to/ca.pem"
```

Kafka auth via KafScale proxy key (SASL/PLAIN over SSL auto-derived by `kshark --auto`):

```bash
./kafclaw config set group.lfsProxyUrl "https://your-kafscale-endpoint"
./kafclaw config set group.lfsProxyApiKey "<kafscale-api-key>"
```

Kafka auth via direct broker settings (Confluent/Redpanda-style SASL/SSL), post-onboarding:

```bash
./kafclaw config set group.kafkaSecurityProtocol "SASL_SSL"
./kafclaw config set group.kafkaSaslMechanism "PLAIN"
./kafclaw config set group.kafkaSaslUsername "<username>"
./kafclaw config set group.kafkaSaslPassword "<password>"
./kafclaw config set group.kafkaTlsCAFile "/path/to/ca.pem"
```

Remote + Ollama/vLLM:

```bash
./kafclaw onboard --non-interactive --profile remote --llm openai-compatible --llm-api-base http://localhost:11434/v1 --llm-model llama3.1:8b
```

### LLM Provider Setup (Interactive)

Run:

```bash
./kafclaw onboard
```

Then choose your LLM provider:

**API key providers** — prompts for API key, sets model and base URL automatically:
- `claude` — Anthropic Claude (default model: `claude/claude-sonnet-4-5`)
- `openai` — OpenAI (default model: `openai/gpt-4o`)
- `gemini` — Google Gemini via API key (default model: `gemini/gemini-2.5-pro`)
- `xai` — xAI/Grok (default model: `xai/grok-3`)
- `openrouter` — OpenRouter (default model: `openrouter/anthropic/claude-sonnet-4-5`)
- `deepseek` — DeepSeek (default model: `deepseek/deepseek-chat`)
- `groq` — Groq (default model: `groq/llama-3.3-70b-versatile`)
- `scalytics-copilot` — Scalytics Copilot (prompts for base URL)

**OAuth providers** — delegates to CLI auth flow:
- `gemini-cli` — Google Gemini via CLI OAuth
- `openai-codex` — OpenAI Codex via CLI OAuth

**Generic / Legacy:**
- `cli-token` — prompts for API token, defaults to OpenRouter base
- `openai-compatible` — prompts for API base and token (Ollama/vLLM/self-hosted)
- `skip` — keeps current provider settings

### Post-Onboarding Provider Management

After onboarding, manage providers with the `models` command:

```bash
# List configured providers
kafclaw models list

# Add another provider
kafclaw models auth set-key --provider groq --key gsk_...

# OAuth login
kafclaw models auth login --provider gemini

# Check usage
kafclaw models stats
```

See [LLM Providers](/reference/providers/) and [Models CLI](/reference/models-cli/) for full details.

To reconfigure provider/token later, run onboarding again (interactive) or use:

```bash
./kafclaw config set providers.openai.apiKey "<token>"
./kafclaw config set providers.openai.apiBase "https://openrouter.ai/api/v1"
./kafclaw config set model.name "anthropic/claude-sonnet-4-5"
```

### Mode behavior (as configured by onboarding)

| Mode | Gateway bind | Group | Orchestrator | Auth token |
|------|--------------|-------|--------------|------------|
| `local` | `127.0.0.1` | disabled | disabled | none |
| `local-kafka` | `127.0.0.1` | enabled | enabled | none |
| `remote` | `0.0.0.0` | disabled | disabled | required (auto-generated if missing) |

### Existing workspace files

- Default behavior: existing files are kept
- Use `--force` to overwrite existing config and identity templates
- Gateway also auto-scaffolds missing identity files at startup if workspace is incomplete

Minimal lifecycle flags:

```bash
./kafclaw onboard --reset-scope config --non-interactive --accept-risk --profile local --llm skip
./kafclaw onboard --wait-for-gateway --health-timeout 20s
```

## 5. Verify

```bash
./kafclaw status
./kafclaw doctor
```

Optional hygiene fix:

```bash
./kafclaw doctor --fix
```

This merges discovered env files into `~/.config/kafclaw/env` and enforces mode `600`.

## 6. Run

Local gateway:

```bash
./kafclaw gateway
```

Check:

- API: `http://localhost:18790`
- Dashboard: `http://localhost:18791`

Single prompt test:

```bash
./kafclaw agent -m "hello"
```

## 7. Systemd Setup (Linux)

To install systemd unit/override/env during onboarding:

```bash
sudo ./kafclaw onboard --systemd
```

This can create service user, install unit files, and write runtime env file.

After onboarding, manage service state with:

```bash
sudo ./kafclaw daemon status
sudo ./kafclaw daemon restart
```

## 8. Where Config Lives

- Main config: `~/.kafclaw/config.json`
- Runtime env: `~/.config/kafclaw/env`
- State DB: `~/.kafclaw/timeline.db`
- Workspace identity files: `<workspace>/{AGENTS.md,SOUL.md,USER.md,TOOLS.md,IDENTITY.md}`

## 9. Subagents (Phase 2)

KafClaw now supports sub-agent spawning through tools used by the agent loop:

- `sessions_spawn`: spawn a background child run
- `subagents`: list, kill, and steer child runs for the current parent session
- `subagents(action=kill_all)`: stop all active child runs for the current parent session
- `agents_list`: discover allowed `agentId` targets for `sessions_spawn`

Steering behavior in v1:

- `subagents(action=steer)` safely stops the target run (if still active)
- then spawns a new child run with the steering input appended to the original task
- control scope is root-session scoped (nested child sessions can manage sibling runs within the same root request)
- target selectors for `kill`/`steer`: run ID, `last`, numeric index, label prefix, or child session key

Timeout support:

- `sessions_spawn` accepts `runTimeoutSeconds` to hard-stop long child runs
- `sessions_spawn` also accepts `timeoutSeconds` as compatibility alias
- `sessions_spawn(cleanup=delete)` deletes child session transcript after successful announce delivery

Default safety limits:

- `tools.subagents.maxSpawnDepth = 1` (no nested subagent-of-subagent by default)
- `tools.subagents.maxChildrenPerAgent = 5` (max active children per parent session)
- `tools.subagents.maxConcurrent = 8` (max active subagents globally)
- `tools.subagents.archiveAfterMinutes = 60` (retention/cleanup default)

Optional defaults for spawned children:

- `tools.subagents.model` (pin a default child model)
- `tools.subagents.thinking` (default thinking level tag)
- `tools.subagents.tools.allow` / `tools.subagents.tools.deny` (child tool policy)

Onboarding supports direct subagent tuning flags:

```bash
./kafclaw onboard --subagents-max-spawn-depth 2 --subagents-max-children 6 --subagents-max-concurrent 8 --subagents-allow-agents agent-main,agent-research --subagents-model anthropic/claude-sonnet-4-5 --subagents-thinking medium
```

Audit hardening:

- spawn, kill, and steer operations are written as timeline `SYSTEM` events (`SUBAGENT` classification) when trace IDs are present
- subagent run registry is persisted under `~/.kafclaw/subagents/` and restored after restart
- completion announce messages are normalized to `Status/Result/Notes`; `ANNOUNCE_SKIP` suppresses the announce
- announce delivery tracks retry/backoff state and retries deferred announcements after restart
- announce target routing uses requester channel/chat first, then session-key fallback (`requester -> root -> parent -> active`)

## 9. Configure (Post-Onboarding)

Use guided configuration updates:

```bash
./kafclaw configure
./kafclaw configure --subagents-allow-agents agent-main,agent-research --non-interactive
```

Clear allowlist (back to current-agent-only default):

```bash
./kafclaw configure --clear-subagents-allow-agents --non-interactive
```

## 10. Kafka Broker Connection Examples

Direct config path:

```bash
./kafclaw config set group.enabled true
./kafclaw config set group.groupName "kafclaw"
./kafclaw config set group.kafkaBrokers "broker1:9092,broker2:9092"
./kafclaw config set group.consumerGroup "kafclaw-workers"
./kafclaw config set group.agentId "agent-local"
```

Join and verify:

```bash
./kafclaw group join kafclaw
./kafclaw group status
./kafclaw group members
./kafclaw kshark --auto --yes
```

## Next

- [Manage KafClaw](/operations-admin/manage-kafclaw/)
- [Operations and Maintenance](/operations-admin/maintenance/)
- [How Agents Work](/agent-concepts/how-agents-work/)
- [Soul and Identity Files](/agent-concepts/soul-identity-tools/)
- [User Manual](/start-here/user-manual/)
- [Admin Guide](/operations-admin/admin-guide/)
