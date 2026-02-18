---
parent: Integrations
---

# WhatsApp Onboarding — Buddy Access

End-to-end guide for connecting a buddy to your KafClaw instance via WhatsApp.

> See also: [WhatsApp Setup (Default Deny)](whatsapp-setup/) for CLI reference.

## Overview

KafClaw uses **default-deny** for WhatsApp. New senders are blocked until the owner explicitly approves them. This guide walks through the full onboarding flow for adding a buddy.

## Step 1: Initial Setup (one-time)

```bash
# First-time setup — creates config and workspace
kafclaw onboard

# Pair your WhatsApp account (scan QR code)
kafclaw whatsapp-auth
```

The QR code is saved to `~/.kafclaw/whatsapp-qr.png`. Open it and scan with WhatsApp on your phone.

## Step 2: Configure Auth Settings

```bash
# Set a pairing token (shared out-of-band with your buddy)
kafclaw whatsapp-setup
```

This prompts for:
- **Pairing token** — a shared secret your buddy sends as their first message
- **Allowlist** — JIDs to pre-approve (format: `1234567890@s.whatsapp.net`)
- **Denylist** — JIDs to permanently block

## Step 3: Start the Gateway

```bash
make run          # standalone mode
# or
make run-full     # group mode (if workshop is running)
```

Silent mode is **enabled by default** on every WhatsApp start. This prevents accidental outbound messages. Disable it from the dashboard when ready.

## Step 4: Buddy Sends First Message

1. Share the pairing token with your buddy out-of-band (text, email, etc.)
2. Buddy sends the token as their first WhatsApp message to the bot's number
3. The message appears in the **pending queue** on the dashboard

## Step 5: Approve on Dashboard

1. Open `http://localhost:18791` (dashboard)
2. Navigate to the WhatsApp section
3. Find the pending message from your buddy
4. Click **Approve** to add them to the allowlist

Once approved, the buddy can send messages normally. The bot responds according to its soul file personality.

## Step 6: Disable Silent Mode

Once setup is verified and the buddy is approved:

1. Dashboard → Settings
2. Toggle **Silent Mode** off
3. The bot will now send outbound responses via WhatsApp

## Security Notes

- **Silent mode resets to ON** on every gateway restart. This is intentional — it prevents accidental sends after updates or crashes.
- **Allowlist is persistent** — approved buddies remain approved across restarts.
- **Denylist is permanent** — denied senders are blocked until explicitly removed.
- **Token is one-time** — after a buddy is approved, the token is no longer needed for that sender.

## Current capability note

For channel parity status versus OpenClaw (what WhatsApp can do today in KafClaw, and what remains limited), see:

- [WhatsApp Setup (Default Deny)](whatsapp-setup/) -> **Parity snapshot (OpenClaw vs KafClaw)**

## CLI Quick Reference

```bash
# Approve a sender
kafclaw whatsapp-auth --approve 1234567890@s.whatsapp.net

# Deny a sender
kafclaw whatsapp-auth --deny 1234567890@s.whatsapp.net

# List all auth entries
kafclaw whatsapp-auth --list
```
