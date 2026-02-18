---
name: skill-creator
description: Create or update KafClaw skills with consistent structure and safety guidance.
---

# skill-creator

Use when creating a new skill or improving an existing one.

## Workflow
- Define clear trigger conditions.
- Keep instructions minimal and deterministic.
- Document required binaries, tokens, and approval gates.
- Add/update matching operator docs in `docs/skills/`.

## Safety
- Do not add implicit privileged actions.
- Require explicit confirmation for destructive/external actions.
