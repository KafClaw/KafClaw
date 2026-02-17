# KafClaw Administration Guide

Comprehensive reference for deploying, configuring, securing, and operating KafClaw.

---

## Table of Contents

1. [Configuration Reference](#1-configuration-reference)
2. [Security Model](#2-security-model)
3. [LLM Provider Configuration](#3-llm-provider-configuration)
4. [Memory and RAG Administration](#4-memory-and-rag-administration)
5. [Token Quota Management](#5-token-quota-management)
6. [Extending KafClaw](#6-extending-kafclaw)
7. [Runtime Settings](#7-runtime-settings)
8. [Web User Management](#8-web-user-management)
9. [Audit and Compliance](#9-audit-and-compliance)

---

## 1. Configuration Reference

### Config File Location

```
~/.kafclaw/config.json
```

Created by `kafclaw onboard`. Directory: `0700` permissions. File: `0600` permissions.

### Loading Order

Configuration values are resolved in this precedence (highest wins):

1. **Environment variables** (prefix: `KAFCLAW_`)
2. **Config file** (`~/.kafclaw/config.json`)
3. **Built-in defaults** (from `DefaultConfig()`)

### Root Config Struct

```go
type Config struct {
    Agents       AgentsConfig       `json:"agents"`
    Channels     ChannelsConfig     `json:"channels"`
    Providers    ProvidersConfig    `json:"providers"`
    Gateway      GatewayConfig      `json:"gateway"`
    Tools        ToolsConfig        `json:"tools"`
    Group        GroupConfig        `json:"group"`
    Orchestrator OrchestratorConfig `json:"orchestrator"`
    Scheduler    SchedulerConfig    `json:"scheduler"`
    ER1          ER1Config          `json:"er1"`
    Observer     ObserverConfig     `json:"observer"`
}
```

### Agent Configuration

| Field | Default | Env Var | Description |
|-------|---------|---------|-------------|
| `Workspace` | `~/.kafclaw/workspace` | `KAFCLAW_AGENTS_WORKSPACE` | Soul files, session state, agent workspace |
| `WorkRepoPath` | `~/.kafclaw/work-repo` | `KAFCLAW_AGENTS_WORK_REPO_PATH` | Root for filesystem write operations |
| `SystemRepoPath` | *(machine-specific)* | `KAFCLAW_AGENTS_SYSTEM_REPO_PATH` | System/identity repo (skills, Day2Day) |
| `Model` | `anthropic/claude-sonnet-4-5` | `KAFCLAW_AGENTS_MODEL` | Default LLM model |
| `MaxTokens` | `8192` | `KAFCLAW_AGENTS_MAX_TOKENS` | Max tokens per LLM response |
| `Temperature` | `0.7` | `KAFCLAW_AGENTS_TEMPERATURE` | LLM sampling temperature |
| `MaxToolIterations` | `20` | `KAFCLAW_AGENTS_MAX_TOOL_ITERATIONS` | Max agentic loop iterations per message |

### Provider Configuration

| Provider | Default API Base | Notes |
|----------|-----------------|-------|
| OpenAI | `https://api.openai.com/v1` | Used when apiBase is empty |
| OpenRouter | `https://openrouter.ai/api/v1` | OpenAI-compatible format |
| Anthropic | Via OpenRouter | Requires OpenRouter key |
| DeepSeek | Custom apiBase | OpenAI-compatible |
| Groq | Custom apiBase | OpenAI-compatible |
| Gemini | Custom apiBase | OpenAI-compatible |
| VLLM | Custom apiBase | Local deployment |
| LocalWhisper | N/A | Binary at `/opt/homebrew/bin/whisper` |

### Channel Configuration

Each channel config has:

| Field | Type | Description |
|-------|------|-------------|
| `Enabled` | `bool` | Whether the channel is active |
| `Token` / `AppID` | `string` | Authentication credential |
| `AllowFrom` | `[]string` | Sender allowlist |

WhatsApp-specific:

| Field | Env Var | Default | Description |
|-------|---------|---------|-------------|
| `DropUnauthorized` | `KAFCLAW_CHANNELS_WHATSAPP_DROP_UNAUTHORIZED` | `false` | Silently drop from unknown senders |
| `IgnoreReactions` | `KAFCLAW_CHANNELS_WHATSAPP_IGNORE_REACTIONS` | `false` | Ignore reaction messages |

### Gateway Configuration

| Field | Default | Env Var | Description |
|-------|---------|---------|-------------|
| `Host` | `127.0.0.1` | `KAFCLAW_GATEWAY_HOST` | Bind address |
| `Port` | `18790` | `KAFCLAW_GATEWAY_PORT` | API port |
| `DashboardPort` | `18791` | `KAFCLAW_GATEWAY_DASHBOARD_PORT` | Dashboard port |
| `AuthToken` | *(empty)* | `KAFCLAW_GATEWAY_AUTH_TOKEN` | Required for headless mode |
| `TLSCert` | *(empty)* | — | Optional TLS certificate path |
| `TLSKey` | *(empty)* | — | Optional TLS private key path |

**LAN access:** The default `Host: 127.0.0.1` only accepts local connections. To expose the gateway on your network, set `Host` to `0.0.0.0` (all interfaces) or a specific LAN IP, and set `AuthToken` to secure it. Use `make run-headless` for the recommended configuration. The gateway serves plain HTTP — do not use `https://` in the browser unless TLS is configured.

### Tools Configuration

| Field | Default | Env Var | Description |
|-------|---------|---------|-------------|
| `Exec.Timeout` | `60s` | — | Shell command timeout |
| `Exec.RestrictToWorkspace` | `true` | `KAFCLAW_TOOLS_EXEC_RESTRICT_WORKSPACE` | Confine shell to workspace |
| `Web.Search.MaxResults` | `10` | — | Max web search results |
| `Web.Search.APIKey` | *(empty)* | `KAFCLAW_BRAVE_API_KEY` | Brave Search API key |
| `Subagents.MaxConcurrent` | `8` | `KAFCLAW_TOOLS_SUBAGENTS_MAX_CONCURRENT` | Max active subagent runs globally |
| `Subagents.MaxSpawnDepth` | `1` | `KAFCLAW_TOOLS_SUBAGENTS_MAX_SPAWN_DEPTH` | Max spawn depth (default prevents nested child spawning) |
| `Subagents.MaxChildrenPerAgent` | `5` | `KAFCLAW_TOOLS_SUBAGENTS_MAX_CHILDREN_PER_AGENT` | Max active child runs per parent session |
| `Subagents.ArchiveAfterMinutes` | `60` | `KAFCLAW_TOOLS_SUBAGENTS_ARCHIVE_AFTER_MINUTES` | Subagent retention/archive window |
| `Subagents.AllowAgents` | *(current agent only)* | `KAFCLAW_TOOLS_SUBAGENTS_ALLOW_AGENTS` | Allowed `agentId` values for `sessions_spawn` (`*` allows any) |
| `Subagents.Model` | *(inherit main model)* | `KAFCLAW_TOOLS_SUBAGENTS_MODEL` | Default model for spawned subagents |
| `Subagents.Thinking` | *(empty)* | `KAFCLAW_TOOLS_SUBAGENTS_THINKING` | Default thinking level for spawned subagents |

Subagent control operations:

- `sessions_spawn`: enqueue child run in background
- `sessions_spawn(agentId=<id>)`: target a specific agent identity (must be allowed by `tools.subagents.allowAgents`)
- `sessions_spawn(timeoutSeconds=<n>)`: compatibility alias for `runTimeoutSeconds`
- `sessions_spawn(cleanup=delete|keep)`: child-session cleanup mode after completion announce
- `agents_list`: discover allowed spawn targets (`agentId`) for the current agent/session
- `subagents(action=list)`: show runs for current root-session scope
- `subagents(action=kill,target=<selector>)`: stop run (cascade kill descendants)
- `subagents(action=kill_all)`: stop all active child runs for current root session scope
- `subagents(action=steer,target=<selector>,input=<text>)`: stop target and spawn a steered replacement run
- child loop policy is depth-aware: `sessions_spawn` is denied at/after max depth
- optional child allow/deny policy via `tools.subagents.tools.allow` and `tools.subagents.tools.deny` (wildcard suffix `*` supported)
- selectors for `target`: run ID, `last`, numeric index, label prefix, or child session key
- `sessions_spawn` supports `runTimeoutSeconds` for per-run timeout
- subagent completion announce retries are tracked with persisted backoff state
- subagent completion announce output is normalized to `Status/Result/Notes` and supports `ANNOUNCE_SKIP`

`agents_list` response contract:

- `currentAgentId`: resolved current agent identity
- `allowAgents`: raw configured allowlist (`*` if wildcard enabled)
- `effectiveTargets`: concrete spawn targets for `sessions_spawn(agentId=...)`
- `wildcard`: whether allowlist contains `*`
- `agents[]`: normalized entries with:
  - `id`
  - `configured` (true when present in effective target set)
  - `name` (optional; populated from `agents.list[].name` when configured)

Audit:

- subagent lifecycle writes timeline `SYSTEM` events with classification `SUBAGENT` (`spawn_accepted`, `kill`, `steer`) when trace IDs are active.
- subagent registry persists under `~/.kafclaw/subagents/` and is restored on restart.

Announce routing parity:

- completion announces resolve target route in this order:
  - explicit requester channel/chat
  - spawn-time defaults
  - requester/root/parent session-key fallback
  - active session fallback (`channel:chat`)

### ER1 Configuration

| Field | Default | Env Var | Description |
|-------|---------|---------|-------------|
| `URL` | *(empty)* | `KAFCLAW_ER1_URL` | ER1 service URL |
| `APIKey` | *(empty)* | `KAFCLAW_ER1_API_KEY` | ER1 API key |
| `UserID` | *(empty)* | `KAFCLAW_ER1_USER_ID` | ER1 user ID |
| `SyncInterval` | `5m` | `KAFCLAW_ER1_SYNC_INTERVAL` | Sync frequency |

### Observer Configuration

| Field | Default | Env Var | Description |
|-------|---------|---------|-------------|
| `Enabled` | `false` | `KAFCLAW_OBSERVER_ENABLED` | Enable observation |
| `Model` | *(agent default)* | `KAFCLAW_OBSERVER_MODEL` | LLM for compression |
| `MessageThreshold` | `50` | `KAFCLAW_OBSERVER_MESSAGE_THRESHOLD` | Messages before observe |
| `MaxObservations` | `200` | `KAFCLAW_OBSERVER_MAX_OBSERVATIONS` | Max before reflect |

---

## 2. Security Model

KafClaw implements defense in depth: tool tiering, policy evaluation, shell filtering, filesystem confinement, and attack intent detection.

> See also: [FR-006 Core Functional Requirements](../requirements/FR-006-core-functional-requirements.md)

### Tool Risk Tiers

| Tier | Level | Tools | Description |
|------|-------|-------|-------------|
| 0 | ReadOnly | `read_file`, `list_dir`, `resolve_path`, `recall` | Always allowed |
| 1 | Write | `write_file`, `edit_file`, `remember` | Allowed for internal senders |
| 2 | HighRisk | `exec` | Requires internal sender + approval or MaxAutoTier >= 2 |

### Policy Engine

Evaluation flow:
1. Tier 0 → always allow (`tier_0_always_allowed`)
2. Check sender allowlist (if configured)
3. Determine effective max tier by message type:
   - Internal (owner, WhatsApp allowlist, CLI, scheduler): MaxAutoTier = 2
   - External (unknown sender): ExternalMaxTier = 0
4. Tool tier > effective max → deny (external) or require approval (internal)
5. Log decision to `policy_decisions` table

### Shell Security

**Strict allow-list mode** (default):

```
git, ls, cat, pwd, rg, grep, sed, head, tail, wc, echo
```

**Deny patterns:**

| Category | Patterns |
|----------|----------|
| Recursive deletion | `rm -rf`, `rm -r .`, `rm *`, `find -delete`, `unlink`, `rmdir` |
| VCS deletion | `git rm` |
| Device ops | `dd of=/dev/`, `mkfs`, `fdisk`, `format` |
| Device redirect | `> /dev/` |
| Permission escalation | `chmod -R 777`, `chown -R` on root/home |
| Fork bombs | `:(){ :|:& };:` |
| System control | `shutdown`, `reboot`, `halt`, `init [0-6]`, `systemctl` |

**Path traversal:** `../`, `..\`, `/..`, `\..` — rejected when workspace restriction is enabled.

**Timeout:** Default 60 seconds. Commands exceeding timeout are killed.

### Filesystem Security

> See also: [FR-005 Bot Work Repo](../requirements/FR-005-bot-work-repo.md)

- `read_file`, `list_dir`: Can access any path (Tier 0)
- `write_file`, `edit_file`: Restricted to work repo root (Tier 1). Writes outside return error.
- `filepath.Rel()` used to verify paths are within work repo
- Tilde expansion: `~` expanded to home directory

### Attack Intent Detection

The agent loop scans user messages for malicious patterns before LLM processing:

```
delete repo, remove repo, wipe repo, delete all files,
rm -rf, losch repo, datei(en) losch
```

Detected → safety response returned, message not processed.

### WhatsApp Authorization

Three-tier system stored in timeline DB:

| Setting Key | Description |
|-------------|-------------|
| `whatsapp_allowlist` | Approved JIDs |
| `whatsapp_denylist` | Blocked JIDs |
| `whatsapp_pending` | JIDs awaiting approval |

Default-deny: empty allowlist means nobody is authorized. See [FR-001](../requirements/FR-001-whatsapp-auth-flow.md).

---

## 3. LLM Provider Configuration

### Provider Architecture

All providers use the OpenAI-compatible API format via a single `OpenAIProvider` implementation.

```go
type LLMProvider interface {
    Chat(ctx, *ChatRequest) (*ChatResponse, error)
    Transcribe(ctx, *AudioRequest) (*AudioResponse, error)
    Speak(ctx, *TTSRequest) (*TTSResponse, error)
    DefaultModel() string
}

type Embedder interface {
    Embed(ctx, *EmbeddingRequest) (*EmbeddingResponse, error)
}
```

### Capabilities

| Capability | Endpoint | Default Model |
|------------|----------|---------------|
| Chat completion | `/chat/completions` | `anthropic/claude-sonnet-4-5` |
| Audio transcription | `/audio/transcriptions` | `whisper-1` |
| Text-to-speech | `/audio/speech` | `tts-1` (voice: nova, format: opus) |
| Embeddings | `/embeddings` | `text-embedding-3-small` |

### API Key Fallback Chain

1. `cfg.Providers.OpenAI.APIKey` (config or `KAFCLAW_OPENAI_API_KEY`)
2. `OPENAI_API_KEY` environment variable
3. `OPENROUTER_API_KEY` environment variable

---

## 4. Memory and RAG Administration

> See also: [FR-019 Memory Architecture](../requirements/FR-019-memory-architecture.md)

### Architecture

```
User Query --> Embed(query) --> VectorStore.Search(top 5) --> Filter(score >= 0.3)
                                                                   |
                                                         Inject into system prompt
```

### Components

| Component | Description |
|-----------|-------------|
| `MemoryService` | High-level store/search with auto-embedding |
| `SQLiteVecStore` | SQLite-based vector store (zero dependencies) |
| `AutoIndexer` | Background batch indexer |
| `SoulFileIndexer` | Indexes soul files by `##` headers |
| `Observer` | LLM conversation compression |
| `WorkingMemoryStore` | Per-user/thread scratchpads |
| `ER1Client` | Personal memory sync |
| `ExpertiseTracker` | Skill proficiency scoring |
| `LifecycleManager` | TTL pruning, max chunks enforcement |

### Memory Layers

| Layer | Source Prefix | TTL | Description |
|-------|--------------|-----|-------------|
| Soul | `soul:` | Permanent | Identity files |
| Conversation | `conversation:` | 30 days | Auto-indexed Q&A |
| Tool | `tool:` | 14 days | Tool outputs |
| Group | `group:` | 60 days | Shared via Kafka |
| ER1 | `er1:` | Permanent | Personal sync |
| Observation | `observation:` | Permanent | LLM-compressed |

### Dashboard Memory API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/memory/status` | GET | Layer stats, observer, ER1, expertise |
| `/api/v1/memory/reset` | POST | Reset layer or all |
| `/api/v1/memory/config` | POST | Update memory settings |
| `/api/v1/memory/prune` | POST | Trigger lifecycle pruning |

### Graceful Degradation

If no embedder available (provider doesn't support it):
- `Store()` returns `("", nil)` — no error, no storage
- `Search()` returns `(nil, nil)` — no error, no results
- Memory tools (`remember`, `recall`) not registered

---

## 5. Token Quota Management

### Per-Task Tracking

Every LLM call records usage via `UpdateTaskTokens()`. Counts are accumulated additively per task.

### Daily Token Limit

Configured as runtime setting:

```
Key: daily_token_limit
Value: integer (e.g., "100000")
```

Enforcement (checked before every LLM call):
1. Read `daily_token_limit` from settings
2. If not set or not positive → skip check (unlimited)
3. Sum `total_tokens` for all tasks created today
4. If usage >= limit → return error, skip LLM call

### Setting the Quota

Via dashboard API:

```bash
curl -X POST http://localhost:18791/api/v1/settings \
  -H "Content-Type: application/json" \
  -d '{"key": "daily_token_limit", "value": "100000"}'
```

Set to `0` or empty to remove quota.

---

## 6. Extending KafClaw

### Adding a New Tool

1. Implement `Tool` interface in `internal/tools/`:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]any   // JSON Schema
    Execute(ctx context.Context, params map[string]any) (string, error)
}
```

2. Optionally implement `TieredTool` for risk tier (0=ReadOnly, 1=Write, 2=HighRisk). Default: Tier 0.

3. Register in `registerDefaultTools()` in `internal/agent/loop.go`.

4. Tool automatically appears in LLM tool definitions and is subject to policy evaluation.

### Adding a New Channel

1. Implement `Channel` interface in `internal/channels/`
2. Subscribe to message bus for outbound delivery
3. Publish inbound messages to bus
4. Add config fields to `internal/config/config.go`
5. Initialize in `gateway.go` startup sequence

### Adding a New CLI Command

1. Create file in `internal/cli/`
2. Define `cobra.Command`
3. Register in `root.go` `init()`

### Adding a New Memory Source

1. Choose source prefix (e.g., `newsource:`)
2. Add retention policy to `DefaultPolicies()` in `lifecycle.go`
3. Call `MemoryService.Store()` with new prefix
4. Chunks automatically appear in RAG search results

### Adding a New API Endpoint

1. Add `mux.HandleFunc()` in `gateway.go`
2. Set CORS headers
3. Handle OPTIONS preflight
4. Use service instances from closure scope

---

## 7. Runtime Settings

Stored in `settings` table of `~/.kafclaw/timeline.db`.

### API Access

- Read: `GET /api/v1/settings` (use `?key=name` for specific key)
- Write: `POST /api/v1/settings` with `{"key": "...", "value": "..."}`

### Known Keys

| Key | Type | Description |
|-----|------|-------------|
| `whatsapp_allowlist` | Newline-separated JIDs | Approved senders |
| `whatsapp_denylist` | Newline-separated JIDs | Blocked senders |
| `whatsapp_pending` | Newline-separated JIDs | Awaiting approval |
| `whatsapp_pair_token` | String | Pairing token |
| `daily_token_limit` | Integer string | Daily token budget |
| `bot_repo_path` | Path | System/identity repo |
| `selected_repo_path` | Path | Selected repo in dashboard |
| `silent_mode` | `"true"/"false"` | Suppress outbound WhatsApp (default: true) |
| `group_name` | String | Active group name |
| `group_active` | `"true"/"false"` | Group participation state |
| `kafscale_lfs_proxy_url` | URL | LFS proxy URL |

---

## 8. Web User Management

> See also: [FR-007 Web UI WhatsApp Linking](../requirements/FR-007-web-ui-whatsapp-linking.md)

### Web Users

Web UI user identities stored in `web_users` table.

Operations:
- `CreateWebUser(name)` — Create or return existing
- `ListWebUsers()` — All users sorted by name
- `SetWebUserForceSend(id, bool)` — Toggle force delivery

### Web Links (Cross-Channel Identity)

Link web user to WhatsApp JID for cross-channel identity.

- `LinkWebUser(webUserID, whatsappJID)` — Upsert
- `UnlinkWebUser(webUserID)` — Remove link
- `GetWebLink(webUserID)` — Returns JID

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/webusers` | GET | List all web users |
| `/api/v1/webusers` | POST | Create web user |
| `/api/v1/webusers/force` | POST | Set force-send flag |
| `/api/v1/weblinks` | GET | Get links |
| `/api/v1/weblinks` | POST | Create/update link |
| `/api/v1/webchat/send` | POST | Send message as web user |

---

## 9. Audit and Compliance

### Policy Decision Log

Every tool invocation triggers a policy evaluation logged to `policy_decisions`:

| Field | Description |
|-------|-------------|
| `trace_id` | End-to-end request trace |
| `task_id` | Agent task that triggered the call |
| `tool` | Tool name |
| `tier` | Tool risk tier |
| `sender` | Sender identifier |
| `channel` | Channel name |
| `allowed` | Whether permitted |
| `reason` | Human-readable reason |

### Task Tracking

Every inbound message creates an AgentTask record:

```
pending --> processing --> completed / failed
```

Preserves `content_in`, `content_out`, `error_text`, token usage, delivery status.

### Deduplication

Idempotency key (format: `auto:{channel}:{trace_id}`) prevents duplicate processing. Completed tasks return cached response.

### Timeline Event History

All interactions logged with full tracing (trace_id, span_id, parent_span_id).

### Querying Audit Data

```bash
# Timeline events
GET /api/v1/timeline?sender=49123456789@s.whatsapp.net&trace_id=trace-123

# Policy decisions
GET /api/v1/policy-decisions?trace_id=trace-123

# Tasks
GET /api/v1/tasks?status=completed&channel=whatsapp&limit=50
```

---

## Database Location

All persistent state in a single SQLite database:

```
~/.kafclaw/timeline.db
```

Uses WAL journal mode, foreign keys, and a 5-second busy timeout.
