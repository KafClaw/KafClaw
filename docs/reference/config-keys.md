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
| `kafscale_lfs_proxy_url` | LFS proxy URL for shared artifacts |

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

## Common Environment Variables

- `OPENAI_API_KEY`
- `OPENROUTER_API_KEY`
- `KAFCLAW_AGENTS_MODEL`
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

- [Getting Started Guide](../start-here/getting-started/)
- [KafClaw Administration Guide](../operations-admin/admin-guide/)
- [Workspace Policy](../architecture-security/workspace-policy/)
