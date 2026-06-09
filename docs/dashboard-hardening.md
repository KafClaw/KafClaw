# KafClaw dashboard hardening

How to operate the KafClaw Control Center (the gateway dashboard on the
DashboardPort, default `18791`) safely. Covers offline rendering, the auth
token, TLS, and the recommended edge-proxy path for exposure beyond a trusted
host. Tracked under the er1-brain v1 release-blocker (scalytics-all-in-one-meta
PLAN-15, Workstream A "UI hardening").

## 1. Offline rendering (no CDN calls)

The dashboard pages (`/`, `/timeline`, `/group`, `/approvals`) previously pulled
Tailwind, Vue, d3, dagre-d3, and the JetBrains Mono webfont from public CDNs
(`cdn.tailwindcss.com`, `unpkg.com`, `d3js.org`, `cdn.jsdelivr.net`,
`fonts.googleapis.com`). On an air-gapped or egress-restricted host that left the
UI unstyled and non-functional, and it leaked a request to third-party CDNs on
every page load.

These assets are now **vendored** and embedded in the binary under `web/vendor/`,
served from the gateway at `/vendor/...`:

| Asset | Local path |
|---|---|
| Tailwind | `/vendor/tailwind.js` |
| Vue 3 (prod build) | `/vendor/vue.global.prod.js` |
| d3 v7 | `/vendor/d3.v7.min.js` |
| dagre-d3 | `/vendor/dagre-d3.min.js` |
| JetBrains Mono (woff2 400/500/600/700/800) | `/vendor/fonts/*.woff2` via `/vendor/fonts.css` |

The dashboard now renders with **zero external network calls**. To refresh a
vendored asset, replace the file under `web/vendor/` and rebuild; the
`//go:embed *.html templates vendor` directive in `web/assets.go` packages it.

> Note: `tailwind.js` is the Tailwind Play CDN runtime (in-browser JIT). It is
> self-contained and offline once vendored. A future improvement is a
> precompiled static `tailwind.css` (scan the HTML with the Tailwind CLI) to
> drop the in-browser compile.

## 2. Auth token

The gateway gates the dashboard + API behind a bearer token when
`KAFCLAW_GATEWAY_AUTH_TOKEN` (config `gateway.authToken`) is set:

```bash
export KAFCLAW_GATEWAY_AUTH_TOKEN="$(openssl rand -hex 24)"
kafclaw gateway
# -> "🔒 Auth token required for dashboard API"
```

When set, **every** path requires `Authorization: Bearer <token>` except the
health/scrape endpoints used by orchestration: `/api/v1/status`, `/metrics`, and
CORS preflight (`OPTIONS`). API clients send the header:

```bash
curl -H "Authorization: Bearer $KAFCLAW_GATEWAY_AUTH_TOKEN" http://host:18791/api/v1/...
```

### Browser caveat (important)

A plain browser navigation to `/timeline` does **not** send an `Authorization`
header, so with the token set the dashboard HTML itself returns `401` in a
browser; there is no login form yet. The token therefore protects **programmatic
API access**, not an interactive browser session. For human access to the
dashboard, do one of:

- keep the gateway bound to a trusted host (the default `gateway.host` is
  `127.0.0.1`) and reach it over an SSH tunnel, or
- put it behind an edge proxy that performs browser authentication (below).

An interactive browser-login flow is tracked as a follow-up (not required for
the API-auth acceptance).

## 3. TLS (direct HTTPS)

For direct TLS on the gateway, set both:

```bash
export KAFCLAW_GATEWAY_TLS_CERT=/path/server.crt
export KAFCLAW_GATEWAY_TLS_KEY=/path/server.key
# -> "🖥️  Dashboard listening on https://..."
```

## 4. Recommended: edge proxy for exposure beyond the host

The robust pattern for anything past a single trusted host is **not** to expose
the gateway directly. Keep it on `127.0.0.1` (or cluster-internal) and terminate
TLS + browser authentication at an edge proxy / ingress:

```
Browser ──TLS + authN──► edge proxy (nginx / ingress / SAO edge) ──► kafclaw gateway (127.0.0.1:18791)
```

In SAO this is the existing edge-proxy convention (ADR-0001: browser UIs reach
backends via the `localhost:8090/<service>` edge; the backend stays
CORS-unaware). The edge owns TLS, authentication, and rate-limiting; the gateway
trusts its loopback/in-cluster boundary. This keeps the gateway's secure default
(`127.0.0.1`) intact and avoids the browser-token gap in section 2.

## Summary

| Concern | State |
|---|---|
| Offline render (no CDN) | done , assets vendored + embedded, served at `/vendor/` |
| API auth token | available , `KAFCLAW_GATEWAY_AUTH_TOKEN`; gates all but `/status`, `/metrics`, `OPTIONS` |
| Direct TLS | available , `KAFCLAW_GATEWAY_TLS_CERT` + `_TLS_KEY` |
| Browser-exposed dashboard | use the edge proxy (TLS + authN at the edge); gateway stays on `127.0.0.1` |
| Interactive browser login | follow-up (tracked) |
