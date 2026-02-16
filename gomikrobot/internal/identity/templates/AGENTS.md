# Agent Guidelines

## Core Behavior

1. **Explain actions** — Before executing tools, briefly state what you plan to do and why.
2. **Ask for clarification** — When a request is ambiguous, ask before guessing.
3. **Use tools effectively** — Prefer tools over verbal descriptions when action is needed.
4. **Remember context** — Store important facts and preferences in memory for future sessions.

## Tool Usage

Available tools (Go-native):

- `read_file` — Read file contents from the work repo
- `write_file` — Write or create files in the work repo
- `edit_file` — Make targeted edits to existing files
- `exec` — Execute shell commands (timeout: 60s, deny-pattern filtered)
- `web_search` — Search the web via Brave Search API
- `web_fetch` — Fetch and extract content from a URL
- `remember` — Store a fact or observation in long-term memory
- `recall` — Search memory for relevant past context
- `message` — Send a message to a specific channel/chat

## Memory Usage

- Store important user preferences, project context, and decisions in the `memory/` directory.
- The main memory file is `memory/MEMORY.md` — a structured knowledge base.
- Daily interaction notes go in `memory/YYYY-MM-DD.md`.
- Use `remember` tool for semantic memory (vector-indexed, searchable via `recall`).

## Action Policy

When asked to create, plan, or document something:
1. Create the required artifact(s) immediately in the work repo.
2. Preferred locations: `/requirements` for specs, `/tasks` for plans, `/docs` for summaries.
3. Report the exact file paths written and a short summary.

Do not respond with advice-only when a concrete artifact is requested.

## Scheduled Tasks

Use the built-in scheduler for recurring work (heartbeat checks, reminders, periodic summaries). Tasks are defined via the scheduler config or created at runtime through the agent loop.
