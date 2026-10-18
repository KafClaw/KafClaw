package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"github.com/spf13/cobra"
)

func TestSkillCommandsRejectCorruptConfiguration(t *testing.T) {
	root := commandHome(t)
	if err := os.WriteFile(filepath.Join(root, ".kafclaw/config.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	for _, c := range []*cobra.Command{skillsEnableCmd, skillsDisableCmd, skillsListCmd, skillsStatusCmd, skillsVerifyCmd, skillsInstallCmd, skillsUpdateCmd, skillsAuthStartCmd, skillsExecCmd} {
		t.Run(c.Name(), func(t *testing.T) {
			err := c.RunE(cmd, []string{"fixture", "printf"})
			if err == nil || !strings.Contains(err.Error(), "CONFIG_LOAD_FAILED") {
				t.Fatalf("config failure: %v", err)
			}
		})
	}
	if err := setSkillEnabled("fixture", true); err == nil {
		t.Fatal("corrupt config accepted")
	}
	if _, err := propsFromConfig(); err == nil {
		t.Fatal("diagnostics accepted corrupt config")
	}
	if err := runConfigure(cmd, nil); err == nil {
		t.Fatal("configure accepted corrupt config")
	}
}

func TestStatusAndKnowledgeOutputContracts(t *testing.T) {
	commandHome(t)
	cfg := config.DefaultConfig()
	cfg.Model.Name = ""
	cfg.Providers.OpenAI.APIKey = "fixture"
	cfg.Providers.Anthropic.APIKey = "fixture"
	cfg.Providers.Gemini.APIKey = "fixture"
	cfg.Providers.XAI.APIKey = "fixture"
	cfg.Providers.ScalyticsCopilot.APIKey = "fixture"
	cfg.Providers.OpenRouter.APIKey = "fixture"
	cfg.Providers.DeepSeek.APIKey = "fixture"
	cfg.Providers.Groq.APIKey = "fixture"
	cfg.Providers.VLLM.APIBase = "http://localhost"
	cfg.ContentClassification.Enabled = true
	cfg.PromptGuard.Enabled = true
	cfg.OutputSanitization.Enabled = true
	cfg.FinOps.Enabled = true
	remaining, requests := 10, 2
	provider.UpdateRateLimitCache("fixture-status", &provider.Usage{RemainingTokens: &remaining, RemainingRequests: &requests})
	out := captureCommandOutput(t, func() { printProviderStatus(cfg) })
	for _, want := range []string{"legacy openai fallback", "claude", "vllm", "prompt-guard", "tokens=10", "requests=2"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s: %s", want, out)
		}
	}
	for _, mode := range []string{"channel", "account", "room", "thread", "user", "unknown"} {
		if sessionScopeHint("slack", mode) == "" {
			t.Errorf("empty scope hint: %s", mode)
		}
	}
	if estimateKnowledgePoolSize(nil, nil) != 1 {
		t.Fatal("default pool")
	}
	cfg.Knowledge.Voting.MinPoolSize = 4
	if estimateKnowledgePoolSize(nil, cfg) != 4 {
		t.Fatal("configured pool")
	}
	tl, err := loadGroupTimeline()
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	if err := tl.UpsertGroupMember(&timeline.GroupMemberRecord{AgentID: "fixture", LastSeen: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if estimateKnowledgePoolSize(tl, cfg) != 1 {
		t.Fatal("live pool should override configured minimum")
	}
	old := knowledgeJSON
	knowledgeJSON = false
	defer func() { knowledgeJSON = old }()
	var buf bytes.Buffer
	if err := printKnowledgeOutput(&buf, map[string]any{"action": "propose", "proposalId": "proposal", "count": 2}); err != nil || !strings.Contains(buf.String(), "proposal: proposal") || !strings.Contains(buf.String(), "count: 2") {
		t.Fatalf("knowledge text: %s %v", buf.String(), err)
	}
	if len(randomShortID()) < 10 {
		t.Fatal("invalid random ID")
	}
}

func TestAgentCascadeConfigurationBoundaries(t *testing.T) {
	if _, ok := getAgentCascadeEnabled(nil, "main"); ok {
		t.Fatal("nil config has override")
	}
	cfg := config.DefaultConfig()
	cfg.Agents = nil
	if _, ok := getAgentCascadeEnabled(cfg, "main"); ok {
		t.Fatal("missing agents has override")
	}
	setAgentCascadeEnabled(cfg, "main", true)
	setAgentCascadeEnabled(cfg, "other", false)
	setAgentCascadeEnabled(cfg, " main ", false)
	if enabled, ok := getAgentCascadeEnabled(cfg, "main"); !ok || enabled {
		t.Fatal("existing override not updated")
	}
	if _, ok := getAgentCascadeEnabled(cfg, "missing"); ok {
		t.Fatal("missing agent has override")
	}
	cfg.Agents.List = append(cfg.Agents.List, config.AgentListEntry{ID: "unset"})
	if _, ok := getAgentCascadeEnabled(cfg, "unset"); ok {
		t.Fatal("nil cascade treated as configured")
	}
}

func TestModelsListAgentOverrides(t *testing.T) {
	commandHome(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Agents = &config.AgentsConfig{List: []config.AgentListEntry{{ID: "worker", Model: &config.AgentModelSpec{Primary: "openai/test", Fallbacks: []string{"gemini/test"}}, Subagents: &config.AgentSubagentSpec{Model: "openai/small"}}}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	out := captureCommandOutput(t, func() { runModelsList(nil, nil) })
	for _, want := range []string{"worker", "fallback[0]: gemini/test", "subagent model: openai/small"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s: %s", want, out)
		}
	}
}
