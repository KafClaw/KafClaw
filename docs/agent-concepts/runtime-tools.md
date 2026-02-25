---
parent: Agent Concepts
title: Runtime Tools and Capabilities
nav_order: 3
---

# Runtime Tools and Capabilities

This is the runtime-accurate tool view for current Go agent loop behavior.

## Why This Page Exists

`TOOLS.md` is part of the prompt and guidance, but tools become real capabilities only when registered in the loop.

## Currently Registered by Default

- `read_file`
- `write_file`
- `edit_file`
- `list_dir`
- `resolve_path`
- `exec`
- `sessions_spawn`
- `subagents`
- `agents_list`

Conditional:

- `remember` (memory service required)
- `recall` (memory service required)

## Capability Export to Group

Group identity announcements use registered tool names as capability list.
If registration changes, group-visible capabilities change automatically.

## Tool Safety Model

- Tools may declare risk tiers: read-only, write, high-risk
- Shell execution has workspace restrictions and guardrails
- Write tools are work-repo scoped through repo path getters

For operational policy and hardening, see:

- `docs/architecture-security/security-risks.md`
- `docs/operations-admin/admin-guide.md`
