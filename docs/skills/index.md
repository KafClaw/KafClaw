---
title: Skills
nav_order: 11
has_children: true
---

# Skills

Operator and security notes for bundled and external skills.

For deep security posture and operational commands, see:

- [Security for Operators](/architecture-security/security-for-ops/)
- [KafClaw Management Guide](/operations-admin/manage-kafclaw/)

## Bundled Skills

- `channel-onboarding` (default enabled)
- `session-logs`
- `summarize`
- `github`
- `gh-issues`
- `weather`
- `google-cli`
- `google-workspace`
- `m365`
- `incident-comms`
- `skill-creator`

## Prerequisites

- Default bundled `channel-onboarding` does not require Node/npm.
- Node.js (`node` and `npm` in `PATH`) is required when enabling broader skills runtime.
- `clawhub` CLI is required for external skill installs/updates.

```bash
npm install -g --ignore-scripts clawhub
```

## Bootstrap

```bash
kafclaw skills enable --install-clawhub
```

This command enables skills, creates secure runtime directories, ensures `.nvmrc`, and validates tooling.

## Runtime Policy and Scope

- Skills scope:
  - `skills.scope=selected` (recommended default): only explicitly enabled skills are available.
  - `skills.scope=all`: all skills are considered enabled.
- Runtime isolation:
  - `skills.runtimeIsolation=auto` (default): use container isolation when available, otherwise host policy mode.
  - `skills.runtimeIsolation=strict`: require container isolation (`docker`/`podman`), fail if unavailable.
  - `skills.runtimeIsolation=host`: host execution with policy enforcement only.

Configure examples:

```bash
kafclaw configure --non-interactive --skills-scope selected
```

## Lifecycle (Onboard / Configure / Doctor)

- Onboarding can bootstrap skills automatically:

```bash
kafclaw onboard --accept-risk --non-interactive --skip-skills=false --install-clawhub --skills-node-major 20
```

- Optional during onboarding: preselect provider capabilities:

```bash
kafclaw onboard --accept-risk --non-interactive --skip-skills --google-workspace-read mail,drive --m365-read calendar
```

- Configure can manage global and per-skill toggles:

```bash
kafclaw configure --non-interactive --skills-enabled-set --skills-enabled=true --skills-node-manager npm --enable-skill github --disable-skill weather
```

- Configure OAuth capability presets for provider skills:

```bash
kafclaw configure --non-interactive --google-workspace-read mail,calendar --m365-read mail,files
```

- Doctor validates skills prerequisites/runtime readiness:

```bash
kafclaw doctor
```

- Security command surface (recommended for ongoing checks):

```bash
kafclaw security check
kafclaw security audit --deep
kafclaw security fix --yes
```

## Commands

- `kafclaw skills list`
- `kafclaw skills status`
- `kafclaw skills enable`
- `kafclaw skills disable`
- `kafclaw skills enable-skill <name>`
- `kafclaw skills disable-skill <name>`
- `kafclaw skills verify <path-or-url>`
- `kafclaw skills install <slug-or-url>`
- `kafclaw skills update [name]`
- `kafclaw skills prereq check <name>`
- `kafclaw skills prereq install <name> --dry-run|--yes`
- `kafclaw skills auth start <provider> ...`
- `kafclaw skills auth complete <provider> --callback-url ...`
- `kafclaw skills auth start <provider> --access mail,calendar,...` (scope presets)

Agent tool names for OAuth-enrolled read-only access:

- `google_workspace_read` (`gmail_list_messages`, `calendar_list_events`, `drive_list_files`)
- `m365_read` (`mail_list_messages`, `calendar_list_events`, `onedrive_list_children`)

## Remediation

- Missing `node`: install Node.js and rerun `kafclaw skills enable`
- Missing `clawhub`: `npm install -g --ignore-scripts clawhub`
- Blocked external installs: set `skills.externalInstalls=true`
- Blocked domain: update `skills.linkPolicy` allow/deny configuration
- Skill-specific setup errors: run `kafclaw doctor` and fix reported prerequisites/tokens

## OAuth Secret Storage

- OAuth pending and token blobs are encrypted at rest.
- OAuth blobs and tomb-stored env secrets both use AES-GCM and the same tomb-managed master key material.
- OAuth runtime files are stored under `~/.kafclaw/skills/tools/auth/<provider>/<profile>/` as `pending.json` and `token.json`; they are written encrypted (not plaintext secret JSON files).
- Master key backend supports local-native tomb key (default) and fallbacks:
  - `KAFCLAW_OAUTH_KEY_BACKEND=local|auto|keyring|file`
  - Local default path: `~/.config/kafclaw/tomb.rr` (0600)
  - Optional override: `KAFCLAW_OAUTH_TOMB_FILE=/custom/path/tomb.rr`
  - Optional explicit key: `KAFCLAW_OAUTH_MASTER_KEY`
- `kafclaw doctor --fix` also syncs sensitive env keys into tomb-managed encrypted storage (`env_tomb_sync` check).
- After successful tomb sync, `doctor --fix` scrubs sensitive keys from `~/.config/kafclaw/env` (`env_sensitive_scrub` check). Runtime then loads them from tomb into process env.
- For provider-specific enrollment flows, use:
  - [google-workspace](/skills/google-workspace/)
  - [m365](/skills/m365/)

## External Source Pinning

- External skill install sources can be pinned with `#sha256=<64-hex>`:
  - `kafclaw skills install 'https://example.org/skills/demo.zip#sha256=<digest>'`
- Install/update fails if the installed package hash does not match the pin.
- `kafclaw security check` reports hash-pinning coverage for installed external skills.

## ClawHub State

- `clawhub:<slug>` installs are staged in quarantine, verified, then installed.
- ClawHub state/metadata is synchronized under skills runtime tools state.
- Security events and install decisions are recorded as chained JSONL audit logs in `~/.kafclaw/skills/audit/`.
- See [KafClaw Management Guide](/operations-admin/manage-kafclaw/) for operations workflow.
