package middleware

import (
	"context"
	"regexp"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
)

// OutputSanitizer scans LLM responses for PII, secrets, and deny patterns
// before they reach the channel delivery path.
type OutputSanitizer struct {
	cfg          config.OutputSanitizationConfig
	detector     *Detector
	denyPatterns []*regexp.Regexp
}

// NewOutputSanitizer builds a sanitizer from config.
func NewOutputSanitizer(cfg config.OutputSanitizationConfig) *OutputSanitizer {
	var piiTypes, secretTypes []string
	if cfg.RedactPII {
		piiTypes = []string{"email", "phone", "ssn", "credit_card", "ip_address"}
	}
	if cfg.RedactSecrets {
		secretTypes = []string{"api_key", "bearer_token", "private_key", "password_literal"}
	}

	var deny []*regexp.Regexp
	for _, p := range cfg.DenyPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		deny = append(deny, re)
	}

	return &OutputSanitizer{
		cfg:          cfg,
		detector:     NewDetector(piiTypes, secretTypes, cfg.CustomRedactPatterns),
		denyPatterns: deny,
	}
}

func (s *OutputSanitizer) Name() string { return "output-sanitizer" }

func (s *OutputSanitizer) ProcessRequest(_ context.Context, _ *provider.ChatRequest, _ *RequestMeta) error {
	return nil
}

func (s *OutputSanitizer) ProcessResponse(_ context.Context, _ *provider.ChatRequest, resp *provider.ChatResponse, meta *RequestMeta) error {
	if !s.cfg.Enabled {
		return nil
	}

	content := resp.Content

	// Check deny patterns — replace entire response.
	for _, re := range s.denyPatterns {
		if re.MatchString(content) {
			resp.Content = "[Response filtered by output sanitizer]"
			meta.Tags["output_sanitized"] = "denied"
			return nil
		}
	}

	// Redact PII/secrets.
	if s.cfg.RedactPII || s.cfg.RedactSecrets || len(s.cfg.CustomRedactPatterns) > 0 {
		redacted := s.detector.Redact(content)
		if redacted != content {
			resp.Content = redacted
			meta.Tags["output_sanitized"] = "redacted"
		}
	}

	// Truncate if needed.
	if s.cfg.MaxOutputLength > 0 && len(resp.Content) > s.cfg.MaxOutputLength {
		resp.Content = resp.Content[:s.cfg.MaxOutputLength] + "\n[truncated by output sanitizer]"
		if _, ok := meta.Tags["output_sanitized"]; !ok {
			meta.Tags["output_sanitized"] = "truncated"
		}
	}

	return nil
}

// SanitizeText is a standalone helper for sanitizing arbitrary text using the
// same detector logic (useful outside the middleware chain).
func (s *OutputSanitizer) SanitizeText(text string) string {
	// Check deny
	for _, re := range s.denyPatterns {
		if re.MatchString(text) {
			return "[Content filtered]"
		}
	}
	result := s.detector.Redact(text)
	if s.cfg.MaxOutputLength > 0 && len(result) > s.cfg.MaxOutputLength {
		result = result[:s.cfg.MaxOutputLength]
	}
	return result
}

// QuickRedact is a convenience function that redacts PII and secrets from text
// without needing a full config setup.
func QuickRedact(text string) string {
	d := NewDefaultDetector()
	return d.Redact(text)
}

// MaskSecret partially masks a secret value, showing only the first and last
// few characters. Useful for displaying credential snippets safely.
func MaskSecret(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "..." + s[len(s)-4:]
}
