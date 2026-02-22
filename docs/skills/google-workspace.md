---
title: Google Workspace
parent: Skills
nav_order: 8
---

# Google Workspace

Headless OAuth flow for Google Workspace integrations.

## Default State

- Bundled with KafClaw
- Disabled by default

## What It Does

- Supports headless OAuth enrollment for Google Workspace APIs.
- Stores token state securely for runtime use by the local agent.
- Enables policy-gated Google Workspace operations after enrollment.

For key backend options and storage/security posture, see [Skills](/skills/).

## Install / Enable

No external install needed (bundled skill). Enable it:

```bash
kafclaw skills enable-skill google-workspace
```

## Start

```bash
kafclaw skills auth start google-workspace \
  --client-id '<client-id>' \
  --client-secret '<client-secret>' \
  --redirect-uri 'http://localhost:53682/callback' \
  --access 'mail,calendar,drive'
```

## Complete

```bash
kafclaw skills auth complete google-workspace \
  --callback-url 'http://localhost:53682/callback?code=...&state=...'
```

## Usage

- Start flow, open returned URL in a browser, approve scopes, and paste callback URL.
- Keep scopes minimal and aligned with tenant policy.
- OAuth flow start/complete events are recorded in chained security audit logs (see [Skills](/skills/)).
- Agent read-only tool:
  - `google_workspace_read` with `operation=gmail_list_messages|calendar_list_events|drive_list_files`
  - include Gmail/Calendar/Drive read scopes during `auth start` for the operation you need.
- Configure capability defaults (used by `skills auth start` when `--scopes` is omitted):
  - `kafclaw configure --google-workspace-read mail,calendar`

## Troubleshooting

- If `state mismatch` appears, restart the flow and use the latest callback.
- If token exchange fails, validate client ID/secret, redirect URI, and consent scopes.
- If skill doctor warns about missing token, rerun `auth start` + `auth complete`.
