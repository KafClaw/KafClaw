package middleware

import (
	"context"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

func TestClassifier_Disabled(t *testing.T) {
	cc := NewContentClassifier(config.ContentClassificationConfig{Enabled: false})
	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "123-45-6789"}}}
	if err := cc.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("ProcessRequest error: %v", err)
	}
	if _, ok := meta.Tags["sensitivity"]; ok {
		t.Error("expected no sensitivity tag when disabled")
	}
}

func TestClassifier_SensitivityDetection(t *testing.T) {
	cc := NewContentClassifier(config.ContentClassificationConfig{
		Enabled: true,
		SensitivityLevels: map[string]config.SensitivityLevel{
			"pii": {
				Patterns: []string{`\b\d{3}-\d{2}-\d{4}\b`},
				RouteTo:  "vllm/llama-3.1-70b",
			},
		},
	})

	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "My SSN is 123-45-6789"}}}
	if err := cc.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("ProcessRequest error: %v", err)
	}
	if meta.Tags["sensitivity"] != "pii" {
		t.Errorf("expected sensitivity=pii, got %q", meta.Tags["sensitivity"])
	}
	if meta.ProviderID != "vllm" {
		t.Errorf("expected provider reroute to vllm, got %q", meta.ProviderID)
	}
	if meta.ModelName != "llama-3.1-70b" {
		t.Errorf("expected model reroute, got %q", meta.ModelName)
	}
}

func TestClassifier_KeywordDetection(t *testing.T) {
	cc := NewContentClassifier(config.ContentClassificationConfig{
		Enabled: true,
		SensitivityLevels: map[string]config.SensitivityLevel{
			"confidential": {
				Keywords: []string{"social security", "passport"},
			},
		},
	})

	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "Need my social security number"}}}
	if err := cc.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("ProcessRequest error: %v", err)
	}
	if meta.Tags["sensitivity"] != "confidential" {
		t.Errorf("expected sensitivity=confidential, got %q", meta.Tags["sensitivity"])
	}
}

func TestClassifier_TaskTypeRouting(t *testing.T) {
	cc := NewContentClassifier(config.ContentClassificationConfig{
		Enabled: true,
		TaskTypeRoutes: map[string]string{
			"security": "claude/claude-opus-4-6",
		},
	})

	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "Find the XSS vulnerability in this code"}}}
	if err := cc.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("ProcessRequest error: %v", err)
	}
	if meta.Tags["task"] != "security" {
		t.Errorf("expected task=security, got %q", meta.Tags["task"])
	}
	if meta.ProviderID != "claude" {
		t.Errorf("expected reroute to claude, got %q", meta.ProviderID)
	}
}

func TestClassifier_NoMatch(t *testing.T) {
	cc := NewContentClassifier(config.ContentClassificationConfig{
		Enabled: true,
		SensitivityLevels: map[string]config.SensitivityLevel{
			"pii": {
				Patterns: []string{`\b\d{3}-\d{2}-\d{4}\b`},
			},
		},
	})

	meta := NewRequestMeta("openai", "gpt-4")
	req := &provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: "What is the weather?"}}}
	if err := cc.ProcessRequest(context.Background(), req, meta); err != nil {
		t.Fatalf("ProcessRequest error: %v", err)
	}
	if _, ok := meta.Tags["sensitivity"]; ok {
		t.Error("expected no sensitivity tag")
	}
}

func TestClassifyTaskType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Find the SQL injection vulnerability", "security"},
		{"Write code for a REST API endpoint", "coding"},
		{"Run this bash command", "tool-heavy"},
		{"Write a story about dragons", "creative"},
		{"What is the capital of France?", ""},
	}
	for _, tt := range tests {
		got := classifyTaskType(tt.input)
		if got != tt.want {
			t.Errorf("classifyTaskType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
