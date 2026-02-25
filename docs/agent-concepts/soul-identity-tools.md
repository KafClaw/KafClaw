---
parent: Agent Concepts
title: Soul and Identity Files
nav_order: 2
---

# Soul and Identity Files

KafClaw agents are shaped by a workspace-level file set scaffolded during onboarding.

## The File Set

Onboarding and workspace scaffold use this canonical order:

1. `AGENTS.md`
2. `SOUL.md`
3. `USER.md`
4. `TOOLS.md`
5. `IDENTITY.md`

These files are embedded templates in the codebase and can be overwritten with `kafclaw onboard --force`.

## How Runtime Uses These Files

### 1. System prompt construction

At runtime, the context builder loads all files above from the workspace and appends them to the system prompt.

- Missing files are skipped
- Existing files are included as-is
- Order is stable and shared with indexing/scaffolding

### 2. Group identity announcement

When joining a Kafka group, identity metadata is derived as follows:

- `SoulSummary`: first paragraph from `SOUL.md` (heading lines are ignored)
- `Capabilities`: names of currently registered tools
- `Channels`: starts with `cli` (then channel-specific fields may be added by gateway setup)

This identity is announced in group onboarding and roster messages.

### 3. Memory indexing

If memory is enabled, all identity files are chunked and indexed as `source=soul:<filename>`.
Soul entries are treated as permanent memory (not pruned by TTL lifecycle).

## Practical Authoring Guidance

### `SOUL.md`

Put stable values and behavior traits in the first paragraph if you want group peers to see them in `SoulSummary`.

### `AGENTS.md`

Use for operational behavior and working rules (tool usage patterns, action policy, collaboration style).

### `TOOLS.md`

Treat this as human-readable guidance for tool intent and conventions. It does not dynamically register tools by itself.

### `IDENTITY.md`

Use for architecture role, capabilities narrative, and deployment context.

### `USER.md`

Use for owner/user profile, preferences, timezone, and project context.

## Important Distinction

`TOOLS.md` describes tools, but actual capabilities in runtime come from tool registration in Go code.
If docs and runtime diverge, runtime registration is source of truth.
