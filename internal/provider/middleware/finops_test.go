package middleware

import (
	"context"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

func TestFinOps_Disabled(t *testing.T) {
	f := NewFinOpsRecorder(config.FinOpsConfig{Enabled: false})
	meta := NewRequestMeta("openai", "gpt-4")
	resp := &provider.ChatResponse{
		Usage: provider.Usage{PromptTokens: 100, CompletionTokens: 50},
	}
	if err := f.ProcessResponse(context.Background(), nil, resp, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if meta.CostUSD != 0 {
		t.Errorf("expected zero cost when disabled, got %f", meta.CostUSD)
	}
}

func TestFinOps_CalculatesCost(t *testing.T) {
	f := NewFinOpsRecorder(config.FinOpsConfig{
		Enabled: true,
		Pricing: map[string]config.ProviderPricing{
			"openai": {PromptPer1kTokens: 0.005, CompletionPer1kTokens: 0.015},
		},
	})
	meta := NewRequestMeta("openai", "gpt-4")
	resp := &provider.ChatResponse{
		Usage: provider.Usage{PromptTokens: 1000, CompletionTokens: 500},
	}
	if err := f.ProcessResponse(context.Background(), nil, resp, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	// cost = (1000*0.005 + 500*0.015) / 1000 = (5 + 7.5) / 1000 = 0.0125
	expected := 0.0125
	if meta.CostUSD < expected-0.0001 || meta.CostUSD > expected+0.0001 {
		t.Errorf("expected cost ~%f, got %f", expected, meta.CostUSD)
	}
}

func TestFinOps_NoPricing(t *testing.T) {
	f := NewFinOpsRecorder(config.FinOpsConfig{
		Enabled: true,
		Pricing: map[string]config.ProviderPricing{},
	})
	meta := NewRequestMeta("openai", "gpt-4")
	resp := &provider.ChatResponse{
		Usage: provider.Usage{PromptTokens: 100, CompletionTokens: 50},
	}
	if err := f.ProcessResponse(context.Background(), nil, resp, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if meta.CostUSD != 0 {
		t.Errorf("expected zero cost for unknown provider, got %f", meta.CostUSD)
	}
}

func TestFinOps_CalculateCostDirect(t *testing.T) {
	f := NewFinOpsRecorder(config.FinOpsConfig{
		Enabled: true,
		Pricing: map[string]config.ProviderPricing{
			"claude": {PromptPer1kTokens: 0.015, CompletionPer1kTokens: 0.075},
		},
	})
	cost := f.CalculateCost("claude", provider.Usage{PromptTokens: 2000, CompletionTokens: 1000})
	// (2000*0.015 + 1000*0.075) / 1000 = (30 + 75) / 1000 = 0.105
	expected := 0.105
	if cost < expected-0.001 || cost > expected+0.001 {
		t.Errorf("expected cost ~%f, got %f", expected, cost)
	}
}
