---
title: channel-onboarding
parent: Skills
nav_order: 1
---

# channel-onboarding

Guides setup and verification for Slack, Teams, and WhatsApp channel integrations.

## Default State

- Bundled with KafClaw
- Enabled by default

## What It Does

- Validates channel prerequisites and required config fields.
- Guides first-time onboarding for Slack, Teams, and WhatsApp.
- Produces concrete remediation steps when checks fail.

## Install / Enable

Already bundled and enabled by default. To ensure the skills system is active:

```bash
kafclaw skills enable
```

## Usage

- Run onboarding:

```bash
kafclaw onboard
```

- Run diagnostics:

```bash
kafclaw doctor
```

## Troubleshooting

- If channels do not respond, check channel `enabled` flags and required tokens in config.
- If onboarding is skipped, rerun with `kafclaw onboard --skip-skills=false`.
- If diagnostics report missing prerequisites, follow the command hints from `doctor`.
