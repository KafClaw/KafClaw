# OWASP Security Audit - KafClaw

**Date:** 2026-02-16
**Scope:** Go application (`gomikrobot/`), Web UI (`gomikrobot/web/`), Gateway HTTP API
**Standard:** OWASP Top 10 (2021), OWASP Non-Human Identity Top 10 (2025)
**Version audited:** v2.6.3

---

## Executive Summary

This audit identifies **6 Critical**, **9 High**, **12 Medium**, and **8 Low** severity findings across the KafClaw Go application, web dashboard, and HTTP gateway. The most pressing issues are unrestricted CORS, unencrypted data at rest, missing HTTP security headers, and prompt injection via soul files.

---

## OWASP Top 10 Mapping

### A01:2021 - Broken Access Control

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A01-1 | **CRITICAL** | Wildcard CORS (`Access-Control-Allow-Origin: *`) on all API endpoints | `cmd/gomikrobot/cmd/gateway.go:610,638,651+` |
| A01-2 | **HIGH** | Authentication is optional - gateway works without `AuthToken` configured | `cmd/gomikrobot/cmd/gateway.go:3077-3091` |
| A01-3 | **HIGH** | Status endpoint bypasses auth middleware, leaks operational mode | `cmd/gomikrobot/cmd/gateway.go:3080-3082` |
| A01-4 | **MEDIUM** | Policy engine `AllowedSenders` empty by default (all senders allowed) | `internal/policy/engine.go:52-56` |
| A01-5 | **MEDIUM** | Gateway sets `MaxAutoTier = 2`, allowing automatic shell execution | `cmd/gomikrobot/cmd/gateway.go:130` |

**Detail: A01-1 - Wildcard CORS**
```go
// gateway.go - repeated across 120+ endpoints
w.Header().Set("Access-Control-Allow-Origin", "*")
w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
```
**Impact:** Any website can make authenticated API requests if the user's bearer token is exposed. Combined with no CSRF protection, this enables cross-site request forgery against all state-changing endpoints (approvals, settings, git operations).

**Remediation:**
- Replace `*` with specific allowed origins (e.g., `http://127.0.0.1:18791`)
- Implement CSRF token validation on all POST/PUT/DELETE endpoints
- Consider `SameSite=Strict` cookies as an alternative to Bearer tokens for browser-based access

**Detail: A01-2 - Optional Authentication**
```go
// gateway.go:3077-3091
if cfg.Gateway.AuthToken != "" {
    // Auth middleware applied only if AuthToken is configured
    handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/api/v1/status" || r.Method == "OPTIONS" {
            mux.ServeHTTP(w, r)
            return
        }
        token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
        if token != authToken {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        mux.ServeHTTP(w, r)
    })
}
```
**Impact:** If the user does not configure an `AuthToken`, all 100+ API endpoints are publicly accessible on the configured port. The `/chat` endpoint on port 18790 has no auth at all.

**Remediation:**
- Generate a random auth token on first run if none is configured
- Require auth by default; allow explicit opt-out only for `127.0.0.1` binding
- Add authentication to the `/chat` API server (port 18790)

---

### A02:2021 - Cryptographic Failures

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A02-1 | **CRITICAL** | Session files stored as unencrypted plaintext JSONL | `internal/session/session.go:152-186` |
| A02-2 | **CRITICAL** | Session directory created with `0755` permissions (world-readable) | `internal/session/session.go:121-130` |
| A02-3 | **HIGH** | Timeline database (SQLite) unencrypted at `~/.gomikrobot/timeline.db` | `internal/timeline/` |
| A02-4 | **HIGH** | WhatsApp session database unencrypted at `~/.gomikrobot/whatsapp.db` | `internal/channels/whatsapp.go:68-75` |
| A02-5 | **HIGH** | API keys stored in plaintext JSON at `~/.gomikrobot/config.json` | `internal/config/loader.go:194` |
| A02-6 | **MEDIUM** | File creation uses `0644` permissions (world-readable) | `internal/tools/filesystem.go` |
| A02-7 | **MEDIUM** | No HSTS header when TLS is enabled | `cmd/gomikrobot/cmd/gateway.go:3096-3110` |

**Detail: A02-1/A02-2 - Unencrypted Session Storage**
```go
// session.go:121-130
func NewManager(workspace string) *Manager {
    home, _ := os.UserHomeDir()
    sessionsDir := filepath.Join(home, ".gomikrobot", "sessions")
    os.MkdirAll(sessionsDir, 0755) // World-readable!
    return &Manager{sessionsDir: sessionsDir, cache: make(map[string]*Session)}
}
```
**Impact:** Conversation history contains all user messages and LLM responses, which may include sensitive business data, personal information, or API keys discussed in conversation. Any local user can read these files.

**Remediation:**
- Change directory permissions to `0700` (user-only)
- Change file permissions to `0600` (user-only)
- Implement at-rest encryption using `crypto/aes` with a key derived from the user's config
- Consider using OS keychain for the encryption key (see Bot Identity proposal)

**Detail: A02-5 - Plaintext API Keys**
```go
// config.go:107-111
type ProviderConfig struct {
    APIKey  string `json:"apiKey" envconfig:"API_KEY"`
    APIBase string `json:"apiBase,omitempty" envconfig:"API_BASE"`
}
```
Config file at `~/.gomikrobot/config.json` stores API keys for: Anthropic, OpenAI, OpenRouter, DeepSeek, Groq, Gemini, VLLM, Telegram, Discord, Feishu, Brave Search, ER1, and the Gateway auth token itself.

**Remediation:**
- Use OS keychain via `go-keyring` for API key storage
- Keep config.json for non-sensitive settings only
- Support environment-variable-only mode for CI/CD deployments
- File permissions already `0600` (good), but insufficient for shared systems

---

### A03:2021 - Injection

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A03-1 | **CRITICAL** | System prompt injection via soul files (no sanitization) | `internal/agent/context.go:37-59` |
| A03-2 | **HIGH** | Shell tool regex bypass via metacharacters (`$()`, backticks) | `internal/tools/shell.go:15-53` |
| A03-3 | **MEDIUM** | Git branch name injection (minimal validation) | `cmd/gomikrobot/cmd/gateway.go:2447-2458` |
| A03-4 | **LOW** | SQL injection not found (parameterized queries used throughout) | `internal/timeline/` |

**Detail: A03-1 - Prompt Injection via Soul Files**
```go
// context.go:37-59
func (b *ContextBuilder) BuildSystemPrompt() string {
    var parts []string
    parts = append(parts, b.getIdentity())
    if bootstrap := b.loadBootstrapFiles(); bootstrap != "" {
        parts = append(parts, bootstrap)
    }
    // ...files from workspace included raw
}
```
Soul files (`IDENTITY.md`, `SOUL.md`, `AGENTS.md`, `TOOLS.md`, `USER.md`) are loaded from the workspace directory and included verbatim in the LLM system prompt. An attacker who modifies these files can inject arbitrary instructions.

**Remediation:**
- Validate soul file structure (expected markdown format, max length)
- Implement content integrity checks (hash verification)
- Log when soul files change between runs
- Consider signing soul files with a known key

**Detail: A03-2 - Shell Tool Bypass**
```go
// shell.go:15-38 - deny patterns
var defaultDenyPatterns = []string{
    `\brm\b`, `\bunlink\b`, `\brmdir\b`, ...
    `\bdd\b`, `\bmkfs\b`, `\bchmod\s+777\b`, ...
}
```
The deny-pattern approach uses regex word boundaries (`\b`) to block dangerous commands. However:
- Command substitution `$(dangerous_command)` may not be caught
- Backtick execution `` `dangerous_command` `` may not be caught
- Shell builtins and aliases can bypass word boundaries
- Pipe chains with encoded/obfuscated commands

**Remediation:**
- Parse the command into an AST instead of regex matching
- Use a command whitelist approach (already available via `StrictAllowList`) and make it mandatory
- Run shell commands in a restricted sandbox (seccomp, namespaces)
- Consider removing shell access for external/untrusted channels

---

### A04:2021 - Insecure Design

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A04-1 | **HIGH** | No rate limiting on any API endpoint | All HTTP endpoints in gateway.go |
| A04-2 | **MEDIUM** | No request body size limits | All POST endpoints in gateway.go |
| A04-3 | **MEDIUM** | Media files saved with predictable paths | `internal/channels/whatsapp.go:246-342` |
| A04-4 | **LOW** | Error messages may reveal system paths | Throughout gateway.go |

**Remediation:**
- Implement rate limiting middleware (e.g., `golang.org/x/time/rate`)
- Set `http.MaxBytesReader` on all request bodies (e.g., 10MB limit)
- Use randomized filenames for media storage
- Sanitize error messages before returning to clients

---

### A05:2021 - Security Misconfiguration

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A05-1 | **HIGH** | Missing HTTP security headers (CSP, X-Frame-Options, X-Content-Type-Options) | All HTTP responses in gateway.go |
| A05-2 | **HIGH** | External CDN scripts without Subresource Integrity (SRI) | `web/index.html:8`, `web/approvals.html:9-10`, `web/timeline.html:9-12` |
| A05-3 | **MEDIUM** | Docker container runs as root (no USER directive) | `gomikrobot/Dockerfile` |
| A05-4 | **MEDIUM** | Docker ports exposed on all interfaces | `gomikrobot/docker-compose.yml` |
| A05-5 | **LOW** | No health check in Docker configuration | `gomikrobot/Dockerfile` |

**Detail: A05-1 - Missing Security Headers**
No endpoints set the following headers:
- `Content-Security-Policy`
- `X-Frame-Options`
- `X-Content-Type-Options`
- `Strict-Transport-Security`
- `X-XSS-Protection`
- `Referrer-Policy`
- `Permissions-Policy`

**Remediation - Add security headers middleware:**
```go
func securityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-XSS-Protection", "0") // Disable legacy; use CSP instead
        w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
        w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
        if r.TLS != nil {
            w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
        }
        next.ServeHTTP(w, r)
    })
}
```

**Detail: A05-2 - Missing SRI**
```html
<!-- web/index.html:8 - no integrity attribute -->
<script src="https://cdn.tailwindcss.com"></script>

<!-- web/timeline.html:9-12 - no integrity attributes -->
<script src="https://cdn.tailwindcss.com"></script>
<script src="https://unpkg.com/vue@3/dist/vue.global.prod.js"></script>
<script src="https://unpkg.com/d3@7"></script>
<script src="https://unpkg.com/dagre-d3@0.6.4/dist/dagre-d3.js"></script>
```
If any CDN is compromised, arbitrary JavaScript executes in the context of the dashboard.

**Remediation:**
```html
<script src="https://cdn.tailwindcss.com"
  integrity="sha384-HASH_HERE" crossorigin="anonymous"></script>
```
Or better: bundle dependencies locally and serve from the Go binary.

---

### A06:2021 - Vulnerable and Outdated Components

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A06-1 | **MEDIUM** | Dependencies should be audited for known CVEs | `gomikrobot/go.mod` |
| A06-2 | **LOW** | No automated dependency scanning in CI/CD | `.github/workflows/` |

**Remediation:**
- Run `govulncheck ./...` regularly
- Add `govulncheck` to CI pipeline
- Pin dependency versions and review updates

---

### A07:2021 - Identification and Authentication Failures

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A07-1 | **CRITICAL** | No CSRF protection on state-changing endpoints | All POST endpoints in gateway.go |
| A07-2 | **HIGH** | Single static bearer token (no per-user or per-session tokens) | `cmd/gomikrobot/cmd/gateway.go:3077-3091` |
| A07-3 | **MEDIUM** | WhatsApp pairing token stored in timeline settings | `internal/channels/whatsapp.go:556-558` |
| A07-4 | **MEDIUM** | No token expiration or rotation mechanism | Gateway auth system |

**Detail: A07-1 - No CSRF Protection**
State-changing endpoints vulnerable to CSRF:
- `POST /api/v1/approvals/{id}` - Approve/deny tool executions
- `POST /api/v1/settings` - Change configuration
- `POST /api/v1/repo/checkout` - Switch git branches
- `POST /api/v1/group/*` - Group management
- `POST /chat` - Execute agent commands

**Remediation:**
- Implement CSRF tokens (double-submit cookie pattern or synchronizer token)
- Restrict CORS to specific origins
- Add `SameSite=Strict` attribute to any session cookies

---

### A08:2021 - Software and Data Integrity Failures

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A08-1 | **HIGH** | External CDN scripts loaded without integrity verification | `web/*.html` |
| A08-2 | **MEDIUM** | No signature verification on auto-updates | N/A (no auto-update currently) |
| A08-3 | **MEDIUM** | Soul files not integrity-checked before inclusion in prompt | `internal/agent/context.go` |

---

### A09:2021 - Security Logging and Monitoring Failures

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A09-1 | **MEDIUM** | No security-specific audit logging | Gateway HTTP handlers |
| A09-2 | **MEDIUM** | Failed auth attempts not logged separately | `cmd/gomikrobot/cmd/gateway.go:3088` |
| A09-3 | **LOW** | Timeline logs tool executions but not HTTP access patterns | `internal/timeline/` |

**Remediation:**
- Log all authentication attempts (success and failure) with source IP
- Log all state-changing operations with actor identity
- Implement alerting on repeated auth failures
- Consider structured logging (JSON) for SIEM integration

---

### A10:2021 - Server-Side Request Forgery (SSRF)

| ID | Severity | Finding | Location |
|----|----------|---------|----------|
| A10-1 | **LOW** | No SSRF vectors identified in current codebase | N/A |

The application does not accept user-provided URLs for server-side fetching (the LLM provider URLs are admin-configured). This category is not a current risk.

---

## OWASP NHI Top 10 (2025) Mapping

| NHI Rank | Risk | Current State | Severity |
|----------|------|---------------|----------|
| NHI1 - Improper Offboarding | No mechanism to revoke bot instances | **HIGH** |
| NHI2 - Secret Leakage | API keys in plaintext config file | **CRITICAL** |
| NHI3 - Vulnerable Third-Party NHI | OAuth tokens to LLM providers not scoped | **MEDIUM** |
| NHI4 - Insecure Authentication | Static bearer token, no JWT/mTLS | **HIGH** |
| NHI5 - Overprivileged NHI | Bot has full access to all configured APIs | **HIGH** |
| NHI7 - Long-Lived Secrets | API keys never expire/rotate | **HIGH** |
| NHI8 - Environment Isolation | No separation between dev/prod bot identities | **MEDIUM** |
| NHI9 - NHI Reuse | All bot instances share same identity | **HIGH** |
| NHI10 - Human Use of NHI | Same API keys used by bot and human | **MEDIUM** |

---

## Positive Security Findings

These areas demonstrate good security practices:

1. **SQL injection prevention** - All database queries use parameterized statements (`?` placeholders)
2. **Path traversal protection** - `isWithin()` uses `filepath.Rel()` with `..` prefix checking
3. **Config file permissions** - `~/.gomikrobot/config.json` created with `0600` (user-only)
4. **Config directory permissions** - `~/.gomikrobot/` created with `0700` (user-only)
5. **Shell command timeout** - Default 60-second timeout on all shell executions
6. **Tiered tool system** - Read-only, write, and high-risk tiers with policy enforcement
7. **External message restrictions** - `ExternalMaxTier = 0` limits external users to read-only tools
8. **WhatsApp allowlist/denylist** - Configurable sender filtering
9. **Default localhost binding** - Gateway binds to `127.0.0.1` by default
10. **No `eval()` or dynamic code** in web frontend JavaScript
11. **Vue.js template escaping** - Proper `{{ }}` interpolation prevents XSS in dynamic content
12. **URL encoding** - API calls use `encodeURIComponent()` for parameters

---

## Remediation Priority Matrix

| Priority | Items | Effort |
|----------|-------|--------|
| **P0 - Immediate** | A01-1 (CORS), A02-2 (session perms), A05-1 (security headers), A07-1 (CSRF) | Low |
| **P1 - This Sprint** | A02-1 (encrypt sessions), A02-5 (keychain), A03-1 (soul file validation), A01-2 (require auth) | Medium |
| **P2 - Next Release** | A03-2 (shell sandbox), A04-1 (rate limiting), A05-2 (SRI), A05-3 (Docker user) | Medium |
| **P3 - Roadmap** | NHI items (per-bot identity, token rotation, offboarding), A06-1 (vuln scanning) | High |

---

## Appendix: Files Audited

| File | Lines | Security Relevance |
|------|-------|-------------------|
| `internal/tools/shell.go` | 272 | Command execution, deny/allow patterns |
| `internal/tools/filesystem.go` | 357 | File I/O, path boundary enforcement |
| `internal/tools/tool.go` | 147 | Tool interface, tier system |
| `internal/config/config.go` | 293 | Secret fields, provider keys |
| `internal/config/loader.go` | 201 | Config loading, file permissions |
| `internal/provider/openai.go` | 380 | API key usage, HTTP requests |
| `internal/channels/whatsapp.go` | 670 | Auth, media handling, DB storage |
| `internal/agent/context.go` | 300+ | System prompt construction |
| `internal/agent/loop.go` | 150+ | Tool registration |
| `internal/session/session.go` | 315 | Conversation persistence |
| `internal/policy/engine.go` | 103 | Access control decisions |
| `internal/approval/manager.go` | 86+ | Approval workflow, argument storage |
| `cmd/gomikrobot/cmd/gateway.go` | 3399 | HTTP API, auth, CORS, endpoints |
| `web/index.html` | 380+ | Dashboard, inline JS, CDN deps |
| `web/timeline.html` | 3200+ | Event viewer, localStorage, CDN deps |
| `web/approvals.html` | 200+ | Approval UI, CDN deps |
| `web/group.html` | 1700+ | Group dashboard, CDN deps |
| `Dockerfile` | 15 | Container image |
| `docker-compose.yml` | 20 | Container orchestration |
| `go.mod` | 50+ | Dependencies |
