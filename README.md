# KafClaw
[![CI (Smoke+Fuzz+Go)](https://github.com/KafClaw/KafClaw/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/KafClaw/KafClaw/actions/workflows/ci.yml)
[![Release](https://github.com/KafClaw/KafClaw/actions/workflows/release.yml/badge.svg?branch=main)](https://github.com/KafClaw/KafClaw/actions/workflows/release.yml)
[![Pages](https://github.com/KafClaw/KafClaw/actions/workflows/pages.yml/badge.svg?branch=main)](https://github.com/KafClaw/KafClaw/actions/workflows/pages.yml)

[![Go Report Card](https://goreportcard.com/badge/github.com/KafClaw/KafClaw)](https://goreportcard.com/report/github.com/KafClaw/KafClaw)
[![License](https://img.shields.io/github/license/KafClaw/KafClaw)](LICENSE)

KafClaw is backed by [Scalytics](https://www.scalytics.io). We do not create, operate, or endorse any crypto tokens. If you see token-based fundraising using the KafClaw name, it is not affiliated with this project.

## Platform Suite

KafClaw is part of a broader infrastructure stack:

- **KafScale**: Kafka-compatible + S3-compatible platform for event transport and large artifact flows.  
  [kafscale.io](https://kafscale.io) • [github.com/kafscale](https://github.com/kafscale)
- **GitClaw**: agent-first self-hosted Git platform for repository workflows and automation.
- **Scalytics Copilot**: open-source operations stack for private AI inference with open models.  
  [github.com/scalytics/ScalyticsCopilot](https://github.com/scalytics/ScalyticsCopilot)
- **KafClaw**: agent runtime and coordination layer (local, Kafka-connected, and remote gateway modes).

**Enterprise-grade multi-agent collaboration over Apache Kafka.**

KafClaw is an agent coordination framework built in Go. It connects autonomous AI agents through Kafka-based messaging, giving them group collaboration, hierarchical orchestration, shared memory, and distributed skill routing — without coupling them to any single LLM provider, runtime, or deployment model.

The name reflects what it does: **Kaf**ka as the backbone, **Claw** as the grip that holds heterogeneous agents together.

---

## Core Contributions

### 1. Enterprise-Ready Agent Communication via Kafka Protocol 

Agents communicate through a structured Kafka topic hierarchy. Every message flows through typed envelopes (`announce`, `request`, `response`, `trace`, `memory`, `audit`) with correlation IDs and timestamps, giving full observability out of the box.

```
group.<name>.announce             # join / leave / heartbeat
group.<name>.requests             # task requests
group.<name>.responses            # task responses
group.<name>.traces               # distributed trace spans
group.<name>.control.roster       # topic registry + member capabilities
group.<name>.control.onboarding   # agent onboarding protocol
group.<name>.tasks.status         # task progress updates
group.<name>.observe.audit        # admin audit trail
group.<name>.memory.shared        # persistent shared knowledge (via LFS/S3)
group.<name>.memory.context       # ephemeral context sharing (TTL-based)
group.<name>.orchestrator         # hierarchy discovery + zone coordination
group.<name>.skill.<s>.requests   # per-skill task routing (dynamic)
group.<name>.skill.<s>.responses  # per-skill responses (dynamic)
```

This is not a toy pub-sub wrapper. It is a deliberate wire protocol designed for production traceability, auditability, and zero-downtime agent onboarding.

### 2. Group Collaboration — Internal and Inter-Group

Agents form **groups**. Within a group, they discover each other via heartbeats, delegate tasks through request/response topics, and share trace spans for distributed debugging.

**Group-internal collaboration:**
- Roster management with automatic heartbeat-based liveness detection
- Task delegation with depth tracking and deadline propagation
- Skill registration — agents advertise capabilities, others route tasks to them
- Onboarding protocol (open or gated with challenge/response handshake)

**Inter-group coordination** is handled by the **orchestrator**, which adds:
- **Hierarchy**: parent-child agent relationships for delegation chains
- **Zones**: security boundaries with `public`, `shared`, and `private` visibility
- **Discovery**: agents announce themselves on the orchestrator topic; the hierarchy and zone graph update in real time

A group of agents working on code review does not need to see the agents running customer support — unless a zone explicitly bridges them.

### 3. Species-Independent Inter-Species Nervous System

KafClaw decouples the *what* from the *how*. The Kafka topic layer is the nervous system; agents are the species.

An agent is anything that can produce and consume Kafka envelopes. It might be:
- A Go binary running the full KafClaw runtime
- A Python script using `kafka-python`
- A Node.js service, a Rust worker, a shell script polling via `kcat`
- A human operating through the WhatsApp or Telegram channel bridge

The wire format is JSON. The envelope schema is simple and documented. No SDK lock-in, no runtime dependency beyond Kafka itself.

**Shared memory** reinforces this:
- Agents publish knowledge artifacts to `memory.shared` (backed by S3/LFS for large payloads)
- Ephemeral context goes to `memory.context` with TTL expiry
- Locally, each agent can index received items into its own vector store (SQLite-vec, Qdrant, or custom) for semantic retrieval
- The result: agents with different architectures, models, and languages can build on each other's work without direct coupling

### 4. Shared Learning

Knowledge does not stay locked inside a single agent's context window.

- **Memory items** published to the group are stored locally in each subscribing agent's vector index, making them available for RAG retrieval in future conversations
- **Expertise tracking** records what each agent knows and has done, so the group can route questions to the right specialist
- **Auto-indexing** captures conversation context and indexes it for later recall
- **Skill channels** let agents register domain expertise as addressable services — other agents don't need to know the implementation, just the skill name

The learning loop: Agent A discovers something, shares it as a memory item, Agent B indexes it, Agent B uses it to answer a question three days later. No central brain required.

---

## Architecture

```
                         ┌──────────────────────────────────┐
                         │           Apache Kafka           │
                         │                                  │
                         │  control ─ tasks ─ observe       │
                         │  memory ─ orchestrator ─ skills  │
                         └──────┬──────────────┬────────────┘
                                │              │
                    ┌───────────┘              └───────────┐
                    │                                      │
             ┌──────┴──────┐                        ┌──────┴──────┐
             │   Agent A   │                        │   Agent B   │
             │  (Go/Full)  │                        │ (Any lang)  │
             │             │                        │             │
             │ Agent Loop  │                        │ Kafka       │
             │ Tool Reg.   │                        │ Consumer    │
             │ Memory Svc  │                        │ + Producer  │
             │ LLM Provider│                        │             │
             │ Channels    │                        │             │
             └─────────────┘                        └─────────────┘
                    │
        ┌───────────┼───────────┐
        │           │           │
   WhatsApp     Telegram     Web UI
   (whatsmeow)  (bridge)    (:18791)
```

**Full KafClaw agents** (Go runtime) include:
- **Agent loop** with LLM provider abstraction (OpenAI, OpenRouter)
- **Tool registry** (filesystem, shell, memory, web — with security sandboxing)
- **Message bus** decoupling channels from the agent loop
- **Timeline DB** (SQLite) for event logging, media, and distributed tracing
- **Policy engine** for message classification, token quotas, and rate limiting
- **Scheduler** for cron jobs and deferred tasks

**Lightweight agents** only need a Kafka client and the envelope JSON schema.

---

## Operating Modes

| Mode | Kafka | Orchestrator | Use Case |
|------|-------|-------------|----------|
| `standalone` | No | No | Single-agent desktop assistant |
| `group` | Yes | No | Peer-to-peer agent collaboration |
| `full` | Yes | Yes | Hierarchical multi-agent with zones |
| `headless` | Yes | Yes | Server deployment (0.0.0.0 + auth) |

```bash
cd KafClaw

make run-standalone    # No Kafka, no orchestrator
make run               # Default gateway
make run-full          # Group + orchestrator enabled
make run-headless      # Server mode (requires KAFCLAW_GATEWAY_AUTH_TOKEN)
```

---

## Quick Start

```bash
# Prerequisites: Go 1.24+, Apache Kafka (for group modes)

cd KafClaw

# Build
make build

# Run single message (standalone, no Kafka needed)
./kafclaw agent -m "hello"

# Run gateway (standalone mode)
make run-standalone

# Run with group collaboration
make run-full

# Run tests
go test ./...
make test-smoke              # critical-path smoke tests
make test-critical           # enforce 100% critical-logic coverage gate
make test-fuzz               # fuzz critical guard logic

# Kafka diagnostics
./kafclaw kshark --broker localhost:9092 --test-connection
```

### Configuration

Loaded in order: environment variables > `~/.kafclaw/config.json` > defaults.

| Variable | Default | Description |
|----------|---------|-------------|
| `KAFCLAW_GROUP_ENABLED` | `false` | Enable Kafka group collaboration (legacy: `MIKROBOT_GROUP_ENABLED`) |
| `KAFCLAW_ORCHESTRATOR_ENABLED` | `false` | Enable hierarchical orchestration (legacy: `MIKROBOT_ORCHESTRATOR_ENABLED`) |
| `KAFCLAW_ORCHESTRATOR_ROLE` | `worker` | Agent role: `orchestrator`, `worker`, `observer` (legacy: `MIKROBOT_ORCHESTRATOR_ROLE`) |
| `KAFCLAW_GROUP_KAFKA_BROKERS` | `localhost:9092` | Kafka broker addresses (legacy: `MIKROBOT_KAFKA_BROKERS`) |
| `KAFCLAW_GATEWAY_HOST` | `127.0.0.1` | API bind address (legacy: `MIKROBOT_GATEWAY_HOST`) |
| `KAFCLAW_GATEWAY_AUTH_TOKEN` | *(empty)* | Bearer token for headless mode (legacy: `MIKROBOT_GATEWAY_AUTH_TOKEN`) |

Gateway ports: **18790** (API), **18791** (dashboard).

---

## Key Packages

```
internal/
├── group/          # Kafka-based group collaboration, onboarding, skills, shared memory
├── orchestrator/   # Hierarchy, zones, discovery
├── agent/          # Core agent loop, context builder, soul file loading
├── bus/            # Async message bus (pub-sub, decouples channels from agent)
├── channels/       # WhatsApp, Telegram, Discord, Web — Channel interface
├── config/         # Env / file / default config loading
├── provider/       # LLM abstraction (OpenAI, OpenRouter, Whisper, TTS)
├── memory/         # Vector store, semantic search, expertise tracking, auto-indexing
├── session/        # Per-session conversation history (JSONL persistence)
├── timeline/       # SQLite event log, media storage, trace/span IDs
├── tools/          # Registry-based tools (fs, shell, memory, web) with security sandbox
├── policy/         # Message classification, token quotas, rate limiting
├── scheduler/      # Cron and deferred task scheduling
├── kshark/         # Kafka diagnostic tool (connection, topics, partitions, metrics)
└── approval/       # Task approval workflows
```

---

## Diagnostic Tooling: KShark

KShark is a built-in Kafka diagnostic tool for verifying connectivity and inspecting group infrastructure:

```bash
./kafclaw kshark --broker localhost:9092 --test-connection
./kafclaw kshark --broker localhost:9092 --probe-topics --group mygroup
./kafclaw kshark --broker localhost:9092 --network-diag
```

---

## Releases

- Local: `make release-patch` (or `release-minor`, `release-major`) in `kafclaw/`
- CI: push a tag `vX.Y.Z` to trigger the build workflow
- Cross-compile: `make dist-go` produces binaries for darwin/linux (amd64/arm64)
- Electron desktop: `make electron-dist` packages for current platform
- See `docs/release.md` for details

## License

Licensed under the [Apache License, Version 2.0](LICENSE).

This project was inspired by [HKUDS/nanobot](https://github.com/HKUDS/nanobot). While originally a fork, KafClaw has been completely rewritten in Go to support enterprise-grade Kafka backends and multi-agent coordination. Original attribution is preserved in the [NOTICE](NOTICE) file.
