---
title: session-logs
parent: Skills
nav_order: 2
---

# session-logs

Analyze session logs to explain failures, regressions, and behavior drift.

## Default State

- Bundled with KafClaw
- Disabled by default

## What It Does

- Reads local session/task traces and extracts key failure points.
- Summarizes timeline, repeated errors, and likely root causes.
- Produces remediation-focused summaries for operators.

## Install / Enable

No external install needed (bundled skill). Enable it:

```bash
kafclaw skills enable-skill session-logs
```

## Usage

- Use when investigating unstable behavior across prior runs.
- Pair with `kafclaw doctor` output to correlate config/runtime issues.

## Troubleshooting

- If no useful data appears, verify session logs exist in your runtime workspace.
- If output may contain secrets, redact before sharing externally.
- If skill appears disabled, confirm with `kafclaw skills list`.
