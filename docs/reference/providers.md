---
title: LLM Providers
parent: Reference
nav_order: 4
---

# LLM Providers

KafClaw supports multiple LLM backends through a unified provider layer. Each provider is identified by a canonical ID and accessed via a model string in the format `<provider-id>/<model-name>`.

## Provider Matrix

| Provider ID | Auth Method | Default API Base | Example Model String |
|---|---|---|---|
| `claude` | API key | `https://api.anthropic.com/v1` | `claude/claude-sonnet-4-5` |
| `openai` | API key | _(must be set)_ | `openai/gpt-4o` |
| `gemini` | API key | Google AI Studio | `gemini/gemini-2.5-pro` |
| `gemini-cli` | OAuth (CLI) | _(via Gemini CLI)_ | `gemini-cli/gemini-2.5-pro` |
| `openai-codex` | OAuth (CLI) | _(via Codex CLI)_ | `openai-codex/gpt-5.3-codex` |
| `xai` | API key | `https://api.x.ai/v1` | `xai/grok-3` |
| `scalytics-copilot` | API key + base URL | _(must be set)_ | `scalytics-copilot/default` |
| `openrouter` | API key | `https://openrouter.ai/api/v1` | `openrouter/anthropic/claude-sonnet-4-5` |
| `deepseek` | API key | `https://api.deepseek.com/v1` | `deepseek/deepseek-chat` |
| `groq` | API key | `https://api.groq.com/openai/v1` | `groq/llama-3.3-70b` |
| `vllm` | API key (optional) + base URL | _(must be set)_ | `vllm/my-model` |

### Provider Aliases

These shorthand names resolve automatically:

| Alias | Resolves To |
|---|---|
| `anthropic` | `claude` |
| `google` | `gemini-cli` (or `gemini` if an API key is configured) |
| `codex` | `openai-codex` |
| `copilot` | `scalytics-copilot` |
| `grok` | `xai` |

## Model String Format

All model references use the format `provider-id/model-name`:

```
claude/claude-opus-4-6
openai/gpt-4o
groq/llama-3.3-70b
openrouter/anthropic/claude-sonnet-4-5   # three segments for OpenRouter
```

A bare model name without a provider prefix (e.g. `gpt-4o`) falls back to the legacy OpenAI provider path.

## Authentication

### API Key Providers

Store an API key:

```bash
kafclaw models auth set-key --provider claude --key sk-ant-...
kafclaw models auth set-key --provider openai --key sk-...
kafclaw models auth set-key --provider gemini --key AIza...
kafclaw models auth set-key --provider xai --key xai-...
kafclaw models auth set-key --provider openrouter --key sk-or-...
kafclaw models auth set-key --provider deepseek --key sk-...
kafclaw models auth set-key --provider groq --key gsk_...
```

For providers that need a custom base URL:

```bash
kafclaw models auth set-key --provider scalytics-copilot --key <token> --base https://copilot.scalytics.io/v1
kafclaw models auth set-key --provider vllm --key <optional> --base http://localhost:8000/v1
```

API keys can also be set via config:

```bash
kafclaw config set providers.anthropic.apiKey sk-ant-...
kafclaw config set providers.openai.apiKey sk-...
```

### OAuth Providers

Gemini and Codex use CLI-based OAuth:

```bash
kafclaw models auth login --provider gemini
kafclaw models auth login --provider openai-codex
```

This installs the provider CLI if absent, then delegates to its auth flow. Credentials are cached by the respective CLI and read at runtime.

## Provider Resolution

When KafClaw needs an LLM provider, it resolves in this order:

1. **Per-agent model** (`agents.list[].model.primary`) — highest priority
2. **Task-type routing** (`model.taskRouting[category]`) — if no per-agent model is set
3. **Global model** (`model.name`) — default fallback
4. **Legacy OpenAI** (`providers.openai`) — backward compatibility

### Per-Agent Configuration

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "model": {
          "primary": "claude/claude-opus-4-6",
          "fallbacks": ["openai/gpt-4o", "groq/llama-3.3-70b"]
        },
        "subagents": {
          "model": "groq/llama-3.3-70b"
        }
      }
    ]
  }
}
```

### Task-Type Routing

Route messages to different models based on content classification:

```json
{
  "model": {
    "name": "claude/claude-sonnet-4-5",
    "taskRouting": {
      "security": "claude/claude-opus-4-6",
      "coding": "openai-codex/gpt-5.3-codex",
      "creative": "openai/gpt-4o",
      "tool-heavy": "openai-codex/gpt-5.3-codex"
    }
  }
}
```

Task categories are detected automatically from message content: `security`, `coding`, `tool-heavy`, `creative`.

### Fallback Chains

When the primary provider returns a transient error, fallbacks are tried in order:

```json
{
  "model": {
    "primary": "claude/claude-opus-4-6",
    "fallbacks": [
      "openai/gpt-4o",
      "deepseek/deepseek-chat"
    ]
  }
}
```

### Subagent Model Inheritance

Subagents resolve their model in this order:

1. `agents.list[parentID].subagents.model`
2. `tools.subagents.model` (global subagent default)
3. Inherit parent agent's resolved model

## Rate Limits

Rate limit data is extracted from provider response headers (both OpenAI-style `x-ratelimit-*` and Anthropic-style `anthropic-ratelimit-*` headers) and cached in memory per provider.

View current rate limit snapshots:

```bash
kafclaw models stats
kafclaw status
```

`kafclaw doctor` warns when any provider's remaining tokens drop below 10% of its limit.

## Verifying Setup

```bash
# List all configured providers
kafclaw models list

# Check provider health
kafclaw doctor

# View today's usage per provider
kafclaw models stats

# Multi-day trend
kafclaw models stats --days 7
```

## Related Docs

- [Models CLI Reference](/reference/models-cli/)
- [Middleware Configuration](/reference/middleware/)
- [Configuration Keys](/reference/config-keys/)
