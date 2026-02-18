package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/session"
	"github.com/KafClaw/KafClaw/internal/tools"
)

func TestContextBuilder(t *testing.T) {
	// Setup temp workspace
	tmpDir := t.TempDir()

	// Create identity file
	os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("Bootstrap Content"), 0644)

	// Create memory
	os.Mkdir(filepath.Join(tmpDir, "memory"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "memory", "MEMORY.md"), []byte("Test Memory"), 0644)

	// Create registry
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool())

	builder := NewContextBuilder(tmpDir, "", "", registry)
	systemPrompt := builder.BuildSystemPrompt()

	// Verify content
	if !strings.Contains(systemPrompt, "KafClaw") {
		t.Error("System prompt missing identity")
	}
	if !strings.Contains(systemPrompt, "Bootstrap Content") {
		t.Error("System prompt missing bootstrap content")
	}
	if !strings.Contains(systemPrompt, "Test Memory") {
		t.Error("System prompt missing memory")
	}
	if !strings.Contains(systemPrompt, "read_file") {
		t.Error("System prompt missing tools summary")
	}
}

func TestBuildMessages(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewContextBuilder(tmpDir, "", "", tools.NewRegistry())
	sess := session.NewSession("test:123")

	// Session has one previous message
	sess.AddMessage("user", "Previous msg")

	// We are processing a new message "Current msg"
	// But note: Loop.ProcessDirect ADDS the current message to the session before calling BuildMessages
	// So let's simulate that
	sess.AddMessage("user", "Current msg")

	msgs := builder.BuildMessages(sess, "Current msg", "cli", "default", "")

	// Expect:
	// 1. System
	// 2. User (Previous msg)
	// 3. User (Current msg)

	if len(msgs) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(msgs))
	}

	if msgs[0].Role != "system" {
		t.Error("First message should be system")
	}
	if msgs[1].Content != "Previous msg" {
		t.Errorf("Second message content mismatch: %s", msgs[1].Content)
	}
	if msgs[2].Content != "Current msg" {
		t.Errorf("Third message content mismatch: %s", msgs[2].Content)
	}
}

func TestBuildMessagesInternalContext(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewContextBuilder(tmpDir, "", "", tools.NewRegistry())
	sess := session.NewSession("test:int")
	sess.AddMessage("user", "hello")

	msgs := builder.BuildMessages(sess, "hello", "whatsapp", "owner@s.whatsapp.net", "internal")

	if len(msgs) == 0 {
		t.Fatal("Expected messages")
	}
	system := msgs[0].Content
	if !strings.Contains(system, "INTERNAL message from the bot owner") {
		t.Error("System prompt should contain internal request context")
	}
	if strings.Contains(system, "EXTERNAL request") {
		t.Error("System prompt should not contain external request context")
	}
}

func TestBuildMessagesExternalContext(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewContextBuilder(tmpDir, "", "", tools.NewRegistry())
	sess := session.NewSession("test:ext")
	sess.AddMessage("user", "hello")

	msgs := builder.BuildMessages(sess, "hello", "whatsapp", "user@s.whatsapp.net", "external")

	if len(msgs) == 0 {
		t.Fatal("Expected messages")
	}
	system := msgs[0].Content
	if !strings.Contains(system, "EXTERNAL request from an authorized user") {
		t.Error("System prompt should contain external request context")
	}
	if strings.Contains(system, "INTERNAL message") {
		t.Error("System prompt should not contain internal request context")
	}
}

func TestBuildIdentityEnvelope(t *testing.T) {
	tmpDir := t.TempDir()
	soul := "# Soul\n\nKafClaw protects operator intent.\nIt keeps responses concise.\n\n## Extra\nIgnored\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte(soul), 0o644); err != nil {
		t.Fatalf("write soul: %v", err)
	}

	builder := NewContextBuilder(tmpDir, "", "", tools.NewRegistry())
	identity := builder.BuildIdentityEnvelope("agent-1", "KafClaw", "gpt-test")

	if identity.AgentID != "agent-1" || identity.AgentName != "KafClaw" || identity.Model != "gpt-test" {
		t.Fatalf("unexpected identity core fields: %+v", identity)
	}
	if !strings.Contains(identity.SoulSummary, "protects operator intent") {
		t.Fatalf("expected summary from first paragraph, got %q", identity.SoulSummary)
	}
	if len(identity.Capabilities) == 0 {
		t.Fatal("expected default tool capabilities fallback")
	}
	if len(identity.Channels) != 1 || identity.Channels[0] != "cli" {
		t.Fatalf("unexpected channels: %#v", identity.Channels)
	}
	if identity.Status != "active" || identity.JoinedAt == "" {
		t.Fatalf("unexpected status/joined fields: %+v", identity)
	}
}

func TestContextCognitivePromptHintAndSystemRepoPath(t *testing.T) {
	if hint := cognitivePromptHint("convergent"); !strings.Contains(hint, "Convergent") {
		t.Fatalf("missing convergent hint: %q", hint)
	}
	if hint := cognitivePromptHint("divergent"); !strings.Contains(hint, "Divergent") {
		t.Fatalf("missing divergent hint: %q", hint)
	}
	if hint := cognitivePromptHint("critical"); !strings.Contains(hint, "Critical") {
		t.Fatalf("missing critical hint: %q", hint)
	}
	if hint := cognitivePromptHint("systems"); !strings.Contains(hint, "Systems") {
		t.Fatalf("missing systems hint: %q", hint)
	}
	if hint := cognitivePromptHint("adaptive"); hint != "" {
		t.Fatalf("expected empty adaptive hint, got: %q", hint)
	}

	ws := t.TempDir()
	explicit := filepath.Join(ws, "custom-system-repo")
	if err := os.MkdirAll(explicit, 0o755); err != nil {
		t.Fatalf("mkdir explicit: %v", err)
	}
	builder := NewContextBuilder(ws, "", explicit, tools.NewRegistry())
	if got := builder.systemRepoPath(); got != explicit {
		t.Fatalf("expected explicit system repo path %q, got %q", explicit, got)
	}

	fallback := filepath.Join(ws, "Bottibot-REPO-01")
	if err := os.MkdirAll(fallback, 0o755); err != nil {
		t.Fatalf("mkdir fallback: %v", err)
	}
	builder = NewContextBuilder(ws, "", "", tools.NewRegistry())
	if got := builder.systemRepoPath(); got != fallback {
		t.Fatalf("expected fallback path %q, got %q", fallback, got)
	}
}

func TestBuildSkillsSummaryLoadsSystemRepoSkills(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfgJSON := `{
	  "skills": {
	    "enabled": true,
	    "allowSystemRepoSkills": true,
	    "scope": "all"
	  }
	}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfgJSON), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	systemRepo := filepath.Join(tmpDir, "system-repo")
	skillDir := filepath.Join(systemRepo, "skills", "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("demo skill instructions"), 0o644); err != nil {
		t.Fatalf("write skill markdown: %v", err)
	}
	day2dayDir := filepath.Join(systemRepo, "operations", "day2day")
	if err := os.MkdirAll(day2dayDir, 0o755); err != nil {
		t.Fatalf("mkdir day2day dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(day2dayDir, "README.md"), []byte("daily guidance"), 0o644); err != nil {
		t.Fatalf("write day2day markdown: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool())
	builder := NewContextBuilder(tmpDir, "", systemRepo, registry)
	summary := builder.buildSkillsSummary()

	if !strings.Contains(summary, "System repo skills:") {
		t.Fatalf("expected system repo section in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "demo skill instructions") {
		t.Fatalf("expected skill markdown in summary, got: %s", summary)
	}
	if !strings.Contains(summary, "Day2Day Guidance") || !strings.Contains(summary, "daily guidance") {
		t.Fatalf("expected day2day guidance in summary, got: %s", summary)
	}
}
