package provider

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
)

// providerAliases maps common aliases to canonical provider IDs.
var providerAliases = map[string]string{
	"google":    "gemini-cli",
	"codex":     "openai-codex",
	"anthropic": "claude",
	"copilot":   "scalytics-copilot",
	"grok":      "xai",
}

// NormalizeProviderID resolves aliases and normalizes the provider ID.
// If "google" is used but an API key exists for gemini, it resolves to "gemini" instead.
func NormalizeProviderID(id string, cfg *config.Config) string {
	lower := strings.ToLower(strings.TrimSpace(id))
	if lower == "google" {
		if cfg != nil && cfg.Providers.Gemini.APIKey != "" {
			return "gemini"
		}
		return "gemini-cli"
	}
	if canonical, ok := providerAliases[lower]; ok {
		return canonical
	}
	return lower
}

// ParseModelString splits a "provider/model" string into provider ID and model name.
// For OpenRouter, the format is "openrouter/vendor/model" (three segments).
func ParseModelString(s string) (providerID, modelName string) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "/", 2)
	if len(parts) < 2 {
		return "", s
	}
	providerID = strings.ToLower(parts[0])
	modelName = parts[1]
	return
}

// Resolve creates the appropriate LLMProvider for the given agent based on config.
// Resolution order:
//  1. agents.list[agentID].model.primary
//  2. model.name (global fallback)
//  3. providers.openai with model.name (legacy compat)
func Resolve(cfg *config.Config, agentID string) (LLMProvider, error) {
	modelStr := resolveModelString(cfg, agentID)
	if modelStr == "" {
		// Legacy fallback: use global OpenAI provider.
		return NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, cfg.Model.Name), nil
	}
	provID, model := ParseModelString(modelStr)
	if provID == "" {
		// Bare model name — use legacy OpenAI path.
		return NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, model), nil
	}
	provID = NormalizeProviderID(provID, cfg)
	prov, err := buildProvider(cfg, provID, model)
	if err != nil {
		// If the model string came from the global model.name (not per-agent config),
		// fall back to legacy OpenAI provider for backward compatibility.
		if !hasPerAgentModel(cfg, agentID) {
			return NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, modelStr), nil
		}
		return nil, err
	}
	return prov, nil
}

// ResolveWithTaskType is like Resolve but checks model.taskRouting first.
// If the task category has a routing override and no per-agent model is set,
// the routed model is used instead of the global default.
func ResolveWithTaskType(cfg *config.Config, agentID, taskCategory string) (LLMProvider, error) {
	// Per-agent model always wins.
	if hasPerAgentModel(cfg, agentID) {
		return Resolve(cfg, agentID)
	}
	// Check task routing.
	if taskCategory != "" && len(cfg.Model.TaskRouting) > 0 {
		if routeModel, ok := cfg.Model.TaskRouting[taskCategory]; ok {
			provID, model := ParseModelString(routeModel)
			if provID != "" {
				provID = NormalizeProviderID(provID, cfg)
				prov, err := buildProvider(cfg, provID, model)
				if err == nil {
					return prov, nil
				}
				// Fall through to normal resolve on error.
			}
		}
	}
	return Resolve(cfg, agentID)
}

// hasPerAgentModel checks if the agent has an explicitly configured model.
func hasPerAgentModel(cfg *config.Config, agentID string) bool {
	if cfg.Agents == nil {
		return false
	}
	for _, entry := range cfg.Agents.List {
		if entry.ID == agentID && entry.Model != nil && entry.Model.Primary != "" {
			return true
		}
	}
	return false
}

// ResolveSubagent creates the LLMProvider for subagents spawned by the given agent.
// Resolution order:
//  1. agents.list[agentID].subagents.model
//  2. tools.subagents.model (global subagent default)
//  3. Inherit parent agent's resolved model (via Resolve)
func ResolveSubagent(cfg *config.Config, agentID string) (LLMProvider, error) {
	if cfg.Agents != nil {
		for _, entry := range cfg.Agents.List {
			if entry.ID == agentID && entry.Subagents != nil && entry.Subagents.Model != "" {
				provID, model := ParseModelString(entry.Subagents.Model)
				if provID == "" {
					return NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, model), nil
				}
				provID = NormalizeProviderID(provID, cfg)
				return buildProvider(cfg, provID, model)
			}
		}
	}
	if cfg.Tools.Subagents.Model != "" {
		provID, model := ParseModelString(cfg.Tools.Subagents.Model)
		if provID == "" {
			return NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, model), nil
		}
		provID = NormalizeProviderID(provID, cfg)
		return buildProvider(cfg, provID, model)
	}
	return Resolve(cfg, agentID)
}

// resolveModelString finds the model string for an agent from config.
func resolveModelString(cfg *config.Config, agentID string) string {
	if cfg.Agents != nil {
		for _, entry := range cfg.Agents.List {
			if entry.ID == agentID && entry.Model != nil && entry.Model.Primary != "" {
				return entry.Model.Primary
			}
		}
	}
	if cfg.Model.Name != "" {
		return cfg.Model.Name
	}
	return ""
}

// ResolveFallbacks returns the fallback providers for a given agent (tried in order on transient errors).
func ResolveFallbacks(cfg *config.Config, agentID string) []LLMProvider {
	if cfg.Agents == nil {
		return nil
	}
	for _, entry := range cfg.Agents.List {
		if entry.ID != agentID || entry.Model == nil {
			continue
		}
		var providers []LLMProvider
		for _, fb := range entry.Model.Fallbacks {
			provID, model := ParseModelString(fb)
			if provID == "" {
				continue
			}
			provID = NormalizeProviderID(provID, cfg)
			p, err := buildProvider(cfg, provID, model)
			if err != nil {
				continue
			}
			providers = append(providers, p)
		}
		return providers
	}
	return nil
}

// buildProvider constructs a provider from its canonical ID and model name.
func buildProvider(cfg *config.Config, providerID, model string) (LLMProvider, error) {
	switch providerID {
	case "claude":
		key := cfg.Providers.Anthropic.APIKey
		base := cfg.Providers.Anthropic.APIBase
		if key == "" {
			return nil, &ProviderError{Provider: "claude", Hint: "set providers.anthropic.apiKey in config or run: kafclaw models auth set-key --provider claude"}
		}
		if base == "" {
			base = "https://api.anthropic.com/v1"
		}
		return NewOpenAIProvider(key, base, model), nil

	case "openai":
		key := cfg.Providers.OpenAI.APIKey
		base := cfg.Providers.OpenAI.APIBase
		if key == "" {
			return nil, &ProviderError{Provider: "openai", Hint: "set providers.openai.apiKey in config or run: kafclaw models auth set-key --provider openai"}
		}
		return NewOpenAIProvider(key, base, model), nil

	case "openai-codex":
		return NewCodexProvider(model), nil

	case "gemini":
		key := cfg.Providers.Gemini.APIKey
		if key == "" {
			return nil, &ProviderError{Provider: "gemini", Hint: "set providers.gemini.apiKey in config or run: kafclaw models auth set-key --provider gemini"}
		}
		return NewGeminiProvider(key, model), nil

	case "gemini-cli":
		return NewGeminiCLIProvider(model), nil

	case "xai":
		key := cfg.Providers.XAI.APIKey
		if key == "" {
			return nil, &ProviderError{Provider: "xai", Hint: "set providers.xai.apiKey in config or run: kafclaw models auth set-key --provider xai"}
		}
		return NewXAIProvider(key, model), nil

	case "scalytics-copilot":
		key := cfg.Providers.ScalyticsCopilot.APIKey
		base := cfg.Providers.ScalyticsCopilot.APIBase
		if key == "" {
			return nil, &ProviderError{Provider: "scalytics-copilot", Hint: "set providers.scalyticsCopilot.apiKey and apiBase in config or run: kafclaw models auth set-key --provider scalytics-copilot --base <url>"}
		}
		if base == "" {
			return nil, &ProviderError{Provider: "scalytics-copilot", Hint: "set providers.scalyticsCopilot.apiBase (e.g. https://copilot.scalytics.io/v1)"}
		}
		return NewOpenAIProvider(key, base, model), nil

	case "openrouter":
		key := cfg.Providers.OpenRouter.APIKey
		base := cfg.Providers.OpenRouter.APIBase
		if key == "" {
			return nil, &ProviderError{Provider: "openrouter", Hint: "set providers.openrouter.apiKey in config or run: kafclaw models auth set-key --provider openrouter"}
		}
		if base == "" {
			base = "https://openrouter.ai/api/v1"
		}
		return NewOpenAIProvider(key, base, model), nil

	case "deepseek":
		key := cfg.Providers.DeepSeek.APIKey
		base := cfg.Providers.DeepSeek.APIBase
		if key == "" {
			return nil, &ProviderError{Provider: "deepseek", Hint: "set providers.deepseek.apiKey in config or run: kafclaw models auth set-key --provider deepseek"}
		}
		if base == "" {
			base = "https://api.deepseek.com/v1"
		}
		return NewOpenAIProvider(key, base, model), nil

	case "groq":
		key := cfg.Providers.Groq.APIKey
		base := cfg.Providers.Groq.APIBase
		if key == "" {
			return nil, &ProviderError{Provider: "groq", Hint: "set providers.groq.apiKey in config or run: kafclaw models auth set-key --provider groq"}
		}
		if base == "" {
			base = "https://api.groq.com/openai/v1"
		}
		return NewOpenAIProvider(key, base, model), nil

	case "vllm":
		base := cfg.Providers.VLLM.APIBase
		key := cfg.Providers.VLLM.APIKey
		if base == "" {
			return nil, &ProviderError{Provider: "vllm", Hint: "set providers.vllm.apiBase in config (e.g. http://localhost:8000/v1)"}
		}
		return NewOpenAIProvider(key, base, model), nil

	default:
		return nil, &ProviderError{Provider: providerID, Hint: fmt.Sprintf("unknown provider ID %q — supported: claude, openai, openai-codex, gemini, gemini-cli, xai, scalytics-copilot, openrouter, deepseek, groq, vllm", providerID)}
	}
}

// ProviderError is returned when a provider cannot be constructed.
type ProviderError struct {
	Provider string
	Hint     string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider %q: %s", e.Provider, e.Hint)
}

// ---------------------------------------------------------------------------
// In-memory rate limit cache
// ---------------------------------------------------------------------------

// RateLimitSnapshot stores the last-seen rate limit data for a provider.
type RateLimitSnapshot struct {
	RemainingTokens   *int
	RemainingRequests *int
	LimitTokens       *int
	ResetAt           *time.Time
	UpdatedAt         time.Time
}

var (
	rateLimitMu    sync.RWMutex
	rateLimitCache = map[string]*RateLimitSnapshot{}
)

// UpdateRateLimitCache updates the cached rate limit data for a provider from a Usage response.
func UpdateRateLimitCache(providerID string, u *Usage) {
	if u == nil {
		return
	}
	if u.RemainingTokens == nil && u.RemainingRequests == nil && u.LimitTokens == nil && u.ResetAt == nil {
		return
	}
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()
	rateLimitCache[providerID] = &RateLimitSnapshot{
		RemainingTokens:   u.RemainingTokens,
		RemainingRequests: u.RemainingRequests,
		LimitTokens:       u.LimitTokens,
		ResetAt:           u.ResetAt,
		UpdatedAt:         time.Now(),
	}
}

// GetRateLimitSnapshot returns the last-seen rate limit data for a provider.
func GetRateLimitSnapshot(providerID string) *RateLimitSnapshot {
	rateLimitMu.RLock()
	defer rateLimitMu.RUnlock()
	return rateLimitCache[providerID]
}

// AllRateLimitSnapshots returns a copy of all cached rate limit snapshots.
func AllRateLimitSnapshots() map[string]*RateLimitSnapshot {
	rateLimitMu.RLock()
	defer rateLimitMu.RUnlock()
	out := make(map[string]*RateLimitSnapshot, len(rateLimitCache))
	for k, v := range rateLimitCache {
		cp := *v
		out[k] = &cp
	}
	return out
}
