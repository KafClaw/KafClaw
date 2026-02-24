---
parent: Operations and Admin
title: KafClaw Management Guide
---

# KafClaw Management Guide

Operator-focused guide for managing KafClaw from CLI and runtime endpoints.

## 1. Core Command Surface

| Command | Purpose |
|--------|---------|
| `kafclaw onboard` | Initialize config and scaffold workspace identity files |
| `kafclaw gateway` | Run full gateway (API, dashboard, channels, memory, group/orchestrator when enabled) |
| `kafclaw status` | Quick operational status: config, providers, channels, pairing, policy diagnostics |
| `kafclaw doctor` | Run setup/config diagnostics including skills readiness checks |
| `kafclaw security` | Unified security checks/audit/fix (`check`, `audit --deep`, `fix --yes`) |
| `kafclaw config` | Low-level dotted-path config read/write/unset |
| `kafclaw configure` | Guided/non-interactive config updates (subagents, skills, Kafka group security) |
| `kafclaw skills` | Skills lifecycle (`enable/disable/list/status/enable-skill/disable-skill/verify/install/update/exec/auth/prereq`) |
| `kafclaw group` | Join/leave/status/members for Kafka collaboration group |
| `kafclaw knowledge` | Shared knowledge governance (`status`, `propose`, `vote`, `decisions`, `facts`) |
| `kafclaw kshark` | Kafka connectivity and protocol diagnostics |
| `kafclaw agent -m` | Single-shot direct CLI interaction with agent loop |
| `kafclaw pairing` | Approve/deny pending Slack/Teams sender pairings |
| `kafclaw whatsapp-setup` | Configure WhatsApp auth and initial lists |
| `kafclaw whatsapp-auth` | Approve/deny/list WhatsApp JIDs |
| `kafclaw install` | Install local binary (`/usr/local/bin` as root, `~/.local/bin` as non-root) |
| `kafclaw daemon` | Manage systemd service lifecycle (`install`, `uninstall`, `start`, `stop`, `restart`, `status`) |
| `kafclaw update` | Update lifecycle (`plan`, `apply`, `backup`, `rollback`) |
| `kafclaw completion` | Generate shell completion scripts (`bash|zsh|fish|powershell`) |
| `kafclaw version` | Print build version |

## 2. First-Time Operator Runbook

```bash
./kafclaw onboard
./kafclaw status
./kafclaw doctor
./kafclaw gateway
```

Then verify:

- API: `http://127.0.0.1:18790`
- Dashboard: `http://127.0.0.1:18791`

## 3. Release Installer (Recommended for Operators)

Install via release script (host OS/arch auto-detected):

```bash
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --latest
```

List available versions:

```bash
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --list-releases
```

Pinned install:

```bash
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --version v2.6.3
```

Unattended/headless install requires explicit version selection:

```bash
# Latest channel
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --unattended --latest

# Pinned version
curl --fail --show-error --silent --location \
  https://raw.githubusercontent.com/kafclaw/kafclaw/main/scripts/install.sh \
  | bash -s -- --unattended --version v2.6.3
```

Security behavior:

- Checksum verification (`SHA256SUMS`) is always required.
- Signature verification (`cosign`) is enabled by default.
- Use `--no-signature-verify` only in constrained environments where `cosign` is unavailable.
- Installer failures use structured error codes (for example `INSTALL_PREREQ_MISSING`, `INSTALL_DOWNLOAD_FAILED`) and include remediation text.

Root install behavior:

- Installer warns that root service install is a security risk.
- If accepted, it creates non-root user `kafclaw` (Linux) for service runtime.
- If declined (`n`), installer continues with root runtime and prints `Installing as root service.`

Install verification path (automatic at end of install):

- version check (`kafclaw version` / `kafclaw --version`)
- PATH check (whether `kafclaw` resolves from current shell)
- status check when config exists (`~/.kafclaw/config.json`), otherwise prints onboarding reminder

## 3.1 Update / Rollback Lifecycle

Plan the flow:

```bash
./kafclaw update plan
```

Create backup snapshot only:

```bash
./kafclaw update backup
```

Apply binary update:

```bash
./kafclaw update apply --latest
./kafclaw update apply --version v2.6.3
```

Apply source update:

```bash
./kafclaw update apply --source --repo-path /path/to/KafClaw
```

Rollback state from latest snapshot:

```bash
./kafclaw update rollback
```

Rollback state from specific snapshot:

```bash
./kafclaw update rollback --backup-path ~/.kafclaw/backups/update-YYYYMMDD-HHMMSSZ
```

`update apply` runs:

- preflight compatibility checks (config + timeline migration readiness)
- pre-update backup snapshot
- update apply (binary/source path)
- post-update health gates (`doctor`, security check)
- config drift report

Lifecycle event logs:

- Critical onboarding/update/rollback phases append JSONL events to:
  - `~/.kafclaw/lifecycle-events.jsonl`
- Use this for troubleshooting automation/non-interactive lifecycle runs.

## 4. Onboarding and Modes

### Interactive

```bash
./kafclaw onboard
```

### Non-interactive examples

```bash
./kafclaw onboard --non-interactive --profile local --llm skip
./kafclaw onboard --non-interactive --profile local-kafka --kafka-brokers localhost:9092 --group-name kafclaw --agent-id agent-local --role worker --llm skip
./kafclaw onboard --non-interactive --profile remote --llm openai-compatible --llm-api-base http://localhost:11434/v1 --llm-model llama3.1:8b
./kafclaw onboard --non-interactive --accept-risk --skip-skills=false --install-clawhub --skills-node-major 20
```

Mode effects applied by onboarding:

| Mode | Gateway Host | Group | Orchestrator | Auth Token |
|------|--------------|-------|--------------|------------|
| `local` | `127.0.0.1` | off | off | none |
| `local-kafka` | `127.0.0.1` | on | on | none |
| `remote` | `0.0.0.0` | off | off | generated/required |

Onboarding also scaffolds workspace files:

- `AGENTS.md`
- `SOUL.md`
- `USER.md`
- `TOOLS.md`
- `IDENTITY.md`

Use `--force` to overwrite existing config and scaffold files.

Lifecycle flags (operator-focused):

```bash
./kafclaw onboard --reset-scope config --non-interactive --accept-risk --profile local --llm skip
./kafclaw onboard --wait-for-gateway --health-timeout 20s
./kafclaw onboard --skip-healthcheck
./kafclaw onboard --daemon-runtime native
```

If onboarding installs systemd (`--systemd`), service activation is automatic by default.
Disable auto-activation with `--systemd-activate=false`.

## 4.1 Daemon / Service Lifecycle (Linux systemd)

Install service and activate immediately:

```bash
sudo ./kafclaw daemon install --activate
```

Service operations:

```bash
sudo ./kafclaw daemon status
sudo ./kafclaw daemon restart
sudo ./kafclaw daemon stop
sudo ./kafclaw daemon start
```

Uninstall service:

```bash
sudo ./kafclaw daemon uninstall
```

## 5. Daily Health Checks

### Status snapshot

```bash
./kafclaw status
```

Highlights include:

- config + API key presence
- WhatsApp session/QR state
- Slack/Teams account diagnostics and policy warnings
- pending pairing counts from timeline

### Doctor checks

```bash
./kafclaw doctor
./kafclaw doctor --fix
./kafclaw doctor --generate-gateway-token
```

`doctor` returns non-zero when failing checks exist.
When skills are enabled, doctor also checks `node`, `clawhub` (if external installs are enabled), runtime dir permissions, and channel-onboarding readiness.
Doctor also enforces memory embedding readiness (`memory.embedding` enabled with provider/model/dimension). If missing, `doctor --fix` applies defaults.
Use `kafclaw security` for consolidated security posture and deep skill audits.

## 6. Config Management

This guide stays as the command-surface index. Detailed flows moved to:

- [Memory Governance Operations](/memory-management/memory-governance-operations/)
- [Group and Kafka Operations](/collaboration/group-kafka-operations/)

Quick examples:

```bash
./kafclaw config get gateway.host
./kafclaw configure --non-interactive --memory-embedding-enabled-set --memory-embedding-enabled=true --memory-embedding-provider local-hf --memory-embedding-model BAAI/bge-small-en-v1.5 --memory-embedding-dimension 384
./kafclaw config set providers.openai.apiBase "https://openrouter.ai/api/v1"
```

## 7. Group Collaboration Operations

See [Group and Kafka Operations](/collaboration/group-kafka-operations/) for full runbook and security config examples.

Quick lifecycle:

```bash
./kafclaw group join mygroup
./kafclaw group status
./kafclaw group members
./kafclaw group leave
```

## 8. Kafka Diagnostics with KShark

Detailed diagnostics and options are in [Group and Kafka Operations](/collaboration/group-kafka-operations/).

Quick commands:

```bash
./kafclaw kshark --auto --yes
./kafclaw kshark --props ./client.properties --topic group.mygroup.requests --group mygroup-workers --yes
```

## 9. Channel Auth and Pairing

### Pairing queue (Slack/Teams)

```bash
./kafclaw pairing pending
./kafclaw pairing approve slack ABC123
./kafclaw pairing deny msteams XYZ999
```

### WhatsApp auth flow

```bash
./kafclaw whatsapp-setup
./kafclaw whatsapp-auth --list
./kafclaw whatsapp-auth --approve "+123456789@s.whatsapp.net"
./kafclaw whatsapp-auth --deny "+123456789@s.whatsapp.net"
```

## 10. Channel Bridge (`cmd/channelbridge`)

Build and run:

```bash
go build -o /tmp/channelbridge ./cmd/channelbridge
/tmp/channelbridge
```

Default bind: `:18888` (set via `CHANNEL_BRIDGE_ADDR`).

Key endpoints:

- `GET /healthz`
- `GET /status`
- `POST /slack/events`
- `POST /slack/commands`
- `POST /slack/interactions`
- `POST /teams/messages`

Core env vars:

- `KAFCLAW_BASE_URL` (default `http://127.0.0.1:18791`)
- `KAFCLAW_SLACK_INBOUND_TOKEN`
- `KAFCLAW_MSTEAMS_INBOUND_TOKEN`
- `SLACK_SIGNING_SECRET`
- `MSTEAMS_INBOUND_BEARER`
- `CHANNEL_BRIDGE_STATE` (bridge state file path)

## 10. State and Paths

Core runtime files:

- Config: `~/.kafclaw/config.json`
- Env: `~/.config/kafclaw/env`
- Timeline DB: `~/.kafclaw/timeline.db`
- WhatsApp session: `~/.kafclaw/whatsapp.db`
- WhatsApp QR image: `~/.kafclaw/whatsapp-qr.png`
- Subagent state: `~/.kafclaw/subagents/`

## 11. Client Auth Token Distribution

This section applies to **direct HTTP clients** that call KafClaw API endpoints.
For Slack/Teams/WhatsApp users, authentication is handled by provider bridge + pairing/allowlist controls, not by manually passing the gateway bearer token.

## 12. Security Command Runbook

```bash
./kafclaw security check
./kafclaw security audit --deep
./kafclaw security fix --yes
```

Recommended usage:

- `security check`: quick operational gate in CI/day-2 operations.
- `security audit --deep`: include installed skill re-verification.
- `security fix --yes`: apply safe remediations; re-run check after changes.
- `doctor --fix`: merges env files, syncs sensitive env keys into tomb-managed encrypted storage, then scrubs those sensitive keys from `~/.config/kafclaw/env`.

For security posture details, see [Security for Operators](/architecture-security/security-for-ops/).
For skills policy, OAuth keying, and source pinning syntax, see [Skills](/skills/).

Recommended CI gate:

```bash
go run ./cmd/kafclaw security check
```

When `KAFCLAW_GATEWAY_AUTH_TOKEN` (or `gateway.authToken`) is set, direct clients do not auto-receive tokens.
Operators must distribute tokens out-of-band (secure chat, secret manager, deployment env injection, etc.).

Operator token sources:

- `~/.kafclaw/config.json` (`gateway.authToken`)
- `~/.config/kafclaw/env` (if managed there)
- `./kafclaw doctor --generate-gateway-token` (rotate/create)

Client request header:

```http
Authorization: Bearer <token>
```

Validation endpoint:

- `POST /api/v1/auth/verify` checks a provided bearer token
- it validates tokens; it does not mint or return a token

## 12. Incident Shortcuts

### Gateway will not start

1. `./kafclaw doctor`
2. Confirm API key/provider config
3. Check timeline DB permissions and disk
4. Re-run `./kafclaw onboard` (or `--force` if needed)

### Kafka/group issues

1. `./kafclaw group status`
2. `./kafclaw kshark --auto --yes`
3. Verify brokers/auth and LFS proxy settings

### Channel ingress issues

1. `./kafclaw status` for account diagnostics
2. Check `pairing pending`
3. If using bridge, check `GET /status` on channel bridge
