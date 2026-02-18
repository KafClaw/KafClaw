package skills

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartOAuthFlowGoogleWritesPending(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))

	out, err := StartOAuthFlow(OAuthStartInput{
		Provider:     ProviderGoogleWorkspace,
		Profile:      "default",
		ClientID:     "cid",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost:53682/callback",
		Scopes:       []string{"openid", "email"},
	})
	if err != nil {
		t.Fatalf("start oauth failed: %v", err)
	}
	if !strings.Contains(out.AuthorizeURL, "accounts.google.com") {
		t.Fatalf("unexpected google auth url: %s", out.AuthorizeURL)
	}
	path, err := oauthStateFile(ProviderGoogleWorkspace, "default", "pending.json")
	if err != nil {
		t.Fatalf("state file path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected pending state file: %v", err)
	}
}

func TestParseOAuthCallbackURL(t *testing.T) {
	code, state, err := ParseOAuthCallbackURL("http://localhost:53682/callback?code=abc123&state=xyz456")
	if err != nil {
		t.Fatalf("parse callback: %v", err)
	}
	if code != "abc123" || state != "xyz456" {
		t.Fatalf("unexpected code/state: %s %s", code, state)
	}
}

func TestCompleteOAuthFlowStateMismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))

	_, err := StartOAuthFlow(OAuthStartInput{
		Provider:     ProviderM365,
		Profile:      "default",
		ClientID:     "cid",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost:53682/callback",
		Scopes:       []string{"openid"},
	})
	if err != nil {
		t.Fatalf("start oauth failed: %v", err)
	}
	_, err = CompleteOAuthFlow(OAuthCompleteInput{
		Provider: ProviderM365,
		Profile:  "default",
		Code:     "test-code",
		State:    "wrong-state",
	})
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch error, got: %v", err)
	}
}

func TestCompleteOAuthFlowSuccessWithMockTokenExchange(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))
	t.Setenv("KAFCLAW_OAUTH_KEY_BACKEND", "file")

	start, err := StartOAuthFlow(OAuthStartInput{
		Provider:     ProviderGoogleWorkspace,
		Profile:      "default",
		ClientID:     "cid",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost:53682/callback",
		Scopes:       []string{"openid"},
	})
	if err != nil {
		t.Fatalf("start oauth failed: %v", err)
	}

	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Host, "oauth2.googleapis.com") {
			body := `{"access_token":"at","refresh_token":"rt","token_type":"Bearer","expires_in":3600}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":"not_found"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	res, err := CompleteOAuthFlow(OAuthCompleteInput{
		Provider: ProviderGoogleWorkspace,
		Profile:  "default",
		Code:     "abc-code",
		State:    start.State,
	})
	if err != nil {
		t.Fatalf("complete oauth failed: %v", err)
	}
	if res.TokenPath == "" {
		t.Fatal("expected token path")
	}
	if _, err := os.Stat(res.TokenPath); err != nil {
		t.Fatalf("expected token file: %v", err)
	}
	data, err := os.ReadFile(res.TokenPath)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "access_token") {
		t.Fatalf("expected encrypted token payload at rest, got plaintext token json: %s", text)
	}
	if !strings.Contains(text, "ciphertext") {
		t.Fatalf("expected encrypted token wrapper with ciphertext field, got: %s", text)
	}
	dirs, err := EnsureStateDirs()
	if err != nil {
		t.Fatalf("ensure state dirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dirs.ToolsDir, "auth", "master.key")); err != nil {
		t.Fatalf("expected oauth master key file fallback/presence: %v", err)
	}
}

func TestStartOAuthFlowWritesSecurityEvent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))
	t.Setenv("KAFCLAW_OAUTH_KEY_BACKEND", "file")

	_, err := StartOAuthFlow(OAuthStartInput{
		Provider:     ProviderGoogleWorkspace,
		Profile:      "audit-profile",
		ClientID:     "cid",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost:53682/callback",
		Scopes:       []string{"openid"},
	})
	if err != nil {
		t.Fatalf("start oauth failed: %v", err)
	}
	dirs, err := EnsureStateDirs()
	if err != nil {
		t.Fatalf("ensure state dirs: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dirs.AuditDir, "security-events.jsonl"))
	if err != nil {
		t.Fatalf("read security events: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "\"eventType\":\"skills_oauth_start\"") {
		t.Fatalf("expected oauth start event, got: %s", text)
	}
	if !strings.Contains(text, "\"success\":true") {
		t.Fatalf("expected oauth success event, got: %s", text)
	}
}

func TestOAuthLocalBackendCreatesTombKeyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))
	t.Setenv("KAFCLAW_OAUTH_KEY_BACKEND", "local")

	_, err := StartOAuthFlow(OAuthStartInput{
		Provider:     ProviderGoogleWorkspace,
		Profile:      "local-key",
		ClientID:     "cid",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost:53682/callback",
		Scopes:       []string{"openid"},
	})
	if err != nil {
		t.Fatalf("start oauth failed: %v", err)
	}
	tombPath, err := ResolveLocalOAuthTombPath()
	if err != nil {
		t.Fatalf("resolve tomb path: %v", err)
	}
	st, err := os.Stat(tombPath)
	if err != nil {
		t.Fatalf("expected tomb key file: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("expected tomb key perms 600, got %o", st.Mode().Perm())
	}
}

func TestStoreAndLoadEnvSecretsInLocalTomb(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))

	written, err := StoreEnvSecretsInLocalTomb(map[string]string{
		"OPENAI_API_KEY":   "sk-test",
		"SOME_OTHER_KEY":   "ignored",
		"GITHUB_TOKEN":     "ghp-test",
		"KAFCLAW_PASSWORD": "pw",
	})
	if err != nil {
		t.Fatalf("store tomb env secrets: %v", err)
	}
	if written == 0 {
		t.Fatalf("expected at least one secret written")
	}
	got, err := LoadEnvSecretsFromLocalTomb()
	if err != nil {
		t.Fatalf("load tomb env secrets: %v", err)
	}
	if got["OPENAI_API_KEY"] != "sk-test" || got["GITHUB_TOKEN"] != "ghp-test" {
		t.Fatalf("unexpected tomb env payload: %#v", got)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
