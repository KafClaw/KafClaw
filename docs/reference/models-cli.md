---
title: Models CLI
parent: Reference
nav_order: 6
---

# kafclaw models

Manage LLM providers, authentication, and usage statistics.

## Subcommands

| Command | Description |
|---|---|
| `kafclaw models list` | Show configured providers and active model per agent |
| `kafclaw models stats` | Show token usage and cost statistics |
| `kafclaw models auth login` | Authenticate with an OAuth-based provider |
| `kafclaw models auth set-key` | Store an API key for a provider |

---

## kafclaw models list

Shows all supported providers, their configuration status, the global model, and per-agent model assignments.

```bash
kafclaw models list
```

Example output:

```
Configured Providers:

  claude               configured
  openai               configured
  gemini               not configured
  gemini-cli           OAuth
  openai-codex         OAuth
  xai                  not configured
  scalytics-copilot    not configured
  openrouter           configured  base: https://openrouter.ai/api/v1
  deepseek             not configured
  groq                 configured
  vllm                 not configured

Global model: claude/claude-sonnet-4-5
Agent main         model: claude/claude-opus-4-6
  fallback[0]: openai/gpt-4o
  subagent model: groq/llama-3.3-70b
```

---

## kafclaw models stats

Shows today's token usage and cost per provider, rate limit snapshots, and optionally a multi-day trend.

```bash
# Today's usage
kafclaw models stats

# 7-day per-provider trend
kafclaw models stats --days 7

# JSON output
kafclaw models stats --json
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--days` | int | 0 | Show per-day per-provider trend for N days |
| `--json` | bool | false | Output in JSON format |

### Example output (today)

```
Today's token usage: 12450

PROVIDER             TOKENS     COST
claude               8200       $0.1640
openai               3500       $0.0700
groq                 750        $0.0002

PROVIDER             REMAINING TOK   REMAINING REQ   LIMIT TOK       RESET
claude               42000           98              50000           14:30:00
openai               87000           450             100000          15:00:00
```

### Example output (--days 7)

```
PROVIDER        DAY          TOKENS     COST
claude          2026-02-22   8200       $0.1640
claude          2026-02-21   15300      $0.3060
openai          2026-02-22   3500       $0.0700
openai          2026-02-21   6200       $0.1240
groq            2026-02-22   750        $0.0002
```

---

## kafclaw models auth login

Authenticates with providers that use CLI-based OAuth flows. Installs the provider CLI if not already present.

```bash
kafclaw models auth login --provider <provider>
```

### Supported providers

| Provider | CLI installed | Auth flow |
|---|---|---|
| `gemini` | Gemini CLI (`gemini`) | `gemini auth login` |
| `openai-codex` | Codex CLI (`codex`) | `codex auth` |

### Example

```bash
kafclaw models auth login --provider gemini
```

---

## kafclaw models auth set-key

Stores an API key or bearer token for a provider in the encrypted credential store.

```bash
kafclaw models auth set-key --provider <provider> --key <token> [--base <url>]
```

### Flags

| Flag | Required | Description |
|---|---|---|
| `--provider` | yes | Provider ID: `claude`, `openai`, `gemini`, `xai`, `scalytics-copilot`, `openrouter`, `deepseek`, `groq`, `vllm` |
| `--key` | yes | API key or bearer token |
| `--base` | for `scalytics-copilot`, `vllm` | Base URL for the provider API |

### Examples

```bash
# Anthropic / Claude
kafclaw models auth set-key --provider claude --key sk-ant-api03-...

# Self-hosted vLLM
kafclaw models auth set-key --provider vllm --key optional-key --base http://gpu-server:8000/v1

# ScalyticsCopilot
kafclaw models auth set-key --provider scalytics-copilot --key <bearer-token> --base https://copilot.scalytics.io/v1
```

---

## Related Docs

- [LLM Providers](/reference/providers/)
- [Chat Middleware](/reference/middleware/)
- [Configuration Keys](/reference/config-keys/)
