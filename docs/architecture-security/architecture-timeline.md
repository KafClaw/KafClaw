---
parent: Architecture and Security
title: "Architecture: Timeline and Memory"
nav_order: 4
---

# Architecture: Timeline and Memory

## Objective

A unified, auditable timeline of all agent interactions (text, audio, media) stored in SQLite, backed by a 6-layer semantic memory system with vector embeddings, and visualized via a web dashboard and Electron app.

---

## 1. Timeline Database

### Schema (`timeline` table)

```sql
CREATE TABLE timeline (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT UNIQUE,
    trace_id TEXT,
    span_id TEXT,
    parent_span_id TEXT,
    timestamp DATETIME,
    sender_id TEXT,
    sender_name TEXT,
    event_type TEXT,       -- TEXT, AUDIO, IMAGE, SYSTEM
    content_text TEXT,
    media_path TEXT,
    vector_id TEXT,
    classification TEXT,
    authorized BOOLEAN DEFAULT 1
);
```

Event types: `TEXT`, `AUDIO`, `IMAGE`, `SYSTEM`

Tracing: Every event carries `trace_id`, `span_id`, and `parent_span_id` for end-to-end request correlation.

### Location

```
~/.kafclaw/timeline.db
```

Uses WAL journal mode, foreign keys, and a 5-second busy timeout for concurrent access.

---

## 2. Memory Architecture (v2)

KafClaw uses a 6-layer memory system stored in a single SQLite database. Each layer has a source prefix, a TTL policy, and a distinct role in context assembly.

### Layer Overview

| Layer | Source Prefix | TTL | Description |
|-------|--------------|-----|-------------|
| Soul | `soul:` | Permanent | Identity/personality files loaded at startup |
| Conversation | `conversation:` | 30 days | Auto-indexed Q&A pairs |
| Tool | `tool:` | 14 days | Tool execution outputs |
| Group | `group:` | 60 days | Shared knowledge from group collaboration |
| ER1 | `er1:` | Permanent | Personal memories synced from ER1 service |
| Observation | `observation:` | Permanent | LLM-compressed conversation observations |

### Storage Backends

| Backend | Technology | Purpose |
|---------|-----------|---------|
| VectorStore | SQLite-vec (1536-dim) | Embeddings for semantic search |
| Timeline DB | SQLite | Structured event/settings storage |
| WorkingMemory | SQLite (per-user/thread) | Scoped scratchpads |

### Memory Pipeline

```
[Capture] --> [Embed] --> [Store] --> [Retrieve] --> [Inject]
    |            |           |            |              |
 Channels    OpenAI     SQLite-vec   Cosine sim    System Prompt
 ER1 Sync    1536-dim   memory_chunks  top-k         RAG section
 Observer                              filtered
```

### Key Components

- **MemoryService** - Store/search with automatic embedding. Graceful degradation if no embedder available.
- **AutoIndexer** - Background batch indexer (5-item flush / 30s interval). Skips greetings and short content.
- **SoulFileIndexer** - Indexes AGENTS.md, SOUL.md, USER.md, TOOLS.md, IDENTITY.md by `##` headers.
- **Observer** - Enqueues messages, triggers LLM compression at threshold (default 50), produces prioritized observations.
- **WorkingMemoryStore** - Per-user/thread scratchpads, falls back from thread-specific to resource-level.
- **ER1Client** - Personal memory sync every 5 minutes (configurable).
- **LifecycleManager** - Daily TTL pruning, max chunks enforcement (default 50,000).
- **ExpertiseTracker** - Skill proficiency scoring per domain.

---

## 3. Context Assembly Order

When processing a user message, the context builder assembles the system prompt in this order:

1. **Identity** - Runtime info, version, date math
2. **Bootstrap Files** - AGENTS.md, SOUL.md, USER.md, TOOLS.md, IDENTITY.md
3. **Working Memory** - Scoped per user/thread
4. **Observations** - Compressed session history, by date
5. **Skills Summary** - Tool descriptions + skill docs
6. **RAG Context** - Vector search across all 6 layers
7. **Conversation** - Recent message history

Sections 1-4 form a stable prefix for prompt caching.

---

## 4. Dashboard Visualization

### Timeline View

Vertical stream of cards:
- Left: Agent responses
- Right: User messages
- Auto-refreshes every 5 seconds

### Features

- Media support: HTML5 audio player (OGG voice notes), click-to-expand images
- Focus mode: Select a user, others become semi-transparent
- Trace viewer: Drill into individual request flows
- Memory dashboard: Layer stats, observer status, expertise, ER1 sync status

### Electron App

Memory-specific views:
- MemoryPipeline - Visual pipeline diagram
- MemoryLayerCard - Per-layer stats with color coding
- WorkingMemoryPreview - Current scratchpad contents
- MemoryStatusLed - Circle LED (purple=healthy, amber=high, red=critical)

---

## 5. Evolution from v1

The original design referenced Qdrant (QMD) as an external vector database. KafClaw v2 replaced this with SQLite-vec - an embedded vector store requiring zero external dependencies. Cosine similarity computed in Go is sub-millisecond at under 10,000 chunks.

The ER1 integration and observer/reflector pattern were added in v2 to support long-term personal memory and conversation compression.
