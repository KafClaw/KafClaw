---
parent: Memory Management
title: CoT Cascading Protocol
nav_order: 2
---

# CoT Cascading Protocol

The Cascading Protocol applies Chain-of-Thought style task decomposition with strict stage gates between subtasks.

- Every subtask declares `required_input`, `produced_output`, and `validation_rules`.
- `Task N+1` is blocked until `Task N` is committed with validated outputs.
- Failures route back with deterministic reasons (`missing_input`, `missing_output`, `invalid_rules`) and retry controls.
- State transitions are durable and idempotent.

## State Machine Infographic

<style>
  .cot-wrap {
    margin: 14px 0 6px;
    color: #18263d;
    font-family: "Avenir Next", "Segoe UI", "Helvetica Neue", sans-serif;
  }
  .cot-grid {
    display: grid;
    grid-template-columns: repeat(12, minmax(0, 1fr));
    gap: 12px;
    margin-top: 14px;
  }
  .cot-card {
    grid-column: span 12;
    border: 1px solid #d4deef;
    border-radius: 12px;
    background: #ffffff;
    padding: 12px;
    box-shadow: 0 6px 18px rgba(24, 45, 84, 0.07);
  }
  .cot-card svg {
    display: block;
    width: 100%;
    height: auto;
  }
</style>

<div class="cot-wrap">
  <section class="cot-grid">
    <article class="cot-card">
      <svg viewBox="0 0 1120 320" role="img" aria-label="Cascading protocol state machine">
        <defs>
          <marker id="cot-arrow" markerWidth="10" markerHeight="8" refX="9" refY="4" orient="auto">
            <polygon points="0,0 10,4 0,8" fill="#24509c"></polygon>
          </marker>
          <marker id="cot-arrow-fail" markerWidth="10" markerHeight="8" refX="9" refY="4" orient="auto">
            <polygon points="0,0 10,4 0,8" fill="#b11f2f"></polygon>
          </marker>
        </defs>

        <rect x="30" y="44" width="140" height="58" rx="10" fill="#eef4ff" stroke="#9bb5e6"></rect>
        <text x="100" y="78" text-anchor="middle" font-size="14" font-weight="700" fill="#1a4288">pending</text>

        <rect x="200" y="44" width="140" height="58" rx="10" fill="#eef4ff" stroke="#9bb5e6"></rect>
        <text x="270" y="78" text-anchor="middle" font-size="14" font-weight="700" fill="#1a4288">running</text>

        <rect x="370" y="44" width="140" height="58" rx="10" fill="#eef4ff" stroke="#9bb5e6"></rect>
        <text x="440" y="78" text-anchor="middle" font-size="14" font-weight="700" fill="#1a4288">self_test</text>

        <rect x="540" y="44" width="140" height="58" rx="10" fill="#eef4ff" stroke="#9bb5e6"></rect>
        <text x="610" y="78" text-anchor="middle" font-size="14" font-weight="700" fill="#1a4288">validated</text>

        <rect x="710" y="44" width="140" height="58" rx="10" fill="#eaf8f3" stroke="#9ad8c4"></rect>
        <text x="780" y="78" text-anchor="middle" font-size="14" font-weight="700" fill="#17634d">committed</text>

        <rect x="880" y="44" width="170" height="58" rx="10" fill="#eaf8f3" stroke="#9ad8c4"></rect>
        <text x="965" y="78" text-anchor="middle" font-size="14" font-weight="700" fill="#17634d">released_next</text>

        <rect x="760" y="212" width="150" height="58" rx="10" fill="#ffeef0" stroke="#de9aa1"></rect>
        <text x="835" y="246" text-anchor="middle" font-size="14" font-weight="700" fill="#8e1f2a">failed</text>

        <rect x="930" y="212" width="170" height="58" rx="10" fill="#fff8ea" stroke="#e3c995"></rect>
        <text x="1015" y="238" text-anchor="middle" font-size="14" font-weight="700" fill="#84511b">Task N+1</text>
        <text x="1015" y="256" text-anchor="middle" font-size="11" fill="#8f6933">can start</text>

        <line x1="170" y1="73" x2="198" y2="73" stroke="#24509c" stroke-width="2.4" marker-end="url(#cot-arrow)"></line>
        <line x1="340" y1="73" x2="368" y2="73" stroke="#24509c" stroke-width="2.4" marker-end="url(#cot-arrow)"></line>
        <line x1="510" y1="73" x2="538" y2="73" stroke="#24509c" stroke-width="2.4" marker-end="url(#cot-arrow)"></line>
        <line x1="680" y1="73" x2="708" y2="73" stroke="#24509c" stroke-width="2.4" marker-end="url(#cot-arrow)"></line>
        <line x1="850" y1="73" x2="878" y2="73" stroke="#24509c" stroke-width="2.4" marker-end="url(#cot-arrow)"></line>

        <path d="M 440 104 C 440 150, 100 150, 100 104" fill="none" stroke="#996221" stroke-width="2.2" marker-end="url(#cot-arrow)"></path>
        <text x="230" y="165" text-anchor="middle" font-size="11" fill="#8f6933">validation failed + retries left</text>

        <line x1="100" y1="104" x2="760" y2="212" stroke="#b11f2f" stroke-width="2" marker-end="url(#cot-arrow-fail)"></line>
        <text x="318" y="186" text-anchor="middle" font-size="11" fill="#8e1f2a">retry budget exhausted</text>

        <line x1="270" y1="104" x2="790" y2="212" stroke="#b11f2f" stroke-width="2" marker-end="url(#cot-arrow-fail)"></line>
        <text x="468" y="205" text-anchor="middle" font-size="11" fill="#8e1f2a">runtime error</text>

        <line x1="610" y1="104" x2="820" y2="212" stroke="#b11f2f" stroke-width="2" marker-end="url(#cot-arrow-fail)"></line>
        <text x="672" y="186" text-anchor="middle" font-size="11" fill="#8e1f2a">commit error</text>

        <line x1="800" y1="104" x2="985" y2="212" stroke="#996221" stroke-width="2" stroke-dasharray="6,5" marker-end="url(#cot-arrow)"></line>
        <text x="905" y="164" text-anchor="middle" font-size="11" fill="#8f6933">unlock Task N+1</text>
      </svg>
    </article>
  </section>
</div>

## When To Use

Use this mode for deterministic, verifiable work where output shape and validation are clear.

- Runbooks and operations workflows
- Config generation and structured updates
- Code mod pipelines and migration steps
- Multi-stage tasks where each step has hard contracts

## When Not To Use

Avoid this mode for open-ended and high-ambiguity work.

- Creative generation and brainstorming
- Exploratory research without stable validation criteria
- Low-latency conversational tasks
- Tasks where strict sequencing blocks useful parallelism

## Implications and Tradeoffs

- Improves reliability by preventing context drift between subtasks.
- Reduces copy-of-copy distortion via explicit output validation.
- Adds latency and compute cost due to retries and gate checks.
- Can stall throughput if contracts are underspecified.
- Requires better task design (`required_input`, `produced_output`, and validation rules must be explicit).
- On retry-threshold exceed, failed transitions now trigger deterministic escalation hints for fallback model and tooling.

## Configuration Guidance

This should be enabled per agent only when the agent's workload is deterministic and auditable.

- Recommended: ops/runbook/config/code-mod agents
- Not recommended: general-purpose chat or exploratory analysis agents

Use `kafclaw configure` to enable or disable cascade behavior at the agent level, and keep default behavior unchanged for agents that do not need gated execution.

## Migration Guide for Existing Agent Configs

Use this flow when upgrading an existing installation that already has `agents.list` entries.

1. Inspect your current agents:

```bash
kafclaw config get agents.list
```

2. Enable cascade only for deterministic agents:

```bash
kafclaw configure --agent-id ops --agent-cascade-enabled-set --agent-cascade-enabled=true
kafclaw configure --agent-id runbook --agent-cascade-enabled-set --agent-cascade-enabled=true
```

3. Keep exploratory or chat agents disabled:

```bash
kafclaw configure --agent-id main --agent-cascade-enabled-set --agent-cascade-enabled=false
```

4. Verify persisted config:

```bash
kafclaw config get agents.list
```

5. Validate runtime behavior on a trace:

```bash
kafclaw task status --trace <trace-id> --json
```

For failure triage, see the runbook snippet in [Memory Governance Operations](/memory-management/memory-governance-operations/#cascade-failure-triage-snippet).
