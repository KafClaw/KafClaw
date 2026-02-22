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
- `kafclaw group` - group collaboration controls
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

Skills execution example:
- `kafclaw skills exec <skill-id> --input '{"text":"..."}'`
