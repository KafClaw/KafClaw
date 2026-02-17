package onboarding

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestApplyModeLocal(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Group.Enabled = true
	cfg.Orchestrator.Enabled = true

	if err := applyMode(cfg, ModeLocal, WizardParams{}); err != nil {
		t.Fatalf("apply mode local: %v", err)
	}
	if cfg.Gateway.Host != "127.0.0.1" {
		t.Fatalf("expected loopback host, got %q", cfg.Gateway.Host)
	}
	if cfg.Group.Enabled || cfg.Orchestrator.Enabled {
		t.Fatal("expected group and orchestrator disabled for local mode")
	}
}

func TestApplyModeLocalKafka(t *testing.T) {
	cfg := config.DefaultConfig()
	err := applyMode(cfg, ModeLocalKafka, WizardParams{
		KafkaBrokers: "localhost:9092",
		GroupName:    "workshop",
		AgentID:      "agent-1",
		Role:         "orchestrator",
	})
	if err != nil {
		t.Fatalf("apply mode local-kafka: %v", err)
	}
	if !cfg.Group.Enabled || !cfg.Orchestrator.Enabled {
		t.Fatal("expected group and orchestrator enabled")
	}
	if cfg.Group.KafkaBrokers != "localhost:9092" {
		t.Fatalf("unexpected kafka brokers: %q", cfg.Group.KafkaBrokers)
	}
	if cfg.Group.GroupName != "workshop" {
		t.Fatalf("unexpected group name: %q", cfg.Group.GroupName)
	}
}

func TestApplyModeRemoteGeneratesToken(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.AuthToken = ""
	if err := applyMode(cfg, ModeRemote, WizardParams{}); err != nil {
		t.Fatalf("apply mode remote: %v", err)
	}
	if cfg.Gateway.Host != "0.0.0.0" {
		t.Fatalf("expected remote bind host, got %q", cfg.Gateway.Host)
	}
	if cfg.Gateway.AuthToken == "" {
		t.Fatal("expected generated auth token")
	}
	if cfg.Group.Enabled || cfg.Orchestrator.Enabled {
		t.Fatal("expected group/orchestrator disabled by default in remote mode")
	}
}

func TestApplyLLMOpenAICompatible(t *testing.T) {
	cfg := config.DefaultConfig()
	err := applyLLM(cfg, LLMPresetOpenAICompatible, bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, WizardParams{
		LLMAPIBase:     "http://localhost:11434/v1",
		LLMToken:       "token",
		LLMModel:       "llama3.1:8b",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("apply llm compatible: %v", err)
	}
	if cfg.Providers.OpenAI.APIBase != "http://localhost:11434/v1" {
		t.Fatalf("unexpected api base: %q", cfg.Providers.OpenAI.APIBase)
	}
	if cfg.Model.Name != "llama3.1:8b" {
		t.Fatalf("unexpected model: %q", cfg.Model.Name)
	}
}

func TestApplyLLMCLITokenDefaultBase(t *testing.T) {
	cfg := config.DefaultConfig()
	err := applyLLM(cfg, LLMPresetCLIToken, bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, WizardParams{
		LLMToken:       "abc123",
		LLMModel:       "anthropic/claude-sonnet-4-5",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("apply llm token: %v", err)
	}
	if cfg.Providers.OpenAI.APIBase != "https://openrouter.ai/api/v1" {
		t.Fatalf("expected openrouter base, got %q", cfg.Providers.OpenAI.APIBase)
	}
	if cfg.Providers.OpenAI.APIKey != "abc123" {
		t.Fatalf("expected token set, got %q", cfg.Providers.OpenAI.APIKey)
	}
}

func TestRunProfileWizardInteractive(t *testing.T) {
	cfg := config.DefaultConfig()
	input := strings.NewReader("2\n2\nhttp://localhost:8000/v1\n\nmistral-small\n")
	out := &bytes.Buffer{}

	err := RunProfileWizard(cfg, input, out, WizardParams{})
	if err != nil {
		t.Fatalf("run profile wizard: %v", err)
	}
	if !cfg.Group.Enabled || !cfg.Orchestrator.Enabled {
		t.Fatal("expected local-kafka mode to enable group/orchestrator")
	}
	if cfg.Providers.OpenAI.APIBase != "http://localhost:8000/v1" {
		t.Fatalf("unexpected api base: %q", cfg.Providers.OpenAI.APIBase)
	}
	if cfg.Model.Name != "mistral-small" {
		t.Fatalf("unexpected model: %q", cfg.Model.Name)
	}
}

func TestResolveModeNonInteractiveDefault(t *testing.T) {
	mode, err := resolveMode(bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, WizardParams{NonInteractive: true})
	if err != nil {
		t.Fatalf("resolve mode: %v", err)
	}
	if mode != ModeLocal {
		t.Fatalf("expected local default mode, got %q", mode)
	}
}

func TestResolveModeUsesProfileAlias(t *testing.T) {
	mode, err := resolveMode(bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, WizardParams{Profile: "local-kafka"})
	if err != nil {
		t.Fatalf("resolve mode with profile: %v", err)
	}
	if mode != ModeLocalKafka {
		t.Fatalf("expected local-kafka mode, got %q", mode)
	}
}

func TestNormalizeHelpers(t *testing.T) {
	if normalizeMode("local+kafka") != ModeLocalKafka {
		t.Fatal("expected local+kafka normalization")
	}
	if normalizeMode("remote-gateway") != ModeRemote {
		t.Fatal("expected remote normalization")
	}
	if normalizeLLMPreset("ollama") != LLMPresetOpenAICompatible {
		t.Fatal("expected ollama -> openai-compatible")
	}
	if normalizeLLMPreset("token") != LLMPresetCLIToken {
		t.Fatal("expected token -> cli-token")
	}
}

func TestApplyLLMCompatibleRequiresBaseInNonInteractive(t *testing.T) {
	cfg := config.DefaultConfig()
	err := applyLLM(cfg, LLMPresetOpenAICompatible, bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, WizardParams{
		NonInteractive: true,
	})
	if err == nil {
		t.Fatal("expected error for missing api base in non-interactive mode")
	}
}

func TestResolveModeInvalidChoice(t *testing.T) {
	_, err := resolveMode(bufio.NewReader(strings.NewReader("9\n")), &bytes.Buffer{}, WizardParams{})
	if err == nil {
		t.Fatal("expected invalid mode choice error")
	}
}

func TestResolveLLMPresetInvalidChoice(t *testing.T) {
	_, err := resolveLLMPreset(bufio.NewReader(strings.NewReader("7\n")), &bytes.Buffer{}, WizardParams{})
	if err == nil {
		t.Fatal("expected invalid llm choice error")
	}
}

func TestResolveLLMPresetNonInteractiveDefault(t *testing.T) {
	preset, err := resolveLLMPreset(bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, WizardParams{NonInteractive: true})
	if err != nil {
		t.Fatalf("resolve llm preset: %v", err)
	}
	if preset != LLMPresetSkip {
		t.Fatalf("expected skip preset, got %q", preset)
	}
}

func TestRunProfileWizardNonInteractive(t *testing.T) {
	cfg := config.DefaultConfig()
	err := RunProfileWizard(cfg, strings.NewReader(""), &bytes.Buffer{}, WizardParams{
		Mode:           "remote",
		LLMPreset:      "cli-token",
		LLMToken:       "abc",
		LLMModel:       "my-model",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("run profile wizard non-interactive: %v", err)
	}
	if cfg.Gateway.Host != "0.0.0.0" {
		t.Fatalf("expected remote host, got %q", cfg.Gateway.Host)
	}
	if cfg.Providers.OpenAI.APIKey != "abc" {
		t.Fatalf("expected api key set, got %q", cfg.Providers.OpenAI.APIKey)
	}
}

func TestApplyModeAndPresetValidation(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := applyMode(cfg, RuntimeMode("invalid"), WizardParams{}); err == nil {
		t.Fatal("expected invalid mode error")
	}
	if err := applyLLM(cfg, LLMPreset("invalid"), bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, WizardParams{}); err == nil {
		t.Fatal("expected invalid llm preset error")
	}
}

func TestApplySubagentTuning(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := RunProfileWizard(cfg, strings.NewReader(""), &bytes.Buffer{}, WizardParams{
		Mode:             "local",
		LLMPreset:        "skip",
		SubMaxSpawnDepth: 3,
		SubMaxChildren:   9,
		SubMaxConcurrent: 4,
		SubArchiveMins:   120,
		SubAllowAgents:   "agent-a,agent-b,*",
		SubModel:         "anthropic/test-model",
		SubThinking:      "medium",
		NonInteractive:   true,
	}); err != nil {
		t.Fatalf("run profile wizard: %v", err)
	}
	if cfg.Tools.Subagents.MaxSpawnDepth != 3 {
		t.Fatalf("expected max spawn depth 3, got %d", cfg.Tools.Subagents.MaxSpawnDepth)
	}
	if cfg.Tools.Subagents.MaxChildrenPerAgent != 9 {
		t.Fatalf("expected max children 9, got %d", cfg.Tools.Subagents.MaxChildrenPerAgent)
	}
	if cfg.Tools.Subagents.MaxConcurrent != 4 {
		t.Fatalf("expected max concurrent 4, got %d", cfg.Tools.Subagents.MaxConcurrent)
	}
	if cfg.Tools.Subagents.ArchiveAfterMinutes != 120 {
		t.Fatalf("expected archive mins 120, got %d", cfg.Tools.Subagents.ArchiveAfterMinutes)
	}
	if cfg.Tools.Subagents.Model != "anthropic/test-model" {
		t.Fatalf("expected subagent model override, got %q", cfg.Tools.Subagents.Model)
	}
	if cfg.Tools.Subagents.Thinking != "medium" {
		t.Fatalf("expected subagent thinking override, got %q", cfg.Tools.Subagents.Thinking)
	}
	if len(cfg.Tools.Subagents.AllowAgents) != 3 || cfg.Tools.Subagents.AllowAgents[0] != "agent-a" {
		t.Fatalf("expected subagent allowAgents override, got %+v", cfg.Tools.Subagents.AllowAgents)
	}
}

func TestErrorsIsEOF(t *testing.T) {
	if !errorsIsEOF(io.EOF) {
		t.Fatal("expected io.EOF recognized")
	}
	if errorsIsEOF(io.ErrClosedPipe) {
		t.Fatal("expected non-EOF not recognized as EOF")
	}
}

func TestBuildProfileSummaryAndConfirm(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Group.Enabled = false
	cfg.Orchestrator.Enabled = false
	cfg.Gateway.AuthToken = "token"
	cfg.Providers.OpenAI.APIKey = "k"
	cfg.Providers.OpenAI.APIBase = "http://localhost:11434/v1"

	s := BuildProfileSummary(cfg)
	if !strings.Contains(s, "mode: remote") {
		t.Fatalf("expected remote summary, got: %s", s)
	}

	ok, err := ConfirmApply(bufio.NewReader(strings.NewReader("y\n")), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("confirm apply: %v", err)
	}
	if !ok {
		t.Fatal("expected confirmation true for y")
	}
}
