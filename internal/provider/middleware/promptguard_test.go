package middleware

import (
	"context"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

func TestPromptGuard_Disabled(t *testing.T) {
	g := NewPromptGuard(config.PromptGuardConfig{Enabled: false})
	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "123-45-6789"}}}
	if err := g.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if meta.Blocked {
		t.Error("expected not blocked when disabled")
	}
}

func TestPromptGuard_WarnMode(t *testing.T) {
	g := NewPromptGuard(config.PromptGuardConfig{
		Enabled: true,
		Mode:    "warn",
		PII:     config.PIIConfig{Detect: []string{"ssn"}},
	})
	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "SSN: 123-45-6789"}}}
	if err := g.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if meta.Blocked {
		t.Error("expected not blocked in warn mode")
	}
	if meta.Tags["prompt_guard"] != "detected" {
		t.Errorf("expected tag=detected, got %q", meta.Tags["prompt_guard"])
	}
}

func TestPromptGuard_BlockMode(t *testing.T) {
	g := NewPromptGuard(config.PromptGuardConfig{
		Enabled: true,
		Mode:    "block",
		PII:     config.PIIConfig{Detect: []string{"ssn"}},
	})
	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "SSN: 123-45-6789"}}}
	if err := g.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if !meta.Blocked {
		t.Error("expected blocked in block mode")
	}
}

func TestPromptGuard_RedactMode(t *testing.T) {
	g := NewPromptGuard(config.PromptGuardConfig{
		Enabled: true,
		Mode:    "redact",
		PII:     config.PIIConfig{Detect: []string{"email"}},
	})
	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "Email me at test@example.com"}}}
	if err := g.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if meta.Blocked {
		t.Error("expected not blocked in redact mode")
	}
	if req.Messages[0].Content != "Email me at [REDACTED:EMAIL]" {
		t.Errorf("expected redacted content, got %q", req.Messages[0].Content)
	}
}

func TestPromptGuard_DenyKeywords(t *testing.T) {
	g := NewPromptGuard(config.PromptGuardConfig{
		Enabled:      true,
		Mode:         "warn",
		DenyKeywords: []string{"bomb", "weapon"},
	})
	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "How to make a bomb"}}}
	if err := g.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if !meta.Blocked {
		t.Error("expected blocked for deny keyword")
	}
	if meta.BlockReason == "" {
		t.Error("expected block reason")
	}
}

func TestPromptGuard_NoMatch(t *testing.T) {
	g := NewPromptGuard(config.PromptGuardConfig{
		Enabled: true,
		Mode:    "block",
		PII:     config.PIIConfig{Detect: []string{"ssn"}},
	})
	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "What is the weather?"}}}
	if err := g.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if meta.Blocked {
		t.Error("expected not blocked for clean message")
	}
}

func TestPromptGuard_SkipsSystemMessages(t *testing.T) {
	g := NewPromptGuard(config.PromptGuardConfig{
		Enabled: true,
		Mode:    "block",
		PII:     config.PIIConfig{Detect: []string{"ssn"}},
	})
	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{
		{Role: "system", Content: "SSN: 123-45-6789"},
		{Role: "user", Content: "hello"},
	}}
	if err := g.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if meta.Blocked {
		t.Error("expected not blocked for system message SSN")
	}
}
