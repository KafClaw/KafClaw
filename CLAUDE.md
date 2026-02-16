# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

KafClaw (formerly GoMikroBot) is a personal AI assistant written in Go. The Go source lives in `gomikrobot/`. Sensitive specs, tasks, research, and governance docs live in the `private/` directory (gitignored — tracked separately in scalytics/KafClaw-PRIVATE-PARTS).

## Build & Run

All Go commands run from the `gomikrobot/` directory:

```bash
cd gomikrobot

# Build
make build                    # or: go build ./cmd/gomikrobot

# Run gateway (multi-channel daemon)
make run                      # build + run
make rerun                    # kill existing ports 18790/18791, rebuild, run

# Run single message
./gomikrobot agent -m "hello"

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
├── CLAUDE.md               ← this file
├── .github/workflows/      ← CI/CD (release.yml)
├── gomikrobot/             ← Go source code
│   ├── cmd/gomikrobot/     ← CLI entry point (cobra commands)
│   ├── internal/           ← Core packages
│   │   ├── agent/          ← Agent loop + context/soul-file loader
│   │   ├── bus/            ← Async message bus (pub-sub)
│   │   ├── channels/       ← WhatsApp (whatsmeow), CLI, Web channels
│   │   ├── config/         ← Config struct (env/file/default loading)
│   │   ├── provider/       ← LLM provider abstraction (OpenAI/OpenRouter)
│   │   ├── session/        ← Per-session conversation history (JSONL)
│   │   ├── timeline/       ← SQLite event log (~/.gomikrobot/timeline.db)
│   │   └── tools/          ← Registry-based tool system
│   ├── web/                ← Web UI (HTML dashboard)
│   ├── electron/           ← Electron desktop app wrapper
│   ├── scripts/            ← install.sh, release.sh
│   ├── go.mod, go.sum, Makefile, Dockerfile, docker-compose.yml
│   └── ARCHITECTURE.md, MEMORY.md
└── private/                ← .gitignored (tracked in separate private repo)
    ├── specs/              ← Architecture, requirements, design specs
    ├── tasks/              ← Dev tasks, migration tasks, inspiration
    ├── research/           ← Hardening research, drafts
    ├── docs/               ← Guides, rebranding assets
    └── governance/         ← AGENTS.md, archived CLAUDE.md, workspace soul files
```

## Architecture

```
CLI/WhatsApp → Message Bus → Agent Loop → LLM Provider (OpenAI/OpenRouter)
                                ↓
                           Tool Registry → Filesystem / Shell / Web
                                ↑
                           Context Builder (loads soul files from workspace/)
```

### Key packages (`gomikrobot/internal/`)

- **agent/** — Core agent loop (`loop.go`) and context/soul-file loader (`context.go`).
- **bus/** — Async message bus decoupling channels from the agent loop (pub-sub).
- **channels/** — External integrations. WhatsApp uses `whatsmeow` (native, no Node bridge).
- **config/** — Config struct with env/file/default loading. Config file: `~/.gomikrobot/config.json`. Env prefix: `MIKROBOT_`.
- **provider/** — LLM provider abstraction. OpenAI/OpenRouter implementations, Whisper transcription, TTS.
- **session/** — Per-session conversation history, JSONL persistence, thread-safe.
- **timeline/** — SQLite event log at `~/.gomikrobot/timeline.db`.
- **tools/** — Registry-based tool system. Filesystem ops have path safety; shell exec has deny-pattern filtering and timeout (default 60s).

## Configuration

Loaded in order: env vars > `~/.gomikrobot/config.json` > defaults.

Default model: `anthropic/claude-sonnet-4-5`. Default workspace: `~/GoMikroBot-Workspace`. Gateway ports: 18790 (API), 18791 (dashboard).

## Tool Security Model

Shell execution (`internal/tools/shell.go`) uses deny-pattern filtering (blocks `rm`, `chmod`, `mkfs`, `shutdown`, fork bombs, etc.) and allow-pattern lists in strict mode. Filesystem writes are restricted to the work repo by default. Path traversal (`../`) is blocked.

## Extending the System

**New tool:** Implement the `Tool` interface in `internal/tools/` (Name, Description, Parameters, Execute methods), then register in the agent loop's `registerDefaultTools()`.

**New channel:** Implement `Channel` interface in `internal/channels/`, subscribe to the message bus, add config fields to `internal/config/config.go`.

**New CLI command:** Create file in `cmd/gomikrobot/cmd/`, define cobra command, register in `root.go` init().

## Note on go.mod

The Go module is still `github.com/kamir/gomikrobot`. Renaming the module path is a separate follow-up task (requires updating imports in 31+ Go files).
