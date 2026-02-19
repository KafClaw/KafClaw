---
title: Summarize
parent: Skills
nav_order: 3
---

# Summarize

Produce concise summaries of long technical artifacts.

## Default State

- Bundled with KafClaw
- Disabled by default

## What It Does

- Compresses large logs, docs, and threads into actionable summaries.
- Preserves technical decisions, risks, and next actions.
- Avoids speculation by calling out unknowns explicitly.

## Install / Enable

No external install needed (bundled skill). Enable it:

```bash
kafclaw skills enable-skill summarize
```

## Usage

- Use for long incident timelines, pull request discussions, and diagnostics output.
- Request output in structured form (findings, risks, actions).

## Troubleshooting

- If summaries miss context, narrow the scope to a specific file set.
- If summaries are too broad, provide the audience and objective first.
- If skill appears unavailable, verify global skills are enabled.
