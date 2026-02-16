# GoMikroBot System Architecture

> Version: 2.5.3 | Updated: 2026-02-16

---

## 1. Overview

GoMikroBot is a personal AI assistant framework written in Go with an Electron desktop frontend. It connects messaging channels (WhatsApp, local network, web UI) to LLM providers through an asynchronous message bus, with a tool registry for filesystem/shell/web operations, a 6-layer semantic memory system, policy-based authorization, multi-agent collaboration via Kafka, and a cron-based job scheduler.

```
WhatsApp ─┐
CLI ──────┤                                    ┌── Filesystem Tools
Local ────┼── Message Bus ── Agent Loop ── LLM ┼── Shell Execution
Web UI ───┤       │              │              ├── Web Search
Scheduler ┘       │              │              └── Memory (remember/recall)
                  │              │
                  │         ContextBuilder
                  │         ├── Soul Files (AGENTS.md, SOUL.md, ...)
                  │         ├── Working Memory (per-user scratchpad)
                  │         ├── Observations (compressed history)
                  │         ├── RAG Injection (vector search)
                  │         └── Tool + Skill definitions
                  │
              Timeline DB (SQLite)
              ├── Event log
              ├── Memory chunks (embeddings)
              ├── Settings, tasks, approvals
              ├── Group roster, skill channels
              └── Orchestrator hierarchy
```

### Design Principles

- **Message bus decoupling**: Channels never call the agent directly
- **Graceful degradation**: Memory, group, orchestrator, ER1 are all optional
- **Behavior parity first**: Migration from Python preserves behavior before optimizing
- **Secure defaults**: Binds 127.0.0.1, tier-restricted tool execution, deny-pattern filtering
- **Single SQLite database**: All persistent state in `~/.gomikrobot/timeline.db`

---

## 2. Directory Structure

```
gomikrobot/
├── cmd/gomikrobot/cmd/           # CLI commands (Cobra)
│   ├── root.go                   # Root command, version (v2.5.3)
│   ├── gateway.go                # Main daemon (~2800 lines, all API endpoints)
│   ├── agent.go                  # Single-message CLI mode
│   ├── onboard.go                # First-time setup wizard
│   ├── status.go                 # Config/key/session health check
│   ├── group.go                  # Group collaboration CLI (join/leave/status/members)
│   ├── kshark.go                 # Kafka connectivity diagnostics
│   ├── install.go                # System install (/usr/local/bin)
│   └── whatsapp_*.go             # WhatsApp setup and auth
│
├── internal/
│   ├── agent/                    # Core agent loop + context builder
│   ├── approval/                 # Interactive tool approval workflow
│   ├── bus/                      # Async message bus (pub-sub)
│   ├── channels/                 # External integrations (WhatsApp)
│   ├── config/                   # Configuration (env/file/defaults)
│   ├── cron/                     # Cron expression parser
│   ├── group/                    # Multi-agent collaboration (Kafka)
│   ├── kshark/                   # Kafka diagnostics library
│   ├── memory/                   # 6-layer semantic memory system
│   ├── orchestrator/             # Agent hierarchy and zones
│   ├── policy/                   # Tiered tool authorization
│   ├── provider/                 # LLM abstraction (OpenAI, OpenRouter, Whisper)
│   ├── scheduler/                # Cron-based job scheduling
│   ├── session/                  # JSONL conversation persistence
│   ├── timeline/                 # SQLite event log + schema
│   ├── tools/                    # Tool registry + implementations
│   └── util/                     # Shared utilities
│
├── electron/                     # Electron desktop application
│   ├── src/main/                 # Main process (sidecar, IPC, menus)
│   ├── src/preload/              # Context-isolated bridge
│   └── src/renderer/             # Vue 3 SPA (Pinia, Vite)
│       ├── stores/               # agent, memory, orchestrator, mode, remote
│       ├── views/                # Dashboard, Memory, Orchestrator, Settings, ...
│       ├── components/           # MemoryPipeline, LayerCard, HierarchyGraph, ...
│       └── composables/          # useApi, useSidecar
│
├── web/                          # Go-served HTML dashboards
├── operations/                   # Day-to-day task templates
├── SPEC/                         # Architecture specifications
├── Makefile                      # Build/run/release targets
├── go.mod                        # Go 1.24.0, whatsmeow, kafka-go, sqlite
└── Dockerfile / docker-compose   # Container deployment
```

---

## 3. Startup Sequence (Gateway)

The gateway command (`runGateway`) initializes all subsystems in a specific dependency order:

```
 1. Load Config          (env > ~/.gomikrobot/config.json > defaults)
 2. Open Timeline DB     (~/.gomikrobot/timeline.db, schema migration)
 3. Seed Settings        (bot_repo_path, work_repo_path, lfs_proxy_url)
 4. Create Message Bus   (100-msg inbound/outbound buffers)
 5. Initialize Provider  (OpenAI + optional LocalWhisper wrapper)
 6. Create Policy Engine (MaxAutoTier=2 internal, ExternalMaxTier=0)
 7. Setup Memory System
    a. SQLiteVecStore    (1536-dim embeddings in timeline.db)
    b. MemoryService     (embed + upsert + search)
    c. AutoIndexer       (background batch indexer)
    d. ExpertiseTracker  (skill proficiency)
    e. WorkingMemoryStore(per-user/thread scratchpads)
    f. Observer          (LLM conversation compression)
    g. ER1Client         (personal memory sync)
    h. LifecycleManager  (TTL pruning, daily)
 8. Setup Group (optional: Kafka consumer, group manager, router)
 9. Setup Orchestrator   (optional: hierarchy, zones, discovery)
10. Create Agent Loop    (wires bus, provider, memory, policy, tools)
11. Index Soul Files     (background: AGENTS.md, SOUL.md, etc.)
12. Start Channels       (WhatsApp via whatsmeow)
13. Start HTTP Servers
    a. API server        (port 18790: /chat endpoint)
    b. Dashboard server  (port 18791: REST API + web UI)
14. Start Background Workers
    a. AutoIndexer       (goroutine)
    b. ER1 SyncLoop      (goroutine, every 5min)
    c. LifecycleManager  (goroutine, daily)
    d. Scheduler         (goroutine, if enabled)
    e. Bus Dispatcher    (goroutine)
    f. Delivery Worker   (goroutine)
15. Start Agent Loop     (goroutine, consumes from bus)
16. Start Orchestrator   (goroutine, if enabled)
17. Start Group Router   (goroutine, Kafka consumer)
18. Wait for SIGINT/SIGTERM
```

---

## 4. Message Processing Pipeline

### 4.1 Inbound Flow

```
Channel receives message
    │
    ▼
Message Bus (PublishInbound)
    │
    ▼
Agent Loop (ConsumeInbound)
    │
    ├─ Dedup via IdempotencyKey
    ├─ Create timeline task (pending)
    ├─ Detect Day2Day commands (dtu/dtp/dts/dtn/dta/dtc)
    ├─ Detect attack intent (German + English patterns)
    ├─ Intercept approval responses (approve:<id> / deny:<id>)
    │
    ▼
processMessage()
    │
    ├─ Load/create session
    ├─ Classify message type (internal vs external)
    │
    ▼
Context Assembly (ContextBuilder)
    │
    ├─ 1. Identity      (version, date, workspace paths)
    ├─ 2. Bootstrap      (AGENTS.md, SOUL.md, USER.md, TOOLS.md, IDENTITY.md)
    ├─ 3. Working Memory (scoped per user/thread from SQLite)
    ├─ 4. Observations   (compressed session history, priority-sorted)
    ├─ 5. Skills Summary (registered tools + skill docs)
    ├─ 6. RAG Context    (vector search across all 6 memory layers)
    └─ 7. Conversation   (recent message history from session)
         │
         ▼
    runAgentLoop()
    │
    ├─ LLM Call (messages + tool definitions)
    │   │
    │   ▼
    │  finish_reason == "tool_calls"?
    │   ├─ YES: for each tool call:
    │   │   ├─ Policy check (tier vs sender vs message type)
    │   │   ├─ If RequiresApproval → approval workflow
    │   │   ├─ Execute tool (registry.Execute)
    │   │   ├─ Record expertise event
    │   │   ├─ Auto-index tool result (if substantive)
    │   │   ├─ Add result to messages
    │   │   └─ Loop back to LLM Call
    │   └─ NO: return final response
    │
    ▼
Post-Processing
    │
    ├─ Save to session (JSONL)
    ├─ Auto-index conversation pair
    ├─ Enqueue messages for observer
    ├─ Update working memory (if agent called update_working_memory)
    ├─ Track expertise (task completion)
    ├─ Publish group trace (if group enabled)
    └─ Publish outbound response
         │
         ▼
    Message Bus (PublishOutbound)
         │
         ▼
    Channel.Send() (WhatsApp, web UI, etc.)
```

### 4.2 Message Classification

Every inbound message carries a type in its metadata:

| Type | Source | Max Auto Tier | Access Level |
|------|--------|---------------|-------------|
| `internal` | Owner (WhatsApp allowlist, CLI, scheduler) | 2 (shell) | Full |
| `external` | Non-owner (unknown sender) | 0 (read-only) | Restricted |

The policy engine uses this classification to gate tool execution.

---

## 5. Package Architecture

### 5.1 internal/agent — Core Loop

**Files**: `loop.go` (~1200 lines), `context.go`, `delivery.go`

`LoopOptions` wires all dependencies:
```go
type LoopOptions struct {
    Bus              *bus.MessageBus
    Provider         provider.LLMProvider
    Timeline         *timeline.TimelineService
    Policy           policy.Engine
    MemoryService    *memory.MemoryService
    AutoIndexer      *memory.AutoIndexer
    ExpertiseTracker *memory.ExpertiseTracker
    WorkingMemory    *memory.WorkingMemoryStore
    Observer         *memory.Observer
    GroupPublisher   GroupTracePublisher
    Workspace, WorkRepo, SystemRepo string
    WorkRepoGetter   func() string
    Model            string
    MaxIterations    int   // default: 20
}
```

The `Loop` struct tracks active processing state (taskID, sender, channel, chatID, traceID, messageType) used by tool execution and audit logging.

**ContextBuilder** assembles the system prompt from modular files:
- `BuildSystemPrompt()` — Identity + bootstrap files + skills
- `BuildMessages()` — Constructs `provider.Message` list with history
- `AssessTask()` — Lightweight classification (security/creative/architecture/bug-fix/quick-answer)
- `BuildIdentityEnvelope()` — Creates `AgentIdentity` for group collaboration

**Day2Day Task Manager** — Built-in commands for daily task tracking:
- `dtu` (update), `dtp` (progress), `dts` (consolidate), `dtn` (next), `dta` (all), `dtc` (close)
- Stores tasks in workspace markdown files with `[ ]`/`[x]` checkbox format

### 5.2 internal/bus — Message Bus

**Files**: `bus.go`

Async pub-sub decoupling channels from the agent loop.

```go
type InboundMessage struct {
    Channel, SenderID, ChatID, TraceID string
    IdempotencyKey string     // dedup key
    Content        string
    Media          []byte
    Metadata       map[string]string  // includes "message_type"
    Timestamp      time.Time
}

type OutboundMessage struct {
    Channel, ChatID, TraceID, TaskID, Content string
}
```

- Buffered channels: 100 inbound, 100 outbound
- `Subscribe(channel, callback)` — channels register for outbound delivery
- `DispatchOutbound()` — runs as goroutine, fans out to subscribers
- `MessageType()` — extracts `internal`/`external` from metadata (defaults to `external`)

### 5.3 internal/channels — External Integrations

**Interface**:
```go
type Channel interface {
    Name() string
    Start(ctx context.Context) error
    Stop() error
    Send(ctx context.Context, msg *OutboundMessage) error
}
```

**WhatsApp** (via whatsmeow — native protocol, no Node bridge):
- Session stored in `~/.gomikrobot/whatsapp.db`
- Config: enabled, allowFrom list, dropUnauthorized, ignoreReactions
- JID normalization (phone numbers to WhatsApp format)
- Audio transcription via provider (Whisper)
- Image handling (receives, stores to temp, references in content)

### 5.4 internal/config — Configuration

Loading precedence: Environment vars (`MIKROBOT_*`) > `~/.gomikrobot/config.json` > defaults.

```
Config
├── Paths
│   ├── Workspace        (default: ~/GoMikroBot-Workspace)
│   ├── WorkRepoPath     (default: ~/GoMikroBot-Workspace)
│   └── SystemRepoPath   (bot source repo)
├── Model
│   ├── Name             (default: anthropic/claude-sonnet-4-5)
│   ├── MaxTokens, Temperature
│   └── MaxToolIterations (default: 20)
├── Channels
│   ├── WhatsApp         (enabled, allowFrom, bridgeURL)
│   ├── Telegram, Discord, Feishu (planned)
├── Providers
│   ├── OpenAI           (apiKey, apiBase — also used for OpenRouter/DeepSeek)
│   ├── LocalWhisper     (enabled, model, binaryPath)
│   └── Anthropic, Groq, Gemini, VLLM (planned)
├── Gateway
│   ├── Host             (default: 127.0.0.1)
│   ├── Port             (default: 18790)
│   ├── DashboardPort    (default: 18791)
│   ├── AuthToken        (required for headless mode)
│   └── TLSCert, TLSKey (optional)
├── Tools
│   ├── Exec (timeout, denyPatterns, allowPatterns)
│   └── WebSearch (provider, apiKey)
├── Group
│   ├── Enabled, GroupName
│   ├── AgentID, ConsumerGroup
│   ├── KafkaBrokers, KafkaUsername/Password
│   └── LFSProxyURL
├── Orchestrator
│   ├── Enabled, Role (orchestrator/worker)
│   ├── ZoneID, ParentID, Endpoint
├── Scheduler
│   ├── Enabled, TickInterval
│   └── MaxConcLLM/Shell/Default
├── ER1
│   ├── URL, APIKey, UserID
│   └── SyncInterval (default: 5min)
└── Observer
    ├── Enabled, Model
    ├── MessageThreshold (default: 50)
    └── MaxObservations (default: 200)
```

Runtime settings stored in SQLite (`settings` table) override config for: `work_repo_path`, `bot_repo_path`, `group_name`, `group_active`, `kafscale_lfs_proxy_url`.

### 5.5 internal/provider — LLM Abstraction

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

- **OpenAI provider**: Chat completions + embeddings (text-embedding-ada-002) + TTS. Supports custom API base for OpenRouter, DeepSeek, etc.
- **LocalWhisper provider**: Wraps OpenAI, delegates chat to fallback, runs Whisper binary locally for transcription.
- Tool calls supported in `ChatResponse.ToolCalls` array (id, name, arguments map).

### 5.6 internal/tools — Tool Registry

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]any  // JSON Schema format
    Execute(ctx, params map[string]any) (string, error)
}

type TieredTool interface {
    Tool
    Tier() int  // 0=read-only, 1=write, 2=high-risk
}
```

**Registry** maps tool names to implementations. `Definitions()` exports as OpenAI tool format for LLM calls.

**Default tools registered in agent loop**:

| Tool | Tier | Description |
|------|------|-------------|
| `read_file` | 0 | Read file contents (~ expansion, any path) |
| `write_file` | 1 | Write to work repo (boundary-checked) |
| `edit_file` | 1 | Replace text in file (work repo only) |
| `list_dir` | 0 | List directory contents |
| `resolve_path` | 0 | Resolve workspace paths |
| `exec` | 2 | Shell execution (deny-pattern filtered, timeout 60s) |
| `remember` | 1 | Store to semantic memory |
| `recall` | 1 | Search semantic memory |

**Shell security** (`shell.go`):
- Deny patterns: `rm -rf`, `git rm`, `dd`, `mkfs`, `chmod 777`, fork bombs, `shutdown`, `reboot`
- Allow patterns (strict mode): `git`, `ls`, `cat`, `pwd`, `grep`, `sed`, `head`, `tail`, `wc`, `echo`
- Path traversal blocked: `../`, `..\`
- Workspace restriction: commands restricted to work repo root
- Timeout: configurable, default 60s

### 5.7 internal/policy — Authorization Engine

```go
type Engine interface {
    Evaluate(ctx Context) Decision
}

type Decision struct {
    Allow            bool
    RequiresApproval bool
    Reason           string
    Tier             int
}
```

Evaluation flow:
1. Tier 0 (read-only) → always allow
2. Check sender allowlist (if configured)
3. Determine effective max tier:
   - Internal messages: `MaxAutoTier` (default 2)
   - External messages: `ExternalMaxTier` (default 0)
4. If tool tier > effective max:
   - External: deny outright
   - Internal: require interactive approval
5. Log decision to `policy_decisions` table

### 5.8 internal/approval — Approval Workflow

Handles interactive approval gates for high-tier tool calls:

1. Policy returns `RequiresApproval=true`
2. `Manager.Create()` stores pending approval, broadcasts prompt to user
3. User responds with `approve:<id>` or `deny:<id>`
4. `Manager.Respond()` unblocks the waiting goroutine
5. Tool execution proceeds or aborts

Persisted to `approval_requests` table. Stale approvals cleaned on startup.

### 5.9 internal/session — Conversation State

JSONL-based session persistence at `~/.nanobot/sessions/{key}.jsonl`:
- Line 1: metadata JSON (created_at, updated_at, custom metadata)
- Lines 2+: message objects (role, content, timestamp)

`Manager` provides `GetOrCreate(key)`, `Save()`, `Delete()`, `List()` with in-memory caching and thread-safe access.

### 5.10 internal/timeline — SQLite Event Log

Central persistence layer. Schema includes:

**Core tables**: `timeline` (events), `settings` (key-value), `tasks` (agent processing), `web_users` / `web_links` (web UI mapping), `policy_decisions`, `approval_requests`, `scheduled_jobs`

**Memory tables**: `memory_chunks` (embeddings + metadata), `working_memory` (scoped scratchpads), `observations` / `observations_queue` (observer), `agent_expertise` / `skill_events`

**Group tables**: `group_members`, `group_membership_history`, `group_tasks`, `group_traces`, `group_memory_items`, `group_skill_channels`, `topic_message_log`, `delegation_events`

**Orchestrator tables**: `orchestrator_zones`, `orchestrator_zone_members`, `orchestrator_hierarchy`

### 5.11 internal/scheduler — Job Scheduling

Cron-based scheduler with distributed locking:
- Ticks every `TickInterval` (default 60s)
- Acquires file lock to prevent multi-process duplication
- Per-category semaphores: LLM (3), shell (1), default (5)
- Publishes jobs to message bus as `scheduler:` channel inbound messages
- Logs execution history to `scheduled_jobs` table

### 5.12 internal/group — Multi-Agent Collaboration

Kafka-based distributed agent coordination.

**Topic naming** (per group): `group.{name}.announce`, `.requests`, `.responses`, `.traces`

**Envelope types**: `announce`, `request`, `response`, `trace`, `heartbeat`, `onboard`, `memory`, `skill_request`, `skill_response`, `audit`, `task_status`, `roster`

**Components**:
- `Manager` — Join/leave, heartbeat, task handling, LFS client
- `KafkaConsumer` — Poll topics, deserialize, route
- `GroupRouter` — Central message router (bridges bus ↔ Kafka)
- `SkillChannelRegistry` — Publish/discover skills across agents
- `OnboardingProtocol` — New member integration

**LFS integration**: Large artifact storage via KafScale LFS proxy (HTTP API with SASL/PLAIN auth).

### 5.13 internal/orchestrator — Hierarchy & Zones

Coordinates multi-agent hierarchies:
- `AgentNode` — role (orchestrator/worker), parent_id, zone_id, endpoint, status
- `Zone` — visibility (private/shared/public), owner, parent zone, allowed IDs
- Discovery via Kafka topic `group.{name}.discovery`
- Hierarchy stored in `orchestrator_hierarchy`, zones in `orchestrator_zones`

---

## 6. Memory Architecture

GoMikroBot uses a 6-layer memory system stored in a single SQLite database. Each layer has a source prefix, a TTL policy, and a distinct role in context assembly.

### 6.1 Layer Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                     MEMORY ARCHITECTURE v2                        │
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│  SOURCES (content producers)                                     │
│  ├── Soul Files       (AGENTS.md, SOUL.md, etc.)   → soul:*     │
│  ├── Conversations    (auto-indexed Q&A pairs)      → conversation:* │
│  ├── Tool Results     (auto-indexed exec output)    → tool:*     │
│  ├── Group Sharing    (Kafka/LFS artifacts)         → group:*    │
│  ├── ER1 Sync         (personal memory transcripts) → er1:*      │
│  └── Observer         (LLM-compressed observations) → observation:* │
│                                                                  │
│  STORAGE                                                         │
│  ├── VectorStore      (SQLite-vec, 1536-dim)        embeddings   │
│  ├── Timeline DB      (SQLite)                      structured   │
│  └── WorkingMemory    (SQLite, per-user/thread)     scratchpad   │
│                                                                  │
│  RETRIEVAL (context assembly)                                    │
│  ├── RAG Injection    (vector search → system prompt)            │
│  ├── Working Memory   (scoped scratchpad → system prompt)        │
│  └── Observation Log  (compressed history → system prompt)       │
│                                                                  │
│  LIFECYCLE                                                       │
│  ├── TTL Pruning      (per-source retention policies)            │
│  ├── Observer Trigger  (message threshold → LLM compress)        │
│  ├── Reflector Trigger (observation overflow → consolidate)      │
│  └── Expertise Tracker (skill proficiency scoring)               │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### 6.2 Layers

| Layer | Source Prefix | TTL | Color | Description |
|-------|-------------|-----|-------|-------------|
| Soul | `soul:` | Permanent | `#a855f7` | Identity/personality files loaded at startup |
| Conversation | `conversation:` | 30 days | `#58a6ff` | Auto-indexed Q&A pairs |
| Tool | `tool:` | 14 days | `#fb923c` | Tool execution outputs |
| Group | `group:` | 60 days | `#22c55e` | Shared knowledge from group collaboration |
| ER1 | `er1:` | Permanent | `#fbbf24` | Personal memories synced from ER1 service |
| Observation | `observation:` | Permanent | `#67e8f9` | LLM-compressed conversation observations |

### 6.3 Memory Pipeline

```
[Capture] ──▶ [Embed] ──▶ [Store] ──▶ [Retrieve] ──▶ [Inject]
    ↑             ↑           ↑            ↑              ↑
 Channels     OpenAI     SQLite-vec    Cosine sim    System Prompt
 ER1 Sync     1536-dim   memory_chunks  top-k          RAG section
 Observer                               filtered
```

### 6.4 Components

**MemoryService** (`service.go`) — High-level API:
- `Store(content, source, tags)` — Embed via OpenAI, upsert to vector store
- `Search(query, limit)` — Embed query, cosine similarity search
- `SearchBySource(query, sourcePrefix, limit)` — Filtered search
- Gracefully degrades if embedder is nil (all ops become no-ops)

**SQLiteVecStore** (`sqlite_vec.go`) — Embedded vector database:
- Stores embeddings as little-endian float32 BLOBs in `memory_chunks` table
- Cosine similarity computed in Go (sub-millisecond at <10K chunks)
- Deterministic chunk IDs via SHA-256 hash of source + content

**AutoIndexer** (`auto_indexer.go`) — Background content indexer:
- Non-blocking `Enqueue()` with 100-item buffer
- Batches: flush every 5 items or 30 seconds
- Skips greetings, short content (<100 chars), raw JSON, error-only responses
- `FormatConversationPair()` — Formats user Q + agent A for indexing
- `FormatToolResult()` — Formats tool output with file path context

**SoulFileIndexer** (`indexer.go`) — Bootstrap file indexer:
- Reads AGENTS.md, SOUL.md, USER.md, TOOLS.md, IDENTITY.md from workspace
- Chunks each file by `##` headers
- Idempotent: deterministic IDs prevent duplication on re-index

**Observer** (`observer.go`) — Conversation compression:
- Enqueues messages to `observations_queue` table
- When unobserved count >= threshold (default 50): calls LLM to compress
- Produces prioritized observations (HIGH/MEDIUM/LOW) grouped by date
- Stores in `observations` table + indexes into vector store
- Reflector: when observations exceed max (default 200), consolidates via LLM

**WorkingMemoryStore** (`working.go`) — Scoped scratchpads:
- Keyed by (resourceID, threadID) — e.g., (WhatsApp phone, session key)
- Thread-specific lookup falls back to resource-level
- Injected into system prompt as `# Working Memory` section

**ER1Client** (`er1.go`) — Personal memory sync:
- Authenticates via POST `/user/access` to obtain ctx_id
- Fetches memories via GET `/memory/{ctx_id}?startDate=...`
- Indexes transcripts with tags and location as `er1:` source chunks
- SyncLoop: every 5 minutes (configurable)

**ExpertiseTracker** (`expertise.go`) — Skill proficiency:
- Records success/failure/quality per skill domain
- Skills: filesystem, shell, memory, research, communication, day2day, general
- Score formula: `0.6*successRate + 0.3*avgQuality + 0.1*experienceBonus`
- Trend: compares last-10 vs previous-10 quality averages

**LifecycleManager** (`lifecycle.go`) — Pruning:
- Runs daily: deletes chunks past their TTL
- If over `MaxChunks` (default 50000): deletes oldest non-permanent chunks
- `Stats()` returns total count, by-source breakdown, oldest/newest timestamps
- `DeleteBySource(prefix)` — Manual layer reset
- `Prune()` — Manual trigger

### 6.5 Context Assembly Order

```
System Prompt:
  1. Identity           (runtime info, version, date math)
  2. Bootstrap Files    (AGENTS.md, SOUL.md, USER.md, TOOLS.md, IDENTITY.md)
  3. Working Memory     (scoped per user/thread)
  4. Observations       (compressed session history, by date)
  5. Skills Summary     (tool descriptions + skill docs)
  6. RAG Context        (vector search across all 6 layers)
  7. Conversation       (recent message history)
```

Sections 1-4 form a **stable prefix** for prompt caching — they rarely change within a session.

---

## 7. API Endpoints

### 7.1 API Server (port 18790)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/chat?message=...&session=...` | Process message via agent loop |

### 7.2 Dashboard Server (port 18791)

**Agent**:
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/status` | Agent health, version, uptime, mode |
| POST | `/api/v1/auth/verify` | Bearer token validation |

**Timeline**:
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/timeline?limit=&offset=&sender=&trace_id=` | Paginated events |
| GET | `/api/v1/trace/{traceID}` | Detailed trace spans |
| GET | `/api/v1/trace-graph/{traceID}` | Trace execution graph |
| GET | `/api/v1/policy-decisions?trace_id=` | Policy audit log |

**Memory**:
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/memory/status` | Layer stats, observer, ER1, expertise, config |
| POST | `/api/v1/memory/reset` | Reset layer or all (`{layer: "conversation"\|"all"}`) |
| POST | `/api/v1/memory/config` | Update memory settings |
| POST | `/api/v1/memory/prune` | Trigger lifecycle pruning |

**Settings & Repo**:
| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/api/v1/settings` | Runtime settings (key-value) |
| GET/POST | `/api/v1/workrepo` | Work repo path |
| GET | `/api/v1/repo/tree` | File tree |
| GET | `/api/v1/repo/file?path=` | Read file |
| GET | `/api/v1/repo/status` | Git status |
| GET | `/api/v1/repo/search?q=` | Grep repo |
| GET | `/api/v1/repo/branches` | List branches |
| POST | `/api/v1/repo/checkout` | Switch branch |
| GET | `/api/v1/repo/log` | Commit history |
| GET | `/api/v1/repo/diff` | Full diff |
| POST | `/api/v1/repo/commit` | Create commit |
| POST | `/api/v1/repo/pull` | Pull changes |
| POST | `/api/v1/repo/push` | Push changes |

**Orchestrator**:
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/orchestrator/status` | Orchestrator state |
| GET | `/api/v1/orchestrator/hierarchy` | Agent tree |
| GET | `/api/v1/orchestrator/zones` | Zone list |
| GET | `/api/v1/orchestrator/agents` | Agent list |
| POST | `/api/v1/orchestrator/dispatch` | Task dispatch |

**Group** (20+ endpoints for collaboration, tasks, traces, skills, topics, audit):
| Prefix | Description |
|--------|-------------|
| `/api/v1/group/status` | Group state |
| `/api/v1/group/members` | Roster |
| `/api/v1/group/join` | Join group |
| `/api/v1/group/leave` | Leave group |
| `/api/v1/group/tasks/*` | Task delegation |
| `/api/v1/group/traces` | Shared traces |
| `/api/v1/group/memory` | Shared memory items |
| `/api/v1/group/skills/*` | Skill registry |
| `/api/v1/group/topics/*` | Kafka topic management |

**Web Chat**: `/api/v1/webchat/send`, `/api/v1/webusers`, `/api/v1/weblinks`

**Tasks & Approvals**: `/api/v1/tasks`, `/api/v1/approvals/pending`, `/api/v1/approvals/{id}`

All endpoints set `Access-Control-Allow-Origin: *` for CORS. Auth middleware applies when `AuthToken` is configured (skips `/api/v1/status` and OPTIONS preflight).

---

## 8. Electron Desktop Application

### 8.1 Architecture

```
┌─────────────────────────────────────────────────┐
│  Main Process (Electron)                        │
│  ├── Sidecar Manager (Go binary lifecycle)      │
│  ├── IPC Handlers (mode, config, sidecar, remote)│
│  ├── Menu (Timeline, Group, Change Mode)        │
│  └── Window (sandbox, context isolation)         │
├─────────────────────────────────────────────────┤
│  Preload (context bridge)                        │
│  └── Exposes electronAPI to renderer             │
├─────────────────────────────────────────────────┤
│  Renderer (Vue 3 SPA)                            │
│  ├── Stores (Pinia): agent, memory, orchestrator,│
│  │   mode, remote                                │
│  ├── Views: Dashboard, Memory, Orchestrator,     │
│  │   ModePicker, RemoteConnect, Settings         │
│  ├── Components: MemoryPipeline, MemoryLayerCard,│
│  │   MemoryStatusLed, WorkingMemoryPreview,      │
│  │   HierarchyGraph, ZoneTree, AgentCard,        │
│  │   ConnectionStatus, SidecarStatus             │
│  └── Composables: useApi, useSidecar             │
└─────────────────────────────────────────────────┘
```

### 8.2 Operation Modes

| Mode | Sidecar | Group | Orchestrator | Network |
|------|---------|-------|-------------|---------|
| Standalone | Local Go binary | No | No | localhost only |
| Full | Local Go binary | Kafka | Yes | localhost + Kafka |
| Remote | None | N/A | N/A | Remote API URL |

### 8.3 API Composable

`useApi()` automatically routes requests based on mode:
- Standalone/Full: `http://127.0.0.1:18791`
- Remote: configured URL with Bearer token auth

### 8.4 Header Status Indicators

The top panel displays three status indicators in the header-left area:
- **Mode badge** — Current operation mode (standalone/full/remote)
- **Memory LED** — Circle LED showing memory health (purple=healthy, amber=high, red=critical, gray=unavailable). Clicks through to `/memory` page. Polls `/api/v1/memory/status` every 30s.
- **Sidecar/Connection** — Right-aligned: sidecar process status (running/starting/error) or remote connection status

---

## 9. Security Model

### 9.1 Tool Authorization (3-Tier)

| Tier | Level | Examples | Internal | External |
|------|-------|---------|----------|----------|
| 0 | Read-only | read_file, list_dir, recall | Auto-allow | Auto-allow |
| 1 | Controlled writes | write_file, edit_file, remember | Auto-allow | Deny |
| 2 | High-risk | exec (shell) | Auto-allow (configurable) | Deny |

### 9.2 Shell Deny Patterns

```
rm -rf, rm -r /, rm -rf ~, rm *           Destructive deletion
git rm, find -delete, unlink, rmdir       Alternative deletion
dd if= of=/dev, mkfs, fdisk, format       Disk destruction
> /dev/, >> /dev/                          Device redirection
chmod 777 -R, chown -R                    Permission escalation
:(){ :|:& };:                             Fork bomb
shutdown, reboot, halt, init 0            System control
```

### 9.3 Network Security

- Default bind: `127.0.0.1` (localhost only)
- Headless mode: `0.0.0.0` with mandatory auth token
- TLS support: optional cert/key configuration
- Electron: sandbox mode, context isolation, no node integration in renderer

### 9.4 Filesystem Boundaries

- Write tools restricted to work repo via `isWithin()` check using `filepath.Rel()`
- Path traversal (`../`) blocked in both filesystem and shell tools
- Read tools can access any path (tier 0, read-only)

---

## 10. Deployment Modes

### 10.1 Standalone Desktop

```bash
make run                    # or: make electron-start-standalone
```
Local Go binary + Electron UI. No Kafka, no orchestrator. WhatsApp + local API only.

### 10.2 Full Desktop (Multi-Agent)

```bash
make run-full               # or: make electron-start-full
```
Adds Kafka consumer + group manager + orchestrator. Requires `MIKROBOT_GROUP_ENABLED=true` + Kafka config.

### 10.3 Headless Server

```bash
make run-headless
```
Binds `0.0.0.0`, requires `MIKROBOT_AUTH_TOKEN`. No GUI. Suitable for cloud deployment.

### 10.4 Remote Client

```bash
make electron-start-remote
```
Electron UI only (no local sidecar). Connects to a headless server via API.

### 10.5 Docker

```bash
make docker-build && make docker-up
```

---

## 11. Build & Release

### Key Makefile Targets

| Target | Action |
|--------|--------|
| `make build` | `go build ./cmd/gomikrobot` |
| `make test` | `go test ./...` |
| `make install` | Copy binary to `/usr/local/bin` |
| `make run` / `run-full` / `run-headless` | Build + run in specific mode |
| `make rerun` | Kill ports 18790/18791 + rebuild + run |
| `make electron-dev` | Vite dev server + Electron (hot reload) |
| `make electron-build` | `npm run build` (TypeScript + Vite) |
| `make electron-dist` | Package for distribution (.dmg, .AppImage) |
| `make release-patch` | Bump version, tag, push |

### Key Dependencies (go.mod)

| Dependency | Purpose |
|-----------|---------|
| `go.mau.fi/whatsmeow` | Native WhatsApp protocol |
| `github.com/segmentio/kafka-go` | Kafka consumer/producer |
| `modernc.org/sqlite` | Pure-Go SQLite driver |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/kelseyhightower/envconfig` | Env var parsing |
| `github.com/skip2/go-qrcode` | QR code for WhatsApp pairing |
| `github.com/fatih/color` | Terminal coloring |

---

## 12. Data Flow Diagrams

### 12.1 WhatsApp Message → Response

```
Phone sends WhatsApp message
    │
    ▼
whatsmeow handler (channels/whatsapp.go)
    ├── Normalize JID
    ├── Check allowlist/denylist
    ├── Transcribe audio (if needed)
    ├── Set metadata: message_type = internal|external
    │
    ▼
bus.PublishInbound(InboundMessage)
    │
    ▼
Agent Loop (bus.ConsumeInbound)
    ├── Dedup, create timeline task
    ├── Build context, inject memory
    ├── LLM call + tool iteration
    ├── Post-process: index, observe, track expertise
    │
    ▼
bus.PublishOutbound(OutboundMessage)
    │
    ▼
WhatsApp subscriber callback
    ├── Lookup web user mapping
    ├── Check silent mode
    └── wa.Send() → phone receives reply
```

### 12.2 Memory Indexing Pipeline

```
Conversation completes
    │
    ├── AutoIndexer.Enqueue(ConversationPair)
    │   └── Background: embed → upsert (source: conversation:whatsapp)
    │
    ├── AutoIndexer.Enqueue(ToolResult)  [for each tool call]
    │   └── Background: embed → upsert (source: tool:exec)
    │
    ├── Observer.EnqueueMessage(session, role, content)
    │   └── If threshold reached:
    │       ├── LLM compress → Observation[]
    │       ├── Store in observations table
    │       └── Embed + upsert (source: observation:session)
    │
    └── ExpertiseTracker.RecordToolUse(tool, task, duration, success)
        └── Upsert agent_expertise row
```

### 12.3 Context Assembly for LLM Call

```
ContextBuilder.BuildSystemPrompt()
    │
    ├── getIdentity()           → "You are GoMikroBot v2.5.3..."
    ├── loadBootstrapFiles()    → SOUL.md, AGENTS.md, USER.md content
    ├── WorkingMemory.Load()    → "## User Profile\n- Name: ..."
    ├── Observer.LoadObservations() → "[HIGH] User prefers Go..."
    ├── buildSkillsSummary()    → Tool list + skill docs
    └── MemoryService.Search()  → Top-k RAG chunks
         │
         ▼
    Assembled system prompt (stable prefix + variable suffix)
         │
         ▼
    provider.Chat(messages=[system, ...history, user])
```

---

## 13. Extension Points

### Adding a New Tool

1. Implement `Tool` interface (or `TieredTool`) in `internal/tools/`
2. Register in `loop.registerDefaultTools()` in `internal/agent/loop.go`
3. Tool automatically appears in LLM tool definitions

### Adding a New Channel

1. Implement `Channel` interface in `internal/channels/`
2. Subscribe to message bus for outbound delivery
3. Add config fields to `internal/config/config.go`
4. Wire in `gateway.go` startup sequence

### Adding a New CLI Command

1. Create file in `cmd/gomikrobot/cmd/`
2. Define cobra command
3. Register in `root.go` `init()`

### Adding a New Memory Source

1. Choose a source prefix (e.g., `newsource:`)
2. Add retention policy to `DefaultPolicies()` in `lifecycle.go`
3. Call `MemoryService.Store()` with the new prefix
4. Chunks automatically appear in RAG search results

### Adding a New API Endpoint

1. Add `mux.HandleFunc()` in `gateway.go` within the dashboard server goroutine
2. Set CORS headers (`Access-Control-Allow-Origin: *`)
3. Handle OPTIONS preflight
4. Use `timeSvc`, `memorySvc`, etc. from closure scope

### Adding a New Dashboard View

1. Create Vue component in `electron/src/renderer/views/`
2. Add route in `router/index.ts`
3. Add nav link in `App.vue`
4. Create Pinia store if needed in `stores/`
