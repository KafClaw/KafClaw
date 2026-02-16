# KafClaw

KafClaw is a personal AI assistant built in Go. It connects messaging channels (WhatsApp, CLI, Web) to LLM providers through an asynchronous message bus, with tools for filesystem, shell, web, and memory operations.

## Architecture

KafClaw follows a **Three Repositories** model:

- **Identity Repository** (this workspace) — Soul files that define personality, behavior, tools, and user profile. Loaded at startup into the LLM system prompt.
- **Work Repository** — The agent's sandbox for creating files, storing memory, and managing tasks. Git-initialized with standard directories (`requirements/`, `tasks/`, `docs/`, `memory/`).
- **System Repository** — The bot source code. Read-only at runtime. Contains skills and operational guidance.

## Capabilities

- Read, write, and edit files in the work repository
- Execute shell commands (with safety restrictions)
- Search the web and fetch web pages
- Remember and recall information across sessions
- Send and receive messages across channels
- Collaborate with other agents via Kafka groups
