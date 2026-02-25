---
title: CLI Reference
parent: Reference
nav_order: 1
---

Primary command groups:

- `kafclaw onboard` - onboarding and initial configuration
- `kafclaw gateway` - start API + dashboard + runtime services
- `kafclaw status` - runtime/config health snapshot
- `kafclaw doctor` - diagnostics and setup checks
- `kafclaw security` - security checks, deep audit, and safe remediation (`check|audit|fix`)
- `kafclaw models` - manage LLM providers and models (`list|stats|auth login|auth set-key`)
- `kafclaw config` / `kafclaw configure` - low-level and guided config changes
- `kafclaw agent -m` - one-shot interaction
- `kafclaw skills` - bundled/external skill lifecycle and auth/prereq flows (`enable|disable|list|status|enable-skill|disable-skill|verify|install|update|exec|prereq|auth`)
- `kafclaw install` - install local built binary (`/usr/local/bin` root, `~/.local/bin` non-root)
- `kafclaw update` - update lifecycle (`plan`, `apply`, `backup`, `rollback`)
- `kafclaw daemon` - system service lifecycle (`install`, `uninstall`, `start`, `stop`, `restart`, `status`)
- `kafclaw completion` - generate shell completion scripts
- `kafclaw whatsapp-setup` / `kafclaw whatsapp-auth` - WhatsApp setup and auth controls
- `kafclaw pairing` - Slack/Teams pairing approvals
- `kafclaw group` - group communication controls
- `kafclaw knowledge` - shared knowledge governance (`status|propose|vote|decisions|facts`)
- `kafclaw task` - cascading task protocol visibility (`status --trace <id>`)
- `kafclaw kshark` - Kafka diagnostics
- `kafclaw version` - print build version

Automation-friendly lifecycle output:
- `kafclaw onboard --json`
- `kafclaw install --json`
- `kafclaw configure --json`
- `kafclaw doctor --json`
- `kafclaw security <check|audit|fix> --json`
- `kafclaw update <plan|backup|apply|rollback> --json`
- `kafclaw daemon <install|uninstall|start|stop|restart|status> --json`

Detailed command examples:
- [Getting Started](/start-here/getting-started/)
- [User Manual - CLI Reference section](/start-here/user-manual/#3-cli-reference)
- [Manage KafClaw](/operations-admin/manage-kafclaw/)
- [Models CLI Reference](/reference/models-cli/) - provider management, auth, usage stats

Memory safety flags:
- `kafclaw doctor --fix` repairs missing memory embedding defaults.
- `kafclaw configure --memory-embedding-enabled-set --memory-embedding-enabled=true --memory-embedding-provider local-hf --memory-embedding-model BAAI/bge-small-en-v1.5 --memory-embedding-dimension 384`
- `kafclaw configure --memory-embedding-model <new-model> --confirm-memory-wipe` when switching an already-used embedding.
- `kafclaw configure --agent-id <agent-id> --agent-cascade-enabled-set --agent-cascade-enabled=<true|false>`

Cascade safety note:
- Enabling per-agent cascade is recommended only for deterministic workflows (ops/runbook/config/code-mod).
- For ambiguous or creative tasks, keep it disabled to avoid extra latency and retry churn.

Knowledge governance notes:
- Knowledge envelopes require `schemaVersion`, `traceId`, `idempotencyKey`, `clawId`, and `instanceId`.
- Duplicate knowledge envelopes (same `idempotencyKey`) are ignored after first apply.
- Voting outcomes follow quorum policy (`approved|rejected|expired|pending`) from `knowledge.voting.*`.

Cascading protocol visibility:
- `kafclaw task status --trace <trace-id> --json`
- Returns ordered cascade task states plus transition audit events for restart-safe debugging.

Skills execution example:
- `kafclaw skills exec <skill-id> --input '{"text":"..."}'`
