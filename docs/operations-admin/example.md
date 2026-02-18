---
parent: Operations and Admin
title: Example Deployment — 4-Agent Group
---

# Example Deployment — 4-Agent Group

Deploy a KafClaw Workshop: 1 desktop Electron agent (Mac) + 3 headless Docker agents sharing one gateway and one KafScale bus.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                      Host Machine (Mac)                       │
│                                                               │
│  ┌──────────────────────┐    ┌─────────────────────────────┐ │
│  │   Electron App       │    │   Docker Compose             │ │
│  │   (desktop-orch.)    │    │                               │ │
│  │                      │    │  MinIO (S3)     ← object store│ │
│  │  Gateway: 18790/91   │    │  etcd           ← metadata    │ │
│  │  WhatsApp channel    │    │  KafScale broker ← messaging  │ │
│  │  Dashboard UI        │    │  KafScale LFS Proxy ← HTTP API│ │
│  │  Role: orchestrator  │    │  KafScale Console  ← web UI   │ │
│  │                      │    │                               │ │
│  └──────────────────────┘    │  agent-researcher             │ │
│         │                    │  agent-coder                   │ │
│         │                    │  agent-reviewer                │ │
│         │                    └─────────────────────────────┘ │
│         └──── KafScale broker (9092) ─────────┘              │
└──────────────────────────────────────────────────────────────┘
```

## Infrastructure Stack

The workshop runs the full KafScale platform locally:

| Service | Port | Purpose |
|---------|------|---------|
| MinIO | 9000 (API), 9001 (console) | S3-compatible object storage for LFS blobs |
| etcd | 2379 | KafScale metadata and service discovery |
| KafScale broker | 9092 | Kafka-compatible message broker |
| KafScale LFS Proxy | 8080 (HTTP), 9093 (Kafka) | Large file transfer + HTTP produce API |
| KafScale Console | 3080 | Web UI for broker monitoring + LFS dashboard |

This is the same architecture as the production KafScale platform, but with a local MinIO instead of the Synology NAS.

## Prerequisites

- Docker + Docker Compose v2
- Git
- Go 1.24+ (for building the Electron app / local gateway)
- An OpenRouter API key
- KafScale images accessible (ghcr.io/kafscale or local registry)

## Quick Start

```bash
cd KafClaw

# Interactive setup — creates .env, builds image, starts stack
make workshop-setup

# Or manual steps:
cp .env.example .env        # edit with your API key + registry
make docker-build            # build kafclaw:local image
make workshop-up             # start KafScale + 3 agents
```

## Configuration

All configuration is via the `.env` file (see `.env.example` for all options).

### Required variables

| Variable | Description |
|----------|-------------|
| `OPENROUTER_API_KEY` | Shared LLM key for all agents |
| `AGENT_AUTH_TOKEN` | Random secret for headless agent APIs |

### KafScale images

```bash
# Default: GitHub Container Registry
KAFSCALE_REGISTRY=ghcr.io/kafscale
KAFSCALE_TAG=dev

# Local registry (if available)
KAFSCALE_REGISTRY=192.168.0.131:5100/kafscale
KAFSCALE_TAG=dev
```

### Agent work repos

Each agent can mount a host directory or auto-clone a Git URL:

```bash
# Mount a local directory
AGENT1_WORK_REPO=/Users/you/repos/research-repo

# Or auto-clone from GitHub (needs GH auth on host)
AGENT1_WORK_REPO_GIT_URL=https://github.com/you/research-repo.git
```

### Agent identity (SOUL.md)

Each agent's personality is defined by its workspace `SOUL.md`. Example templates are in `examples/workshop/`:

```
examples/workshop/
├── agent-researcher/workspace/SOUL.md
├── agent-coder/workspace/SOUL.md
└── agent-reviewer/workspace/SOUL.md
```

Customize these before starting the stack.

## Desktop Agent (Electron / CLI)

The Mac runs as the 4th agent and orchestrator. Configure group settings in `~/.kafclaw/config.json`:

```json
{
  "group": {
    "enabled": true,
    "groupName": "workshop",
    "agentId": "desktop-orchestrator",
    "kafkaBrokers": "localhost:9092",
    "lfsProxyUrl": "http://localhost:8080"
  },
  "orchestrator": {
    "enabled": true,
    "role": "orchestrator"
  }
}
```

Then start:

```bash
make run-full             # CLI gateway
# or
make electron-start-full  # Electron desktop app
```

## Operations

```bash
make workshop-up     # Start all containers
make workshop-down   # Stop all containers
make workshop-logs   # Tail logs
make workshop-ps     # Container status
```

## How It Works

1. **MinIO** provides S3-compatible object storage for LFS blobs (replaces the Synology NAS in production).
2. **etcd** stores KafScale broker metadata and service discovery.
3. **KafScale broker** provides Kafka-compatible messaging. All agents produce via LFS Proxy HTTP API and consume directly from the broker.
4. **KafScale LFS Proxy** bridges HTTP produce requests to the broker and handles large file storage in S3.
5. **Headless agents** run `kafclaw gateway` in Docker with `MIKROBOT_GATEWAY_HOST=0.0.0.0`. Their ports are not exposed to the host — they communicate only via KafScale.
6. **The desktop agent** exposes ports 18790 (API) and 18791 (dashboard) and owns the WhatsApp channel.
7. On startup, each agent auto-scaffolds its workspace (soul files) if missing, and auto-joins the group specified in its config.

## Verification

1. Check all containers are running: `make workshop-ps`
2. MinIO console at `http://localhost:9001` (minioadmin/minioadmin) — verify `kafscale` bucket exists
3. KafScale console at `http://localhost:3080` (kafscaleadmin/kafscale) — verify broker is healthy
4. Check agent logs for group join: `make workshop-logs`
5. Open KafClaw dashboard at `http://localhost:18791` — group roster should show 4 agents
6. Send a message via the dashboard — it should be delegated to a headless agent

## Troubleshooting

| Issue | Fix |
|-------|-----|
| Broker not healthy | Wait 30s for startup. Check MinIO + etcd first: `docker compose -f docker-compose.group.yml logs broker` |
| LFS proxy readiness fails | Broker must be healthy first. Check: `docker compose -f docker-compose.group.yml logs lfs-proxy` |
| Agent can't join group | Verify `MIKROBOT_GROUP_KAFKA_BROKERS` points to `broker:9092` (internal) for Docker agents, `localhost:9092` for host |
| Work repo empty | Set `WORK_REPO_GIT_URL` in `.env` or pre-populate the mounted directory |
| Soul files missing | Agent auto-scaffolds defaults on startup. Customize `examples/workshop/*/workspace/SOUL.md` |
| KafScale images not found | Set `KAFSCALE_REGISTRY` in `.env` to your registry (e.g., `192.168.0.131:5100/kafscale`) |
