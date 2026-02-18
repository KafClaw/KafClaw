---
parent: Architecture and Security
---

# Workspace Policy

> See also: [FR-025 Workspace Policy](../requirements/FR-025-workspace-policy.md)

## Fixed Workspace Path

KafClaw uses a fixed workspace for all bot state:

```
~/.kafclaw/workspace
```

## What Lives There

- Bot state (sessions, media, runtime artifacts)
- Persistent local data needed by the bot
- Soul files (AGENTS.md, SOUL.md, USER.md, TOOLS.md, IDENTITY.md)

## What Does NOT Live There

- Work repo artifacts (these go to `~/.kafclaw/work-repo`)
- System repo (identity/skills repo — cloned separately)

## Rules

1. Workspace is always `~/.kafclaw/workspace`. Not configurable per project.
2. Switching work repo does not change workspace.
3. All state remains in the workspace regardless of work repo selection.
4. System repo contains only identity and skills data, no runtime state.

## Path Summary

| Concept | Path | Purpose |
|---------|------|---------|
| Workspace | `~/.kafclaw/workspace` | Bot state, sessions, media |
| Work Repo | `~/.kafclaw/work-repo` (default) | Agent-generated artifacts |
| System Repo | Cloned bot repo | Skills, Day2Day, identity |
| Config | `~/.kafclaw/config.json` | Configuration |
| Timeline DB | `~/.kafclaw/timeline.db` | Event log, settings |
