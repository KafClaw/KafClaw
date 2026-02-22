package middleware

import (
	"context"
	"log"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

// FinOpsRecorder calculates per-request cost and records attribution metadata.
type FinOpsRecorder struct {
	cfg config.FinOpsConfig
}

// NewFinOpsRecorder builds a recorder from config.
func NewFinOpsRecorder(cfg config.FinOpsConfig) *FinOpsRecorder {
	return &FinOpsRecorder{cfg: cfg}
}

func (f *FinOpsRecorder) Name() string { return "finops" }

func (f *FinOpsRecorder) ProcessRequest(_ context.Context, _ *provider.ChatRequest, _ *RequestMeta) error {
	return nil
}

func (f *FinOpsRecorder) ProcessResponse(_ context.Context, _ *provider.ChatRequest, resp *provider.ChatResponse, meta *RequestMeta) error {
	if !f.cfg.Enabled {
		return nil
	}
	if resp == nil {
		return nil
	}

	provID := meta.ProviderID
	pricing, ok := f.cfg.Pricing[provID]
	if !ok {
		return nil
	}

	promptTokens := float64(resp.Usage.PromptTokens)
	completionTokens := float64(resp.Usage.CompletionTokens)
	cost := (promptTokens*pricing.PromptPer1kTokens + completionTokens*pricing.CompletionPer1kTokens) / 1000.0

	meta.CostUSD = cost

	// Budget warnings (logged only; enforcement is up to the caller).
	if f.cfg.DailyBudget > 0 && cost > f.cfg.DailyBudget*0.1 {
		log.Printf("[finops] single request cost $%.4f exceeds 10%% of daily budget $%.2f for provider %s",
			cost, f.cfg.DailyBudget, provID)
	}

	return nil
}

// CalculateCost computes the USD cost for a given usage and provider.
func (f *FinOpsRecorder) CalculateCost(providerID string, usage provider.Usage) float64 {
	if !f.cfg.Enabled {
		return 0
	}
	pricing, ok := f.cfg.Pricing[providerID]
	if !ok {
		return 0
	}
	return (float64(usage.PromptTokens)*pricing.PromptPer1kTokens +
		float64(usage.CompletionTokens)*pricing.CompletionPer1kTokens) / 1000.0
}
