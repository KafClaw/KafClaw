# Provider Support Execution Board

Updated: 2026-02-20
Branch: `modelsupport`

## Goal

Implement a clean, runtime-wired provider layer with:

- canonical provider identifier per backend (used in config and model strings)
- per-agent and per-subagent configurable model path with `primary` + `fallbacks`
- model string format: `<provider-id>/<model-id>` (e.g. `claude/claude-opus-4-6`)
- CLI OAuth flows for Gemini and Codex (install CLI if absent, delegate auth, read credential cache)
- API key flows for OpenAI, Anthropic/Claude, xAI, Gemini (key path), OpenRouter, DeepSeek, Groq, vLLM
- ScalyticsCopilot as OpenAI-compatible provider with bearer token + configurable base URL
- provider resolver replacing all hardcoded `NewOpenAIProvider` calls in `agent` and `gateway`
- token consumption stats per request, per session, per day — with per-provider breakdown
- rate limit / quota remaining surfaced from provider response headers where available
- chat middleware chain between agent loop and LLM provider
- content classification → model routing (PII → private model, coding → coding model)
- prompt guard (PII/secret/keyword scan pre-LLM: block / redact / reroute)
- output sanitization (PII/secret redaction post-LLM before channel delivery)
- FinOps cost attribution ($/token per provider, daily/monthly budgets, per-agent/channel breakdown)
- task-type model routing (security → strong model, quick-answer → fast model)

> **Related:** Non-model enterprise features (OTEL, DLQ/Replay, HITL, Registry, Multi-tenancy, etc.)
> are tracked in `_tasks/enterprise-features.md`.

---

## Documentation Contract (applies to every item in this file)

Every feature that ships must update docs **as part of the same PR** — not after. No exceptions.

| Trigger | Required doc update |
|---------|-------------------|
| New or changed CLI command / flag | `docs/reference/cli-reference.md` |
| New or changed config key | `docs/reference/config-keys.md` |
| New provider added | `docs/reference/config-keys.md` (provider block) + `docs/operations-admin/admin-guide.md` (setup steps) |
| New `kafclaw models auth` flow | `docs/start-here/getting-started.md` (user) + `docs/operations-admin/admin-guide.md` (admin) |
| New onboarding preset | `docs/start-here/getting-started.md` |
| Token stats / `kafclaw models stats` | `docs/reference/cli-reference.md` + `docs/operations-admin/operations-guide.md` |
| Doctor / status surface changes | `docs/reference/cli-reference.md` + `docs/operations-admin/operations-guide.md` |
| Security / credential changes | `docs/architecture-security/security-for-ops.md` |
| Gateway API change | `docs/reference/api-endpoints.md` |
| Middleware chain / prompt guard / sanitizer | `docs/architecture-security/security-for-ops.md` + `docs/operations-admin/admin-guide.md` |
| Content classification / model routing | `docs/reference/config-keys.md` + `docs/operations-admin/admin-guide.md` |
| FinOps / cost attribution | `docs/reference/config-keys.md` + `docs/operations-admin/operations-guide.md` |
| Task-type routing | `docs/reference/config-keys.md` |

**Rule of thumb:** if a user or admin would need to know about it to configure, operate, or
troubleshoot KafClaw, it goes in `docs/`. If it only affects internal code structure, it does not.

---

## Provider Identifier Matrix

These are the canonical IDs used in `model:` strings and config keys.

### OAuth / CLI-delegated (no static key in config)

| Provider ID    | Auth method           | Model string example              | API surface                         |
|----------------|-----------------------|-----------------------------------|-------------------------------------|
| `gemini-cli`   | CLI OAuth (Gemini CLI)| `gemini-cli/gemini-2.0-flash`     | Gemini REST, OAuth bearer           |
| `openai-codex` | CLI OAuth (Codex CLI) | `openai-codex/gpt-5.3-codex`     | OpenAI REST, OAuth bearer           |

### API key (static key in `providers.<id>.apiKey`)

| Provider ID         | Auth method    | Model string example                   | API surface               | Notes                        |
|---------------------|----------------|----------------------------------------|---------------------------|------------------------------|
| `claude`            | API key        | `claude/claude-opus-4-6`               | Anthropic REST            | native Anthropic API         |
| `openai`            | API key        | `openai/gpt-4o`                        | OpenAI REST               | ChatGPT models, API key only |
| `gemini`            | API key        | `gemini/gemini-2.0-flash`              | Gemini REST               | Google AI Studio key         |
| `xai`               | API key        | `xai/grok-3`                           | OpenAI-compat (xAI)       | API key only                 |
| `scalytics-copilot` | Bearer + URL   | `scalytics-copilot/default`            | OpenAI-compat             | configurable base URL        |
| `openrouter`        | API key        | `openrouter/anthropic/claude-opus-4-6` | OpenAI-compat proxy       | routes to any backend        |
| `deepseek`          | API key        | `deepseek/deepseek-chat`               | OpenAI-compat             |                              |
| `groq`              | API key        | `groq/llama-3.3-70b-versatile`         | OpenAI-compat             |                              |
| `vllm`              | API key / none | `vllm/llama-3.1-8b-instruct`           | OpenAI-compat             | self-hosted                  |

### Auth capability summary

| Provider        | API key | CLI OAuth | Notes                                              |
|-----------------|---------|-----------|----------------------------------------------------|
| `claude`        | ✓       | —         | Anthropic API key                                  |
| `openai`        | ✓       | —         | ChatGPT/GPT models, no OAuth path                  |
| `openai-codex`  | —       | ✓         | Codex CLI OAuth only; same OpenAI API endpoint     |
| `gemini`        | ✓       | —         | Google AI Studio key, Gemini REST                  |
| `gemini-cli`    | —       | ✓         | Gemini CLI OAuth only; same Gemini REST endpoint   |
| `xai`           | ✓       | —         | xAI/Grok, API key only                             |
| `scalytics-copilot` | ✓  | —         | Bearer token + user-configurable base URL          |
| `openrouter`    | ✓       | —         |                                                    |
| `deepseek`      | ✓       | —         |                                                    |
| `groq`          | ✓       | —         |                                                    |
| `vllm`          | ✓       | —         | key optional for local deployments                 |

**Aliases** recognized by the resolver (normalized at parse time):
- `google` → `gemini-cli` (OAuth path preferred when no key configured; `gemini` if API key present)
- `codex` → `openai-codex`
- `anthropic` → `claude`
- `copilot` → `scalytics-copilot`
- `grok` → `xai`

---

## Per-Agent Model Config (config.json schema)

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "default": true,
        "model": {
          "primary": "claude/claude-opus-4-6",
          "fallbacks": ["openai/gpt-4o", "openrouter/anthropic/claude-sonnet-4-5"]
        },
        "subagents": {
          "model": "claude/claude-sonnet-4-6"
        }
      },
      {
        "id": "coder",
        "model": {
          "primary": "openai-codex/gpt-5.3-codex",
          "fallbacks": ["openai/gpt-4o"]
        }
      }
    ]
  }
}
```

Resolution order for an agent's model:
1. `agents.list[agentId].model.primary`
2. `agents.list[agentId].model.fallbacks[0..n]` (tried in order on transient errors)
3. `model.name` (global fallback, same `provider/model` format)
4. `providers.openai` with `model.name` as bare model name (legacy compat)

Resolution order for subagents spawned by an agent:
1. `agents.list[agentId].subagents.model`
2. `tools.subagents.model` (global subagent default)
3. Inherit parent agent's resolved model

---

## Done (Context Confirmed)

- [x] Runtime uses `OpenAIProvider` as single LLM path (`internal/provider/openai.go`)
- [x] Onboarding writes through `providers.openai` only (`cli-token` / `openai-compatible`)
- [x] Config schema has `anthropic`, `openrouter`, `deepseek`, `groq`, `gemini`, `vllm` blocks (unwired; no `xai` or `scalyticsCopilot` yet)
- [x] `AgentListEntry` exists but has no `model` or `subagents` fields
- [x] Local Whisper available for transcription (`internal/provider/whisper_local.go`)
- [x] Two provider injection points confirmed: `cli/agent.go:52` and `cli/gateway.go:128`
- [x] ScalyticsCopilot API confirmed: OpenAI-compat, bearer auth, configurable base URL, `/v1/chat/completions` + `/v1/models`
- [x] Gemini CLI OAuth: credentials at `~/.gemini/oauth_creds.json`, CLI `@google/gemini-cli` (npm)
- [x] Codex CLI OAuth: credentials at `~/.codex/auth.json`, CLI `@openai/codex` (npm)
- [x] Encrypted keystore already exists: `internal/skills/oauth_crypto.go` — AES-256-GCM, master key via OS keyring → local tomb (`~/.config/kafclaw/tomb.rr`) → key file; `encryptOAuthStateBlob`/`decryptOAuthStateBlob` unexported
- [x] `StoreEnvSecretsInLocalTomb`/`LoadEnvSecretsFromLocalTomb` already available for encrypted key-value secrets (static API keys can use this)
- [x] `cliconfig/security.go` already audits `auth/*/token.json` for encryption coverage (`gap_oauth_encryption` check)
- [x] `internal/skills` does not import `internal/provider` — no import cycle risk
- [x] Token tracking already exists: `AgentTask` has `prompt_tokens`/`completion_tokens`/`total_tokens`; `UpdateTaskTokens` accumulates per-task; `GetDailyTokenUsage()` sums today; `checkTokenQuota()` enforces configurable daily limit; agent loop calls `trackTokens()` after every LLM call
- [x] `tasks` table has no `provider` or `model` column — per-provider breakdown not yet possible
- [x] No rate limit / quota header parsing in current `OpenAIProvider`
- [x] No `kafclaw models stats` command exists

---

## Implementation Scope

### 1 — Config schema (`internal/config/config.go`)

- [x] Add `AgentModelSpec` type: `{ Primary string; Fallbacks []string }`
- [x] Add `AgentSubagentSpec` type: `{ Model string }`
- [x] Extend `AgentListEntry` with `Model *AgentModelSpec` and `Subagents *AgentSubagentSpec`
- [x] Add `ScalyticsCopilot ProviderConfig` to `ProvidersConfig` (`apiKey` + `apiBase`)
- [x] Add `XAI ProviderConfig` to `ProvidersConfig` (`apiKey`; base fixed to `api.x.ai/v1`)
- [x] Keep existing `Gemini ProviderConfig` for API key path (`providers.gemini.apiKey`)
- [x] `gemini-cli` auth path uses credential cache only — no config block needed

### 2 — Shared secrets package (`internal/secrets/`)

The encryption layer already exists in `internal/skills/oauth_crypto.go` (AES-256-GCM, master key via OS keyring → local tomb → key file). The crypto functions are currently unexported and skills-private. Extract them to a shared package so both `internal/skills` and `internal/provider` can use them without an import cycle.

- [x] Create `internal/secrets/blob.go` — move `encryptOAuthStateBlob`/`decryptOAuthStateBlob`/`loadOrCreateOAuthMasterKey` here as exported `EncryptBlob`/`DecryptBlob`/`LoadOrCreateMasterKey`; keep three key backends (keyring → tomb → file)
- [x] Update `internal/skills/oauth_crypto.go` to call `secrets.EncryptBlob`/`secrets.DecryptBlob` instead of local functions (no behaviour change)
- [x] `StoreEnvSecretsInLocalTomb` / `LoadEnvSecretsFromLocalTomb` stay in `internal/skills` — they are used for skill env secrets, not provider credentials

**Static API keys** (claude, openai, xai, scalytics-copilot, etc.) are stored via the **tomb's env secret map**, keyed as `provider.apikey.<provider-id>`. This keeps them out of config.json plaintext.

### 3 — Provider credential store (`internal/provider/credentials/store.go`)

Uses `internal/secrets` for encryption. Token files land at `<ToolsDir>/auth/providers/<provider-id>/token.json` — inside the existing `auth/*/token.json` tree already audited by `cliconfig/security.go`'s `gap_oauth_encryption` check, so encryption coverage is automatic.

- [x] `OAuthToken` struct: `{ Access, Refresh string; Expires int64; Email string }`
- [x] `LoadToken(providerID string) (*OAuthToken, error)` — reads and decrypts `<ToolsDir>/auth/providers/<id>/token.json`
- [x] `SaveToken(providerID string, t *OAuthToken) error` — encrypts via `secrets.EncryptBlob`, writes 0600
- [x] `IsExpired(t *OAuthToken) bool` — 60s grace margin
- [x] `LoadAPIKey(providerID string) (string, error)` — reads from tomb env map key `provider.apikey.<id>`
- [x] `SaveAPIKey(providerID string, key string) error` — writes to tomb env map

### 4 — CLI credential cache readers

- [x] `internal/provider/clicache/gemini.go`: `ReadGeminiCLICredential()` — reads `~/.gemini/oauth_creds.json`, shells to `gemini auth` on expiry, returns `*credentials.OAuthToken`
- [x] `internal/provider/clicache/codex.go`: `ReadCodexCLICredential()` — reads `~/.codex/auth.json`, shells to `codex auth` on expiry, returns `*credentials.OAuthToken`

### 5 — CLI installer (`internal/provider/clinst/install.go`)

- [x] `EnsureGeminiCLI() error` — check PATH, install via `npm install -g @google/gemini-cli`, fallback build from source
- [x] `EnsureCodexCLI() error` — check PATH, install via `npm install -g @openai/codex`, fallback build from source

### 6 — Provider implementations

- [x] `internal/provider/gemini.go` — `GeminiProvider` calling Gemini REST (`generativelanguage.googleapis.com/v1beta`); two auth modes: static `apiKey` (query param) for `gemini` ID, OAuth bearer from CLI cache for `gemini-cli` ID; same request/response conversion for both
- [x] `internal/provider/codex.go` — `CodexProvider` wrapping `OpenAIProvider` with OAuth bearer token from Codex CLI cache instead of static key
- [x] `internal/provider/xai.go` — `XAIProvider` wrapping `OpenAIProvider` with fixed base `https://api.x.ai/v1` and `providers.xai.apiKey`; no OAuth path

### 7 — Provider resolver (`internal/provider/resolver.go`)

- [x] `Resolve(cfg *config.Config, agentID string) (LLMProvider, error)` — reads per-agent model spec, parses `provider/model`, dispatches to correct provider
- [x] `ResolveSubagent(cfg *config.Config, agentID string) (LLMProvider, error)` — reads `subagents.model` first, delegates to `Resolve` on miss
- [x] Normalize provider aliases at parse time (`google`→`gemini`, `codex`→`openai-codex`, etc.)
- [x] Return structured error with provider ID and remediation hint on config/credential miss

### 8 — Wire resolver into runtime

- [x] `internal/cli/agent.go` — replace hardcoded `NewOpenAIProvider` with `provider.Resolve(cfg, agentID)`
- [x] `internal/cli/gateway.go` — same replacement

### 9 — Models CLI command (`internal/cli/models.go`)

- [x] `kafclaw models auth login --provider gemini` — install Gemini CLI if absent, run `gemini auth login` (interactive TTY), verify credential written
- [x] `kafclaw models auth login --provider openai-codex` — install Codex CLI if absent, run `codex auth` (interactive TTY), verify credential written
- [x] `kafclaw models auth set-key --provider <id> --key <token> [--base <url>]` — writes to config for API-key providers; `--base` required for `scalytics-copilot`; accepted providers: `claude`, `openai`, `gemini`, `xai`, `scalytics-copilot`, `openrouter`, `deepseek`, `groq`, `vllm`
- [x] `kafclaw models list` — print configured providers, active model per agent, credential status

### 10 — Onboarding (`internal/onboarding/profile.go`)

- [x] Add LLM presets to `resolveLLMPreset` and interactive menu:
  - `gemini` (API key), `gemini-cli` (OAuth), `openai-codex` (OAuth), `scalytics-copilot` (key + URL), `xai` (API key), `claude` (API key)
- [x] `applyLLM` handlers for each new preset: OAuth presets trigger `models auth login`; API key presets prompt for key (and base URL where applicable)
- [x] Include active provider in `BuildProfileSummary` output

### 11 — Doctor + Status

- [x] `internal/cliconfig/doctor.go` — add per-provider readiness checks: credential configured, CLI tools present
- [x] `internal/cli/status.go` — surface active provider ID and model per agent in status output
- [x] Remediation hints per failure type (missing CLI, expired token, bad URL, wrong key)

### 13 — Token Stats & Quota (`internal/provider/`, `internal/timeline/`, `internal/cli/`)

#### What each provider actually returns

| Provider        | Consumed tokens (body)                                        | Rate limit remaining (headers)                                        | Reset time (headers)                        |
|-----------------|---------------------------------------------------------------|-----------------------------------------------------------------------|---------------------------------------------|
| `claude`        | `usage.input_tokens` + `output_tokens`                        | `anthropic-ratelimit-tokens-remaining`                                | `anthropic-ratelimit-tokens-reset` (RFC3339) |
| `openai`        | `usage.prompt_tokens` + `completion_tokens`                   | `x-ratelimit-remaining-tokens`, `x-ratelimit-remaining-requests`      | `x-ratelimit-reset-tokens`                  |
| `openai-codex`  | same as openai                                                | same as openai                                                        | same as openai                              |
| `gemini`        | `usageMetadata.promptTokenCount` + `candidatesTokenCount`     | — (Google Cloud quota, no header)                                     | —                                           |
| `gemini-cli`    | same as gemini                                                | —                                                                     | —                                           |
| `xai`           | `usage.prompt_tokens` + `completion_tokens` (OpenAI-compat)   | `x-ratelimit-remaining-tokens` (OpenAI-compat)                       | `x-ratelimit-reset-tokens`                  |
| `scalytics-copilot` | `usage.prompt_tokens` + `completion_tokens` (OpenAI-compat) | exposed if server sets headers                                      | exposed if server sets headers              |
| `openrouter`    | `usage.prompt_tokens` + `completion_tokens`                   | `x-ratelimit-remaining-requests`                                      | `x-ratelimit-reset-requests`                |
| `deepseek`      | `usage.*` (OpenAI-compat)                                     | `x-ratelimit-remaining-tokens`                                        | `x-ratelimit-reset-tokens`                  |
| `groq`          | `usage.*` (OpenAI-compat)                                     | `x-ratelimit-remaining-tokens`                                        | `x-ratelimit-reset-tokens`                  |
| `vllm`          | `usage.*` (OpenAI-compat)                                     | varies by deployment                                                  | varies                                      |

#### `Usage` struct extension (`internal/provider/provider.go`)

```go
type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
    // Populated from HTTP response headers where the provider exposes them.
    // nil means the provider did not report this value.
    RemainingTokens   *int       `json:"remaining_tokens,omitempty"`
    RemainingRequests *int       `json:"remaining_requests,omitempty"`
    LimitTokens       *int       `json:"limit_tokens,omitempty"`
    ResetAt           *time.Time `json:"reset_at,omitempty"`
}
```

#### Tasks

- [x] Extend `Usage` struct with `RemainingTokens *int`, `RemainingRequests *int`, `LimitTokens *int`, `ResetAt *time.Time`
- [x] Parse rate limit headers in `OpenAIProvider.Chat()` and populate `Usage` fields (covers openai, openai-codex, xai, scalytics-copilot, openrouter, deepseek, groq)
- [x] Parse `anthropic-ratelimit-*` headers in the `claude` provider implementation
- [x] Parse Gemini `usageMetadata` body fields and normalize to `Usage` (no rate limit headers available for Gemini)
- [x] Add `provider TEXT` and `model TEXT` columns to `tasks` table via migration in `timeline/service.go` (best-effort `ALTER TABLE`)
- [x] Pass `providerID` and `model` into `UpdateTaskTokens` (or a new `UpdateTaskTokensWithProvider` variant); write them on first update for a task
- [x] Add `GetDailyTokenUsageByProvider() (map[string]int, error)` to timeline service — query `SELECT provider, SUM(total_tokens) FROM tasks WHERE created_at >= date('now') GROUP BY provider`
- [x] Add `GetTokenUsageSummary(days int) ([]ProviderDayStat, error)` — per-provider per-day totals for trend data
- [x] In-memory rate limit cache in resolver/provider: store last `RemainingTokens`/`RemainingRequests`/`ResetAt` per provider ID, updated on every successful `Chat()` response; safe for concurrent reads
- [x] `kafclaw models stats` command — print table: provider | today tokens | today requests | remaining tokens | remaining requests | reset at
- [x] `kafclaw models stats --days 7` — print per-day per-provider trend from timeline DB
- [x] `kafclaw models stats --json` — machine-readable output
- [x] Doctor check: warn if `RemainingTokens < 10%` of `LimitTokens` for any provider (using cached last-seen value)
- [x] `status` output: include today's total tokens + active provider rate limit remaining inline

### 12 — Security + Tests

- [x] Ensure credentials never appear in logs or status output (mask/redact) — both API keys (tomb) and OAuth tokens (encrypted blob)
- [x] `security check` already covers `auth/*/token.json` encryption — extend walk to include `auth/providers/*/token.json`
- [x] Unit tests for resolver: selection, alias normalization, fallback chain, error on missing config
- [x] Unit tests for credential store: encrypt/decrypt roundtrip, expiry, missing file
- [x] Unit tests for per-agent model spec parsing
- [x] Unit tests for `secrets.EncryptBlob`/`DecryptBlob` after extraction (verify no regression in skills OAuth)
- [x] Unit tests for rate limit header parsing: present, missing, malformed values
- [x] Unit tests for `GetDailyTokenUsageByProvider` and `GetTokenUsageSummary`
- [x] Unit test for in-memory rate limit cache: concurrent update + read safety

### 14 — Chat Middleware Chain (`internal/provider/middleware/`)

The agent loop currently calls `provider.Chat()` directly (`agent/loop.go:1113`). There is no hook to inspect, transform, reroute, or filter messages before or after the LLM call. All four model-adjacent enterprise features (classification routing, prompt guard, output sanitization, FinOps tagging) need a middleware chain inserted at this call site.

#### Architecture

```
Agent Loop
  → checkTokenQuota()            (existing)
  → middleware.Chain.Process()    (NEW)
      ├─ ContentClassifier        (tag sensitivity, task type)
      ├─ PromptGuard              (PII/secret scan → block / redact / reroute)
      ├─ ModelRouter              (override provider based on tags)
      ├─ provider.Chat()          (actual LLM call)
      ├─ OutputSanitizer          (strip PII/secrets from response)
      └─ FinOpsRecorder           (attach cost metadata)
  → trackTokens()                (existing)
```

#### `ChatMiddleware` interface (`internal/provider/middleware/middleware.go`)

```go
// ChatMiddleware intercepts LLM requests and/or responses.
type ChatMiddleware interface {
    // Name returns a short identifier for logging/metrics.
    Name() string
    // ProcessRequest is called before the LLM call. It may modify the request,
    // replace the provider (reroute), or return an error to abort.
    ProcessRequest(ctx context.Context, req *provider.ChatRequest, meta *RequestMeta) error
    // ProcessResponse is called after the LLM call. It may modify the response
    // or return an error to suppress delivery.
    ProcessResponse(ctx context.Context, req *provider.ChatRequest, resp *provider.ChatResponse, meta *RequestMeta) error
}

// RequestMeta carries mutable context through the chain.
type RequestMeta struct {
    ProviderID      string            // resolved provider; middleware can override
    ModelName       string            // resolved model; middleware can override
    SenderID        string
    Channel         string
    MessageType     string            // "internal" / "external"
    Tags            map[string]string // classification tags (e.g. "sensitivity":"pii", "task":"coding")
    Blocked         bool              // set by PromptGuard to abort
    BlockReason     string
    CostUSD         float64           // populated by FinOpsRecorder post-call
}

// Chain holds an ordered list of middleware and the underlying provider.
type Chain struct {
    Middlewares []ChatMiddleware
    Provider    provider.LLMProvider
}
```

#### Tasks

- [x] Create `internal/provider/middleware/middleware.go` — `ChatMiddleware` interface, `RequestMeta`, `Chain` with `Process(ctx, req) (*ChatResponse, error)` that runs pre/post hooks in order
- [x] Wire `Chain` into `agent/loop.go:runAgentLoop` — replace direct `l.provider.Chat()` with `l.chain.Process()`; pass `RequestMeta` with sender/channel/messageType from loop context
- [x] Wire `Chain` into `cli/gateway.go` — pass `Config` to `LoopOptions` for middleware chain setup
- [x] `Chain.Process()` must pass original `provider.LLMProvider` to the call site but allow middleware to swap `meta.ProviderID`/`meta.ModelName` (triggering re-resolve)
- [x] No-op when no middleware is configured — zero-overhead passthrough

### 15 — Content Classification & Model Routing (`internal/provider/middleware/classifier.go`)

Classifies message content and routes to the appropriate provider/model based on sensitivity level and task type. The existing `agent.AssessTask()` (`agent/context.go:360`) already does keyword-based task classification — extend it with sensitivity detection and wire it into routing decisions.

#### Config schema additions (`internal/config/config.go`)

```json
{
  "contentClassification": {
    "enabled": false,
    "sensitivityLevels": {
      "pii": {
        "patterns": ["\\b\\d{3}-\\d{2}-\\d{4}\\b", "\\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\\.[A-Z]{2,}\\b"],
        "keywords": ["social security", "credit card", "passport"],
        "routeTo": "vllm/llama-3.1-70b-instruct"
      },
      "confidential": {
        "keywords": ["internal only", "confidential", "restricted"],
        "routeTo": "scalytics-copilot/default"
      }
    },
    "taskRouting": {
      "coding": {
        "keywords": ["implement", "refactor", "debug", "fix bug", "write code", "function", "class"],
        "routeTo": "openai-codex/gpt-5.3-codex"
      },
      "creative": {
        "keywords": ["brainstorm", "idea", "design", "propose"],
        "routeTo": "claude/claude-opus-4-6"
      }
    },
    "defaultSensitivity": "standard"
  }
}
```

#### Tasks

- [x] Add `ContentClassificationConfig` struct to `internal/config/config.go`: `Enabled`, `SensitivityLevels map[string]SensitivityLevel`, `TaskTypeRoutes map[string]string`
- [x] `SensitivityLevel` struct: `Patterns []string` (regex), `Keywords []string`, `RouteTo string` (provider/model)
- [x] Add `ContentClassification ContentClassificationConfig` to root `Config`
- [x] Create `internal/provider/middleware/classifier.go` — implements `ChatMiddleware`; `ProcessRequest` scans message content against configured patterns/keywords, sets `meta.Tags["sensitivity"]` and `meta.Tags["task"]`, overrides `meta.ProviderID`/`meta.ModelName` when a routing rule matches
- [x] Keyword-based task classification (security, coding, tool-heavy, creative) — mirrors `agent.AssessTask()` categories
- [x] Classification result stored in `meta.Tags` — available to downstream middleware and for timeline audit

### 16 — Prompt Guard (`internal/provider/middleware/promptguard.go`)

Pre-LLM middleware that scans inbound messages for PII, secrets, and prohibited content. Can block, redact, or reroute.

#### Config schema additions

```json
{
  "promptGuard": {
    "enabled": false,
    "mode": "warn",
    "pii": {
      "detect": ["email", "phone", "ssn", "credit_card", "ip_address"],
      "action": "redact",
      "customPatterns": [
        {"name": "employee_id", "pattern": "EMP-\\d{6}"}
      ]
    },
    "secrets": {
      "detect": ["api_key", "bearer_token", "private_key", "password_literal"],
      "action": "block"
    },
    "denyKeywords": {
      "keywords": [],
      "action": "block"
    },
    "onViolation": "log"
  }
}
```

**Modes:** `block` (reject message, return error), `redact` (replace matched content with `[REDACTED:<type>]`), `warn` (log warning, allow), `reroute` (send to private provider from `contentClassification.sensitivityLevels.pii.routeTo`)

#### Tasks

- [x] Add `PromptGuardConfig` struct to `internal/config/config.go`: `Enabled`, `Mode`, `PII PIIConfig`, `Secrets SecretsConfig`, `DenyKeywords`, `CustomPatterns`
- [x] `PIIConfig`: `Detect []string`, `Action string`, `CustomPatterns []NamedPattern`
- [x] `SecretsConfig`: `Detect []string`, `Action string`, `CustomPatterns []NamedPattern`
- [x] Add `PromptGuard PromptGuardConfig` to root `Config`
- [x] Create `internal/provider/middleware/promptguard.go` — implements `ChatMiddleware`
- [x] Built-in PII detectors: `email`, `phone`, `ssn`, `credit_card`, `ip_address`
- [x] Built-in secret detectors: `api_key`, `bearer_token`, `private_key`, `password_literal`
- [x] `ProcessRequest`: scan user-role messages, apply action per match type; block/redact/warn modes
- [x] Log violations to timeline as security events (reuse `LogSecurityEvent` if available)
- [x] `ProcessResponse`: no-op (output scanning is in OutputSanitizer)

### 17 — Output Sanitization (`internal/provider/middleware/sanitizer.go`)

Post-LLM middleware that scans the response content before it reaches the channel delivery path.

#### Config schema additions

```json
{
  "outputSanitization": {
    "enabled": false,
    "redactPII": true,
    "redactSecrets": true,
    "customRedactPatterns": [],
    "denyPatterns": [],
    "maxOutputLength": 0
  }
}
```

#### Tasks

- [x] Add `OutputSanitizationConfig` struct to `internal/config/config.go`: `Enabled`, `RedactPII`, `RedactSecrets`, `CustomRedactPatterns []NamedPattern`, `DenyPatterns []string`, `MaxOutputLength int`
- [x] `NamedPattern` struct (shared with PromptGuard): `Name string`, `Pattern string`
- [x] Add `OutputSanitization OutputSanitizationConfig` to root `Config`
- [x] Create `internal/provider/middleware/sanitizer.go` — implements `ChatMiddleware`
- [x] `ProcessRequest`: no-op
- [x] `ProcessResponse`: scan `resp.Content` for PII/secrets using shared detectors; redact matches; truncate to `MaxOutputLength`; deny patterns replace response
- [x] Shared detector logic extracted to `internal/provider/middleware/detectors.go` — reused by both PromptGuard and OutputSanitizer

### 18 — FinOps Cost Attribution (`internal/provider/middleware/finops.go`)

Post-LLM middleware that attaches dollar cost per request based on provider pricing and records attribution metadata.

#### Config schema additions

```json
{
  "finops": {
    "enabled": false,
    "pricing": {
      "claude": {"promptPer1kTokens": 0.015, "completionPer1kTokens": 0.075},
      "openai": {"promptPer1kTokens": 0.005, "completionPer1kTokens": 0.015},
      "gemini": {"promptPer1kTokens": 0.00025, "completionPer1kTokens": 0.001},
      "xai":    {"promptPer1kTokens": 0.003, "completionPer1kTokens": 0.015},
      "openrouter": {"promptPer1kTokens": 0.0, "completionPer1kTokens": 0.0},
      "deepseek": {"promptPer1kTokens": 0.00014, "completionPer1kTokens": 0.00028},
      "groq":   {"promptPer1kTokens": 0.00005, "completionPer1kTokens": 0.00008}
    },
    "budgets": {
      "daily": 10.00,
      "monthly": 200.00
    },
    "attribution": {
      "tagBy": ["provider", "model", "agent", "channel", "sender"]
    }
  }
}
```

#### Tasks

- [x] Add `FinOpsConfig` struct to `internal/config/config.go`: `Enabled`, `Pricing map[string]ProviderPricing`, `DailyBudget`, `MonthlyBudget`
- [x] `ProviderPricing`: `PromptPer1kTokens float64`, `CompletionPer1kTokens float64`
- [x] Add `FinOps FinOpsConfig` to root `Config`
- [x] Create `internal/provider/middleware/finops.go` — implements `ChatMiddleware`
- [x] `ProcessRequest`: no-op
- [x] `ProcessResponse`: calculate cost, set `meta.CostUSD`, budget warnings
- [x] Add `cost_usd REAL DEFAULT 0` column to `tasks` table (best-effort migration)
- [x] Extend `UpdateTaskTokensWithProvider` to write `cost_usd`
- [x] `kafclaw models stats` — add cost column to output when finops is enabled
- [x] `kafclaw models stats --days 7` — include cost per day per provider

### 19 — Task-Type Model Routing in Resolver (`internal/provider/resolver.go`)

Extend the resolver to dynamically select a model based on `TaskAssessment.Category` when the agent has no explicit per-agent model override. This bridges the existing `AssessTask()` classification with the provider layer.

#### Config schema additions

```json
{
  "model": {
    "name": "claude/claude-sonnet-4-6",
    "taskRouting": {
      "security":     "claude/claude-opus-4-6",
      "tool-heavy":   "openai-codex/gpt-5.3-codex",
      "creative":     "claude/claude-opus-4-6",
      "multi-step":   "claude/claude-sonnet-4-6",
      "quick-answer": "groq/llama-3.3-70b-versatile"
    }
  }
}
```

#### Tasks

- [x] Add `TaskRouting map[string]string` to `ModelConfig` (`internal/config/config.go`)
- [x] Add `ResolveWithTaskType(cfg *config.Config, agentID string, taskCategory string) (LLMProvider, error)` to `internal/provider/resolver.go`
- [x] Wire into `agent/loop.go`: call `agent.AssessTask(message)` early, pass `assessment.Category` to `ResolveWithTaskType` on first iteration
- [x] Log task-type routing decisions to timeline span metadata for observability

---

## Execution Queue (ordered, no-discussion)

1. `internal/secrets/blob.go` — extract `EncryptBlob`/`DecryptBlob`/`LoadOrCreateMasterKey` from `skills/oauth_crypto.go`; update `skills/oauth_crypto.go` to delegate; no behaviour change
2. `config.go` — schema additions (AgentModelSpec, AgentSubagentSpec, XAI, ScalyticsCopilot)
   - **Docs:** `docs/reference/config-keys.md` (new provider blocks + per-agent model spec)
3. `provider/provider.go` — extend `Usage` with rate limit fields (`RemainingTokens`, `RemainingRequests`, `LimitTokens`, `ResetAt`)
4. `provider/credentials/store.go` — `LoadToken`/`SaveToken` (encrypted blob via `secrets`) + `LoadAPIKey`/`SaveAPIKey` (tomb env map)
   - **Docs:** `docs/architecture-security/security-for-ops.md` (credential storage model)
5. `provider/clicache/gemini.go` + `provider/clicache/codex.go`
6. `provider/clinst/install.go`
7. `provider/gemini.go` + `provider/codex.go` + `provider/xai.go` — each parses their own usage + rate limit headers/body and populates `Usage`
8. `provider/openai.go` — add rate limit header parsing to existing `Chat()` (covers openai, openai-codex, xai, scalytics-copilot, openrouter, deepseek, groq in one shot)
9. `provider/resolver.go` — including in-memory rate limit cache updated on every `Chat()` response
10. `timeline/service.go` — add `provider`/`model` columns (migration), `UpdateTaskTokensWithProvider`, `GetDailyTokenUsageByProvider`, `GetTokenUsageSummary`
11. Wire `cli/agent.go` + `cli/gateway.go` — pass `providerID` + `model` through to token tracking
12. `cli/models.go` — `auth login`, `auth set-key`, `list`, `stats` commands
    - **Docs:** `docs/reference/cli-reference.md` (all new `kafclaw models` subcommands) · `docs/start-here/getting-started.md` (provider setup for users) · `docs/operations-admin/admin-guide.md` (provider setup for admins)
13. `onboarding/profile.go` additions
    - **Docs:** `docs/start-here/getting-started.md` (new onboarding presets)
14. `cliconfig/doctor.go` + `cliconfig/security.go` — rate limit warn check + extend `auth/providers/*/token.json` walk
    - **Docs:** `docs/reference/cli-reference.md` (new doctor checks) · `docs/operations-admin/operations-guide.md` (troubleshooting provider issues)
15. Tests
16. `provider/middleware/middleware.go` — `ChatMiddleware` interface, `RequestMeta`, `Chain`; wire into `agent/loop.go` and `cli/gateway.go` replacing direct `provider.Chat()` calls
    - **Docs:** `docs/architecture-security/security-for-ops.md` (middleware chain overview)
17. `config.go` — add `ContentClassificationConfig`, `PromptGuardConfig`, `OutputSanitizationConfig`, `FinOpsConfig` structs; add `TaskRouting` to `ModelConfig`; add `NamedPattern` shared type
    - **Docs:** `docs/reference/config-keys.md` (all new config blocks)
18. `provider/middleware/detectors.go` — shared PII + secret detection (regex-based, used by both PromptGuard and OutputSanitizer)
19. `provider/middleware/classifier.go` — content classification + model routing middleware; integrates with `agent.AssessTask()`
    - **Docs:** `docs/operations-admin/admin-guide.md` (classification rules setup)
20. `provider/middleware/promptguard.go` — PII/secret/deny-keyword scan on inbound messages; block / redact / reroute / warn modes
    - **Docs:** `docs/architecture-security/security-for-ops.md` (prompt guard config) · `docs/operations-admin/admin-guide.md` (PII/secret patterns)
21. `provider/middleware/sanitizer.go` — output redaction post-LLM; shared detectors
    - **Docs:** `docs/architecture-security/security-for-ops.md` (output sanitization)
22. `provider/middleware/finops.go` — cost calculation, budget enforcement, attribution tagging; `cost_usd` column migration
    - **Docs:** `docs/operations-admin/operations-guide.md` (FinOps cost tracking) · `docs/reference/cli-reference.md` (`kafclaw models stats` cost columns)
23. `provider/resolver.go` — add `ResolveWithTaskType()` for task-category → model routing
    - **Docs:** `docs/reference/config-keys.md` (`model.taskRouting` block)
24. Tests for middleware chain: classifier, promptguard, sanitizer, finops, task-type routing
