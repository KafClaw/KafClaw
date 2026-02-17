## Project Overview

KafClaw (formerly KafClaw) is a personal AI assistant written in Go. The Go source lives in `kafclaw/`. Sensitive specs, tasks, research, and governance docs live in the `private/` directory (gitignored — tracked separately).

## Build & Run

All Go commands run from the `kafclaw/` directory:

```bash
cd KafClaw

# Build
make build                    # or: go build ./cmd/kafclaw

# Run gateway (multi-channel daemon)
make run                      # build + run
make rerun                    # kill existing ports 18790/18791, rebuild, run

# Run single message
./kafclaw agent -m "hello"

# Run tests
go test ./...                 # all tests
go test ./internal/tools/     # single package

# Install to /usr/local/bin
make install

# Release (bump version + build)
make release-patch            # or release-minor, release-major
```

**Go version:** 1.24.0+ (toolchain 1.24.13)

## Repository Structure

```
KafClaw/
├── project.md              ← this file
├── .github/workflows/      ← CI/CD (release.yml)
├── kafclaw/             ← Go source code
│   ├── cmd/kafclaw/     ← CLI entry point (cobra commands)
│   ├── internal/           ← Core packages
│   │   ├── agent/          ← Agent loop + context/soul-file loader
│   │   ├── bus/            ← Async message bus (pub-sub)
│   │   ├── channels/       ← WhatsApp (whatsmeow), CLI, Web channels
│   │   ├── config/         ← Config struct (env/file/default loading)
│   │   ├── identity/       ← Embedded soul-file templates + workspace scaffolding
│   │   ├── provider/       ← LLM provider abstraction (OpenAI/OpenRouter)
│   │   ├── session/        ← Per-session conversation history (JSONL)
│   │   ├── timeline/       ← SQLite event log (~/.kafclaw/timeline.db)
│   │   └── tools/          ← Registry-based tool system
│   ├── web/                ← Web UI (HTML dashboard)
│   ├── electron/           ← Electron desktop app wrapper
│   ├── scripts/            ← install.sh, release.sh
│   ├── go.mod, go.sum, Makefile, Dockerfile, docker-compose.yml
│   └── ARCHITECTURE.md, MEMORY.md
├── docs/                   ← Public documentation
│   ├── bugs/               ← Bug reports (BUG-xxx-*.md)
│   ├── tasklogs/           ← Completed task logs (TASK-xxx-*.md)
│   └── v2/                 ← Guides, manuals, architecture docs
└── private/                ← .gitignored (tracked in separate private repo)
    └── v2/
        ├── tasks/          ← Task plans (private)
        ├── requirements/   ← Specs and requirements (FR-xxx-*.md)
        ├── research/       ← Research drafts
        └── docs/           ← Internal guides
```

## Three Repositories Model

KafClaw organizes state across three logical repositories:

- **Identity (Workspace)** — Soul files (IDENTITY.md, SOUL.md, AGENTS.md, TOOLS.md, USER.md) loaded at startup into the LLM system prompt. Scaffolded by `kafclaw onboard`, user-customizable.
- **Work Repo** — Agent sandbox for files, memory, tasks, docs. Git-initialized. Default: `~/KafClaw-Workspace/`.
- **System Repo** — Bot source code (this repo). Read-only at runtime. Contains skills and operational guidance.

The canonical soul file list is `identity.TemplateNames` in `internal/identity/embed.go` — single source of truth used by both `agent/context.go` and `memory/indexer.go`.

## Workspace Scaffolding

Running `kafclaw onboard` creates `~/.kafclaw/config.json` **and** scaffolds soul files into the workspace:

```
~/KafClaw-Workspace/
├── AGENTS.md       ← Behavioral guidelines, tool usage
├── SOUL.md         ← Personality, values, communication style
├── USER.md         ← User profile (customize this!)
├── TOOLS.md        ← Tool reference with safety notes
└── IDENTITY.md     ← Bot self-description, architecture overview
```

With `--force`, existing soul files are overwritten. Without it, existing files are preserved.

Templates are embedded in the binary via `go:embed` (`internal/identity/templates/`).

## Architecture

```
CLI/WhatsApp → Message Bus → Agent Loop → LLM Provider (OpenAI/OpenRouter)
                                ↓
                           Tool Registry → Filesystem / Shell / Web
                                ↑
                           Context Builder (loads soul files from workspace/)
```

### Key packages (`internal/`)

- **agent/** — Core agent loop (`loop.go`) and context/soul-file loader (`context.go`).
- **bus/** — Async message bus decoupling channels from the agent loop (pub-sub).
- **channels/** — External integrations. WhatsApp uses `whatsmeow` (native, no Node bridge).
- **config/** — Config struct with env/file/default loading. Config file: `~/.kafclaw/config.json`. Env prefix: `MIKROBOT_`.
- **provider/** — LLM provider abstraction. OpenAI/OpenRouter implementations, Whisper transcription, TTS.
- **session/** — Per-session conversation history, JSONL persistence, thread-safe.
- **timeline/** — SQLite event log at `~/.kafclaw/timeline.db`.
- **tools/** — Registry-based tool system. Filesystem ops have path safety; shell exec has deny-pattern filtering and timeout (default 60s).

## Configuration

Loaded in order: env vars > `~/.kafclaw/config.json` > defaults.

Default model: `anthropic/claude-sonnet-4-5`. Default workspace: `~/KafClaw-Workspace`. Gateway ports: 18790 (API), 18791 (dashboard).

## Tool Security Model

Shell execution (`internal/tools/shell.go`) uses deny-pattern filtering (blocks `rm`, `chmod`, `mkfs`, `shutdown`, fork bombs, etc.) and allow-pattern lists in strict mode. Filesystem writes are restricted to the work repo by default. Path traversal (`../`) is blocked.

## Extending the System

**New tool:** Implement the `Tool` interface in `internal/tools/` (Name, Description, Parameters, Execute methods), then register in the agent loop's `registerDefaultTools()`.

**New channel:** Implement `Channel` interface in `internal/channels/`, subscribe to the message bus, add config fields to `internal/config/config.go`.

**New CLI command:** Create file in `internal/cli/`, define cobra command, register in `root.go` init().

## Task Workflow

All implementation tasks follow a **plan → implement → log** cycle:

1. **Plan**: Create a task file in `private/v2/tasks/` using `TASK-xxx-short-description.md`. Include: Status, Priority, Objective, Steps, Verification, Acceptance Criteria.
2. **Implement**: Set Status to `In Progress`, do the work.
3. **Log**: When done, set Status to `Done` in the task file, then create a **public** tasklog entry in `docs/tasklogs/task-xxx-short-description.md` with: completion date, summary of what was done, insights/lessons learned, and relevant commit references.

### What goes where

| Type | Location | Visibility | Naming |
|------|----------|------------|--------|
| **Bug reports** | `docs/bugs/` | Public (code repo) | `BUG-xxx-short-description.md` |
| **Task logs** | `docs/tasklogs/` | Public (code repo) | `TASK-xxx-short-description.md` |
| **Task plans** | `private/v2/tasks/` | Private | `TASK-xxx-short-description.md` |
| **Specs & requirements** | `private/v2/requirements/` | Private | `FR-xxx-short-description.md` |

Bug reports and task logs are **public** — they go in the code repo under `docs/` so anyone can see what was fixed and what was learned. Task plans and specs stay **private** because they may contain sensitive implementation details.

**Private repo sync**: The `private/` directory is tracked separately in `KafClaw-PRIVATE-PARTS` (sibling repo at `/Users/kamir/GITHUB.kamir/KafClaw-PRIVATE-PARTS`). Use `sync-from-kafclaw.sh` to push changes, `sync-to-kafclaw.sh` to pull.

## Go Module

The Go module path is `github.com/KafClaw/KafClaw`.
