---
parent: Architecture and Security
---

# Workspace Policy

> See also: [FR-025 Workspace Policy](../requirements/FR-025-workspace-policy/)

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

1. Single-agent default workspace is `~/.kafclaw/workspace`.
2. Multi-agent deployments should set explicit per-agent workspace paths in config.
3. Switching work repo does not change workspace.
4. All state remains in the workspace regardless of work repo selection.
5. System repo contains only identity and skills data, no runtime state.

## Multi-Agent Workspace Isolation

For multi-agent deployments, use separate workspace paths per agent in `kafclaw.json` (or your generated runtime config) so runtime state does not bleed across agents.

Example:

```json
{
  "id": "worker-monitor",
  "workspace": "/home/clawdia/workspace-monitor",
  "model": {
    "primary": "google-gemini-cli/gemini-2.5-flash",
    "fallbacks": [
      "anthropic/claude-haiku-4-5",
      "google-gemini-cli/gemini-3-flash-preview"
    ]
  },
  "identity": {
    "name": "Sentinel",
    "theme": "Infrastructure monitor - uptime checks, log analysis, alerting, system health",
    "emoji": "📡",
    "avatar": "avatars/sentinel.png"
  },
  "tools": {
    "profile": "minimal",
    "allow": [
      "web_fetch",
      "read",
      "write",
      "exec",
      "bash"
    ]
  }
}
```

Operational guidance:

1. One agent, one workspace directory.
2. Never share a workspace between production agents.
3. Back up each workspace independently.
4. Keep agent identity files and runtime state scoped to that agent's workspace.

## Path Summary

| Concept | Path | Purpose |
|---------|------|---------|
| Workspace | `~/.kafclaw/workspace` | Bot state, sessions, media |
| Work Repo | `~/.kafclaw/work-repo` (default) | Agent-generated artifacts |
| System Repo | Cloned bot repo | Skills, Day2Day, identity |
| Config | `~/.kafclaw/config.json` | Configuration |
| Timeline DB | `~/.kafclaw/timeline.db` | Event log, settings |
