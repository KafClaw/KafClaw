---
title: incident-comms
parent: Skills
nav_order: 10
---

# incident-comms

Generate and send standardized incident communications.

## Default State

- Bundled with KafClaw
- Disabled by default

## What It Does

- Builds structured incident updates (impact, scope, timeline, next update).
- Adapts message shape for enabled channels.
- Enforces explicit approval before outbound send actions.

## Install / Enable

No external install needed (bundled skill). Enable it:

```bash
kafclaw skills enable-skill incident-comms
```

## Usage

- Use for first alert, periodic updates, and resolution notices.
- Keep a fixed update cadence and include next expected update time.

## Troubleshooting

- If sends are blocked, check approval policy and channel enablement.
- If channel formatting is inconsistent, use a shared incident template.
- If channels are not ready, run onboarding/doctor before incident publishing.
