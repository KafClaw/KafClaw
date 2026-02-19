---
title: Weather
parent: Skills
nav_order: 6
---

# Weather

Provide weather and short forecast responses.

## Default State

- Bundled with KafClaw
- Disabled by default

## What It Does

- Resolves location-based current weather and short forecasts.
- Returns concise forecast windows with units and timing context.
- Handles ambiguous location requests with follow-up clarification.

## Install / Enable

No external install needed (bundled skill). Enable it:

```bash
kafclaw skills enable-skill weather
```

## Usage

- Use for quick weather lookups used in planning or incident operations.
- Provide explicit location format when possible (`City, Country/State`).

## Troubleshooting

- If location is ambiguous, include region/country in your query.
- If data looks stale, re-run query with explicit date/time window.
- If skill is not selectable, verify `skills.enabled=true`.
