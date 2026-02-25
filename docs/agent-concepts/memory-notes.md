---
parent: Memory Management
title: Memory Architecture and Notes
nav_order: 1
---

# Memory Architecture and Notes

Private memory lanes, restart durability, model-switch behavior, and distributed knowledge sharing in one place.

<style>
  .km-wrap {
    margin: 14px 0 6px;
    color: #18263d;
    font-family: "Avenir Next", "Segoe UI", "Helvetica Neue", sans-serif;
  }
  .km-hero {
    border-radius: 14px;
    padding: 20px;
    background: linear-gradient(132deg, #0f5fff 0%, #1a49a8 58%, #14306c 100%);
    color: #fff;
    box-shadow: 0 12px 24px rgba(16, 45, 108, 0.22);
  }
  .km-hero h2 {
    margin: 0 0 8px;
    font-size: 1.5rem;
    line-height: 1.2;
  }
  .km-hero p {
    margin: 0;
    opacity: 0.95;
  }
  .km-grid {
    display: grid;
    grid-template-columns: repeat(12, minmax(0, 1fr));
    gap: 12px;
    margin-top: 14px;
  }
  .km-card {
    border: 1px solid #d4deef;
    border-radius: 12px;
    background: #ffffff;
    padding: 12px;
    box-shadow: 0 6px 18px rgba(24, 45, 84, 0.07);
  }
  .km-card h3 {
    margin: 0 0 8px;
    font-size: 1rem;
    color: #1e3454;
  }
  .km-card p,
  .km-card li {
    margin: 0;
    color: #425674;
    line-height: 1.45;
    font-size: 0.92rem;
  }
  .km-card ul {
    margin: 8px 0 0;
    padding-left: 18px;
  }
  .km-private { grid-column: span 4; }
  .km-durable { grid-column: span 4; }
  .km-shared { grid-column: span 4; }
  .km-flow { grid-column: span 12; }
  .km-flow svg {
    display: block;
    width: 100%;
    height: auto;
  }
  .km-chip {
    display: inline-block;
    margin: 4px 6px 0 0;
    padding: 3px 9px;
    border-radius: 999px;
    border: 1px solid #b9ccf6;
    background: #ecf2ff;
    color: #17458b;
    font-size: 0.78rem;
    font-weight: 600;
  }
  .km-callout {
    border-left: 4px solid #10986b;
    background: #eaf8f3;
    color: #175a48;
    border-radius: 10px;
    padding: 10px 12px;
    font-size: 0.92rem;
  }
  @media (max-width: 960px) {
    .km-private, .km-durable, .km-shared { grid-column: span 12; }
  }
</style>

<div class="km-wrap">
  <section class="km-hero">
    <h2>Memory Architecture</h2>
    <p>Built for precise task memory, durability across restarts, and clean shared knowledge exchange over Kafka.</p>
  </section>

  <section class="km-grid">
    <article class="km-card km-private">
      <h3>Private Agent Memory</h3>
      <p>Per agent and per thread memory stores task facts, decisions, and open work context.</p>
      <ul>
        <li>Scoped to session and resource lanes</li>
        <li>Semantic retrieval from embeddings</li>
        <li>Low-noise indexing policy</li>
      </ul>
    </article>

    <article class="km-card km-durable">
      <h3>Durability and Recovery</h3>
      <p>State persists in local timeline and memory stores so restarts do not drop tasks or heartbeats.</p>
      <ul>
        <li>Restart-safe session continuity</li>
        <li>Embedding runtime health endpoints</li>
        <li>First embedding enable does not wipe memory</li>
      </ul>
    </article>

    <article class="km-card km-shared">
      <h3>Shared Knowledge Pool</h3>
      <p>Agents publish structured facts over Kafka with source, confidence, and version metadata.</p>
      <ul>
        <li>Claw identity for machine-to-machine trust</li>
        <li>Proposal, vote, and decision events</li>
        <li>Quorum voting when 3 or more claws participate</li>
      </ul>
    </article>

    <article class="km-card km-flow">
      <h3>End-to-End Memory Flow</h3>
      <span class="km-chip">ingest</span>
      <span class="km-chip">embed</span>
      <span class="km-chip">store</span>
      <span class="km-chip">retrieve</span>
      <span class="km-chip">share</span>
      <span class="km-chip">vote</span>
      <span class="km-chip">finalize</span>
      <svg viewBox="0 0 1120 360" role="img" aria-label="Memory lifecycle and shared knowledge flow">
        <defs>
          <marker id="km-arrow" markerWidth="10" markerHeight="8" refX="9" refY="4" orient="auto">
            <polygon points="0,0 10,4 0,8" fill="#24509c"></polygon>
          </marker>
        </defs>
        <rect x="40" y="32" width="220" height="72" rx="10" fill="#eef4ff" stroke="#9bb5e6"></rect>
        <text x="150" y="60" text-anchor="middle" font-size="16" font-weight="700" fill="#1a4288">Task Input</text>
        <text x="150" y="82" text-anchor="middle" font-size="12" fill="#33598f">message and context id</text>
        <rect x="316" y="32" width="220" height="72" rx="10" fill="#ffffff" stroke="#c8d6ec"></rect>
        <text x="426" y="60" text-anchor="middle" font-size="16" font-weight="700" fill="#1d2e46">Embedding Runtime</text>
        <text x="426" y="82" text-anchor="middle" font-size="12" fill="#516786">local model health checked</text>
        <rect x="592" y="32" width="220" height="72" rx="10" fill="#ffffff" stroke="#c8d6ec"></rect>
        <text x="702" y="60" text-anchor="middle" font-size="16" font-weight="700" fill="#1d2e46">Private Memory</text>
        <text x="702" y="82" text-anchor="middle" font-size="12" fill="#516786">precision chunks per lane</text>
        <rect x="868" y="32" width="220" height="72" rx="10" fill="#eaf8f3" stroke="#9ad8c4"></rect>
        <text x="978" y="60" text-anchor="middle" font-size="16" font-weight="700" fill="#17634d">Response Context</text>
        <text x="978" y="82" text-anchor="middle" font-size="12" fill="#2d7a64">top facts added to prompt</text>
        <rect x="316" y="180" width="220" height="72" rx="10" fill="#fff8ea" stroke="#e3c995"></rect>
        <text x="426" y="208" text-anchor="middle" font-size="16" font-weight="700" fill="#84511b">Kafka Knowledge</text>
        <text x="426" y="230" text-anchor="middle" font-size="12" fill="#8f6933">proposals and updates</text>
        <rect x="592" y="180" width="220" height="72" rx="10" fill="#fff8ea" stroke="#e3c995"></rect>
        <text x="702" y="208" text-anchor="middle" font-size="16" font-weight="700" fill="#84511b">Voting and Quorum</text>
        <text x="702" y="230" text-anchor="middle" font-size="12" fill="#8f6933">3+ claws validate facts</text>
        <rect x="868" y="180" width="220" height="72" rx="10" fill="#fff8ea" stroke="#e3c995"></rect>
        <text x="978" y="208" text-anchor="middle" font-size="16" font-weight="700" fill="#84511b">Shared Fact Set</text>
        <text x="978" y="230" text-anchor="middle" font-size="12" fill="#8f6933">versioned distributed truth</text>
        <line x1="260" y1="68" x2="314" y2="68" stroke="#24509c" stroke-width="2.6" marker-end="url(#km-arrow)"></line>
        <line x1="536" y1="68" x2="590" y2="68" stroke="#24509c" stroke-width="2.6" marker-end="url(#km-arrow)"></line>
        <line x1="812" y1="68" x2="866" y2="68" stroke="#24509c" stroke-width="2.6" marker-end="url(#km-arrow)"></line>
        <line x1="702" y1="104" x2="702" y2="178" stroke="#24509c" stroke-width="2.2" marker-end="url(#km-arrow)"></line>
        <line x1="536" y1="216" x2="590" y2="216" stroke="#996221" stroke-width="2.2" marker-end="url(#km-arrow)"></line>
        <line x1="812" y1="216" x2="866" y2="216" stroke="#996221" stroke-width="2.2" marker-end="url(#km-arrow)"></line>
      </svg>
    </article>
  </section>
</div>

<div class="km-callout">
  Model switch rule: if a previously used embedding model changes, memory vectors are wiped before reindex. Adding the first embedding model later does not wipe existing text records.
</div>

## Cascading Protocol (Gated Subtasks)

Full protocol details, state machine infographic, and operational guidance are documented in [CoT Cascading Protocol](/agent-concepts/CoT/).

## Kafka Knowledge Pool: How It Works

The shared knowledge pool is the distributed memory layer used when multiple claws collaborate.
It is event-based and runs on Kafka with typed envelopes (`proposal`, `vote`, `decision`, `fact`, `presence`, `capabilities`).
Each message includes identity and trace metadata (`clawId`, `instanceId`, `traceId`, `idempotencyKey`) so receivers can deduplicate and audit every state change.

### ID Model and Anti-Tampering Controls

ID fields are used as a trust and replay-control layer:

- `clawId`: stable node identity for a claw in the group.
- `instanceId`: runtime instance identity for that claw process.
- `traceId`: end-to-end correlation id across proposal, votes, decision, and fact events.
- `idempotencyKey`: unique apply key for a logical knowledge event.

Controls implemented now:

1. Strict envelope validation  
   Required fields are enforced before apply (`schemaVersion`, `type`, `traceId`, `timestamp`, `idempotencyKey`, `clawId`, `instanceId`).
2. Replay and duplicate blocking  
   `idempotencyKey` is recorded in `knowledge_idempotency` with a unique constraint, so the same event is applied once.
3. One-vote-per-claw rule  
   Votes are keyed by `(proposal_id, claw_id)` in storage, so one claw cannot stack multiple votes on one proposal.
4. Fact version integrity  
   Facts must start at `v1` and then advance sequentially (`+1` only). Regressions, gaps, and mismatched stale updates are rejected as conflict or stale.
5. Governance gate  
   Governed message types (`proposal`, `vote`, `decision`, `fact`) are blocked when governance is disabled.
6. Self-loop suppression  
   A claw ignores envelopes that claim its own local `clawId`, preventing local re-apply loops.
7. Audit trail  
   Accepted and rejected apply outcomes are written with trace metadata to timeline events for forensic review.

Important scope note:

- These controls prevent many tampering effects inside the app flow (duplicate replay, invalid structure, vote inflation, version rewrites).
- Transport/auth hardening still depends on Kafka security posture (TLS, SASL, topic ACLs, producer identity controls).
- Knowledge envelopes are not currently signed end-to-end at message level, so broker and network trust boundaries still matter.

### Why 3+ Claws for Voting

Voting only becomes meaningful at 3 or more participating claws.

- With 1 claw, there is no consensus. It is only a local assertion.
- With 2 claws, ties and bilateral bias are common.
- With 3 or more claws, majority outcome is possible and resistant to one wrong or stale node.

Practical effect:
- 3 claws: majority threshold is 2.
- 5 claws: majority threshold is 3.
- Quorum should be based on currently active and healthy participants, not total registered claws.

This is why governance mode requires quorum logic before a `fact` is promoted as shared truth.

### End-to-End Governance Flow

1. A claw publishes a `proposal` with a clear statement and tags.
2. Other claws publish `vote` events (`yes` or `no`) with optional reason.
3. A coordinator path emits `decision` after quorum and majority are reached (or expired/rejected).
4. If approved, a versioned `fact` is published and applied to shared memory.
5. Claws consume the new fact and can use it for routing, delegation, and prompt context.

### Capability Discovery and Skill Sharing Use Case

Example with 4 claws:

- `claw-1` has filesystem access to production log paths.
- `claw-2` has read-only database access.
- `claw-3` has incident triage and runbook skills.
- `claw-4` is the coordinator handling incoming user requests.

Flow:

1. Each claw publishes `capabilities` to Kafka, for example:
   - `claw-1`: `logs.read:/var/log/app/*`
   - `claw-2`: `db.read:orders,customers`
   - `claw-3`: `incident.runbook:v2`
2. `claw-4` receives a task: "Why did checkout fail in the last hour?"
3. `claw-4` routes sub-tasks based on announced capabilities:
   - ask `claw-1` for log error windows
   - ask `claw-2` for failed transaction rows
   - ask `claw-3` to map findings to runbook actions
4. A new reusable conclusion is proposed:
   - "Error E142 with DB timeout pattern maps to mitigation M7"
5. 3+ active claws vote.
6. Majority decision publishes a versioned `fact`.
7. Next incident can be solved faster by any claw because the fact is now in the shared pool.

This keeps agent memory precise:
- private memory stays task-local and focused
- shared pool only accepts promoted, voted knowledge
- noisy one-off chat does not become distributed truth

### Topic and Contract References

For envelope schema and payload contracts, see:
- [Knowledge Contracts](/reference/knowledge-contracts/)
- [Group and Kafka Communication Operations](/collaboration/group-kafka-operations/)
