# KafClaw Security Documentation

**Created:** 2026-02-16
**Version:** v2.6.3

---

## Documents

| Document | Description |
|----------|-------------|
| [OWASP-AUDIT.md](OWASP-AUDIT.md) | Full OWASP Top 10 + NHI Top 10 security audit of the Go application, web UI, and HTTP gateway |
| [ELECTRON-AUDIT.md](ELECTRON-AUDIT.md) | Security audit of the Electron desktop application |
| [BOT-IDENTITY-PROPOSAL.md](BOT-IDENTITY-PROPOSAL.md) | Per-bot identity architecture proposal with GitHub App integration |
| [AUTH-TOKEN-CONCEPT.md](AUTH-TOKEN-CONCEPT.md) | JWT-based authentication and authorization system design |

---

## Findings Summary

### By Severity

| Severity | Go App | Electron | Total |
|----------|--------|----------|-------|
| CRITICAL | 6 | 1 | 7 |
| HIGH | 9 | 0 | 9 |
| MEDIUM | 12 | 3 | 15 |
| LOW | 8 | 4 | 12 |
| **Total** | **35** | **8** | **43** |

### Top 5 Critical Issues

1. **Wildcard CORS** (`Access-Control-Allow-Origin: *`) on all 120+ API endpoints
2. **Unencrypted session files** with world-readable permissions (0755)
3. **No CSRF protection** on state-changing endpoints (approvals, settings, git ops)
4. **Prompt injection** via unsanitized soul files in LLM system prompt
5. **API keys in plaintext** config file on disk

### Top 5 Positive Findings

1. SQL injection prevented everywhere (parameterized queries)
2. Electron app uses context isolation, sandbox, and safe IPC
3. Path traversal protection with `filepath.Rel()` boundary checks
4. Tiered tool system with policy-based access control
5. Default localhost binding for gateway

---

## Remediation Roadmap

### P0 - Immediate (Low Effort, High Impact)

- [ ] Fix CORS: Replace `*` with specific allowed origins
- [ ] Fix session directory permissions: `0755` -> `0700`
- [ ] Add HTTP security headers (X-Frame-Options, X-Content-Type-Options, CSP)
- [ ] Fix Electron TLS: Remove `rejectUnauthorized: false`

### P1 - This Sprint (Medium Effort)

- [ ] Implement CSRF protection on all POST/PUT/DELETE endpoints
- [ ] Encrypt session files at rest
- [ ] Move API keys from config.json to OS keychain
- [ ] Require auth token by default (generate random on first run)
- [ ] Add Subresource Integrity (SRI) to CDN scripts
- [ ] Validate soul files before inclusion in system prompt

### P2 - Next Release (Medium Effort)

- [ ] Implement JWT-based authentication (see AUTH-TOKEN-CONCEPT.md)
- [ ] Add rate limiting to HTTP endpoints
- [ ] Add request body size limits
- [ ] Shell tool: make strict allow-list mandatory
- [ ] Docker: add non-root USER directive
- [ ] Encrypt Electron remote tokens with `safeStorage`

### P3 - Roadmap (High Effort)

- [ ] Implement per-bot identity system (see BOT-IDENTITY-PROPOSAL.md)
- [ ] GitHub App integration for bot GitHub access
- [ ] Scope-based authorization per endpoint
- [ ] Inter-bot authenticated communication (group mode)
- [ ] Add `govulncheck` to CI pipeline
- [ ] Implement security audit logging

---

## Standards Referenced

- OWASP Top 10 (2021)
- OWASP Non-Human Identity Top 10 (2025)
- NIST SP 800-63-4: Digital Identity Guidelines (2025)
- Electron Security Checklist
- RFC 7519 (JWT), RFC 6749 (OAuth 2.0), RFC 7523 (JWT Client Auth)
- SPIFFE Standard (CNCF)
