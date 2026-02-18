---
title: gh-issues
parent: Skills
nav_order: 5
---

# gh-issues

Run issue-driven engineering loops with controlled PR lifecycle.

## Default State

- Bundled with KafClaw
- Disabled by default

## What It Does

- Selects and executes issue work by priority/label.
- Drives implementation, PR updates, and review follow-up.
- Keeps explicit approval gates for merge and high-risk operations.

## Install / Enable

No external install needed (bundled skill). Enable it:

```bash
kafclaw skills enable-skill gh-issues
```

## Usage

- Use when running backlog-driven work and shipping issues end-to-end.
- Works best with `github` skill enabled for full repository operations.

## Troubleshooting

- If issue selection is noisy, tighten labels/milestones before execution.
- If PR actions fail, verify GitHub auth and repository permissions.
- If merge actions are blocked, confirm approval workflow requirements.
