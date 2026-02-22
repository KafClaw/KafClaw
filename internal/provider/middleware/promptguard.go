package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

// PromptGuard scans inbound messages for PII, secrets, and deny-keywords.
// Depending on mode it can warn (tag only), redact, or block.
type PromptGuard struct {
	cfg          config.PromptGuardConfig
	detector     *Detector
	denyKeywords []string
}

// NewPromptGuard builds a guard from config.
func NewPromptGuard(cfg config.PromptGuardConfig) *PromptGuard {
	piiTypes := cfg.PII.Detect
	secretTypes := cfg.Secrets.Detect
	var custom []config.NamedPattern
	custom = append(custom, cfg.PII.CustomPatterns...)
	custom = append(custom, cfg.Secrets.CustomPatterns...)
	custom = append(custom, cfg.CustomPatterns...)

	return &PromptGuard{
		cfg:          cfg,
		detector:     NewDetector(piiTypes, secretTypes, custom),
		denyKeywords: cfg.DenyKeywords,
	}
}

func (g *PromptGuard) Name() string { return "prompt-guard" }

func (g *PromptGuard) ProcessRequest(_ context.Context, req *provider.ChatRequest, meta *RequestMeta) error {
	if !g.cfg.Enabled {
		return nil
	}

	mode := g.cfg.Mode
	if mode == "" {
		mode = "warn"
	}

	// Scan user messages only.
	for i, msg := range req.Messages {
		if msg.Role != "user" {
			continue
		}

		// Check deny keywords first — always block.
		found := ContainsKeywords(msg.Content, g.denyKeywords)
		if len(found) > 0 {
			meta.Blocked = true
			meta.BlockReason = fmt.Sprintf("denied keyword(s): %s", strings.Join(found, ", "))
			return nil
		}

		matches := g.detector.Scan(msg.Content)
		if len(matches) == 0 {
			continue
		}

		// Determine action.
		action := mode
		// PII-specific action override.
		if g.cfg.PII.Action != "" {
			for _, m := range matches {
				if isPIIType(m.Type) {
					action = g.cfg.PII.Action
					break
				}
			}
		}
		// Secret-specific action override.
		if g.cfg.Secrets.Action != "" {
			for _, m := range matches {
				if isSecretType(m.Type) {
					action = g.cfg.Secrets.Action
					break
				}
			}
		}

		switch action {
		case "block":
			types := matchTypes(matches)
			meta.Blocked = true
			meta.BlockReason = fmt.Sprintf("detected %s in message", strings.Join(types, ", "))
			return nil
		case "redact":
			req.Messages[i].Content = g.detector.Redact(msg.Content)
			meta.Tags["prompt_guard"] = "redacted"
		default: // "warn"
			meta.Tags["prompt_guard"] = "detected"
		}
	}

	return nil
}

func (g *PromptGuard) ProcessResponse(_ context.Context, _ *provider.ChatRequest, _ *provider.ChatResponse, _ *RequestMeta) error {
	// Output scanning is handled by OutputSanitizer.
	return nil
}

func isPIIType(t string) bool {
	switch t {
	case "email", "phone", "ssn", "credit_card", "ip_address":
		return true
	}
	return false
}

func isSecretType(t string) bool {
	switch t {
	case "api_key", "bearer_token", "private_key", "password_literal":
		return true
	}
	return false
}

func matchTypes(matches []DetectorMatch) []string {
	seen := make(map[string]bool)
	var types []string
	for _, m := range matches {
		if !seen[m.Type] {
			seen[m.Type] = true
			types = append(types, m.Type)
		}
	}
	return types
}
