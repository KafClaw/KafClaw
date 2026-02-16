# KafClaw

Personal AI assistant — multi-channel (CLI, WhatsApp, Web UI, Electron desktop), tool-using agent powered by LLMs.

## Quick Start

```bash
cd gomikrobot

# Build
make build

# Run gateway (CLI + WhatsApp + Web dashboard)
make run

# Single message
./gomikrobot agent -m "hello"

# Run tests
go test ./...
```

**Requirements:** Go 1.24+, Node.js 20+ (for Electron app)

## Architecture

```
CLI / WhatsApp / Web UI / Electron
        ↓
    Message Bus (pub-sub)
        ↓
    Agent Loop → LLM Provider (OpenAI / OpenRouter)
        ↓
    Tool Registry → Filesystem / Shell / Web Search
        ↑
    Context Builder (soul files from workspace/)
```

## Operation Modes

| Mode | Command | Description |
|------|---------|-------------|
| Standalone | `make run` | Local desktop, no Kafka |
| Full | `make run-full` | Group collaboration via Kafka + orchestrator |
| Headless | `make run-headless` | Server deployment, binds 0.0.0.0, requires auth token |

## Electron Desktop App

```bash
cd gomikrobot
make electron-build    # Build Go + Electron
make electron-start    # Launch desktop app
make electron-dist     # Package for current platform
```

## Configuration

Config file: `~/.gomikrobot/config.json`
Env prefix: `MIKROBOT_`
Gateway: port 18790 (API), port 18791 (dashboard)

## License

See [LICENSE](LICENSE).
