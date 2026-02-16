# KafClaw Security Roadmap

> Date: 2026-02-16 | Status: Active | Owner: Core Team

This roadmap is informed by a competitive analysis of Gobii's production security architecture, OpenClaw's publicly documented threat model and CVE history, and a thorough audit of KafClaw's current codebase. It identifies concrete gaps, prioritizes them by blast radius and implementation cost, and lays out both quick wins and long-term structural investments.

---

## Table of Contents

1. [Context: Gobii vs. OpenClaw Lessons](#1-context)
2. [Current KafClaw Security Posture](#2-current-posture)
3. [Gap Analysis](#3-gap-analysis)
4. [Quick Wins (Ship Within Weeks)](#4-quick-wins)
5. [Medium-Term Hardening (1–3 Months)](#5-medium-term)
6. [Long-Term Structural Investments (3–12 Months)](#6-long-term)
7. [What NOT to Copy](#7-what-not-to-copy)
8. [Long-Term Implications](#8-long-term-implications)
9. [Appendix: Key File References](#9-appendix)

---

## 1. Context: Gobii vs. OpenClaw Lessons <a id="1-context"></a>

### What Gobii Gets Right

Gobii (first commit June 2025, MIT open-source August 2025) was built by former defense contractors and infrastructure engineers. Their security posture reflects that background:

| Capability | Gobii Implementation | Why It Matters |
|-----------|---------------------|---------------|
| **Credential encryption at rest** | AES-256-GCM authenticated encryption for all agent secrets. The LLM never sees raw credentials — only placeholder names. Actual values substituted at request time, domain-scoped. | Eliminates the #1 attack surface: plaintext token theft. |
| **Mandatory sandboxing** | gVisor-sandboxed Kubernetes pods per agent. Default runtime class is gVisor with seccomp profiles on pod specs. | A compromised agent cannot escape to the host. This is not optional. |
| **Egress-only network policies** | Kubernetes NetworkPolicy restricts sandbox pods to egress-only. No inbound connections to agent containers. Proxy-only outbound. | Prevents reverse shells, C2 callbacks, and lateral movement. |
| **Browser profile portability** | Browser profiles persisted as compressed archives to cloud storage, portable across stateless workers. | Decouples agent state from host. Enables horizontal scaling and disaster recovery. |
| **Proxy rotation with health scoring** | Dedicated IP inventory with health-scored rotation for agent browser traffic. | Prevents IP-based blocking and fingerprinting at scale. |
| **Endpoint-addressable agent identity** | Each agent has its own email and SMS identities. | Enables independent agent communication channels, audit trails per agent. |
| **Full audit trails** | Every action logged with traceable provenance. | Enterprise compliance (SOC 2, FedRAMP). |

### What OpenClaw's Failures Teach Us

OpenClaw (first commit November 2025) has experienced significant security incidents that validate several of Gobii's design choices:

| Issue | Impact | Lesson for KafClaw |
|-------|--------|-------------------|
| **CVE-2026-25253** (CVSS 8.8) | 1-click RCE via WebSocket hijacking. Stolen auth tokens grant full gateway control. 17,500+ exposed instances found across 52 countries. | Never trust origin headers blindly. Validate WebSocket origins. Auth tokens must not be exfiltrable via URL parameters. |
| **Plaintext token storage** | OpenClaw's own threat model flags this as high residual risk. Token theft rated high severity. | Encrypt credentials at rest. Period. |
| **Optional sandboxing** | Host execution is the default. CVE-2026-25253 exploit chain includes disabling sandbox to escape Docker. | Sandboxing must be default-on, not opt-in. Sandbox config should not be modifiable via API. |
| **Wildcard CORS** | `Access-Control-Allow-Origin: *` enables cross-origin attacks. | Restrict CORS to known origins. |
| **ClawHub supply chain** | 12% of skills (341/2,857) found to be malicious. | Vet third-party integrations. Never auto-execute untrusted code. |
| **"Lethal trifecta"** | Private data access + untrusted content exposure + external communication = exploitable by design. | Defense-in-depth: encrypt data, sandbox execution, restrict egress. |

---

## 2. Current KafClaw Security Posture <a id="2-current-posture"></a>

### Strengths (Already Implemented)

| Area | Implementation | Rating |
|------|---------------|--------|
| **Shell execution** | 38 deny-pattern regexes, strict allow-list (11 commands), path traversal blocking, workspace restriction, 60s timeout | Excellent |
| **Tool authorization** | 3-tier policy engine (read/write/high-risk), external senders locked to Tier 0 | Excellent |
| **Config file permissions** | Directory 0700, files 0600 | Good |
| **Default localhost binding** | `127.0.0.1:18790` / `127.0.0.1:18791` | Good |
| **Approval workflow** | Interactive approval gates for high-tier tools from internal senders | Good |
| **WhatsApp auth** | Default-deny allowlist, pending queue for unknowns, silent inbound | Good |
| **Attack intent detection** | German + English pattern scanning before LLM processing | Good |
| **Credential masking** | API status endpoint masks secrets (first 2 + last 2 chars) | Adequate |

### Gaps (Not Yet Implemented)

| Area | Current State | Risk Level |
|------|--------------|------------|
| **Credential encryption at rest** | Plain JSON in `~/.gomikrobot/config.json`, plaintext SQLite databases | **Critical** |
| **CORS policy** | Hardcoded `Access-Control-Allow-Origin: *` on all 10+ API endpoints | **Critical** |
| **Database encryption** | `timeline.db`, `whatsapp.db`, session JSONL files all unencrypted | **High** |
| **Process sandboxing** | No container isolation or seccomp profiles for tool execution | **High** |
| **Network egress controls** | No outbound filtering on LLM provider calls or tool network access | **High** |
| **TLS enforcement** | Optional, not enforced in any deployment mode | **High** |
| **Content-Security-Policy** | No CSP headers on web UI or dashboard | **Medium** |
| **WebSocket origin validation** | Not applicable yet (no WebSocket), but critical if added | **Medium** |
| **Browser profile management** | No headed browser execution yet | **Low** (future concern) |
| **Proxy rotation** | Not applicable yet | **Low** (future concern) |
| **Rate limiting** | No rate limiting on gateway endpoints | **Medium** |
| **Audit log completeness** | Timeline tracks events but no dedicated security audit log | **Medium** |

---

## 3. Gap Analysis <a id="3-gap-analysis"></a>

### Mapping Gobii Capabilities to KafClaw Gaps

```
Gobii Feature                    KafClaw Status         Priority    Effort
─────────────────────────────    ──────────────────     ────────    ──────
AES-256-GCM credential encrypt  Not implemented        CRITICAL    Medium
gVisor/seccomp sandboxing        Not implemented        HIGH        Large
Egress-only network policies     Not implemented        HIGH        Large
Browser profile portability      N/A (no browser yet)   LOW         Future
Proxy rotation + health score    N/A (no browser yet)   LOW         Future
Endpoint-addressable identity    Partial (AgentID)      MEDIUM      Medium
Full audit trail                 Partial (timeline)     MEDIUM      Small
Domain-scoped secret injection   Not implemented        HIGH        Medium
```

---

## 4. Quick Wins — Ship Within Weeks <a id="4-quick-wins"></a>

These items have high security impact, low implementation complexity, and no architectural changes required.

### 4.1 Fix CORS Policy (CRITICAL)

**Problem:** `Access-Control-Allow-Origin: *` on all endpoints in `gateway.go` allows any website to make authenticated API calls if a user is on the same network.

**Fix:** Replace wildcard with explicit origin allowlist.

**Files:** `gomikrobot/cmd/gomikrobot/cmd/gateway.go` (10 instances)

**Implementation:**
```go
// Add to config:
AllowedOrigins []string // default: ["http://127.0.0.1:18791", "http://localhost:18791"]

// Replace all instances of:
//   w.Header().Set("Access-Control-Allow-Origin", "*")
// With:
//   setCORSHeaders(w, r, cfg.Gateway.AllowedOrigins)

func setCORSHeaders(w http.ResponseWriter, r *http.Request, allowed []string) {
    origin := r.Header.Get("Origin")
    for _, a := range allowed {
        if origin == a {
            w.Header().Set("Access-Control-Allow-Origin", origin)
            w.Header().Set("Vary", "Origin")
            return
        }
    }
    // No match → no CORS header → browser blocks the request
}
```

### 4.2 Encrypt Credentials at Rest (CRITICAL)

**Problem:** API keys stored in plaintext JSON. SQLite databases contain WhatsApp session tokens, conversation history, and memory embeddings in the clear.

**Fix — Phase 1 (config file):** Encrypt sensitive fields in `config.json` using AES-256-GCM with a key derived from a user-supplied passphrase (or machine-specific key).

**Files:** `gomikrobot/internal/config/loader.go`, new `gomikrobot/internal/config/crypto.go`

**Implementation sketch:**
```go
// crypto.go
package config

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "golang.org/x/crypto/pbkdf2"
)

const (
    saltLen    = 16
    nonceLen   = 12
    iterations = 100_000
    keyLen     = 32 // AES-256
    prefix     = "enc:v1:"
)

func DeriveKey(passphrase string, salt []byte) []byte {
    return pbkdf2.Key([]byte(passphrase), salt, iterations, keyLen, sha256.New)
}

func Encrypt(plaintext, passphrase string) (string, error) {
    salt := make([]byte, saltLen)
    rand.Read(salt)
    key := DeriveKey(passphrase, salt)

    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    nonce := make([]byte, nonceLen)
    rand.Read(nonce)

    ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
    // Format: enc:v1:<base64(salt + nonce + ciphertext)>
    blob := append(salt, append(nonce, ciphertext...)...)
    return prefix + base64.StdEncoding.EncodeToString(blob), nil
}

func Decrypt(encoded, passphrase string) (string, error) {
    // Strip prefix, base64 decode, extract salt/nonce/ciphertext, decrypt
    // ...
}
```

**Key management options (choose one):**
- `MIKROBOT_MASTER_KEY` environment variable (simplest, recommended for servers)
- OS keychain integration via `go-keyring` (best for desktop/Electron)
- Prompt at startup (acceptable for CLI usage)

**Migration:** On first startup after upgrade, detect unencrypted fields, encrypt them in-place, and rewrite config. Existing plaintext values remain readable (graceful upgrade path).

### 4.3 Add Content-Security-Policy Headers (MEDIUM)

**Problem:** Web UI and dashboard serve HTML without CSP headers, enabling XSS if any injection point exists.

**Fix:** Add CSP headers to the dashboard HTTP handler.

```go
w.Header().Set("Content-Security-Policy",
    "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'")
```

**Files:** `gomikrobot/cmd/gomikrobot/cmd/gateway.go` (dashboard server section)

### 4.4 Add Rate Limiting to Gateway (MEDIUM)

**Problem:** No rate limiting. A malicious or misbehaving client can exhaust LLM tokens or DoS the agent.

**Fix:** Token-bucket rate limiter per IP on the `/chat` and dashboard API endpoints.

```go
// Use golang.org/x/time/rate
limiter := rate.NewLimiter(rate.Every(time.Second), 10) // 10 req/s burst
```

### 4.5 Enforce TLS for Non-Localhost Deployments (HIGH)

**Problem:** Headless mode binds `0.0.0.0` over plain HTTP. Auth tokens sent in cleartext over the network.

**Fix:** When `Host != "127.0.0.1"` and TLS is not configured, log a prominent warning and optionally refuse to start (configurable `RequireTLS` flag).

**Files:** `gomikrobot/cmd/gomikrobot/cmd/gateway.go`

---

## 5. Medium-Term Hardening — 1 to 3 Months <a id="5-medium-term"></a>

### 5.1 SQLite Database Encryption (HIGH)

**Problem:** `timeline.db` contains conversation history, memory embeddings, approval records, and policy decisions. `whatsapp.db` contains device key material. Both stored in plaintext.

**Options:**
1. **SQLCipher** — Drop-in encrypted SQLite. Requires replacing the pure-Go `modernc.org/sqlite` driver with a CGo-based SQLCipher wrapper.
2. **Application-layer encryption** — Encrypt sensitive columns (content, embeddings, tokens) before writing. Keep schema/queries unchanged.
3. **Filesystem-level encryption** — LUKS/FileVault/BitLocker. Zero code changes but depends on OS configuration.

**Recommendation:** Option 2 (application-layer) for portability. Encrypt `memory_chunks.content`, `observations.content`, `timeline.content`, `working_memory.content` columns. Use the same AES-256-GCM scheme from 4.2.

### 5.2 Domain-Scoped Secret Injection (HIGH)

**Problem:** Currently all API keys are loaded into the config struct at startup and remain in memory for the process lifetime. If the LLM is tricked into reflecting config values, keys leak.

**Fix (Gobii pattern):** Separate secrets from config. Store secrets in an encrypted secrets store, keyed by domain/service name. Inject secrets only at the point of use (HTTP call to LLM provider, API request) and zero them from memory after use.

**Implementation:**
- New `internal/secrets/` package with `SecretStore` interface
- Backends: encrypted file (default), environment variables, HashiCorp Vault (optional)
- Provider code (`internal/provider/`) fetches keys from SecretStore per-request instead of holding them in struct fields
- Secret placeholders in config: `"openaiApiKey": "$secret:openai_api_key"`

### 5.3 Docker-Based Tool Sandboxing (HIGH)

**Problem:** Shell execution runs on the host process. A shell escape (command obfuscation bypassing deny patterns) gives full host access.

**Fix:** Execute shell commands inside a disposable Docker container.

**Architecture:**
```
Agent Loop → exec tool → SandboxExecutor
                              │
                    ┌─────────┴──────────┐
                    │  Docker container   │
                    │  - Read-only rootfs │
                    │  - Mounted workdir  │
                    │  - No network       │
                    │  - 60s timeout      │
                    │  - Memory limit     │
                    │  - seccomp profile  │
                    └────────────────────┘
```

**Implementation in `internal/tools/shell.go`:**
```go
type SandboxMode string
const (
    SandboxNone   SandboxMode = "none"    // current behavior (deny-pattern only)
    SandboxDocker SandboxMode = "docker"  // Docker container
    SandboxGVisor SandboxMode = "gvisor"  // Docker + gVisor runtime
)
```

**Config addition:**
```json
{
  "tools": {
    "exec": {
      "sandboxMode": "docker",
      "sandboxImage": "kafclaw/sandbox:latest",
      "sandboxMemoryMB": 256,
      "sandboxNetworkEnabled": false
    }
  }
}
```

**Key decisions:**
- Default should be `docker` when Docker is available, fall back to `none` with a warning
- Work directory mounted read-write; everything else read-only
- No network access by default (blocks exfiltration)
- Keep the existing deny-pattern filtering as defense-in-depth inside the container

### 5.4 Security Audit Logging (MEDIUM)

**Problem:** Timeline tracks agent events but doesn't provide a focused security audit trail suitable for compliance.

**Fix:** Add a dedicated `security_audit` table:

```sql
CREATE TABLE security_audit (
    id INTEGER PRIMARY KEY,
    timestamp TEXT NOT NULL,
    event_type TEXT NOT NULL,  -- 'auth_attempt', 'tool_execution', 'policy_deny',
                               -- 'config_change', 'secret_access', 'sandbox_escape_attempt'
    severity TEXT NOT NULL,    -- 'info', 'warn', 'critical'
    actor TEXT,                -- sender ID, channel, IP
    resource TEXT,             -- tool name, file path, endpoint
    action TEXT,               -- 'allow', 'deny', 'approve', 'block'
    detail TEXT,               -- JSON blob with context
    trace_id TEXT
);
```

### 5.5 WebSocket Origin Validation (MEDIUM)

**Problem:** KafClaw doesn't use WebSockets yet, but the Electron remote mode and future real-time features will likely add them. CVE-2026-25253 showed this is a critical attack vector.

**Fix:** When adding WebSocket support, validate the `Origin` header against the same allowlist used for CORS. Reject connections from unknown origins.

---

## 6. Long-Term Structural Investments — 3 to 12 Months <a id="6-long-term"></a>

### 6.1 Kubernetes-Native Deployment with Network Policies

**Why:** Gobii's strongest differentiator is per-agent pod isolation with egress-only network policies. For multi-tenant or enterprise deployments, this is table stakes.

**Components:**
- Helm chart with per-agent pod templates
- gVisor RuntimeClass as default (with fallback to runc)
- Kubernetes NetworkPolicy: deny all ingress, allow egress only to LLM provider endpoints and configured webhook destinations
- Horizontal pod autoscaling based on queue depth

**Architecture:**
```
┌─────────────────────────────────────────────┐
│  Kubernetes Cluster                          │
│                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
│  │ Agent Pod │  │ Agent Pod │  │ Agent Pod │  │
│  │ (gVisor)  │  │ (gVisor)  │  │ (gVisor)  │  │
│  │ egress    │  │ egress    │  │ egress    │  │
│  │ only      │  │ only      │  │ only      │  │
│  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘  │
│        │              │              │          │
│  ┌─────▼──────────────▼──────────────▼─────┐   │
│  │          Gateway Service                 │   │
│  │  (API + Dashboard + Bus)                 │   │
│  └──────────────────────────────────────────┘   │
│                                              │
│  ┌──────────────┐  ┌──────────────┐          │
│  │ Kafka Cluster │  │ SQLite/PG    │          │
│  └──────────────┘  └──────────────┘          │
└─────────────────────────────────────────────┘
```

**Impact:** Enables enterprise sales, SOC 2 compliance, and multi-tenant SaaS offering.

### 6.2 Browser Automation Security Layer

**Why:** If KafClaw adds headed browser execution (a likely future direction given the market trajectory), Gobii's patterns should be adopted from the start.

**Components:**
- Browser profiles as compressed archives in object storage (S3/MinIO)
- Profile encryption at rest (same AES-256-GCM scheme)
- Proxy rotation with health scoring for browser egress
- Fingerprint randomization (user-agent, viewport, WebGL, canvas)
- Screenshot redaction for sensitive page content before logging

### 6.3 Per-Agent Identity Isolation

**Why:** Gobii agents are endpoint-addressable with their own email and SMS identities. KafClaw's identity currently lives in workspace files shared across the process.

**Evolution path:**
1. **Current:** Single identity per KafClaw instance (soul files in workspace)
2. **Next:** Per-agent identity config in orchestrator hierarchy (identity fields on `AgentNode`)
3. **Future:** Dedicated communication channels per agent (email via IMAP/SMTP, SMS via Twilio) with isolated credentials from the secrets store

### 6.4 Secrets Management Integration

**Why:** AES-256-GCM encrypted files are good for single-server deployments. Enterprise customers need centralized secrets management.

**Integration targets:**
- HashiCorp Vault (KV v2 secrets engine)
- AWS Secrets Manager / Parameter Store
- GCP Secret Manager
- Azure Key Vault
- 1Password Connect (for teams already using 1Password)

**Interface:**
```go
type SecretProvider interface {
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key, value string) error
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]string, error)
    RotationCallback(key string, fn func(newValue string)) // optional
}
```

### 6.5 Supply Chain Security

**Why:** OpenClaw's ClawHub had 12% malicious skills. If KafClaw ever adds a skill marketplace or third-party tool loading, this becomes critical.

**Preventive measures:**
- Code-sign all skill packages
- Static analysis scan before loading third-party tools
- Capability-based permissions (skill declares what it needs; user approves)
- Sandboxed skill execution (inherit Docker sandbox from 5.3)
- Dependency auditing for Go modules (`go vet`, `govulncheck`)

---

## 7. What NOT to Copy from Gobii <a id="7-what-not-to-copy"></a>

Not every Gobii pattern fits KafClaw's architecture and user base:

| Gobii Pattern | Why to Skip (For Now) |
|--------------|----------------------|
| **Mandatory Kubernetes** | KafClaw's strength is running on a single machine (desktop, Raspberry Pi, Jetson Nano). Don't make K8s a requirement. Offer it as an option for enterprise. |
| **Proxy IP inventory** | Overkill without browser automation. Add only when headed browsers ship. |
| **Per-agent email/SMS endpoints** | Expensive infrastructure. Add when the orchestrator has >5 production agents needing independent channels. |
| **Cloud-managed SaaS billing** | KafClaw is a personal assistant first. Don't add billing complexity prematurely. |

---

## 8. Long-Term Implications <a id="8-long-term-implications"></a>

### 8.1 Market Positioning

The agent platform market is bifurcating into two camps:
- **Security-first platforms** (Gobii): Enterprise, defense, regulated industries. Encryption, sandboxing, audit trails as defaults.
- **Developer-experience-first platforms** (OpenClaw pre-hardening): Fast setup, local execution, broad channel support. Security as optional hardening.

KafClaw currently sits closer to the DX-first camp. The items in this roadmap move us toward security-first **without sacrificing** the single-machine simplicity that is our differentiator. The key insight from Gobii is that encryption and sandboxing don't have to mean Kubernetes complexity — they can be implemented at the application layer.

### 8.2 Regulatory Trajectory

- **EU AI Act** (effective August 2026): High-risk AI systems require risk management, data governance, and human oversight. KafClaw's approval workflow and policy engine are good foundations, but encrypted audit logs and sandboxed execution will likely become requirements.
- **SOC 2 Type II**: Enterprise customers will ask for this. Encryption at rest, access controls, audit logging, and change management are the four pillars. Items 4.2, 5.1, 5.4, and 6.1 directly address these.
- **GDPR/CCPA**: Conversation history and memory embeddings may contain PII. Encryption at rest (5.1) and data lifecycle management (already in LifecycleManager) are the key controls.

### 8.3 Attack Surface Evolution

As KafClaw gains capabilities (browser execution, more channels, multi-agent orchestration), the attack surface grows multiplicatively:

```
Today:       Shell + Filesystem + LLM API + WhatsApp
Near-term:   + Web UI WebSocket + Docker sandbox + Secrets store
Long-term:   + Browser automation + Proxy infrastructure + Multi-tenant K8s
```

Each new capability needs to be added **behind** the security layers established by this roadmap. The order matters: encrypt first, sandbox second, then add capabilities.

### 8.4 Competitive Timeline

Based on commit timestamps and release cadence:
- Gobii shipped their security fundamentals (encryption, sandboxing) before opening the MIT repo. Security was day-one architecture, not a retrofit.
- OpenClaw is now retrofitting security after CVE-2026-25253 forced their hand. Their 2026.2.12 release fixes 40+ vulnerabilities.
- **KafClaw's window:** We can implement the critical items (4.1, 4.2) and claim parity with Gobii's credential security within weeks. Full sandbox parity (5.3) is achievable in a few months. This puts us ahead of OpenClaw's retrofit timeline and demonstrates to users that we take security seriously from the start.

---

## 9. Appendix: Key File References <a id="9-appendix"></a>

### Security-Critical Source Files

| File | What It Controls |
|------|-----------------|
| `gomikrobot/cmd/gomikrobot/cmd/gateway.go` | All API endpoints, CORS headers, auth middleware, TLS config |
| `gomikrobot/internal/tools/shell.go` | Shell execution sandbox (deny/allow patterns, path traversal, timeout) |
| `gomikrobot/internal/tools/filesystem.go` | Path traversal protection, workspace boundary enforcement |
| `gomikrobot/internal/policy/engine.go` | 3-tier tool authorization, sender classification |
| `gomikrobot/internal/config/loader.go` | Config file loading, file permissions (0600/0700) |
| `gomikrobot/internal/config/config.go` | All config fields, default values, gateway binding address |
| `gomikrobot/internal/approval/manager.go` | Interactive approval workflow for high-tier tools |
| `gomikrobot/internal/session/session.go` | JSONL session persistence (currently plaintext) |
| `gomikrobot/internal/timeline/service.go` | SQLite database connection, schema, WAL mode |
| `gomikrobot/internal/channels/whatsapp.go` | WhatsApp auth, allowlist/denylist, JID normalization |
| `gomikrobot/internal/agent/loop.go` | Agent loop, tool registration, message classification |
| `gomikrobot/internal/agent/context.go` | Context builder, soul file loading, identity envelope |
| `gomikrobot/internal/memory/service.go` | Memory storage (embeddings, content — currently plaintext) |

### Existing Security Documentation

| Document | Location |
|---------|----------|
| Security risks assessment | `docs/v2/security-risks.md` |
| Architecture (security model section) | `gomikrobot/ARCHITECTURE.md` §9 |
| Admin guide (gateway auth) | `docs/v2/admin-guide.md` |
| WhatsApp authorization model | `docs/v2/whatsapp-setup.md` |
| **This roadmap** | `docs/security/security-roadmap.md` |

### Research Sources

- [Gobii vs. OpenClaw comparison](https://gobii.ai/blog/gobii-vs-openclaw/)
- [Gobii secrets management](https://gobii.ai/docs/guides/secrets/)
- [Gobii open source platform announcement](https://gobii.ai/blog/oss-agent-platform/)
- [OpenClaw security documentation](https://docs.openclaw.ai/gateway/security)
- [CVE-2026-25253 analysis (SOCRadar)](https://socradar.io/blog/cve-2026-25253-rce-openclaw-auth-token/)
- [OpenClaw security hardening guide](https://aimaker.substack.com/p/openclaw-security-hardening-guide)
- [OpenClaw tool security (DeepWiki)](https://deepwiki.com/openclaw/openclaw/6.2-tool-security-and-sandboxing)
