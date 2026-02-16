# Electron App Security Audit - KafClaw

**Date:** 2026-02-16
**Scope:** Electron desktop application (`gomikrobot/electron/`)
**Standard:** Electron Security Checklist, OWASP Desktop App Guidelines
**Assessment:** MEDIUM risk (strong fundamentals, one critical TLS issue)

---

## Executive Summary

The KafClaw Electron application demonstrates **strong security fundamentals** with proper context isolation, sandboxing, and safe IPC design. One critical vulnerability (`rejectUnauthorized: false`) undermines TLS security for remote connections. Overall architecture follows Electron security best practices.

---

## Architecture Overview

```
Electron Main Process
  |-- BrowserWindow (sandbox: true, contextIsolation: true, nodeIntegration: false)
  |-- Preload Script (contextBridge: whitelist-only API)
  |-- Sidecar Manager (Go binary spawned as child process)
  |-- Remote Client (HTTP/HTTPS to remote agents)
  |-- Mode Resolver (config: ~/.gomikrobot/electron.json)
```

---

## Findings

### CRITICAL

#### E-01: TLS Certificate Validation Disabled
**File:** `electron/src/main/remote-client.ts:65`
**Severity:** CRITICAL

```typescript
const req = mod.get(url.toString(), {
    headers: {
        'Authorization': `Bearer ${conn.token}`,
        'Accept': 'application/json',
    },
    timeout: 10000,
    rejectUnauthorized: false,  // VULNERABILITY
}, ...)
```

**Impact:**
- Disables SSL/TLS certificate validation for all remote agent connections
- Makes the application vulnerable to Man-in-the-Middle (MITM) attacks
- An attacker on the network can intercept, read, and modify all remote agent communication
- Bearer authentication tokens can be stolen in transit

**Remediation:**
```typescript
// Remove rejectUnauthorized: false (defaults to true in Node 17+)
const req = mod.get(url.toString(), {
    headers: {
        'Authorization': `Bearer ${conn.token}`,
        'Accept': 'application/json',
    },
    timeout: 10000,
    // rejectUnauthorized defaults to true
}, ...)
```

For self-signed certificates, use certificate pinning:
```typescript
import { readFileSync } from 'fs';
const ca = readFileSync('/path/to/server-ca.pem');
const req = https.get(url.toString(), {
    ca: ca,
    rejectUnauthorized: true,
}, ...)
```

---

### MEDIUM

#### E-02: Remote Connection Tokens in Plaintext Config
**File:** `electron/src/main/mode-resolver.ts`
**Severity:** MEDIUM

```typescript
const CONFIG_FILE = path.join(CONFIG_DIR, 'electron.json');
// Contains: { remoteConnections: [{ id, name, url, token }] }
```

Bearer tokens for remote agent connections are stored in plaintext JSON on disk.

**Remediation:**
Use Electron's `safeStorage` API for credential encryption:
```typescript
import { safeStorage } from 'electron';

// Encrypt before saving
const encrypted = safeStorage.encryptString(token);
const base64 = encrypted.toString('base64');

// Decrypt when needed
const decrypted = safeStorage.decryptString(Buffer.from(base64, 'base64'));
```

#### E-03: Remote URL Not Validated
**File:** `electron/src/renderer/views/RemoteConnect.vue:78`
**Severity:** MEDIUM

Users can connect to any arbitrary URL without scheme validation. Non-HTTPS connections send Bearer tokens in cleartext.

**Remediation:**
```typescript
function validateRemoteUrl(url: string): boolean {
    try {
        const parsed = new URL(url);
        if (parsed.protocol !== 'https:' && parsed.hostname !== '127.0.0.1' && parsed.hostname !== 'localhost') {
            return false; // Require HTTPS for non-local connections
        }
        return true;
    } catch {
        return false;
    }
}
```

#### E-04: Sidecar Inherits All Environment Variables
**File:** `electron/src/main/sidecar.ts:77`
**Severity:** MEDIUM

```typescript
const env: Record<string, string> = { ...process.env as Record<string, string> };
```

The Go sidecar process inherits the full Electron process environment, which may contain sensitive variables from the parent shell.

**Remediation:**
```typescript
const env: Record<string, string> = {
    'PATH': process.env.PATH!,
    'HOME': process.env.HOME!,
    'MIKROBOT_GROUP_ENABLED': mode === 'full' ? 'true' : 'false',
    // Only pass explicitly needed variables
};
```

---

### LOW

#### E-05: CSP Allows Inline Styles
**File:** `electron/src/renderer/index.html:6`
**Severity:** LOW

```html
<meta http-equiv="Content-Security-Policy"
  content="default-src 'self'; script-src 'self';
  style-src 'self' 'unsafe-inline' https://fonts.googleapis.com;
  font-src 'self' https://fonts.gstatic.com;
  connect-src 'self' http://127.0.0.1:* https://*:*" />
```

`'unsafe-inline'` for styles is necessary for Vue's computed styles but reduces XSS protection.

**Remediation:** Consider nonce-based CSP if feasible with Vite build:
```html
style-src 'self' 'nonce-{random}' https://fonts.googleapis.com
```

#### E-06: Sidecar Logs May Expose Sensitive Data
**File:** `electron/src/main/sidecar.ts:96-98`
**Severity:** LOW

```typescript
this.proc.stdout?.on('data', (data: Buffer) => {
    const line = data.toString();
    this.logs.push(line); // Exposed in settings view
})
```

Go binary stdout/stderr is captured and displayed in the settings view. Logs may contain tokens or sensitive data.

**Remediation:**
```typescript
const sanitized = line.replace(/(?:token|key|secret|password)=\S+/gi, '$1=***');
this.logs.push(sanitized);
```

#### E-07: DevTools Available in Production
**File:** `electron/src/main/index.ts`
**Severity:** LOW (INFO)

DevTools are accessible via menu. Acceptable for a personal tool; consider disabling for distributed builds.

#### E-08: No Auto-Update Signature Verification
**Severity:** LOW (INFO)

No auto-update mechanism exists currently. If added in the future, use signed updates via `electron-updater` with code signing certificates.

---

## Positive Security Findings

The Electron app demonstrates strong security practices:

| Control | Implementation | Status |
|---------|---------------|--------|
| **Context Isolation** | `contextIsolation: true` | SECURE |
| **Node Integration** | `nodeIntegration: false` | SECURE |
| **Sandbox** | `sandbox: true` | SECURE |
| **Preload Script** | `contextBridge.exposeInMainWorld()` with whitelist API | SECURE |
| **Window Open Handler** | Blocks `window.open()`, redirects to system browser | SECURE |
| **No `remote` Module** | Not used (deprecated since Electron 13) | SECURE |
| **No `eval()`** | No dynamic code execution in renderer | SECURE |
| **No `innerHTML`** | Vue templates use safe interpolation | SECURE |
| **CSP** | Strict policy (except inline styles) | GOOD |
| **Spawn vs Exec** | Uses `spawn()` with array arguments (no shell injection) | SECURE |
| **Minimal Dependencies** | Only Vue 3, Router, Pinia in production | SECURE |
| **TypeScript** | `strict: true` prevents unsafe type coercion | GOOD |

---

## IPC API Surface Analysis

The exposed IPC API is well-scoped and safe:

```typescript
window.electronAPI = {
    sidecar: {
        getStatus(): Promise<string>,        // Read-only
        getLogs(): Promise<string[]>,         // Read-only
        start(mode): Promise<string>,         // Controlled action
        stop(): Promise<string>,              // Controlled action
        onStatusChanged(cb): void,            // Event subscription
    },
    mode: {
        get(): Promise<string>,              // Read-only
        set(mode): Promise<string>,          // Safe enum value
        activate(mode): Promise<{ok,error}>, // Controlled action
        reset(): Promise<{ok,error}>,        // Controlled action
    },
    config: {
        get(): Promise<any>,                 // Read-only
        save(partial): Promise<any>,         // Merge config
    },
    remote: {
        list(): Promise<Connection[]>,        // Read-only
        add(conn): Promise<Connection[]>,     // Add connection
        remove(id): Promise<Connection[]>,    // Remove connection
        verify(conn): Promise<{ok,error}>,    // Test connection
        setActive(id): Promise<boolean>,      // Select connection
        getActive(): Promise<Connection>,     // Read-only
        connect(id): Promise<{ok,error}>,     // Connect action
    }
}
```

**Assessment:** No dangerous capabilities exposed. No filesystem access, no process execution, no `require()` available from renderer context.

---

## Remediation Priority

| Priority | Finding | Effort |
|----------|---------|--------|
| **P0** | E-01: Enable TLS certificate validation | Trivial (1-line fix) |
| **P1** | E-02: Encrypt stored tokens with `safeStorage` | Low |
| **P1** | E-03: Validate remote URLs (require HTTPS) | Low |
| **P2** | E-04: Restrict sidecar environment variables | Low |
| **P3** | E-05-E-08: Low-severity improvements | Low-Medium |

---

## Files Audited

| File | Purpose | Risk Level |
|------|---------|------------|
| `electron/src/main/index.ts` | Window creation, app lifecycle | LOW |
| `electron/src/main/ipc-handlers.ts` | IPC message routing | LOW |
| `electron/src/main/sidecar.ts` | Go binary process management | MEDIUM |
| `electron/src/main/remote-client.ts` | Remote HTTPS client | **HIGH** |
| `electron/src/main/mode-resolver.ts` | Config persistence | MEDIUM |
| `electron/src/preload/index.ts` | Context bridge API | LOW |
| `electron/src/renderer/index.html` | CSP, HTML structure | LOW |
| `electron/src/renderer/main.ts` | Vue app entry | LOW |
| `electron/src/renderer/stores/mode.ts` | State management | LOW |
| `electron/src/renderer/stores/remote.ts` | Remote state | LOW |
| `electron/src/renderer/composables/useApi.ts` | API client | LOW |
| `electron/src/renderer/views/RemoteConnect.vue` | Remote UI | MEDIUM |
| `electron/src/renderer/views/Settings.vue` | Settings panel | LOW |
| `electron/package.json` | Dependencies | LOW |
| `electron/electron-builder.yml` | Build config | LOW |
