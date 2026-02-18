---
parent: Integrations
---

# Slack + Teams Bridge

This bridge provides pairing and message flow for Slack and Microsoft Teams with KafClaw.

## Run

Build:

```bash
go build -o /tmp/channelbridge ./cmd/channelbridge
```

Start:

```bash
KAFCLAW_BASE_URL=http://127.0.0.1:18791 \
KAFCLAW_SLACK_INBOUND_TOKEN=... \
KAFCLAW_MSTEAMS_INBOUND_TOKEN=... \
SLACK_BOT_TOKEN=xoxb-... \
SLACK_APP_TOKEN=xapp-... \
SLACK_ACCOUNT_ID=default \
SLACK_REPLY_MODE=all \
SLACK_BOT_USER_ID=U... \
SLACK_API_BASE=https://slack.com/api \
MSTEAMS_APP_ID=... \
MSTEAMS_APP_PASSWORD=... \
MSTEAMS_ACCOUNT_ID=default \
MSTEAMS_REPLY_MODE=all \
MSTEAMS_TENANT_ID=botframework.com \
MSTEAMS_INBOUND_BEARER=... \
MSTEAMS_OPENID_CONFIG=https://login.botframework.com/v1/.well-known/openidconfiguration \
MSTEAMS_API_BASE= \
MSTEAMS_GRAPH_BASE=https://graph.microsoft.com/v1.0 \
SLACK_SIGNING_SECRET=... \
CHANNEL_BRIDGE_STATE=/path/to/channelbridge-state.json \
/tmp/channelbridge
```

Default bind: `:18888`.

Health/status:

- `GET /healthz` basic liveness
- `GET /status` bridge counters and Teams reference/token cache info

## KafClaw config

Set these in `~/.kafclaw/config.json`:

```json
{
  "channels": {
    "slack": {
      "enabled": true,
      "dmPolicy": "pairing",
      "groupPolicy": "allowlist",
      "inboundToken": "YOUR_SLACK_INBOUND_TOKEN",
      "outboundUrl": "http://127.0.0.1:18888/slack/outbound"
    },
    "msteams": {
      "enabled": true,
      "dmPolicy": "pairing",
      "groupPolicy": "allowlist",
      "inboundToken": "YOUR_MSTEAMS_INBOUND_TOKEN",
      "outboundUrl": "http://127.0.0.1:18888/teams/outbound"
    }
  }
}
```

## Inbound endpoints

- Slack Events API -> `POST /slack/events`
- Slack slash commands -> `POST /slack/commands`
- Slack interactions -> `POST /slack/interactions`
- Teams bot messages -> `POST /teams/messages`

Resolver endpoints:

- `POST /slack/resolve/users` with `{"entries":["alice","user:U123"]}`
- `POST /slack/resolve/channels` with `{"entries":["eng","channel:C111"]}`
- `POST /teams/resolve/users` with `{"entries":["alex@example.com","user:GUID"]}`
- `POST /teams/resolve/channels` with `{"entries":["eng/general","conversation:..."]}`

Probe endpoints:

- `GET /slack/probe` validates Slack token with `auth.test`
- `GET /teams/probe` validates Teams bot token flow and returns decoded bot/graph claims plus diagnostics (audience, expiry, scopes/roles), permission coverage, tenant/app identity checks, and live Graph capability checks (`users`, `teams`, `channels`, `organization`)

Outbound endpoints:

- `POST /slack/outbound`
- `POST /teams/outbound`

Socket mode ingress:

- If `SLACK_APP_TOKEN` is set, the bridge also consumes Slack Events API, slash commands, and interactions via Socket Mode.

## Auth checks

Slack request verification:

- If `SLACK_SIGNING_SECRET` is set, `X-Slack-Signature` + `X-Slack-Request-Timestamp` are enforced
- If `SLACK_SIGNING_SECRET` is empty, signature verification is skipped

Teams request verification:

- If `MSTEAMS_INBOUND_BEARER` is set, `Authorization: Bearer <token>` is required on `POST /teams/messages`
- If `MSTEAMS_INBOUND_BEARER` is empty, bearer verification is skipped
- If `MSTEAMS_APP_ID` is set, the bridge validates Bot Framework JWTs using OpenID config + JWKS (`MSTEAMS_OPENID_CONFIG`)
- JWT validation includes trusted Teams/Bot Framework service URL host checks and audience matching (string or array claim forms)

Forward targets:

- `POST /api/v1/channels/slack/inbound`
- `POST /api/v1/channels/msteams/inbound`

## Pairing flow

1. Unknown sender triggers pairing reply with a code.
2. Owner approves:
- `kafclaw pairing approve slack <CODE>`
- `kafclaw pairing approve msteams <CODE>`
3. KafClaw updates allowlist and sends approval confirmation via bridge outbound.

## Isolation

Session scope is selectable via channel config `sessionScope` with modes:

- `channel` -> `channel`
- `account` -> `channel:account`
- `room` (default) -> `channel:account:chat`
- `thread` -> `channel:account:chat:thread` (falls back to room if no thread id)
- `user` -> `channel:account:sender`

For Slack/Teams this is configured via:

- `channels.slack.sessionScope` (or per account `channels.slack.accounts[].sessionScope`)
- `channels.msteams.sessionScope` (or per account `channels.msteams.accounts[].sessionScope`)

For WhatsApp this is configured via:

- `channels.whatsapp.sessionScope`

## Inbound dedupe

- Slack duplicate suppression uses `event_id` and message fallback key (`channel+ts`)
- Teams duplicate suppression uses message activity key (`conversation+activity id`)
- Dedupe cache is persisted in `CHANNEL_BRIDGE_STATE` and restored on restart

## Anti-leakage regression coverage

Regression tests cover isolation boundaries across:

- cross-channel same chat identifiers (Slack vs Teams vs WhatsApp)
- cross-room separation inside the same provider
- cross-thread separation where thread mode/session scope applies
- WhatsApp regression path to ensure no session leakage into Slack/Teams scopes

## Outbound payload

Bridge outbound accepts optional fields:

- `account_id` (`string`, defaults to `default`)
- `reply_mode` (`off|first|all`, defaults to channel env default)
- `stream_mode` (`replace|append|status_final`, Slack draft/native stream behavior)
- `stream_chunk_chars` (`int`, Slack native stream chunk sizing)
- `media_urls` (`[]string`)
- `card` (`object`, Teams adaptive card payload)
- `action` + `action_params` (Slack action operations)
- `poll_question` + `poll_options` + `poll_max_selections` (Teams poll baseline)
- `thread_id` (thread reply target)

Slack behavior:

- First media URL is fetched and uploaded using `files.uploadV2`
- Text/card/action/probe/resolve/send paths use the Go SDK module `github.com/slack-go/slack`
- Text send maps `thread_id` -> `thread_ts`
- Native streaming parity: `chat.startStream`/`chat.appendStream`/`chat.stopStream` with fallback to `chat.postMessage`
- Supported action baseline: `react`, `edit`, `delete`, `pin`, `unpin`, `read`
- Target normalization: `user:U...`, `channel:C...`
- Inbound normalization covers `message`, `app_mention`, and key message subtypes (`message_changed`, `message_deleted`, `message_replied`, `file_share`) with bot-message filtering
- Multi-account baseline: account-aware inbound/outbound payload routing via `account_id`
- Reply strategy parity: `off` (never thread), `first` (thread first reply per account/chat), `all` (thread all replies)
- Reply-by-chat-type parity via `SLACK_REPLY_MODE_BY_CHAT_TYPE` (`direct|group|channel`)
- History hint forwarding parity via `SLACK_HISTORY_LIMIT` / `SLACK_DM_HISTORY_LIMIT`
- Chunking parity: long markdown payloads are split into safe chunks for multi-message fallback delivery

Teams behavior:

- Media URLs are attached as `application/octet-stream` attachment URLs (multi-media supported)
- `card` is attached as adaptive card (`application/vnd.microsoft.card.adaptive`)
- Text send maps `thread_id` -> `replyToId`
- Poll lifecycle parity builds adaptive-card polls with stable `poll_id`, validates/limits selections, and stores per-option results/totals in bridge state
- Target normalization: `conversation:...`, `user:...`
- Inbound normalization includes `channelData` extraction (`team/channel/tenant`), mention-text stripping, card-text fallback extraction, and attachment media URL extraction
- Multi-account baseline: account-aware inbound/outbound payload routing via `account_id`
- Group target allowlist parity baseline: `groupAllowFrom` supports team/channel entries (for example `team:<team-id>/channel:<channel-id>`, `<team-id>/<channel-id>`, `team:<team-id>`, `channel:<channel-id>`)
- Reply strategy parity: `off` (omit `replyToId`), `first` (set `replyToId` only for first reply per account/chat), `all` (set `replyToId` whenever thread id is present)
- Attachment URL host gating parity via `MSTEAMS_MEDIA_ALLOW_HOSTS`
- History hint forwarding parity via `MSTEAMS_HISTORY_LIMIT` / `MSTEAMS_DM_HISTORY_LIMIT`

## Known limitations

Current limitations for parity tracking:

- Teams runtime remains custom Go HTTP/JWT logic (not Microsoft Agents Hosting runtime)
- Bridge process account credentials are still single-account per process; for multiple provider accounts run one bridge instance per account and set `SLACK_ACCOUNT_ID`/`MSTEAMS_ACCOUNT_ID`

## Parity snapshot (OpenClaw vs KafClaw)

### Slack

KafClaw can do:

- Pairing-gated inbound access with `pairing|allowlist|open|disabled` semantics
- Mention-gated group handling via access policy
- Inbound signature verification (`SLACK_SIGNING_SECRET`)
- Inbound dedupe and persisted dedupe cache
- Outbound text + thread replies
- Outbound first-file media upload
- Action baseline: `react`, `edit`, `delete`, `pin`, `unpin`, `read`
- Resolve/probe endpoints (`resolve users/channels`, `probe`)
- SDK-backed Slack API calls via `github.com/slack-go/slack`

Compared with OpenClaw, currently limited:

- Operational model differs (bridge process + gateway vs plugin runtime), but Slack transport parity goals are implemented in this bridge

### Teams

KafClaw can do:

- Pairing-gated inbound access with `pairing|allowlist|open|disabled` semantics
- Inbound bearer gate and Bot Framework JWT baseline checks (openid/jwks/signature/claims/service url host)
- Inbound dedupe and persisted dedupe cache
- Outbound text + thread replies
- Outbound URL attachment + adaptive card baseline
- Poll baseline (card creation + vote record baseline + persisted poll state)
- Resolve/probe endpoints (`resolve users/channels`, `probe`)

Compared with OpenClaw, currently limited:

- No Microsoft Agents Hosting runtime parity in Go (runtime architecture differs)

## Rate limit handling

Retry paths honor `Retry-After` (seconds or HTTP-date) before retrying.

## Delivery telemetry + error taxonomy

Delivery updates now record reason codes into task `error_text` for failed/pending delivery paths.

Reason taxonomy:

- `transient:rate_limited`
- `transient:upstream_5xx`
- `transient:network`
- `terminal:unauthorized`
- `terminal:invalid_target_or_payload`
- `terminal:max_retries_exceeded`
- `terminal:send_failed`
