---
parent: Agent Concepts
title: How Agents Work
---

# How Agents Work

This page describes the live runtime path from message input to response output.

## Runtime Flow

1. Inbound message enters via a channel (CLI, Web UI, WhatsApp, Slack, Teams, etc.)
2. Message is published to the internal bus
3. Agent loop consumes inbound event
4. Context builder assembles system prompt:
   - runtime identity block
   - workspace identity files (`AGENTS.md`, `SOUL.md`, `USER.md`, `TOOLS.md`, `IDENTITY.md`)
   - `memory/MEMORY.md` (if present)
   - tool summary and system-repo skills (if available)
5. Model responds, optionally issuing tool calls
6. Tool policy evaluates requested calls
7. Tool results are fed back into loop (up to configured iteration limit)
8. Final response is stored, indexed, and published as outbound message

## Default Tool Registration

The loop registers these tools by default:

- `read_file`
- `write_file`
- `edit_file`
- `list_dir`
- `resolve_path`
- `exec`
- `sessions_spawn`
- `subagents`
- `agents_list`

When memory service is enabled, it also registers:

- `remember`
- `recall`

## Subagent Memory Security Model

Subagents are isolated workers by default and do not write directly into parent private working-memory scope.
Memory behavior is controlled by `tools.subagents.memoryShareMode`:

- `isolated`: child session is isolated and no automatic parent handoff is written
- `handoff` (default): child remains isolated, then a structured completion handoff is appended to parent session
- `inherit-readonly`: child receives a read-only snapshot of parent context and still returns handoff to parent

Security intent:

- child sessions are separate keys (`subagent:<id>`) and do not share thread scratchpad storage with parent
- parent state updates happen via explicit handoff message, not direct child writes
- optional inherited context is read-only to reduce state pollution risk
- child tool policy remains depth-aware and respects subagent allow/deny constraints

<style>
  .sa-wrap {
    margin: 14px 0 8px;
    border: 1px solid #d6e0ef;
    border-radius: 14px;
    background: linear-gradient(165deg, #f8fbff 0%, #edf4ff 100%);
    padding: 14px;
  }
  .sa-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 10px;
  }
  .sa-card {
    border: 1px solid #c9d7ed;
    border-radius: 12px;
    background: #fff;
    padding: 10px;
    box-shadow: 0 4px 12px rgba(28, 58, 106, 0.08);
  }
  .sa-card h3 {
    margin: 0 0 6px;
    font-size: 0.96rem;
    color: #183f7a;
  }
  .sa-card p {
    margin: 0;
    color: #37506f;
    font-size: 0.86rem;
    line-height: 1.35;
  }
  .sa-flow {
    margin-top: 10px;
    border: 1px solid #c9d7ed;
    border-radius: 12px;
    background: #fff;
    padding: 10px;
  }
  .sa-flow svg {
    width: 100%;
    height: auto;
    display: block;
  }
  @media (max-width: 940px) {
    .sa-grid { grid-template-columns: 1fr; }
  }
</style>

<div class="sa-wrap">
  <div class="sa-grid">
    <article class="sa-card">
      <h3>Parent Session</h3>
      <p>Owns durable private thread memory and decides what to persist after child completion.</p>
    </article>
    <article class="sa-card">
      <h3>Child Session</h3>
      <p>Runs in isolated session key. No direct writes into parent working memory scope.</p>
    </article>
    <article class="sa-card">
      <h3>Controlled Handoff</h3>
      <p>Child output is normalized and handed back to parent as explicit ingest path.</p>
    </article>
  </div>
  <div class="sa-flow">
    <svg viewBox="0 0 980 210" role="img" aria-label="Subagent isolation and handoff flow">
      <defs>
        <marker id="sa-arrow" markerWidth="10" markerHeight="8" refX="9" refY="4" orient="auto">
          <polygon points="0,0 10,4 0,8" fill="#2f5aa3"></polygon>
        </marker>
      </defs>
      <rect x="20" y="36" width="270" height="72" rx="10" fill="#eef4ff" stroke="#9eb8e8"></rect>
      <text x="155" y="64" text-anchor="middle" font-size="16" font-weight="700" fill="#194585">Parent Agent</text>
      <text x="155" y="86" text-anchor="middle" font-size="12" fill="#3a5f98">private memory scope</text>
      <rect x="355" y="36" width="270" height="72" rx="10" fill="#ffffff" stroke="#cad8ee"></rect>
      <text x="490" y="64" text-anchor="middle" font-size="16" font-weight="700" fill="#22334c">Subagent Session</text>
      <text x="490" y="86" text-anchor="middle" font-size="12" fill="#536a89">isolated runtime lane</text>
      <rect x="690" y="36" width="270" height="72" rx="10" fill="#e9f8f2" stroke="#9fd8c3"></rect>
      <text x="825" y="64" text-anchor="middle" font-size="16" font-weight="700" fill="#1b654e">Parent Handoff</text>
      <text x="825" y="86" text-anchor="middle" font-size="12" fill="#317462">explicit ingest and persist</text>
      <line x1="290" y1="72" x2="353" y2="72" stroke="#2f5aa3" stroke-width="2.5" marker-end="url(#sa-arrow)"></line>
      <line x1="625" y1="72" x2="688" y2="72" stroke="#2f5aa3" stroke-width="2.5" marker-end="url(#sa-arrow)"></line>
      <text x="490" y="145" text-anchor="middle" font-size="12" fill="#6d7f97">inherit-readonly mode can pass parent snapshot to child; child still cannot directly mutate parent memory scope</text>
    </svg>
  </div>
</div>

## Group and Orchestrator Identity

When group mode is enabled:

- agent identity is built from runtime + workspace files
- capabilities are exported from active tool registry
- onboarding/announce messages publish identity to group control topics
- roster and timeline persist identity snapshots

## Auto-Scaffold and Startup Behavior

At gateway startup:

- if workspace identity files are missing, scaffold runs automatically (non-destructive)
- if memory service is enabled, soul files are indexed in background

This makes headless and container setups self-healing for missing baseline files.
