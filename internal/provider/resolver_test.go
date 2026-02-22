package provider

import (
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
)

// ---------------------------------------------------------------------------
// ParseModelString
// ---------------------------------------------------------------------------

func TestParseModelString(t *testing.T) {
	tests := []struct {
		input      string
		wantProvID string
		wantModel  string
	}{
		{"claude/claude-sonnet-4-5", "claude", "claude-sonnet-4-5"},
		{"openai/gpt-4.1", "openai", "gpt-4.1"},
		{"openrouter/anthropic/claude-sonnet-4-5", "openrouter", "anthropic/claude-sonnet-4-5"},
		{"bare-model-name", "", "bare-model-name"},
		{"", "", ""},
		{"  gemini/gemini-2.5-pro  ", "gemini", "gemini-2.5-pro"},
	}
	for _, tt := range tests {
		provID, model := ParseModelString(tt.input)
		if provID != tt.wantProvID || model != tt.wantModel {
			t.Errorf("ParseModelString(%q) = (%q, %q), want (%q, %q)",
				tt.input, provID, model, tt.wantProvID, tt.wantModel)
		}
	}
}

// ---------------------------------------------------------------------------
// NormalizeProviderID
// ---------------------------------------------------------------------------

func TestNormalizeProviderID(t *testing.T) {
	cfg := config.DefaultConfig()
	tests := []struct {
		input string
		want  string
	}{
		{"google", "gemini-cli"},
		{"codex", "openai-codex"},
		{"anthropic", "claude"},
		{"copilot", "scalytics-copilot"},
		{"grok", "xai"},
		{"CLAUDE", "claude"},
		{"  OpenAI  ", "openai"},
		{"deepseek", "deepseek"},
	}
	for _, tt := range tests {
		got := NormalizeProviderID(tt.input, cfg)
		if got != tt.want {
			t.Errorf("NormalizeProviderID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeProviderID_GoogleWithGeminiKey(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.Gemini.APIKey = "test-key"
	got := NormalizeProviderID("google", cfg)
	if got != "gemini" {
		t.Errorf("NormalizeProviderID(google) with Gemini key = %q, want %q", got, "gemini")
	}
}

// ---------------------------------------------------------------------------
// Resolve
// ---------------------------------------------------------------------------

func TestResolve_LegacyFallback(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "test-key"
	cfg.Model.Name = ""
	prov, err := Resolve(cfg, "main")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider for legacy fallback")
	}
	if oaiProv.apiKey != "test-key" {
		t.Errorf("expected api key 'test-key', got %q", oaiProv.apiKey)
	}
}

func TestResolve_BareModelName(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "test-key"
	cfg.Model.Name = "gpt-4.1"
	prov, err := Resolve(cfg, "main")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider for bare model name")
	}
	if oaiProv.defaultModel != "gpt-4.1" {
		t.Errorf("expected model 'gpt-4.1', got %q", oaiProv.defaultModel)
	}
}

func TestResolve_OpenAIProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Model.Name = "openai/gpt-4.1"
	prov, err := Resolve(cfg, "main")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if _, ok := prov.(*OpenAIProvider); !ok {
		t.Fatal("expected OpenAIProvider")
	}
}

func TestResolve_ClaudeProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.Anthropic.APIKey = "sk-ant-test"
	cfg.Model.Name = "claude/claude-sonnet-4-5"
	prov, err := Resolve(cfg, "main")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if _, ok := prov.(*OpenAIProvider); !ok {
		t.Fatal("expected OpenAIProvider for claude (uses OpenAI-compatible API)")
	}
}

func TestResolve_PerAgentModel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Providers.Anthropic.APIKey = "sk-ant-test"
	cfg.Model.Name = "openai/gpt-4.1"
	cfg.Agents = &config.AgentsConfig{
		List: []config.AgentListEntry{
			{
				ID:    "main",
				Model: &config.AgentModelSpec{Primary: "claude/claude-opus-4"},
			},
		},
	}
	prov, err := Resolve(cfg, "main")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider")
	}
	if oaiProv.defaultModel != "claude-opus-4" {
		t.Errorf("expected model 'claude-opus-4', got %q", oaiProv.defaultModel)
	}
}

func TestResolve_UnknownProviderFallsBackToLegacy(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Model.Name = "unknownprov/some-model"
	// Since no per-agent model, should fall back to legacy OpenAI
	prov, err := Resolve(cfg, "main")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if _, ok := prov.(*OpenAIProvider); !ok {
		t.Fatal("expected OpenAIProvider for legacy fallback")
	}
}

func TestResolve_UnknownProviderPerAgentFails(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents = &config.AgentsConfig{
		List: []config.AgentListEntry{
			{
				ID:    "main",
				Model: &config.AgentModelSpec{Primary: "unknownprov/some-model"},
			},
		},
	}
	_, err := Resolve(cfg, "main")
	if err == nil {
		t.Fatal("expected error for unknown provider with per-agent config")
	}
	provErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T", err)
	}
	if provErr.Provider != "unknownprov" {
		t.Errorf("expected provider 'unknownprov', got %q", provErr.Provider)
	}
}

// ---------------------------------------------------------------------------
// ResolveSubagent
// ---------------------------------------------------------------------------

func TestResolveSubagent_InheritsParent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Model.Name = "openai/gpt-4.1"
	prov, err := ResolveSubagent(cfg, "main")
	if err != nil {
		t.Fatalf("ResolveSubagent() error: %v", err)
	}
	if _, ok := prov.(*OpenAIProvider); !ok {
		t.Fatal("expected OpenAIProvider")
	}
}

func TestResolveSubagent_GlobalSubagentModel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Tools.Subagents.Model = "openai/gpt-4.1-mini"
	prov, err := ResolveSubagent(cfg, "main")
	if err != nil {
		t.Fatalf("ResolveSubagent() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider")
	}
	if oaiProv.defaultModel != "gpt-4.1-mini" {
		t.Errorf("expected model 'gpt-4.1-mini', got %q", oaiProv.defaultModel)
	}
}

// ---------------------------------------------------------------------------
// ResolveFallbacks
// ---------------------------------------------------------------------------

func TestResolveFallbacks_None(t *testing.T) {
	cfg := config.DefaultConfig()
	fbs := ResolveFallbacks(cfg, "main")
	if len(fbs) != 0 {
		t.Errorf("expected no fallbacks, got %d", len(fbs))
	}
}

func TestResolveFallbacks_WithAgentFallbacks(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Providers.Anthropic.APIKey = "sk-ant-test"
	cfg.Agents = &config.AgentsConfig{
		List: []config.AgentListEntry{
			{
				ID: "main",
				Model: &config.AgentModelSpec{
					Primary:   "claude/claude-sonnet-4-5",
					Fallbacks: []string{"openai/gpt-4.1"},
				},
			},
		},
	}
	fbs := ResolveFallbacks(cfg, "main")
	if len(fbs) != 1 {
		t.Fatalf("expected 1 fallback, got %d", len(fbs))
	}
}

// ---------------------------------------------------------------------------
// buildProvider
// ---------------------------------------------------------------------------

func TestBuildProvider_GeminiRequiresKey(t *testing.T) {
	cfg := config.DefaultConfig()
	_, err := buildProvider(cfg, "gemini", "gemini-2.5-pro")
	if err == nil {
		t.Fatal("expected error for gemini without API key")
	}
}

func TestBuildProvider_DeepSeekDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.DeepSeek.APIKey = "test-key"
	prov, err := buildProvider(cfg, "deepseek", "deepseek-chat")
	if err != nil {
		t.Fatalf("buildProvider() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider for deepseek")
	}
	if oaiProv.apiBase != "https://api.deepseek.com/v1" {
		t.Errorf("expected deepseek default base, got %q", oaiProv.apiBase)
	}
}

func TestBuildProvider_GroqDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.Groq.APIKey = "test-key"
	prov, err := buildProvider(cfg, "groq", "llama-3.3-70b")
	if err != nil {
		t.Fatalf("buildProvider() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider for groq")
	}
	if oaiProv.apiBase != "https://api.groq.com/openai/v1" {
		t.Errorf("expected groq default base, got %q", oaiProv.apiBase)
	}
}

func TestBuildProvider_VLLMRequiresBase(t *testing.T) {
	cfg := config.DefaultConfig()
	_, err := buildProvider(cfg, "vllm", "my-model")
	if err == nil {
		t.Fatal("expected error for vllm without apiBase")
	}
}

func TestBuildProvider_ScalyticsCopilotRequiresKeyAndBase(t *testing.T) {
	cfg := config.DefaultConfig()
	_, err := buildProvider(cfg, "scalytics-copilot", "model")
	if err == nil {
		t.Fatal("expected error for scalytics-copilot without key")
	}

	cfg.Providers.ScalyticsCopilot.APIKey = "test"
	_, err = buildProvider(cfg, "scalytics-copilot", "model")
	if err == nil {
		t.Fatal("expected error for scalytics-copilot without base")
	}
}

func TestBuildProvider_UnknownProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	_, err := buildProvider(cfg, "nonexistent", "model")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	provErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected *ProviderError, got %T", err)
	}
	if provErr.Provider != "nonexistent" {
		t.Errorf("expected provider 'nonexistent', got %q", provErr.Provider)
	}
}

// ---------------------------------------------------------------------------
// Rate limit cache
// ---------------------------------------------------------------------------

func TestRateLimitCache(t *testing.T) {
	// Clean slate
	rateLimitMu.Lock()
	rateLimitCache = map[string]*RateLimitSnapshot{}
	rateLimitMu.Unlock()

	// Nil usage should be no-op
	UpdateRateLimitCache("test", nil)
	if snap := GetRateLimitSnapshot("test"); snap != nil {
		t.Fatal("expected nil snapshot for no data")
	}

	// Empty usage fields should be no-op
	UpdateRateLimitCache("test", &Usage{})
	if snap := GetRateLimitSnapshot("test"); snap != nil {
		t.Fatal("expected nil snapshot for empty usage")
	}

	// With data
	remaining := 500
	UpdateRateLimitCache("test", &Usage{
		RemainingTokens: &remaining,
	})
	snap := GetRateLimitSnapshot("test")
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.RemainingTokens == nil || *snap.RemainingTokens != 500 {
		t.Errorf("expected remaining tokens 500, got %v", snap.RemainingTokens)
	}
	if snap.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}

	// AllRateLimitSnapshots
	all := AllRateLimitSnapshots()
	if len(all) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(all))
	}
	// Verify it's a copy
	all["test"].RemainingTokens = nil
	origSnap := GetRateLimitSnapshot("test")
	if origSnap.RemainingTokens == nil {
		t.Error("AllRateLimitSnapshots should return copies")
	}
}

// ---------------------------------------------------------------------------
// ProviderError
// ---------------------------------------------------------------------------

func TestProviderError(t *testing.T) {
	err := &ProviderError{Provider: "test", Hint: "missing key"}
	msg := err.Error()
	if msg != `provider "test": missing key` {
		t.Errorf("unexpected error message: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// hasPerAgentModel
// ---------------------------------------------------------------------------

func TestHasPerAgentModel(t *testing.T) {
	cfg := config.DefaultConfig()
	if hasPerAgentModel(cfg, "main") {
		t.Error("expected false for nil Agents")
	}

	cfg.Agents = &config.AgentsConfig{
		List: []config.AgentListEntry{
			{ID: "main"},
		},
	}
	if hasPerAgentModel(cfg, "main") {
		t.Error("expected false for nil Model")
	}

	cfg.Agents.List[0].Model = &config.AgentModelSpec{Primary: "openai/gpt-4.1"}
	if !hasPerAgentModel(cfg, "main") {
		t.Error("expected true for configured model")
	}

	if hasPerAgentModel(cfg, "other") {
		t.Error("expected false for non-matching agent ID")
	}
}

// ---------------------------------------------------------------------------
// resolveModelString
// ---------------------------------------------------------------------------

func TestResolveModelString(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Model.Name = "openai/gpt-4.1"

	// Global fallback
	if got := resolveModelString(cfg, "main"); got != "openai/gpt-4.1" {
		t.Errorf("expected global model, got %q", got)
	}

	// Per-agent override
	cfg.Agents = &config.AgentsConfig{
		List: []config.AgentListEntry{
			{
				ID:    "main",
				Model: &config.AgentModelSpec{Primary: "claude/claude-opus-4"},
			},
		},
	}
	if got := resolveModelString(cfg, "main"); got != "claude/claude-opus-4" {
		t.Errorf("expected per-agent model, got %q", got)
	}

	// Empty model.name
	cfg2 := config.DefaultConfig()
	cfg2.Model.Name = ""
	if got := resolveModelString(cfg2, "main"); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// RateLimitSnapshot with ResetAt
// ---------------------------------------------------------------------------

func TestRateLimitCacheWithResetAt(t *testing.T) {
	rateLimitMu.Lock()
	rateLimitCache = map[string]*RateLimitSnapshot{}
	rateLimitMu.Unlock()

	resetTime := time.Now().Add(5 * time.Minute)
	remaining := 100
	UpdateRateLimitCache("provider-x", &Usage{
		RemainingTokens: &remaining,
		ResetAt:         &resetTime,
	})
	snap := GetRateLimitSnapshot("provider-x")
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.ResetAt == nil {
		t.Fatal("expected non-nil ResetAt")
	}
	if !snap.ResetAt.Equal(resetTime) {
		t.Errorf("expected reset time %v, got %v", resetTime, *snap.ResetAt)
	}
}

// ---------------------------------------------------------------------------
// ResolveWithTaskType
// ---------------------------------------------------------------------------

func TestResolveWithTaskType_NoRouting(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Model.Name = "openai/gpt-4.1"
	prov, err := ResolveWithTaskType(cfg, "main", "security")
	if err != nil {
		t.Fatalf("ResolveWithTaskType() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider")
	}
	if oaiProv.defaultModel != "gpt-4.1" {
		t.Errorf("expected default model without routing, got %q", oaiProv.defaultModel)
	}
}

func TestResolveWithTaskType_WithRouting(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Providers.Anthropic.APIKey = "sk-ant-test"
	cfg.Model.Name = "openai/gpt-4.1"
	cfg.Model.TaskRouting = map[string]string{
		"security": "claude/claude-opus-4-6",
	}
	prov, err := ResolveWithTaskType(cfg, "main", "security")
	if err != nil {
		t.Fatalf("ResolveWithTaskType() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider")
	}
	if oaiProv.defaultModel != "claude-opus-4-6" {
		t.Errorf("expected routed model 'claude-opus-4-6', got %q", oaiProv.defaultModel)
	}
}

func TestResolveWithTaskType_PerAgentOverridesRouting(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Providers.Anthropic.APIKey = "sk-ant-test"
	cfg.Model.Name = "openai/gpt-4.1"
	cfg.Model.TaskRouting = map[string]string{
		"security": "claude/claude-opus-4-6",
	}
	cfg.Agents = &config.AgentsConfig{
		List: []config.AgentListEntry{
			{
				ID:    "main",
				Model: &config.AgentModelSpec{Primary: "openai/gpt-4.1"},
			},
		},
	}
	prov, err := ResolveWithTaskType(cfg, "main", "security")
	if err != nil {
		t.Fatalf("ResolveWithTaskType() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider")
	}
	// Per-agent model should win over task routing
	if oaiProv.defaultModel != "gpt-4.1" {
		t.Errorf("expected per-agent model to win, got %q", oaiProv.defaultModel)
	}
}

func TestResolveWithTaskType_EmptyCategory(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers.OpenAI.APIKey = "sk-test"
	cfg.Model.Name = "openai/gpt-4.1"
	cfg.Model.TaskRouting = map[string]string{
		"security": "claude/claude-opus-4-6",
	}
	prov, err := ResolveWithTaskType(cfg, "main", "")
	if err != nil {
		t.Fatalf("ResolveWithTaskType() error: %v", err)
	}
	oaiProv, ok := prov.(*OpenAIProvider)
	if !ok {
		t.Fatal("expected OpenAIProvider")
	}
	if oaiProv.defaultModel != "gpt-4.1" {
		t.Errorf("expected default model for empty category, got %q", oaiProv.defaultModel)
	}
}
