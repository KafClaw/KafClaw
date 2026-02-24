---
title: Onboarding
nav_order: 3
has_children: true
---

Human-centered onboarding flows: first setup, channel onboarding, and early validation.

## Recommended Flow

1. [Getting Started Guide](/start-here/getting-started/)
2. `kafclaw doctor` then `kafclaw doctor --fix`
3. `kafclaw configure` for runtime/profile and embedding defaults
4. [WhatsApp Onboarding — Buddy Access](/integrations/whatsapp-onboarding/)
5. [WhatsApp Setup (Default Deny)](/integrations/whatsapp-setup/)

## Validation

- `kafclaw status`
- `kafclaw doctor --json`
- `curl -s http://127.0.0.1:18791/api/v1/status`
