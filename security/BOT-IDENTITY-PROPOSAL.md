# Bot Identity Proposal - KafClaw

**Date:** 2026-02-16
**Status:** PROPOSAL
**Author:** Security Audit
**Scope:** Per-bot identity architecture for KafClaw agents

---

## Problem Statement

KafClaw currently has no concept of bot identity. All bot instances:
- Share the same API keys and credentials
- Cannot be individually identified, authorized, or revoked
- Use a single identity when accessing GitHub, LLM providers, and external services
- Have no mechanism for per-bot permissions or audit trails

This creates risks: a compromised bot exposes all credentials, there is no accountability for individual bot actions, and scaling to multiple bots (each needing separate GitHub access, permissions, and secrets) is impossible.

---

## Market Analysis

### Industry Landscape (2025-2026)

The machine identity / non-human identity (NHI) space has exploded. Key developments:

| Category | Key Players | Approach |
|----------|------------|----------|
| **Cloud NHI** | Azure Managed Identities, AWS IAM Roles, GCP Workload Identity | Platform-native, secretless auth |
| **AI Agent Identity** | Microsoft Entra ID Agent Identities, HashiCorp Vault AI Agent Pattern | Purpose-built for AI agents |
| **Standards** | SPIFFE/SPIRE (CNCF), OAuth 2.0/2.1, MCP Authorization | Interoperable, standards-based |
| **Secrets Management** | HashiCorp Vault, CyberArk, 1Password Agentic AI, Auth0 Token Vault | Centralized secret lifecycle |
| **NHI Security** | Astrix Security, Oasis Security, Aembit, Entro Security | Discovery, posture, governance |

### Key Industry Trends

1. **NHIs outnumber humans 40:1 to 100:1** in enterprises
2. **OWASP NHI Top 10** published in 2025 -- first dedicated standard for machine identity security
3. **SPIFFE + OAuth bridge** -- IETF draft (Dec 2025) allows SPIFFE identities to serve as OAuth client credentials
4. **MCP Authorization** now uses OAuth 2.1, with machine-to-machine support via client credentials
5. **No major AI agent framework** (LangChain, CrewAI, AutoGPT) provides built-in per-agent identity -- this is treated as an external concern everywhere

### GitHub Bot Identity: The GitHub App Model

GitHub Apps are the modern, recommended mechanism for bot identity on GitHub:

- Each App gets a built-in bot identity (`app-name[bot]`)
- Fine-grained permissions per installation (per-repo, per-resource-type)
- Short-lived installation tokens (1-8 hours, generated on demand)
- No license seat consumption (unlike machine users)
- Rate limits scale with organization size

**Authentication flow:**
1. Bot generates JWT signed with App's private key (RS256, 10-min TTL)
2. Exchanges JWT for installation access token via GitHub API
3. Uses installation token for all API calls (regenerated as needed)

---

## Proposed Architecture

### Design Principles

1. **One bot = one identity** -- each KafClaw instance gets a unique, cryptographic identity
2. **Ephemeral credentials** -- short-lived tokens replace static API keys wherever possible
3. **Least privilege** -- each bot gets only the permissions it needs
4. **OS keychain for secrets** -- no plaintext secrets on disk
5. **Auditable** -- every action is traceable to a specific bot identity
6. **Standards-based** -- JWT for tokens, JWKS for key publication, OAuth 2.0 for external auth

### Identity Model

```
KafClaw Identity Registry
  |
  |-- Bot Instance "alpha"
  |     |-- bot_id: "kafclaw:alpha:a1b2c3d4"
  |     |-- key_pair: RSA-2048 or ECDSA P-256
  |     |-- github_installation_id: 12345678
  |     |-- allowed_scopes: ["repo:read", "repo:write", "shell:read-only"]
  |     |-- created_at: 2026-02-16T10:00:00Z
  |     |-- last_active: 2026-02-16T14:30:00Z
  |
  |-- Bot Instance "beta"
  |     |-- bot_id: "kafclaw:beta:e5f6g7h8"
  |     |-- key_pair: RSA-2048 or ECDSA P-256
  |     |-- github_installation_id: 87654321
  |     |-- allowed_scopes: ["repo:read", "shell:strict"]
  |     |-- created_at: 2026-02-10T08:00:00Z
  |     |-- last_active: 2026-02-16T12:15:00Z
```

### Component Design

```
                  +------------------+
                  |  Identity Store  |
                  |  (SQLite + OS    |
                  |   Keychain)      |
                  +--------+---------+
                           |
              +------------+------------+
              |                         |
    +---------v---------+    +----------v----------+
    |  Token Generator  |    |  Key Manager        |
    |  (JWT creation,   |    |  (Key pair gen,     |
    |   refresh,        |    |   rotation,         |
    |   revocation)     |    |   OS keychain)      |
    +--------+----------+    +----------+----------+
             |                          |
    +--------v--------------------------v----------+
    |              Identity Provider               |
    |  (Central interface for all identity ops)    |
    +--------+----------+----------+---------------+
             |          |          |
    +--------v--+ +-----v----+ +--v-----------+
    | GitHub    | | Gateway  | | LLM Provider |
    | App Auth  | | JWT Auth | | API Key      |
    +-----------+ +----------+ | Delegation   |
                               +--------------+
```

### Bot Identity Lifecycle

```
1. CREATION
   gomikrobot identity create --name "alpha"
   -> Generates ECDSA P-256 key pair
   -> Stores private key in OS keychain
   -> Creates bot_id: "kafclaw:alpha:{random}"
   -> Writes identity record to ~/.gomikrobot/identities.db

2. CONFIGURATION
   gomikrobot identity configure "alpha" \
     --github-app-id 123456 \
     --github-installation-id 12345678 \
     --github-private-key /path/to/key.pem \
     --scopes repo:read,repo:write,shell:read-only
   -> Stores GitHub App credentials in OS keychain
   -> Records scopes in identity database

3. ACTIVATION
   gomikrobot gateway --identity "alpha"
   -> Loads identity from store
   -> Generates short-lived JWT for internal auth
   -> Authenticates to GitHub via App installation tokens
   -> Starts gateway with identity context

4. TOKEN ISSUANCE
   Internal JWT for each request:
   {
     "sub": "kafclaw:alpha:a1b2c3d4",
     "iss": "kafclaw-identity",
     "aud": "kafclaw-gateway",
     "exp": <now + 15 minutes>,
     "jti": "<unique-id>",
     "scopes": ["repo:read", "repo:write", "shell:read-only"]
   }

5. ROTATION
   Key pairs rotate every 90 days (configurable)
   Old keys remain valid for 24 hours after rotation
   GitHub App keys managed separately via GitHub UI

6. REVOCATION
   gomikrobot identity revoke "alpha"
   -> Marks identity as revoked in database
   -> Deletes private key from OS keychain
   -> Invalidates all active tokens
   -> Logs revocation event to timeline
```

---

## Implementation Plan

### Phase 1: Identity Foundation (internal package)

Create `internal/identity/` package (separate from the existing `internal/identity/` which handles soul file templates -- this would be `internal/botidentity/` to avoid collision):

```go
// internal/botidentity/identity.go

package botidentity

import "crypto/ecdsa"

// BotIdentity represents a unique bot instance identity.
type BotIdentity struct {
    BotID        string            `json:"bot_id"`
    Name         string            `json:"name"`
    PublicKey    *ecdsa.PublicKey   `json:"-"`
    Scopes       []string          `json:"scopes"`
    GitHubAppID  int64             `json:"github_app_id,omitempty"`
    GitHubInstID int64             `json:"github_installation_id,omitempty"`
    CreatedAt    time.Time         `json:"created_at"`
    RevokedAt    *time.Time        `json:"revoked_at,omitempty"`
}

// Store manages bot identity persistence.
type Store interface {
    Create(name string, scopes []string) (*BotIdentity, error)
    Get(botID string) (*BotIdentity, error)
    GetByName(name string) (*BotIdentity, error)
    List() ([]*BotIdentity, error)
    Revoke(botID string) error
}

// KeyManager handles cryptographic key operations.
type KeyManager interface {
    GenerateKeyPair(botID string) (*ecdsa.PrivateKey, error)
    GetPrivateKey(botID string) (*ecdsa.PrivateKey, error)
    RotateKeyPair(botID string) (*ecdsa.PrivateKey, error)
    DeleteKey(botID string) error
}

// TokenIssuer generates and validates JWTs.
type TokenIssuer interface {
    Issue(identity *BotIdentity, audience string, ttl time.Duration) (string, error)
    Validate(tokenString string) (*Claims, error)
    Revoke(jti string) error
}
```

### Phase 2: OS Keychain Integration

Use `github.com/zalando/go-keyring` for cross-platform secret storage:

```go
// internal/botidentity/keychain.go

package botidentity

import "github.com/zalando/go-keyring"

const serviceName = "kafclaw"

type KeychainManager struct{}

func (k *KeychainManager) StorePrivateKey(botID string, pemBytes []byte) error {
    return keyring.Set(serviceName, botID+":private-key", string(pemBytes))
}

func (k *KeychainManager) GetPrivateKey(botID string) ([]byte, error) {
    val, err := keyring.Get(serviceName, botID+":private-key")
    if err != nil {
        return nil, err
    }
    return []byte(val), nil
}

func (k *KeychainManager) StoreAPIKey(botID, provider, key string) error {
    return keyring.Set(serviceName, botID+":"+provider, key)
}

func (k *KeychainManager) GetAPIKey(botID, provider string) (string, error) {
    return keyring.Get(serviceName, botID+":"+provider)
}

func (k *KeychainManager) DeleteAll(botID string) error {
    _ = keyring.Delete(serviceName, botID+":private-key")
    // Delete all known provider keys...
    return nil
}
```

### Phase 3: GitHub App Integration

```go
// internal/botidentity/github.go

package botidentity

import (
    "crypto/rsa"
    "time"
    "github.com/golang-jwt/jwt/v5"
)

type GitHubAppAuth struct {
    AppID          int64
    InstallationID int64
    PrivateKey     *rsa.PrivateKey
}

// GenerateJWT creates a short-lived JWT for GitHub App authentication.
func (g *GitHubAppAuth) GenerateJWT() (string, error) {
    now := time.Now()
    claims := jwt.RegisteredClaims{
        Issuer:    fmt.Sprintf("%d", g.AppID),
        IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
        ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
    }
    token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
    return token.SignedString(g.PrivateKey)
}

// GetInstallationToken exchanges the JWT for a short-lived installation token.
func (g *GitHubAppAuth) GetInstallationToken(ctx context.Context) (string, time.Time, error) {
    jwtToken, err := g.GenerateJWT()
    if err != nil {
        return "", time.Time{}, err
    }
    // POST /app/installations/{id}/access_tokens with JWT bearer
    // Returns { "token": "...", "expires_at": "..." }
    // ...
}
```

### Phase 4: CLI Commands

```
gomikrobot identity create --name <name> [--scopes <scope-list>]
gomikrobot identity list
gomikrobot identity show <name>
gomikrobot identity configure <name> --github-app-id <id> --github-installation-id <id>
gomikrobot identity rotate <name>
gomikrobot identity revoke <name>
gomikrobot identity export <name> --public-key  # Export public key for verification
```

---

## Comparison of Approaches

| Approach | Pros | Cons | Fit for KafClaw |
|----------|------|------|-----------------|
| **Local identity store + OS keychain** | Simple, no external deps, works offline, cross-platform | Single-machine, no central management | Best for v1 |
| **HashiCorp Vault** | Enterprise-grade, dynamic secrets, audit logging | Infrastructure overhead, requires running Vault server | Good for future multi-node |
| **SPIFFE/SPIRE** | Standards-based, automatic rotation, federated trust | Requires SPIRE server, best for containerized workloads | Overkill for desktop app |
| **Cloud-native (Azure MI/AWS IRSA)** | Secretless, platform-managed | Cloud-locked, not for desktop apps | Not applicable |

**Recommendation:** Start with **local identity store + OS keychain** (Phase 1-3). This provides per-bot identity without infrastructure overhead. Evolve to Vault or SPIFFE when/if KafClaw becomes a multi-node system.

---

## GitHub Integration Strategy

### Recommended: One GitHub App, Multiple Installations

```
GitHub App: "KafClaw Bot"
  |
  |-- Installation in org/repo A -> Bot "alpha" uses this
  |-- Installation in org/repo B -> Bot "beta" uses this
  |-- Installation in org/repo C -> Bot "gamma" uses this
```

Each bot instance:
1. Stores the GitHub App private key in OS keychain (shared across bots on same machine, or per-bot if separate machines)
2. Knows its own `installation_id`
3. Generates its own short-lived installation tokens
4. Has independent rate limits per installation
5. Can be individually revoked by uninstalling from the org/repo

### Self-Hosted Git (Gitea/Forgejo)

For self-hosted ("own clone of GitHub"):
- Gitea supports OAuth 2.0 applications with scoped tokens
- Create one OAuth application per bot, each with its own `client_id`/`client_secret`
- Use client credentials flow for token generation
- Store credentials in OS keychain per bot identity

---

## Migration Path from Current State

### Step 1: Create Default Identity
On first run after upgrade, automatically create a "default" identity:
```
gomikrobot identity migrate
-> Creates identity "default" with existing config's API keys
-> Moves API keys from config.json to OS keychain
-> Updates config.json to reference identity by name
-> Preserves backward compatibility
```

### Step 2: Gradual Adoption
- Existing single-bot users continue working with "default" identity
- Multi-bot users create additional identities as needed
- Config format supports both legacy (inline keys) and new (identity reference)

### Step 3: Deprecate Inline Keys
- After transition period, deprecate `apiKey` in config.json
- Print warning if inline keys detected
- Provide migration tool

---

## Security Considerations

| Concern | Mitigation |
|---------|------------|
| Key compromise | OS keychain encryption, key rotation every 90 days |
| Token theft | Short TTL (15 min), single-use JTI tracking |
| Identity spoofing | ECDSA signatures on all internal tokens |
| Lateral movement | Scoped permissions per identity, least privilege |
| Revocation lag | Revocation list checked on every token validation |
| Keychain unavailable | Graceful fallback to encrypted file (AES-256-GCM) |

---

## References

- [OWASP Non-Human Identity Top 10 (2025)](https://owasp.org/www-project-non-human-identities-top-10/)
- [GitHub Docs: Creating GitHub Apps](https://docs.github.com/en/apps/creating-github-apps)
- [GitHub Docs: Generating Installation Access Tokens](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app)
- [NIST SP 800-63-4: Digital Identity Guidelines (2025)](https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-63-4.pdf)
- [SPIFFE Standard](https://spiffe.io/)
- [HashiCorp Vault AI Agent Identity Pattern](https://developer.hashicorp.com/validated-patterns/vault/ai-agent-identity-with-hashicorp-vault)
- [Microsoft Entra ID Agent Identities](https://learn.microsoft.com/en-us/azure/ai-foundry/agents/concepts/agent-identity)
- [zalando/go-keyring](https://github.com/zalando/go-keyring)
- [beatlabs/github-auth](https://pkg.go.dev/github.com/beatlabs/github-auth)
