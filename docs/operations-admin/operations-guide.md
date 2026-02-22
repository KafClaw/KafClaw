---
parent: Operations and Admin
title: KafClaw Operations Guide
---

# KafClaw Operations Guide

Build, deploy, monitor, and operate KafClaw.

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Build and Release](#2-build-and-release)
3. [Deployment](#3-deployment)
4. [Network and Ports](#4-network-and-ports)
5. [Database](#5-database)
6. [Logging and Observability](#6-logging-and-observability)
7. [API Reference](#7-api-reference)
8. [Health Checks and Backup](#8-health-checks-and-backup)
9. [Graceful Shutdown](#9-graceful-shutdown)

---

## 1. Architecture Overview

### Data Flow

```
WhatsApp/CLI/Web/Scheduler --> Message Bus --> Agent Loop --> LLM Provider
                                                  |
                                             Tool Registry --> Filesystem / Shell / Memory
                                                  ^
                                             Context Builder (soul files + memory + RAG)
```

### Key Packages

| Package | Responsibility |
|---------|----------------|
| `agent/` | Core agent loop and context/soul-file loader |
| `bus/` | Async message bus (pub-sub, 100-msg buffers) |
| `channels/` | WhatsApp via whatsmeow (native Go, no Node bridge) |
| `config/` | Config loading: env vars > config.json > defaults |
| `provider/` | LLM abstraction (OpenAI, OpenRouter, Whisper, TTS) |
| `memory/` | 6-layer semantic memory with SQLite-vec |
| `policy/` | Tiered tool authorization engine |
| `session/` | JSONL conversation persistence |
| `timeline/` | SQLite event log, schema, settings |
| `tools/` | Tool registry with path safety and shell filtering |
| `group/` | Kafka-based multi-agent collaboration |
| `orchestrator/` | Agent hierarchy and zones |
| `scheduler/` | Cron-based job scheduling |

### Request Lifecycle

1. Message arrives via channel (WhatsApp, CLI, Web UI)
2. Published to message bus as InboundMessage
3. Agent loop consumes, creates task record, dedup check
4. Context builder assembles system prompt (soul files + memory + RAG)
5. LLM called with tool definitions
6. Tool calls evaluated by policy engine, executed if allowed
7. Agentic loop iterates up to 20 times until final text response
8. Response published as OutboundMessage, delivered via channel
9. Task status updated (completed/failed)

---

## 2. Build and Release

> See also: [Release Process](/operations-admin/release/) for versioning details

### Prerequisites

- Go 1.24.0+ (toolchain 1.24.13)
- All Go commands run from the KafClaw source directory

### Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the `kafclaw` binary |
| `make run` | Build and run the gateway |
| `make rerun` | Kill ports 18790/18791, rebuild, run |
| `make install` | Install local binary via `kafclaw install` |
| `make test` | `go test ./...` |
| `make test-smoke` | Fast critical-path smoke tests |
| `make test-critical` | Enforce 100% critical logic coverage |
| `make test-fuzz` | Run fuzz tests on critical guard logic |
| `make release-patch` | Bump patch version, tag, push |
| `make release-minor` | Bump minor version, tag, push |
| `make release-major` | Bump major version, tag, push |
| `make docker-build` | Build binary + Docker image |
| `make docker-up` | Start docker-compose |
| `make docker-down` | Stop docker-compose |
| `make docker-logs` | Tail docker-compose logs |

### Tests

```bash
go test ./...                  # all tests
make test-smoke                # critical-path smoke tests
make test-critical             # hard 100% coverage gate for critical logic
make test-fuzz                 # fuzz critical guard logic
go test ./internal/tools/      # single package
go test ./internal/memory/     # memory tests
```

### CI/CD

- Workflow: `.github/workflows/release-go.yml`
- Trigger: tag push `v*` or manual `workflow_dispatch`
- Build matrix: ubuntu, macOS, Windows
- Artifacts attached to GitHub Release

---

## 3. Deployment

### Local

```bash
kafclaw onboard      # first-time setup
kafclaw gateway      # start daemon
```

### Docker

```bash
make docker-build    # build binary + image
make docker-up       # start (detached)
make docker-down     # stop
```

Container mounts:

| Host | Container | Purpose |
|------|-----------|---------|
| `$SYSTEM_REPO_PATH` | `/opt/system-repo` | System/identity repo |
| `$WORK_REPO_PATH` | `/opt/work-repo` | Work repo |
| `~/.kafclaw` | `/root/.kafclaw` | Config + DB + sessions |

### System Install

```bash
kafclaw install      # root: /usr/local/bin, non-root: ~/.local/bin
```

For release-binary install flows (`--latest`, `--version`, `--list-releases`, unattended, signature verification), see [KafClaw Management Guide](/operations-admin/manage-kafclaw/).

### Deployment Modes

| Mode | Command | Bind Address | Auth Required | Description |
|------|---------|-------------|---------------|-------------|
| Standalone | `make run` | `127.0.0.1` | No | Local binary, no Kafka/orchestrator |
| Full | `make run-full` | `127.0.0.1` | No | + Kafka group + orchestrator |
| Headless | `make run-headless` | `0.0.0.0` | Dashboard API: Yes | LAN/cloud accessible, no GUI |
| Remote | `make electron-start-remote` | N/A | N/A | Electron UI connects to headless server |

### LAN / Remote Access

By default, KafClaw binds to `127.0.0.1` — only reachable from the local machine. This is an intentional security default.

To make the gateway accessible from other machines on your LAN (e.g., Jetson Nano serving a home network):

```bash
export KAFCLAW_GATEWAY_AUTH_TOKEN=mysecrettoken
make run-headless
```

Then access from another machine:

```
http://<server-ip>:18791/          # Dashboard
http://<server-ip>:18790/chat      # API
```

Important auth scope note:

- `gateway.authToken` protects dashboard API routes on port `18791` (except `/api/v1/status`).
- `gateway.authToken` also protects `POST /chat` on port `18790`.

**Common pitfalls:**
- **Wrong protocol:** The gateway serves plain `http://`. Using `https://` in the browser will fail silently unless TLS is configured (`tlsCert`/`tlsKey` in gateway config).
- **Still binding localhost:** If the startup log shows `http://127.0.0.1:18791`, the gateway is not network-accessible. Check that `KAFCLAW_GATEWAY_HOST=0.0.0.0` is set.
- **Firewall:** Ensure ports 18790 and 18791 are open on the server's firewall.

To bind to a specific IP instead of all interfaces:

```bash
KAFCLAW_GATEWAY_HOST=192.168.0.199 make run
```

Or set permanently in `~/.kafclaw/config.json`:

```json
{
  "gateway": {
    "host": "0.0.0.0",
    "authToken": "mysecrettoken"
  }
}
```

### channel bridge operations (slack/teams)

Slack and Teams provider traffic is handled by `cmd/channelbridge` (default bind `:18888`) and forwarded to gateway inbound APIs.

Build and run:

```bash
go build -o /tmp/channelbridge ./cmd/channelbridge
/tmp/channelbridge
```

Bridge ingress endpoints:

- `POST /slack/events`
- `POST /slack/commands`
- `POST /slack/interactions`
- `POST /teams/messages`

Forward targets in gateway:

- `POST /api/v1/channels/slack/inbound`
- `POST /api/v1/channels/msteams/inbound`

Bridge auth controls:

- Slack request signature verification with `SLACK_SIGNING_SECRET`
- Teams ingress bearer gate with `MSTEAMS_INBOUND_BEARER`
- Teams Bot Framework JWT validation via `MSTEAMS_OPENID_CONFIG` + JWKS (`aud`, `iss`, `exp`, `nbf`, trusted service URL host)

Bridge observability and diagnostics:

- `GET /healthz`
- `GET /status`
- `GET /slack/probe` (Slack token diagnostics)
- `GET /teams/probe` (bot + graph diagnostics, permission coverage, capability checks)

---

## 4. Network and Ports

| Port | Service | Description |
|------|---------|-------------|
| 18790 | API Server | POST /chat endpoint |
| 18791 | Dashboard | REST API + Web UI |
| 18888 | Channel bridge (optional) | Slack/Teams ingress and outbound bridge |

Default bind: `127.0.0.1` (localhost only). Configure via:

```json
{
  "gateway": {
    "host": "127.0.0.1",
    "port": 18790,
    "dashboardPort": 18791
  }
}
```

Environment variables: `KAFCLAW_GATEWAY_HOST`, `KAFCLAW_GATEWAY_PORT`, `KAFCLAW_GATEWAY_DASHBOARD_PORT`.

CORS: All dashboard API endpoints include `Access-Control-Allow-Origin: *`.

---

## 5. Database

### Location

```
~/.kafclaw/timeline.db
```

SQLite with WAL mode, foreign keys, 5-second busy timeout.

### Core Tables

| Table | Purpose |
|-------|---------|
| `timeline` | Event log (messages, audio, images, system events) |
| `settings` | Key-value runtime settings |
| `tasks` | Agent task lifecycle tracking |
| `web_users` | Web UI user identities |
| `web_links` | Web user to WhatsApp JID mapping |
| `policy_decisions` | Tool access audit log |
| `approval_requests` | Interactive approval gates |
| `scheduled_jobs` | Cron job execution history |

### Memory Tables

| Table | Purpose |
|-------|---------|
| `memory_chunks` | Vector embeddings + metadata |
| `working_memory` | Per-user/thread scratchpads |
| `observations` | LLM-compressed conversation observations |
| `observations_queue` | Observer message queue |
| `agent_expertise` | Skill proficiency tracking |
| `skill_events` | Skill usage events |

### Group Tables

| Table | Purpose |
|-------|---------|
| `group_members` | Group roster |
| `group_tasks` | Delegated tasks |
| `group_traces` | Shared traces |
| `group_memory_items` | Shared memory |
| `group_skill_channels` | Skill registry |

### Key Settings

| Key | Description |
|-----|-------------|
| `whatsapp_allowlist` | Approved WhatsApp JIDs |
| `whatsapp_denylist` | Blocked JIDs |
| `whatsapp_pending` | JIDs awaiting approval |
| `daily_token_limit` | Daily token budget |
| `silent_mode` | Suppress outbound WhatsApp (default: true) |
| `bot_repo_path` | System/identity repo path |
| `work_repo_path` | Active work repo path |

---

## 6. Logging and Observability

### Structured Logging

Uses Go's `log/slog` with key-value pairs:

```
INFO  Agent loop started
INFO  Delivery worker started interval=5s max_retry=5
DEBUG Tool executed name=read_file result_length=1234
WARN  RAG search failed error=...
ERROR Failed to process message error=...
```

### Tracing

Every message gets a trace ID on ingestion (format: `trace-{unix_nano}`). Trace IDs link all events, tasks, and policy decisions for a single request.

### Token Usage

- Tracked per task (prompt, completion, total)
- Daily aggregation available
- Configurable `daily_token_limit` enforces quota before each LLM call
- Quota exceeded returns error message, skips LLM call

### Policy Audit Trail

Every tool call evaluation logged to `policy_decisions` with trace ID, task ID, tool, tier, sender, channel, allow/deny, reason.

### Task Lifecycle

```
pending --> processing --> completed
                      \-> failed

Delivery: pending --> sent / failed / skipped
```

Delivery worker polls every 5 seconds, retries up to 5 times with exponential backoff (30s * 2^attempts, max 5 minutes).

---

## 7. API Reference

### Port 18790 — API Server

| Method | Path | Description |
|--------|------|-------------|
| POST | `/chat?message=...&session=...` | Process message via agent loop |

Auth note:

- For direct HTTP clients: if `gateway.authToken` is configured, clients must send `Authorization: Bearer <token>` on `/chat`.
- For Slack/Teams/WhatsApp provider users: auth is enforced through provider bridge + channel access controls (not manual gateway bearer tokens).
- Direct clients obtain this token out-of-band from the operator; the API does not issue tokens.

### Port 18791 — Dashboard API

**Status and Auth:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/status` | Health, version, uptime, mode |
| POST | `/api/v1/auth/verify` | Bearer token validation |

`/api/v1/auth/verify` validates a supplied token and auth requirement state; it does not return or mint a token.

**Timeline and Traces:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/timeline` | Paginated events (limit, offset, sender, trace_id) |
| GET | `/api/v1/trace/{traceID}` | Detailed trace spans |
| GET | `/api/v1/trace-graph/{traceID}` | Trace execution graph |
| GET | `/api/v1/policy-decisions` | Policy audit log |

**Memory:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/memory/status` | Layer stats, observer, ER1, expertise |
| POST | `/api/v1/memory/reset` | Reset layer or all |
| POST | `/api/v1/memory/config` | Update memory settings |
| POST | `/api/v1/memory/prune` | Trigger lifecycle pruning |

**Settings and Repo:**

| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/api/v1/settings` | Runtime settings |
| GET/POST | `/api/v1/workrepo` | Work repo path |
| GET | `/api/v1/repo/tree` | File tree |
| GET | `/api/v1/repo/file?path=` | Read file |
| GET | `/api/v1/repo/status` | Git status |
| GET | `/api/v1/repo/branches` | List branches |
| GET | `/api/v1/repo/log` | Commit history |
| GET | `/api/v1/repo/diff` | Full diff |
| POST | `/api/v1/repo/checkout` | Switch branch |
| POST | `/api/v1/repo/commit` | Stage all + commit |
| POST | `/api/v1/repo/pull` | Pull (fast-forward) |
| POST | `/api/v1/repo/push` | Push |
| POST | `/api/v1/repo/init` | Initialize repo |
| POST | `/api/v1/repo/pr` | Create PR via gh |
| GET | `/api/v1/repo/search` | Search for repos |
| GET | `/api/v1/repo/gh-auth` | Check gh auth |

**Orchestrator:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/orchestrator/status` | Orchestrator state |
| GET | `/api/v1/orchestrator/hierarchy` | Agent tree |
| GET | `/api/v1/orchestrator/zones` | Zone list |
| POST | `/api/v1/orchestrator/dispatch` | Task dispatch |

**Group (20+ endpoints):**

| Prefix | Description |
|--------|-------------|
| `/api/v1/group/status` | Group state |
| `/api/v1/group/members` | Roster |
| `/api/v1/group/join` | Join |
| `/api/v1/group/leave` | Leave |
| `/api/v1/group/tasks/*` | Task delegation |
| `/api/v1/group/traces` | Shared traces |
| `/api/v1/group/memory` | Shared memory |
| `/api/v1/group/skills/*` | Skill registry |

**Web Chat and Users:**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/webchat/send` | Send message from web UI |
| GET/POST | `/api/v1/webusers` | List/create web users |
| POST | `/api/v1/webusers/force` | Toggle force-send |
| GET/POST | `/api/v1/weblinks` | Web user to WhatsApp JID links |

**Tasks and Approvals:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/tasks` | List tasks (status, channel, limit) |
| GET | `/api/v1/tasks/{taskID}` | Get task details |
| GET | `/api/v1/approvals/pending` | Pending approvals |
| POST | `/api/v1/approvals/{id}` | Approve/deny |

### Port 18888 — channel bridge sidecar

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Liveness |
| GET | `/status` | Counters and caches |
| GET | `/slack/probe` | Slack token/auth probe |
| GET | `/teams/probe` | Teams bot + graph credential diagnostics |
| POST | `/slack/events` | Slack Events API ingress |
| POST | `/slack/commands` | Slack slash command ingress |
| POST | `/slack/interactions` | Slack interactions ingress |
| POST | `/teams/messages` | Teams bot activity ingress |

---

## 8. Health Checks and Backup

### Health Checks

```bash
# Check API server
curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:18790/chat

# Check dashboard
curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:18791/api/v1/status

# Check ports
lsof -i tcp:18790 -sTCP:LISTEN
lsof -i tcp:18791 -sTCP:LISTEN
```

### Backup

| Path | Description |
|------|-------------|
| `~/.kafclaw/timeline.db` | Main database |
| `~/.kafclaw/whatsapp.db` | WhatsApp session |
| `~/.kafclaw/config.json` | Configuration |
| `~/.kafclaw/workspace/` | Soul files, sessions, media |

```bash
BACKUP_DIR="$HOME/kafclaw-backup-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"
sqlite3 ~/.kafclaw/timeline.db ".backup '$BACKUP_DIR/timeline.db'"
cp ~/.kafclaw/whatsapp.db "$BACKUP_DIR/" 2>/dev/null || true
cp ~/.kafclaw/config.json "$BACKUP_DIR/" 2>/dev/null || true
cp -r ~/.kafclaw/workspace "$BACKUP_DIR/" 2>/dev/null || true
```

---

## 9. Graceful Shutdown

### Signal Handling

The gateway listens for `SIGINT` (Ctrl+C) and `SIGTERM`:

```
Signal received
    |
    v
WhatsApp channel stopped
    |
    v
Agent loop stopped
    |
    v
ER1 sync stopped
    |
    v
Observer stopped
    |
    v
Timeline database closed
    |
    v
Process exits
```

### Port Cleanup

After a crash:

```bash
make rerun   # auto-kills processes on 18790/18791, rebuilds, starts
```

Manual:

```bash
lsof -ti tcp:18790 -sTCP:LISTEN | xargs kill
lsof -ti tcp:18791 -sTCP:LISTEN | xargs kill
```

### Dashboard Failure

If the dashboard server fails to bind its port, it triggers context cancellation that stops the entire gateway. The dashboard is considered essential for operation.

### channel bridge troubleshooting

If Slack/Teams messages are not processing:

1. Verify bridge liveness: `curl -s http://127.0.0.1:18888/healthz`
2. Verify KafClaw channel outbound URL targets bridge endpoints
3. Probe credentials:
   - `curl -s http://127.0.0.1:18888/slack/probe`
   - `curl -s http://127.0.0.1:18888/teams/probe`
4. Verify inbound tokens and provider auth settings (signing secret, bearer, app credentials)
5. Inspect timeline delivery reason taxonomy for retry state:
   - `transient:rate_limited`
   - `transient:upstream_5xx`
   - `transient:network`
   - `terminal:unauthorized`
   - `terminal:invalid_target_or_payload`
   - `terminal:max_retries_exceeded`
   - `terminal:send_failed`
