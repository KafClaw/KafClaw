---
parent: Architecture and Security
nav_order: 1
---

# Security for Operators

This is the practical security runbook for KafClaw operators.

## 1. Baseline Deployment Rules

1. Run KafClaw as a non-root user.
2. Keep default bind on loopback unless you explicitly need remote access.
3. If remote access is enabled, always set `KAFCLAW_GATEWAY_AUTH_TOKEN`.
4. Keep bot workspace and work repo isolated from sensitive host paths.
5. Treat the host as production: patched OS, firewall, no shared admin accounts.

## 2. Required Configuration

Minimum hardened remote setup:

```bash
export KAFCLAW_GATEWAY_HOST=0.0.0.0
export KAFCLAW_GATEWAY_PORT=18790
export KAFCLAW_GATEWAY_DASHBOARD_PORT=18791
export KAFCLAW_GATEWAY_AUTH_TOKEN='<long-random-token>'
export KAFCLAW_TOOLS_EXEC_RESTRICT_WORKSPACE=true
```

Provider keys should be set from environment variables, not committed config files.

## 3. Token Handling

`KAFCLAW_GATEWAY_AUTH_TOKEN` is a shared bearer token for direct API clients.

Operational policy:

1. Generate with high entropy (>= 32 random bytes).
2. Distribute through your secret manager only.
3. Never paste tokens in chat, tickets, or docs.
4. Rotate immediately if exposed.
5. Rotate on schedule (for example, every 30-90 days).

## 4. Access Control Model

1. External senders are constrained by policy tiering.
2. Keep WhatsApp/bridge allowlists tight.
3. Use room/account session scopes to avoid cross-tenant leakage.
4. Keep default-deny behavior for unknown senders where supported.

## 5. Runtime Hardening Checklist

Before go-live:

- [ ] `kafclaw doctor` passes
- [ ] `kafclaw status` shows expected providers/channels
- [ ] Gateway auth token configured for non-localhost deployments
- [ ] Work repo path and workspace path are correct and isolated
- [ ] Backups configured for `~/.kafclaw/timeline.db`
- [ ] Log retention and access controls defined
- [ ] Incident owner and escalation path defined

## 6. Monitoring and Incident Response

Watch for:

1. Unexpected spikes in token usage.
2. Unknown sender activity.
3. Suspicious or repetitive high-risk tool attempts.
4. Repeated auth failures on gateway endpoints.

Immediate response playbook:

1. Rotate gateway auth token.
2. Disable affected channel integrations.
3. Restrict tool surface (temporary deny/allow tightening).
4. Snapshot logs and timeline DB for investigation.
5. Restore service with staged re-enable of channels/tools.

## 7. Backup and Recovery

Back up at minimum:

1. `~/.kafclaw/timeline.db`
2. `~/.kafclaw/config.json` (sanitized in secure storage)
3. Workspace identity/soul files if operationally required

Test restore regularly on a clean host.

## 8. Operator Commands

```bash
./kafclaw status
./kafclaw doctor
./kafclaw doctor --fix
```

Use these as part of daily operations and after any security-relevant change.
