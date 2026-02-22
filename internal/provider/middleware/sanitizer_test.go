package middleware

import (
	"context"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

func TestSanitizer_Disabled(t *testing.T) {
	s := NewOutputSanitizer(config.OutputSanitizationConfig{Enabled: false})
	meta := NewRequestMeta("openai", "gpt-4")
	resp := &provider.ChatResponse{Content: "SSN: 123-45-6789"}
	if err := s.ProcessResponse(context.Background(), nil, resp, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "SSN: 123-45-6789" {
		t.Error("expected content unchanged when disabled")
	}
}

func TestSanitizer_RedactPII(t *testing.T) {
	s := NewOutputSanitizer(config.OutputSanitizationConfig{
		Enabled:   true,
		RedactPII: true,
	})
	meta := NewRequestMeta("openai", "gpt-4")
	resp := &provider.ChatResponse{Content: "Contact test@example.com for help"}
	if err := s.ProcessResponse(context.Background(), nil, resp, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "Contact [REDACTED:EMAIL] for help" {
		t.Errorf("expected redacted email, got %q", resp.Content)
	}
	if meta.Tags["output_sanitized"] != "redacted" {
		t.Errorf("expected tag=redacted, got %q", meta.Tags["output_sanitized"])
	}
}

func TestSanitizer_RedactSecrets(t *testing.T) {
	s := NewOutputSanitizer(config.OutputSanitizationConfig{
		Enabled:       true,
		RedactSecrets: true,
	})
	meta := NewRequestMeta("openai", "gpt-4")
	resp := &provider.ChatResponse{Content: "Key: sk-abcdefghijklmnopqrstuvwx done"}
	if err := s.ProcessResponse(context.Background(), nil, resp, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "Key: [REDACTED:API_KEY] done" {
		t.Errorf("expected redacted key, got %q", resp.Content)
	}
}

func TestSanitizer_DenyPatterns(t *testing.T) {
	s := NewOutputSanitizer(config.OutputSanitizationConfig{
		Enabled:      true,
		DenyPatterns: []string{`(?i)forbidden\s+content`},
	})
	meta := NewRequestMeta("openai", "gpt-4")
	resp := &provider.ChatResponse{Content: "This contains Forbidden Content that should be blocked"}
	if err := s.ProcessResponse(context.Background(), nil, resp, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "[Response filtered by output sanitizer]" {
		t.Errorf("expected filtered response, got %q", resp.Content)
	}
}

func TestSanitizer_MaxOutputLength(t *testing.T) {
	s := NewOutputSanitizer(config.OutputSanitizationConfig{
		Enabled:         true,
		MaxOutputLength: 20,
	})
	meta := NewRequestMeta("openai", "gpt-4")
	resp := &provider.ChatResponse{Content: "This is a very long response that exceeds the maximum length"}
	if err := s.ProcessResponse(context.Background(), nil, resp, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(resp.Content) > 60 {
		t.Errorf("expected truncated response, got len=%d", len(resp.Content))
	}
}

func TestSanitizer_NoMatch(t *testing.T) {
	s := NewOutputSanitizer(config.OutputSanitizationConfig{
		Enabled:   true,
		RedactPII: true,
	})
	meta := NewRequestMeta("openai", "gpt-4")
	resp := &provider.ChatResponse{Content: "Safe response with no PII"}
	if err := s.ProcessResponse(context.Background(), nil, resp, meta); err != nil {
		t.Fatalf("error: %v", err)
	}
	if resp.Content != "Safe response with no PII" {
		t.Errorf("expected unchanged content, got %q", resp.Content)
	}
}

func TestQuickRedact(t *testing.T) {
	result := QuickRedact("My email is test@example.com and SSN 123-45-6789")
	if result == "My email is test@example.com and SSN 123-45-6789" {
		t.Error("expected redacted output")
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "***"},
		{"sk-abcdefghijklmnop", "sk-a...mnop"},
		{"", "***"},
	}
	for _, tt := range tests {
		got := MaskSecret(tt.input)
		if got != tt.want {
			t.Errorf("MaskSecret(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
