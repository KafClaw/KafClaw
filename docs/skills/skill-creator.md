---
title: Skill Creator
parent: Skills
nav_order: 11
---

# Skill Creator

Create and maintain KafClaw skills with consistent structure and safety requirements.

## Default State

- Bundled with KafClaw
- Disabled by default

## What It Does

- Provides a workflow for creating and improving skill definitions.
- Enforces structure (`SKILL.md`, clear workflows, explicit safety gates).
- Ensures operator docs are added for each shipped skill.

## Install / Enable

No external install needed (bundled skill). Enable it:

```bash
kafclaw skills enable-skill skill-creator
```

## Usage

- Use when introducing a new skill or hardening an existing one.
- Require docs update in `/docs/skills` as part of completion criteria.

## Troubleshooting

- If a new skill is not discovered, verify folder path and `SKILL.md` frontmatter.
- If execution is blocked, verify policy settings (`skills.enabled`, source toggles, link policy).
- If rollout is risky, gate with verify/install first and keep update rollback path.
