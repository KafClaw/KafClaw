package provider

import (
	"github.com/KafClaw/KafClaw/internal/config"
	"testing"
)

func TestProviderConstructionRequiresKeysAndPreservesModel(t *testing.T) {
	for _, id := range []string{"openai", "xai", "openrouter", "deepseek", "groq"} {
		cfg := config.DefaultConfig()
		cfg.Providers.OpenAI.APIKey = ""
		cfg.Providers.XAI.APIKey = ""
		cfg.Providers.OpenRouter.APIKey = ""
		cfg.Providers.DeepSeek.APIKey = ""
		cfg.Providers.Groq.APIKey = ""
		if _, err := buildProvider(cfg, id, "fixture-model"); err == nil {
			t.Fatalf("%s accepted missing key", id)
		}
	}
	cfg := config.DefaultConfig()
	cfg.Providers.Gemini.APIKey = "fixture"
	cfg.Providers.XAI.APIKey = "fixture"
	cfg.Providers.OpenRouter.APIKey = "fixture"
	cfg.Providers.OpenRouter.APIBase = ""
	cfg.Providers.ScalyticsCopilot.APIKey = "fixture"
	cfg.Providers.ScalyticsCopilot.APIBase = "https://example.invalid/v1"
	cfg.Providers.VLLM.APIBase = "https://example.invalid/v1"
	for _, id := range []string{"openai-codex", "gemini", "gemini-cli", "xai", "openrouter", "scalytics-copilot", "vllm"} {
		p, err := buildProvider(cfg, id, "fixture-model")
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if p.DefaultModel() != "fixture-model" {
			t.Fatalf("%s model=%s", id, p.DefaultModel())
		}
	}
	cfg.Tools.Subagents.Model = "global-model"
	p, err := ResolveSubagent(cfg, "worker")
	if err != nil || p.DefaultModel() != "global-model" {
		t.Fatalf("global subagent default: %v %v", p, err)
	}
	for _, model := range []string{"agent-model", "vllm/agent-model"} {
		cfg.Agents = &config.AgentsConfig{List: []config.AgentListEntry{{ID: "other"}, {ID: "worker", Subagents: &config.AgentSubagentSpec{Model: model}}}}
		p, err := ResolveSubagent(cfg, "worker")
		if err != nil || p.DefaultModel() != "agent-model" {
			t.Fatalf("agent override: %v %v", p, err)
		}
	}
}
