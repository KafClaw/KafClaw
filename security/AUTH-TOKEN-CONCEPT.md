# Authentication and Authorization via Tokens - KafClaw

**Date:** 2026-02-16
**Status:** PROPOSAL
**Author:** Security Audit
**Scope:** Token-based auth/authz system for KafClaw gateway, channels, and inter-bot communication

---

## Problem Statement

KafClaw's current authentication model has fundamental weaknesses:

1. **Single static token** -- One `AuthToken` string shared across all clients, never expires
2. **No authorization model** -- If authenticated, a client can access all endpoints
3. **No per-user identity** -- All authenticated requests look identical
4. **No token lifecycle** -- No issuance, rotation, or revocation
5. **No channel-level auth** -- WhatsApp uses allowlist/denylist, web dashboard uses bearer token, `/chat` API has no auth at all

---

## Design Goals

1. **JWT-based authentication** with short-lived access tokens and rotating refresh tokens
2. **Scope-based authorization** -- tokens carry specific permissions
3. **Per-bot identity integration** -- tokens are bound to bot identities (see BOT-IDENTITY-PROPOSAL.md)
4. **Multiple auth methods** -- support API keys (backward compat), JWT tokens (new), and mTLS (future)
5. **Channel-aware** -- different auth requirements for web, CLI, WhatsApp, API clients
6. **Zero-trust internals** -- internal service calls also authenticated

---

## Token Architecture

### Token Types

```
+------------------+--------+----------+------------------------------+
| Token Type       | TTL    | Rotates? | Purpose                      |
+------------------+--------+----------+------------------------------+
| Access Token     | 15 min | No       | API request authorization    |
| Refresh Token    | 7 days | Yes      | Obtain new access tokens     |
| API Key          | 1 year | Manual   | Backward-compat, CI/CD       |
| Installation Token| 1 hour | Auto    | GitHub App operations        |
| Channel Token    | Session| No       | WhatsApp/Telegram binding    |
+------------------+--------+----------+------------------------------+
```

### JWT Structure

#### Access Token Claims
```json
{
  "sub": "kafclaw:alpha:a1b2c3d4",
  "iss": "kafclaw-gateway",
  "aud": "kafclaw-api",
  "exp": 1739712000,
  "iat": 1739711100,
  "jti": "tok_a1b2c3d4e5f6",
  "scopes": [
    "chat:send",
    "chat:read",
    "timeline:read",
    "settings:read",
    "tools:read-only"
  ],
  "bot_id": "kafclaw:alpha:a1b2c3d4",
  "client_type": "web-dashboard"
}
```

#### Refresh Token Claims
```json
{
  "sub": "kafclaw:alpha:a1b2c3d4",
  "iss": "kafclaw-gateway",
  "aud": "kafclaw-refresh",
  "exp": 1740316800,
  "iat": 1739712000,
  "jti": "rtk_g7h8i9j0k1l2",
  "token_family": "fam_m3n4o5p6"
}
```

### Scopes

Scopes map to the existing tool tier system and API endpoint groups:

```
Scope                  Description                          Tier Equivalent
-----                  -----------                          ---------------
chat:send              Send messages to agent               N/A
chat:read              Read conversation history            N/A
timeline:read          Query timeline events                N/A
timeline:write         Create timeline events               N/A
settings:read          Read configuration                   N/A
settings:write         Modify configuration                 N/A
tools:read-only        Execute TierReadOnly tools (0)       TierReadOnly
tools:write            Execute TierWrite tools (1)          TierWrite
tools:high-risk        Execute TierHighRisk tools (2)       TierHighRisk
approvals:read         View pending approvals               N/A
approvals:manage       Approve/deny tool executions         N/A
group:read             View group state                     N/A
group:manage           Manage group collaboration           N/A
repo:read              Read repository files                N/A
repo:write             Write repository files               N/A
repo:git               Execute git operations               N/A
identity:read          View bot identities                  N/A
identity:manage        Create/revoke identities             N/A
admin:*                Full administrative access            All
```

### Scope Profiles (Pre-defined Sets)

```
Profile          Scopes Included
-------          ---------------
viewer           chat:read, timeline:read, settings:read, approvals:read
operator         viewer + chat:send, tools:read-only, tools:write, approvals:manage
admin            operator + settings:write, tools:high-risk, repo:*, group:*, identity:*
ci-cd            chat:send, chat:read, tools:read-only
external         chat:send, chat:read (most restrictive)
```

---

## Authentication Flows

### Flow 1: Web Dashboard Login

```
Browser                     Gateway
  |                            |
  |-- GET /api/v1/auth/login --|
  |      (no credentials)     |
  |                            |
  |<-- 200 {auth_required,  --|
  |         methods: ["token",|
  |         "api-key"]}       |
  |                            |
  |-- POST /api/v1/auth/token-|
  |      {api_key: "..."}     |
  |                            |
  |<-- 200 {                 --|
  |     access_token: "ey...",|
  |     refresh_token: "ey..",|
  |     expires_in: 900,      |
  |     token_type: "Bearer", |
  |     scopes: ["admin:*"]   |
  |   }                       |
  |                            |
  |-- GET /api/v1/timeline   --|
  |   Authorization: Bearer   |
  |   ey...                    |
  |                            |
  |<-- 200 {events: [...]}  --|
```

### Flow 2: Token Refresh

```
Browser                     Gateway
  |                            |
  |-- POST /api/v1/auth/     --|
  |   refresh                  |
  |   {refresh_token: "ey.."} |
  |                            |
  |<-- 200 {                 --|
  |     access_token: "ey...",| (new access token)
  |     refresh_token: "ey..",| (new refresh token, old one invalidated)
  |     expires_in: 900       |
  |   }                       |
```

**Refresh Token Rotation:** Each refresh invalidates the previous refresh token. If a previously-used refresh token is presented (replay attack), ALL tokens in the family are revoked immediately.

### Flow 3: API Key Authentication (Backward Compatible)

```
Client                      Gateway
  |                            |
  |-- GET /api/v1/timeline   --|
  |   Authorization: Bearer   |
  |   <static-api-key>        |
  |                            |
  |   (Gateway detects this   |
  |    is not a JWT, checks   |
  |    against stored API     |
  |    keys)                   |
  |                            |
  |<-- 200 {events: [...]}  --|
```

API keys are mapped to a scope profile and a bot identity. This maintains backward compatibility while the ecosystem migrates to JWT.

### Flow 4: CLI Authentication

```
gomikrobot agent -m "hello" --identity alpha

1. CLI loads identity "alpha" from local store
2. CLI generates JWT signed with alpha's private key
3. JWT sent as Bearer token to gateway
4. Gateway validates JWT signature against alpha's public key
5. Scopes extracted from JWT claims
6. Request authorized per scope policy
```

### Flow 5: Inter-Bot Communication (Group Mode)

```
Bot Alpha                   Gateway                    Bot Beta
  |                            |                          |
  |-- POST /api/v1/group/msg --|                          |
  |   Authorization: Bearer    |                          |
  |   <alpha-jwt>              |                          |
  |   {to: "beta", msg: ".."}  |                          |
  |                            |-- Forward to beta ------>|
  |                            |   X-Sender-Bot: alpha    |
  |                            |   X-Sender-Verified: true|
  |                            |                          |
  |<-- 200 {delivered: true} --|<-- 200 ack -------------|
```

---

## Implementation Design

### Package Structure

```go
// internal/auth/

auth/
├── token.go        // JWT creation, validation, claims
├── refresh.go      // Refresh token rotation, family tracking
├── middleware.go    // HTTP auth middleware
├── store.go        // Token/key persistence (SQLite)
├── scopes.go       // Scope definitions and checking
├── apikey.go       // API key validation (backward compat)
└── auth_test.go    // Tests
```

### Core Types

```go
package auth

import (
    "crypto/ecdsa"
    "time"
    "github.com/golang-jwt/jwt/v5"
)

// Claims extends JWT standard claims with KafClaw-specific fields.
type Claims struct {
    jwt.RegisteredClaims
    Scopes     []string `json:"scopes"`
    BotID      string   `json:"bot_id"`
    ClientType string   `json:"client_type,omitempty"`
}

// TokenPair represents an access + refresh token pair.
type TokenPair struct {
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresIn    int       `json:"expires_in"`
    TokenType    string    `json:"token_type"`
    Scopes       []string  `json:"scopes"`
}

// TokenService handles token lifecycle.
type TokenService struct {
    signingKey    *ecdsa.PrivateKey
    verifyKey     *ecdsa.PublicKey
    store         TokenStore
    accessTTL     time.Duration // default: 15 minutes
    refreshTTL    time.Duration // default: 7 days
}

// Issue creates a new token pair for the given identity and scopes.
func (ts *TokenService) Issue(botID string, scopes []string, clientType string) (*TokenPair, error) {
    // 1. Generate access token JWT
    // 2. Generate refresh token JWT
    // 3. Store refresh token family in database
    // 4. Return pair
}

// Refresh exchanges a valid refresh token for a new token pair.
func (ts *TokenService) Refresh(refreshToken string) (*TokenPair, error) {
    // 1. Validate refresh token signature and expiry
    // 2. Check token family is not revoked
    // 3. Check this specific refresh token is the current one
    //    If not (replay detected): revoke entire family
    // 4. Issue new token pair in same family
    // 5. Invalidate old refresh token
    // 6. Return new pair
}

// Validate checks an access token and returns its claims.
func (ts *TokenService) Validate(tokenString string) (*Claims, error) {
    // 1. Parse JWT
    // 2. Verify signature with public key
    // 3. Check expiry
    // 4. Check JTI not revoked
    // 5. Return claims
}
```

### Middleware

```go
package auth

import "net/http"

// Middleware creates HTTP middleware that validates tokens and enforces scopes.
func (ts *TokenService) Middleware(requiredScopes ...string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. Extract token from Authorization header
            token := extractBearerToken(r)
            if token == "" {
                http.Error(w, `{"error":"missing_token"}`, http.StatusUnauthorized)
                return
            }

            // 2. Try JWT validation first
            claims, err := ts.Validate(token)
            if err != nil {
                // 3. Fall back to API key validation
                claims, err = ts.ValidateAPIKey(token)
                if err != nil {
                    http.Error(w, `{"error":"invalid_token"}`, http.StatusUnauthorized)
                    return
                }
            }

            // 4. Check required scopes
            if !hasScopes(claims.Scopes, requiredScopes) {
                http.Error(w, `{"error":"insufficient_scope"}`, http.StatusForbidden)
                return
            }

            // 5. Add claims to request context
            ctx := withClaims(r.Context(), claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// RequireScope is a convenience for single-scope checks.
func (ts *TokenService) RequireScope(scope string) func(http.Handler) http.Handler {
    return ts.Middleware(scope)
}
```

### Scope Enforcement per Endpoint

```go
// In gateway.go setup:

// Public endpoints (no auth required)
mux.Handle("/api/v1/auth/login", authLoginHandler)
mux.Handle("/api/v1/auth/token", authTokenHandler)
mux.Handle("/api/v1/auth/refresh", authRefreshHandler)

// Read-only endpoints
mux.Handle("/api/v1/status",
    tokenService.RequireScope("timeline:read")(statusHandler))
mux.Handle("/api/v1/timeline",
    tokenService.RequireScope("timeline:read")(timelineHandler))

// Chat endpoints
mux.Handle("/api/v1/chat",
    tokenService.RequireScope("chat:send")(chatHandler))

// Administrative endpoints
mux.Handle("/api/v1/settings",
    tokenService.Middleware("settings:read", "settings:write")(settingsHandler))

// High-risk endpoints
mux.Handle("/api/v1/approvals/{id}",
    tokenService.RequireScope("approvals:manage")(approvalsHandler))
mux.Handle("/api/v1/repo/checkout",
    tokenService.RequireScope("repo:git")(checkoutHandler))
```

---

## Token Storage

### Server-Side (Gateway)

```sql
-- ~/.gomikrobot/auth.db

CREATE TABLE api_keys (
    key_hash    TEXT PRIMARY KEY,     -- SHA-256 of the API key
    bot_id      TEXT NOT NULL,
    name        TEXT NOT NULL,        -- Human-readable label
    scopes      TEXT NOT NULL,        -- JSON array of scopes
    created_at  TEXT NOT NULL,
    expires_at  TEXT,                 -- NULL = no expiry (legacy)
    last_used   TEXT,
    revoked_at  TEXT
);

CREATE TABLE refresh_token_families (
    family_id       TEXT PRIMARY KEY,
    bot_id          TEXT NOT NULL,
    current_jti     TEXT NOT NULL,    -- JTI of the current valid refresh token
    scopes          TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    revoked_at      TEXT              -- Set if family is revoked (replay detected)
);

CREATE TABLE revoked_tokens (
    jti         TEXT PRIMARY KEY,
    revoked_at  TEXT NOT NULL,
    reason      TEXT                  -- "rotation", "manual", "replay_detected"
);
```

### Client-Side (Browser)

```javascript
// Access token: stored in memory only (not localStorage)
let accessToken = null;

// Refresh token: stored in httpOnly secure cookie (set by server)
// OR in memory with periodic refresh

// On page load:
async function initAuth() {
    // Try to refresh from stored refresh token
    const pair = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        credentials: 'include' // Send httpOnly cookie
    }).then(r => r.json());

    if (pair.access_token) {
        accessToken = pair.access_token;
        scheduleRefresh(pair.expires_in);
    } else {
        redirectToLogin();
    }
}

// Auto-refresh before expiry
function scheduleRefresh(expiresIn) {
    const refreshAt = (expiresIn - 60) * 1000; // 1 minute before expiry
    setTimeout(async () => {
        const pair = await refreshToken();
        if (pair) {
            accessToken = pair.access_token;
            scheduleRefresh(pair.expires_in);
        }
    }, refreshAt);
}
```

---

## Migration Strategy

### Phase 1: Add JWT Infrastructure (Non-Breaking)

1. Add `internal/auth/` package
2. Add `/api/v1/auth/token`, `/api/v1/auth/refresh` endpoints
3. Existing static `AuthToken` continues to work as an API key
4. New JWT tokens accepted alongside existing tokens
5. No existing behavior changes

### Phase 2: Scope Enforcement

1. Add scope checking to middleware (log-only mode first)
2. Monitor which endpoints are accessed with which scopes
3. Switch to enforce mode after validation
4. API keys mapped to scope profiles

### Phase 3: Mandatory JWT

1. Deprecation warning for static `AuthToken` usage
2. CLI tools generate JWTs from bot identities
3. Web dashboard uses JWT flow
4. Remove static token support (or limit to `viewer` scope)

### Phase 4: Inter-Bot Auth

1. Group-mode communication uses bot-identity JWTs
2. Gateway validates sender identity on forwarded messages
3. Per-bot rate limits and quotas
4. Cross-instance trust via public key exchange

---

## Security Properties

| Property | Implementation |
|----------|---------------|
| **Confidentiality** | Tokens are signed (not encrypted) -- use TLS for transport |
| **Integrity** | ECDSA P-256 signatures prevent token tampering |
| **Freshness** | 15-minute access token TTL, refresh rotation |
| **Replay prevention** | Unique JTI per token, tracked in revocation store |
| **Scope limitation** | Scopes enforce least-privilege per endpoint |
| **Revocation** | Immediate via JTI revocation list, family revocation on replay |
| **Backward compatibility** | Static API keys accepted with reduced scope |
| **Audit trail** | Token issuance and usage logged to timeline |

---

## Threat Model

| Threat | Mitigation |
|--------|------------|
| Token theft (XSS) | Access tokens in memory only (not localStorage); short TTL |
| Token theft (network) | TLS required for non-localhost; HSTS header |
| Refresh token replay | Token family tracking; replay revokes entire family |
| Brute force API key | Rate limiting on auth endpoints; key hashing with SHA-256 |
| Scope escalation | Scopes are server-enforced, not client-modifiable |
| Token forgery | ECDSA signature verification; JWKS key publication |
| Cross-site request | CORS restricted to specific origins; CSRF token for cookies |
| Key compromise | Key rotation; OS keychain storage; revocation mechanism |

---

## Configuration

```json
// ~/.gomikrobot/config.json additions
{
  "auth": {
    "method": "jwt",
    "access_token_ttl": "15m",
    "refresh_token_ttl": "7d",
    "api_key_enabled": true,
    "require_auth": true,
    "allowed_origins": ["http://127.0.0.1:18791"],
    "rate_limit": {
      "auth_attempts": "10/minute",
      "api_requests": "100/minute"
    }
  }
}
```

---

## Dependencies

| Package | Purpose | License |
|---------|---------|---------|
| `github.com/golang-jwt/jwt/v5` | JWT creation and validation | MIT |
| `github.com/zalando/go-keyring` | OS keychain access | MIT |
| `golang.org/x/time/rate` | Rate limiting | BSD-3 |

All are well-maintained, widely-used Go packages with permissive licenses.

---

## References

- [RFC 7519: JSON Web Token (JWT)](https://tools.ietf.org/html/rfc7519)
- [RFC 6749: OAuth 2.0 Authorization Framework](https://tools.ietf.org/html/rfc6749)
- [RFC 7523: JWT Profile for OAuth 2.0 Client Authentication](https://tools.ietf.org/html/rfc7523)
- [OWASP JWT Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_for_Java_Cheat_Sheet.html)
- [Auth0: Token Best Practices](https://auth0.com/docs/secure/tokens/token-best-practices)
- [Token Rotation Strategies (2026)](https://oneuptime.com/blog/post/2026-01-30-token-rotation-strategies/view)
