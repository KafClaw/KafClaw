---
parent: Onboarding
title: WhatsApp Setup (Default Deny)
nav_order: 3
---

# WhatsApp Setup (Default Deny)

## Overview

WhatsApp access is **default-deny**. Only explicitly whitelisted senders can reach the bot. Unknown senders are placed in a pending queue for admin review.

## CLI Setup

```bash
kafclaw whatsapp-setup
```

Prompts:
- Enable WhatsApp (y/N)
- Pairing token (share out-of-band for device linking)
- Initial allowlist (comma or newline separated JIDs)
- Initial denylist (comma or newline separated JIDs)

Settings are stored in `~/.kafclaw/timeline.db`. After changes, restart `kafclaw gateway` to apply.

## CLI Approve/Deny

```bash
kafclaw whatsapp-auth --approve <jid>
kafclaw whatsapp-auth --deny <jid>
kafclaw whatsapp-auth --list
```

Use this to manage pending approvals directly from the terminal.

## Web UI Setup

Open the Config Manager in the Web Dashboard (`http://localhost:18791`):
- Set pairing token
- Edit allowlist / denylist
- Review pending senders
- Approve or deny pending

## Token Verification

- Unknown senders who submit the correct pairing token are added to **Pending**.
- They remain unauthorized until explicitly approved.

## Storage Keys

| Key | Description |
|-----|-------------|
| `whatsapp_pair_token` | Pairing token for device linking |
| `whatsapp_allowlist` | Newline-separated approved JIDs |
| `whatsapp_denylist` | Newline-separated blocked JIDs |
| `whatsapp_pending` | Newline-separated JIDs awaiting approval |

## Security Rules

1. If allowlist is empty, nobody is authorized.
2. Denylist always blocks, regardless of allowlist.
3. No automatic responses to unauthorized senders.
4. Silent mode (default on) suppresses all outbound WhatsApp delivery until explicitly disabled.

## Parity snapshot (OpenClaw vs KafClaw)

KafClaw can do:

- Native WhatsApp transport via `whatsmeow`
- QR-based device pairing and local session persistence
- Default-deny access with allowlist, denylist, pending queue, and pairing token gate
- Silent-mode default-on safety at startup/reconnect
- Inbound text handling and authorization-aware routing to the bus
- Inbound media capture baseline (image/audio/document download to workspace)
- Audio transcription baseline for inbound audio

Compared with OpenClaw, currently limited:

- No full WhatsApp outbound feature parity yet (OpenClaw has richer outbound media/reaction/poll flows)
- No OpenClaw-style group mention/reply behavior parity across all cases
- No full parity for advanced auto-reply monitor features and heartbeat/ack-reaction behavior
- No published WhatsApp parity matrix yet for all edge-case delivery semantics

## Key Files

| File | Purpose |
|------|---------|
| `~/.kafclaw/config.json` | WhatsApp enabled flag |
| `~/.kafclaw/whatsapp.db` | Session/device link persistence |
| `~/.kafclaw/whatsapp-qr.png` | QR code for initial device linking |
| `~/.kafclaw/timeline.db` | Allowlist, denylist, pending lists, pairing token |
