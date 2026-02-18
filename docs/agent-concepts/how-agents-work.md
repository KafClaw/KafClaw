---
parent: Agent Concepts
---

# How Agents Work

This page describes the live runtime path from message input to response output.

## Runtime Flow

1. Inbound message enters via a channel (CLI, Web UI, WhatsApp, Slack, Teams, etc.)
2. Message is published to the internal bus
3. Agent loop consumes inbound event
4. Context builder assembles system prompt:
   - runtime identity block
   - workspace identity files (`AGENTS.md`, `SOUL.md`, `USER.md`, `TOOLS.md`, `IDENTITY.md`)
   - `memory/MEMORY.md` (if present)
   - tool summary and system-repo skills (if available)
5. Model responds, optionally issuing tool calls
6. Tool policy evaluates requested calls
7. Tool results are fed back into loop (up to configured iteration limit)
8. Final response is stored, indexed, and published as outbound message

## Default Tool Registration

The loop registers these tools by default:

- `read_file`
- `write_file`
- `edit_file`
- `list_dir`
- `resolve_path`
- `exec`
- `sessions_spawn`
- `subagents`
- `agents_list`

When memory service is enabled, it also registers:

- `remember`
- `recall`

## Group and Orchestrator Identity

When group mode is enabled:

- agent identity is built from runtime + workspace files
- capabilities are exported from active tool registry
- onboarding/announce messages publish identity to group control topics
- roster and timeline persist identity snapshots

## Auto-Scaffold and Startup Behavior

At gateway startup:

- if workspace identity files are missing, scaffold runs automatically (non-destructive)
- if memory service is enabled, soul files are indexed in background

This makes headless and container setups self-healing for missing baseline files.
