# FR-001 — WhatsApp Auth Flow (Default Deny)

## Requirement
WhatsApp access must be **default‑deny**. No one receives bot responses until explicitly whitelisted.

## Rules
1. **Default blacklist**: If allowlist is empty, all senders are unauthorized.
2. **Whitelist required**: A sender must be explicitly whitelisted to receive bot responses.
3. **Token proof required**: A sender must submit the shared token before being eligible for approval.
4. **No auto‑response**: Unauthorized senders never receive a bot response.
5. **Owner approval**: Even after token submission, the owner must approve (whitelist) the sender.

## Storage
Settings keys (timeline DB):
- `whatsapp_pair_token`
- `whatsapp_allowlist`
- `whatsapp_denylist`
- `whatsapp_pending`

## UX
- CLI setup flow to generate/enter token and manage lists.
- Web UI flow to view pending users and approve/deny.

