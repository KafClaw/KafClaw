package credentials

import (
	"testing"
	"time"
)

func TestIsExpired_NilToken(t *testing.T) {
	if !IsExpired(nil) {
		t.Fatal("expected nil token to be expired")
	}
}

func TestIsExpired_ZeroExpiry(t *testing.T) {
	tok := &OAuthToken{Access: "tok", Expires: 0}
	if !IsExpired(tok) {
		t.Fatal("expected zero-expiry token to be expired")
	}
}

func TestIsExpired_FutureExpiry(t *testing.T) {
	tok := &OAuthToken{
		Access:  "tok",
		Expires: time.Now().Unix() + 3600, // 1 hour from now
	}
	if IsExpired(tok) {
		t.Fatal("expected token with future expiry to not be expired")
	}
}

func TestIsExpired_PastExpiry(t *testing.T) {
	tok := &OAuthToken{
		Access:  "tok",
		Expires: time.Now().Unix() - 120, // 2 minutes ago
	}
	if !IsExpired(tok) {
		t.Fatal("expected token with past expiry to be expired")
	}
}

func TestIsExpired_Within60sGrace(t *testing.T) {
	// Token expires 30 seconds from now, but the 60-second grace window
	// means it should already be considered expired.
	tok := &OAuthToken{
		Access:  "tok",
		Expires: time.Now().Unix() + 30,
	}
	if !IsExpired(tok) {
		t.Fatal("expected token within 60s grace window to be expired")
	}
}

func TestApiKeyTombKey(t *testing.T) {
	tests := []struct {
		providerID string
		want       string
	}{
		{"OpenAI", "provider.apikey.openai"},
		{"anthropic", "provider.apikey.anthropic"},
		{"XAI", "provider.apikey.xai"},
		{"Google-Gemini", "provider.apikey.google-gemini"},
	}
	for _, tc := range tests {
		got := apiKeyTombKey(tc.providerID)
		if got != tc.want {
			t.Errorf("apiKeyTombKey(%q) = %q, want %q", tc.providerID, got, tc.want)
		}
	}
}
