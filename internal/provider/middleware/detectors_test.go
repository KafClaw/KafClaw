package middleware

import (
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestDetector_Email(t *testing.T) {
	d := NewDetector([]string{"email"}, nil, nil)
	matches := d.Scan("Contact us at test@example.com for help.")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Type != "email" {
		t.Errorf("expected type 'email', got %q", matches[0].Type)
	}
	if matches[0].Value != "test@example.com" {
		t.Errorf("expected 'test@example.com', got %q", matches[0].Value)
	}
}

func TestDetector_SSN(t *testing.T) {
	d := NewDetector([]string{"ssn"}, nil, nil)
	matches := d.Scan("SSN is 123-45-6789")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Value != "123-45-6789" {
		t.Errorf("expected '123-45-6789', got %q", matches[0].Value)
	}
}

func TestDetector_CreditCard(t *testing.T) {
	d := NewDetector([]string{"credit_card"}, nil, nil)
	matches := d.Scan("Card: 4111-1111-1111-1111")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestDetector_IPAddress(t *testing.T) {
	d := NewDetector([]string{"ip_address"}, nil, nil)
	matches := d.Scan("Server at 192.168.1.100 is down")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Value != "192.168.1.100" {
		t.Errorf("expected '192.168.1.100', got %q", matches[0].Value)
	}
}

func TestDetector_APIKey(t *testing.T) {
	d := NewDetector(nil, []string{"api_key"}, nil)
	matches := d.Scan("Key: sk-abcdefghijklmnopqrstuvwx")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Type != "api_key" {
		t.Errorf("expected type 'api_key', got %q", matches[0].Type)
	}
}

func TestDetector_BearerToken(t *testing.T) {
	d := NewDetector(nil, []string{"bearer_token"}, nil)
	matches := d.Scan("Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.test")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestDetector_PrivateKey(t *testing.T) {
	d := NewDetector(nil, []string{"private_key"}, nil)
	matches := d.Scan("-----BEGIN RSA PRIVATE KEY-----\nMIIE...")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestDetector_PasswordLiteral(t *testing.T) {
	d := NewDetector(nil, []string{"password_literal"}, nil)
	matches := d.Scan("password=mysecret123")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestDetector_NoMatches(t *testing.T) {
	d := NewDefaultDetector()
	matches := d.Scan("This is a perfectly safe message with no sensitive data.")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d: %+v", len(matches), matches)
	}
}

func TestDetector_HasMatches(t *testing.T) {
	d := NewDetector([]string{"email"}, nil, nil)
	if !d.HasMatches("Email me at test@example.com") {
		t.Error("expected HasMatches to return true")
	}
	if d.HasMatches("No email here") {
		t.Error("expected HasMatches to return false")
	}
}

func TestDetector_Redact(t *testing.T) {
	d := NewDetector([]string{"email", "ssn"}, nil, nil)
	result := d.Redact("Email: test@example.com SSN: 123-45-6789")
	if result != "Email: [REDACTED:EMAIL] SSN: [REDACTED:SSN]" {
		t.Errorf("unexpected redaction: %q", result)
	}
}

func TestDetector_CustomPatterns(t *testing.T) {
	d := NewDetector(nil, nil, []config.NamedPattern{
		{Name: "employee_id", Pattern: `EMP-\d{6}`},
	})
	matches := d.Scan("Employee EMP-123456 submitted a request")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Type != "employee_id" {
		t.Errorf("expected type 'employee_id', got %q", matches[0].Type)
	}
}

func TestDetector_ScanPII(t *testing.T) {
	d := NewDetector([]string{"email"}, []string{"api_key"}, nil)
	text := "Email: test@example.com Key: sk-abcdefghijklmnopqrstuvwx"
	pii := d.ScanPII(text)
	if len(pii) != 1 {
		t.Fatalf("expected 1 PII match, got %d", len(pii))
	}
	if pii[0].Type != "email" {
		t.Errorf("expected 'email', got %q", pii[0].Type)
	}
}

func TestDetector_ScanSecrets(t *testing.T) {
	d := NewDetector([]string{"email"}, []string{"api_key"}, nil)
	text := "Email: test@example.com Key: sk-abcdefghijklmnopqrstuvwx"
	secrets := d.ScanSecrets(text)
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret match, got %d", len(secrets))
	}
	if secrets[0].Type != "api_key" {
		t.Errorf("expected 'api_key', got %q", secrets[0].Type)
	}
}

func TestContainsKeywords(t *testing.T) {
	found := ContainsKeywords("Please share your Social Security number", []string{"social security", "credit card"})
	if len(found) != 1 {
		t.Fatalf("expected 1 keyword, got %d", len(found))
	}
	if found[0] != "social security" {
		t.Errorf("expected 'social security', got %q", found[0])
	}

	none := ContainsKeywords("Hello world", []string{"password", "secret"})
	if len(none) != 0 {
		t.Errorf("expected 0 keywords, got %d", len(none))
	}
}

func TestNewDefaultDetector(t *testing.T) {
	d := NewDefaultDetector()
	if len(d.piiDetectors) == 0 {
		t.Error("expected non-empty PII detectors")
	}
	if len(d.secretDetectors) == 0 {
		t.Error("expected non-empty secret detectors")
	}
}
