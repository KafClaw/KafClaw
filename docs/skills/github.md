---
title: GitHub
parent: Skills
nav_order: 4
---

# GitHub

Operate GitHub issues, pull requests, and checks safely.

## Default State

- Bundled with KafClaw
- Disabled by default

## What It Does

- Supports issue/PR/check/release workflows with controlled safety gates.
- Encourages minimal, auditable changes with clear status reporting.
- Applies approval expectations before merge/release/destructive actions.

## Install / Enable

No external install needed (bundled skill). Enable it:

```bash
kafclaw skills enable-skill github
```

## Usage

- Use for repo triage, PR follow-up, and CI signal interpretation.
- Pair with `gh-issues` for issue-driven execution loops.

## Troubleshooting

- If GitHub actions fail, verify `gh auth status`.
- If operations are blocked, check your policy/approval settings.
- If API calls are rate-limited, retry with reduced query scope.
