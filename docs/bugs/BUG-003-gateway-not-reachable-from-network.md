# BUG-003 — Gateway dashboard not reachable from other machines on LAN

## Status: Closed (documented)

## Reported: 2026-02-16

## Platform: Jetson Nano (ARM64 Ubuntu), accessed from macOS browser on same LAN

## Symptoms

Gateway starts successfully on Jetson Nano:

```
🌐 KafClaw Gateway
─────────────────────
Starting KafClaw Gateway...
📡 API Server listening on http://127.0.0.1:18790
🖥️  Dashboard listening on http://127.0.0.1:18791
```

But browsing to `https://192.168.0.199:18791/` from another machine on the LAN shows nothing.

## Root Causes

### 1. Binding to 127.0.0.1 (localhost only)

The gateway binds to `127.0.0.1` by default — a secure default that only accepts connections from the local machine. It is not reachable from the network.

To be reachable from LAN, it must bind to `0.0.0.0` (all interfaces) or a specific LAN IP.

**Evidence:** The log says `http://127.0.0.1:18790` and `http://127.0.0.1:18791`.

### 2. Wrong protocol: HTTPS vs HTTP

The browser URL uses `https://` but the gateway serves plain `http://`. Unless TLS is configured (`tlsCert`/`tlsKey` in config), the connection will fail silently or show a browser error.

## Fix

### Option A: Use headless mode (recommended for LAN access)

```bash
export MIKROBOT_GATEWAY_AUTH_TOKEN=mysecrettoken
make run-headless
```

This sets `MIKROBOT_GATEWAY_HOST=0.0.0.0` and requires an auth token for security.

Then access: `http://192.168.0.199:18791/` (note: `http`, not `https`).

### Option B: Override host manually

```bash
MIKROBOT_GATEWAY_HOST=0.0.0.0 make run
```

Or set in `~/.gomikrobot/config.json`:

```json
{
  "gateway": {
    "host": "0.0.0.0"
  }
}
```

### Option C: Bind to specific LAN IP

```bash
MIKROBOT_GATEWAY_HOST=192.168.0.199 make run
```

## Security Note

Binding to `0.0.0.0` exposes the gateway to any device on the network. When doing this:
- Set `MIKROBOT_GATEWAY_AUTH_TOKEN` to require authentication
- Or use TLS (`tlsCert`/`tlsKey` in gateway config)
- Or restrict at the firewall level

The `127.0.0.1` default is intentional — it's the secure default per the project's design principles.

## Affected Config

| Setting | Default | Change needed |
|---------|---------|---------------|
| `gateway.host` | `127.0.0.1` | `0.0.0.0` or LAN IP |
| `gateway.authToken` | (empty) | Set a token when exposing |
| `gateway.tlsCert` / `tlsKey` | (empty) | Optional, enables HTTPS |
