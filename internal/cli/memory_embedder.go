package cli

import (
	"context"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

// resolveMemoryEmbedder returns the embedder used by the memory system.
// It is intentionally independent from the chat provider selection path so
// memory can remain available even when the active chat provider has no Embed API.
func resolveMemoryEmbedder(cfg *config.Config, main provider.LLMProvider) (provider.Embedder, string) {
	if cfg == nil {
		return nil, "config unavailable"
	}

	embCfg := cfg.Memory.Embedding
	if !embCfg.Enabled || strings.EqualFold(strings.TrimSpace(embCfg.Provider), "disabled") {
		return nil, "disabled by config"
	}

	// Keep compatibility for custom/"auto-like" values by trying resilient fallbacks.
	providerID := strings.ToLower(strings.TrimSpace(embCfg.Provider))
	if providerID == "" {
		providerID = "auto"
	}

	switch providerID {
	case "openai":
		return provider.NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, ""), "openai"
	case "local-hf", "auto":
		if emb, ok := main.(provider.Embedder); ok {
			return withDefaultEmbeddingModel(emb, embCfg.Model), "main-provider"
		}
		if strings.TrimSpace(cfg.Providers.OpenAI.APIKey) != "" {
			return provider.NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, ""), "openai-fallback"
		}
		return nil, "no embedder available"
	default:
		if emb, ok := main.(provider.Embedder); ok {
			return withDefaultEmbeddingModel(emb, embCfg.Model), "main-provider-fallback"
		}
		if strings.TrimSpace(cfg.Providers.OpenAI.APIKey) != "" {
			return provider.NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, ""), "openai-fallback"
		}
		return nil, "unsupported embedding provider"
	}
}

func withDefaultEmbeddingModel(inner provider.Embedder, model string) provider.Embedder {
	model = strings.TrimSpace(model)
	if inner == nil || model == "" {
		return inner
	}
	return &defaultModelEmbedder{inner: inner, model: model}
}

type defaultModelEmbedder struct {
	inner provider.Embedder
	model string
}

func (d *defaultModelEmbedder) Embed(ctx context.Context, req *provider.EmbeddingRequest) (*provider.EmbeddingResponse, error) {
	if req == nil {
		req = &provider.EmbeddingRequest{}
	}
	clone := *req
	if strings.TrimSpace(clone.Model) == "" {
		clone.Model = d.model
	}
	return d.inner.Embed(ctx, &clone)
}

