---
title: Chat Middleware
parent: Reference
nav_order: 5
---

# Chat Middleware

KafClaw runs a configurable middleware chain between the agent loop and the LLM provider. Middleware can inspect, transform, reroute, or block messages before and after each LLM call.

## Architecture

```
User Message
    |
    v
[Content Classifier] --> tags: sensitivity, task type; may reroute provider
[Prompt Guard]       --> scans for PII/secrets; may warn, redact, or block
    |
    v
  LLM Provider (Chat)
    |
    v
[Output Sanitizer]   --> redacts PII/secrets in response; enforces deny patterns
[FinOps Recorder]    --> calculates per-request cost; budget warnings
    |
    v
Channel Delivery
```

Each middleware implements `ProcessRequest` (pre-LLM) and `ProcessResponse` (post-LLM). The chain is zero-overhead when no middleware is configured.

## Content Classification

Classifies messages by sensitivity level and task type. Can reroute to a different model based on content.

### Config

```json
{
  "contentClassification": {
    "enabled": true,
    "sensitivityLevels": {
      "pii": {
        "keywords": ["social security", "passport number"],
        "patterns": ["\\b\\d{3}-\\d{2}-\\d{4}\\b"],
        "routeTo": "claude/claude-opus-4-6"
      },
      "confidential": {
        "keywords": ["confidential", "internal only"],
        "routeTo": "vllm/private-model"
      }
    },
    "taskTypeRoutes": {
      "security": "claude/claude-opus-4-6",
      "coding": "openai-codex/gpt-5.3-codex"
    }
  }
}
```

### Fields

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Enable content classification |
| `sensitivityLevels` | map | Named sensitivity classes with keywords, patterns, and optional model reroute |
| `taskTypeRoutes` | map | Task category to model string routing (`security`, `coding`, `tool-heavy`, `creative`) |

## Prompt Guard

Scans inbound user messages for PII, secrets, and deny-listed keywords before they reach the LLM. Three action modes: `warn` (tag only), `redact` (replace matches with `[REDACTED]`), or `block` (abort the request).

### Config

```json
{
  "promptGuard": {
    "enabled": true,
    "mode": "redact",
    "pii": {
      "detect": ["email", "phone", "ssn", "credit_card", "ip_address"],
      "action": "redact",
      "customPatterns": [
        {"name": "employee_id", "pattern": "EMP-\\d{6}"}
      ]
    },
    "secrets": {
      "detect": ["api_key", "bearer_token", "private_key", "password_literal"],
      "action": "block"
    },
    "denyKeywords": ["DROP TABLE", "rm -rf"],
    "customPatterns": [
      {"name": "internal_url", "pattern": "https://internal\\.corp\\.\\S+"}
    ]
  }
}
```

### Fields

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Enable prompt guard |
| `mode` | string | Default action: `warn`, `redact`, or `block` |
| `pii.detect` | []string | PII types to scan: `email`, `phone`, `ssn`, `credit_card`, `ip_address` |
| `pii.action` | string | Override action for PII matches (default: inherits `mode`) |
| `pii.customPatterns` | []NamedPattern | Additional PII patterns |
| `secrets.detect` | []string | Secret types: `api_key`, `bearer_token`, `private_key`, `password_literal` |
| `secrets.action` | string | Override action for secret matches |
| `secrets.customPatterns` | []NamedPattern | Additional secret patterns |
| `denyKeywords` | []string | Keywords that always block (case-insensitive) |
| `customPatterns` | []NamedPattern | Additional patterns for both PII and secrets |

### Behavior

- **Deny keywords** are checked first and always block, regardless of `mode`.
- **PII action** overrides `mode` for PII matches. **Secret action** overrides for secret matches.
- When `mode` is `redact`, matched content is replaced with `[REDACTED:<type>]` before the LLM sees it.
- Blocked requests return a `[blocked by prompt-guard]` response to the user.
- All prompt guard actions are logged as `SECURITY` events in the timeline.

## Output Sanitizer

Scans LLM responses for PII, secrets, and deny patterns before channel delivery.

### Config

```json
{
  "outputSanitization": {
    "enabled": true,
    "redactPII": true,
    "redactSecrets": true,
    "customRedactPatterns": [
      {"name": "internal_ip", "pattern": "10\\.\\d+\\.\\d+\\.\\d+"}
    ],
    "denyPatterns": [
      "(?i)BEGIN\\s+(RSA|EC|DSA|OPENSSH)\\s+PRIVATE\\s+KEY"
    ],
    "maxOutputLength": 50000
  }
}
```

### Fields

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Enable output sanitization |
| `redactPII` | bool | Redact PII types (email, phone, SSN, credit card, IP) in responses |
| `redactSecrets` | bool | Redact secrets (API keys, bearer tokens, private keys) in responses |
| `customRedactPatterns` | []NamedPattern | Additional patterns to redact |
| `denyPatterns` | []string | Regex patterns that cause the entire response to be replaced with a filter message |
| `maxOutputLength` | int | Truncate responses exceeding this character count (0 = unlimited) |

### Behavior

- **Deny patterns** are checked first. If any match, the entire response is replaced with `[Response filtered by output sanitizer]`.
- **Redaction** replaces matched substrings with `[REDACTED:<type>]`.
- **Truncation** appends `[truncated by output sanitizer]` when the response exceeds `maxOutputLength`.
- All sanitizer actions are logged as `SECURITY` events in the timeline.

## FinOps Cost Attribution

Calculates per-request USD cost from token usage and provider-specific pricing. Logs budget warnings.

### Config

```json
{
  "finops": {
    "enabled": true,
    "pricing": {
      "claude": {
        "promptPer1kTokens": 0.003,
        "completionPer1kTokens": 0.015
      },
      "openai": {
        "promptPer1kTokens": 0.005,
        "completionPer1kTokens": 0.015
      },
      "groq": {
        "promptPer1kTokens": 0.0001,
        "completionPer1kTokens": 0.0002
      }
    },
    "dailyBudget": 10.00,
    "monthlyBudget": 200.00
  }
}
```

### Fields

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Enable cost tracking |
| `pricing` | map | Provider ID to per-1k-token pricing |
| `pricing[].promptPer1kTokens` | float | USD per 1,000 prompt tokens |
| `pricing[].completionPer1kTokens` | float | USD per 1,000 completion tokens |
| `dailyBudget` | float | Max USD per day (0 = unlimited). Logs warnings when a single request exceeds 10% of budget. |
| `monthlyBudget` | float | Max USD per month (0 = unlimited) |

### Cost Formula

```
cost = (promptTokens * promptPer1kTokens + completionTokens * completionPer1kTokens) / 1000
```

### Viewing Costs

```bash
# Today's cost by provider
kafclaw models stats

# Multi-day cost trend
kafclaw models stats --days 7

# JSON output for automation
kafclaw models stats --json
```

## Observability

All middleware actions are logged to the timeline as events:

| Event Type | Classification | When |
|---|---|---|
| `SECURITY` | `BLOCKED` | Prompt guard blocks a request |
| `SECURITY` | `GUARD` | Prompt guard detects but doesn't block (warn/redact mode) |
| `SECURITY` | `SANITIZED` | Output sanitizer redacts or filters a response |
| `SYSTEM` | `ROUTING` | Task-type routing selects a different model |

View events:

```bash
kafclaw status
```

## Related Docs

- [LLM Providers](providers/)
- [Models CLI Reference](models-cli/)
- [Configuration Keys](config-keys/)
