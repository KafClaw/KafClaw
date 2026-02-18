package skills

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OAuthProvider identifies supported auth providers.
type OAuthProvider string

const (
	ProviderGoogleWorkspace OAuthProvider = "google-workspace"
	ProviderM365            OAuthProvider = "m365"
)

// OAuthStartInput defines auth-start parameters.
type OAuthStartInput struct {
	Provider     OAuthProvider
	Profile      string
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
	TenantID     string
}

// OAuthStartResult returns URL + metadata to continue auth.
type OAuthStartResult struct {
	Provider     OAuthProvider `json:"provider"`
	Profile      string        `json:"profile"`
	AuthorizeURL string        `json:"authorizeUrl"`
	State        string        `json:"state"`
	StartedAt    time.Time     `json:"startedAt"`
}

// OAuthCompleteInput defines auth completion payload.
type OAuthCompleteInput struct {
	Provider OAuthProvider
	Profile  string
	Code     string
	State    string
}

// OAuthTokenResult is a sanitized token response.
type OAuthTokenResult struct {
	Provider   OAuthProvider `json:"provider"`
	Profile    string        `json:"profile"`
	TokenPath  string        `json:"tokenPath"`
	ExpiresAt  time.Time     `json:"expiresAt,omitempty"`
	Scope      string        `json:"scope,omitempty"`
	TokenType  string        `json:"tokenType,omitempty"`
	ObtainedAt time.Time     `json:"obtainedAt"`
}

type oauthPending struct {
	Provider     OAuthProvider `json:"provider"`
	Profile      string        `json:"profile"`
	ClientID     string        `json:"clientId"`
	ClientSecret string        `json:"clientSecret"`
	RedirectURI  string        `json:"redirectUri"`
	Scopes       []string      `json:"scopes"`
	State        string        `json:"state"`
	CodeVerifier string        `json:"codeVerifier"`
	TenantID     string        `json:"tenantId,omitempty"`
	CreatedAt    time.Time     `json:"createdAt"`
}

type oauthTokenRaw struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// StartOAuthFlow starts browser/copy auth flow for Google Workspace or M365.
func StartOAuthFlow(in OAuthStartInput) (*OAuthStartResult, error) {
	pending, err := buildPending(in)
	if err != nil {
		_ = appendOAuthSecurityEvent("skills_oauth_start", in.Provider, in.Profile, false, err.Error())
		return nil, err
	}
	if err := savePendingOAuth(pending); err != nil {
		_ = appendOAuthSecurityEvent("skills_oauth_start", pending.Provider, pending.Profile, false, err.Error())
		return nil, err
	}
	authURL := buildAuthorizeURL(pending)
	if authURL == "" {
		_ = appendOAuthSecurityEvent("skills_oauth_start", pending.Provider, pending.Profile, false, "failed to build authorization URL")
		return nil, errors.New("failed to build authorization URL")
	}
	_ = appendOAuthSecurityEvent("skills_oauth_start", pending.Provider, pending.Profile, true, "")
	return &OAuthStartResult{
		Provider:     pending.Provider,
		Profile:      pending.Profile,
		AuthorizeURL: authURL,
		State:        pending.State,
		StartedAt:    pending.CreatedAt,
	}, nil
}

// CompleteOAuthFlow exchanges auth code for tokens and stores them securely.
func CompleteOAuthFlow(in OAuthCompleteInput) (*OAuthTokenResult, error) {
	profile := sanitizeSkillName(in.Profile)
	if profile == "" {
		profile = "default"
	}
	pending, err := loadPendingOAuth(in.Provider, profile)
	if err != nil {
		_ = appendOAuthSecurityEvent("skills_oauth_complete", in.Provider, profile, false, err.Error())
		return nil, err
	}
	if in.State == "" || in.State != pending.State {
		_ = appendOAuthSecurityEvent("skills_oauth_complete", pending.Provider, pending.Profile, false, "oauth state mismatch")
		return nil, errors.New("oauth state mismatch")
	}
	code := strings.TrimSpace(in.Code)
	if code == "" {
		_ = appendOAuthSecurityEvent("skills_oauth_complete", pending.Provider, pending.Profile, false, "oauth code is required")
		return nil, errors.New("oauth code is required")
	}

	tokenRaw, err := exchangeCodeForToken(pending, code)
	if err != nil {
		_ = appendOAuthSecurityEvent("skills_oauth_complete", pending.Provider, pending.Profile, false, err.Error())
		return nil, err
	}
	if strings.TrimSpace(tokenRaw.AccessToken) == "" {
		_ = appendOAuthSecurityEvent("skills_oauth_complete", pending.Provider, pending.Profile, false, "token response missing access_token")
		return nil, errors.New("token response missing access_token")
	}
	now := time.Now().UTC()
	var expiresAt time.Time
	if tokenRaw.ExpiresIn > 0 {
		expiresAt = now.Add(time.Duration(tokenRaw.ExpiresIn) * time.Second)
	}

	tokenPath, err := saveOAuthToken(pending.Provider, pending.Profile, tokenRaw, now, expiresAt)
	if err != nil {
		_ = appendOAuthSecurityEvent("skills_oauth_complete", pending.Provider, pending.Profile, false, err.Error())
		return nil, err
	}
	_ = removePendingOAuth(pending.Provider, pending.Profile)
	_ = appendOAuthSecurityEvent("skills_oauth_complete", pending.Provider, pending.Profile, true, "")
	return &OAuthTokenResult{
		Provider:   pending.Provider,
		Profile:    pending.Profile,
		TokenPath:  tokenPath,
		ExpiresAt:  expiresAt,
		Scope:      tokenRaw.Scope,
		TokenType:  tokenRaw.TokenType,
		ObtainedAt: now,
	}, nil
}

// ParseOAuthCallbackURL extracts code/state from a provider callback URL.
func ParseOAuthCallbackURL(raw string) (code string, state string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", err
	}
	q := u.Query()
	code = strings.TrimSpace(q.Get("code"))
	state = strings.TrimSpace(q.Get("state"))
	if code == "" || state == "" {
		return "", "", errors.New("callback URL missing code or state")
	}
	return code, state, nil
}

func buildPending(in OAuthStartInput) (*oauthPending, error) {
	p := OAuthProvider(strings.ToLower(strings.TrimSpace(string(in.Provider))))
	if p != ProviderGoogleWorkspace && p != ProviderM365 {
		return nil, fmt.Errorf("unsupported provider: %s", in.Provider)
	}
	profile := sanitizeSkillName(in.Profile)
	if profile == "" {
		profile = "default"
	}
	clientID := strings.TrimSpace(in.ClientID)
	clientSecret := strings.TrimSpace(in.ClientSecret)
	redirectURI := strings.TrimSpace(in.RedirectURI)
	if clientID == "" || clientSecret == "" || redirectURI == "" {
		return nil, errors.New("client-id, client-secret, and redirect-uri are required")
	}
	if _, err := url.ParseRequestURI(redirectURI); err != nil {
		return nil, fmt.Errorf("invalid redirect-uri: %w", err)
	}
	scopes := normalizeScopes(in.Scopes)
	if len(scopes) == 0 {
		if p == ProviderGoogleWorkspace {
			scopes = []string{"openid", "email", "profile", "https://www.googleapis.com/auth/userinfo.email"}
		} else {
			scopes = []string{"openid", "offline_access", "User.Read"}
		}
	}
	state, err := randomB64URL(24)
	if err != nil {
		return nil, err
	}
	verifier, err := randomB64URL(48)
	if err != nil {
		return nil, err
	}
	return &oauthPending{
		Provider:     p,
		Profile:      profile,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		Scopes:       scopes,
		State:        state,
		CodeVerifier: verifier,
		TenantID:     strings.TrimSpace(in.TenantID),
		CreatedAt:    time.Now().UTC(),
	}, nil
}

func buildAuthorizeURL(p *oauthPending) string {
	values := url.Values{}
	values.Set("client_id", p.ClientID)
	values.Set("redirect_uri", p.RedirectURI)
	values.Set("response_type", "code")
	values.Set("scope", strings.Join(p.Scopes, " "))
	values.Set("state", p.State)
	values.Set("code_challenge_method", "S256")
	values.Set("code_challenge", pkceChallenge(p.CodeVerifier))
	values.Set("access_type", "offline")
	values.Set("prompt", "consent")

	switch p.Provider {
	case ProviderGoogleWorkspace:
		return "https://accounts.google.com/o/oauth2/v2/auth?" + values.Encode()
	case ProviderM365:
		tenant := p.TenantID
		if tenant == "" {
			tenant = "common"
		}
		values.Del("access_type")
		values.Del("prompt")
		return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize?%s", tenant, values.Encode())
	default:
		return ""
	}
}

func exchangeCodeForToken(p *oauthPending, code string) (*oauthTokenRaw, error) {
	form := url.Values{}
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", p.RedirectURI)
	form.Set("code_verifier", p.CodeVerifier)

	tokenURL := ""
	switch p.Provider {
	case ProviderGoogleWorkspace:
		tokenURL = "https://oauth2.googleapis.com/token"
	case ProviderM365:
		tenant := p.TenantID
		if tenant == "" {
			tenant = "common"
		}
		tokenURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenant)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", p.Provider)
	}

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out oauthTokenRaw
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if out.Error != "" {
			return nil, fmt.Errorf("token exchange failed: %s (%s)", out.Error, out.ErrorDesc)
		}
		return nil, fmt.Errorf("token exchange failed: status %d", resp.StatusCode)
	}
	return &out, nil
}

func savePendingOAuth(p *oauthPending) error {
	dir, err := oauthStateDir(p.Provider, p.Profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	sealed, err := encryptOAuthStateBlob(append(data, '\n'))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "pending.json"), sealed, 0o600)
}

func loadPendingOAuth(provider OAuthProvider, profile string) (*oauthPending, error) {
	path, err := oauthStateFile(provider, profile, "pending.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data, err = decryptOAuthStateBlob(data)
	if err != nil {
		return nil, err
	}
	var p oauthPending
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func removePendingOAuth(provider OAuthProvider, profile string) error {
	path, err := oauthStateFile(provider, profile, "pending.json")
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func saveOAuthToken(provider OAuthProvider, profile string, raw *oauthTokenRaw, obtainedAt, expiresAt time.Time) (string, error) {
	dir, err := oauthStateDir(provider, profile)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	payload := map[string]any{
		"provider":      provider,
		"profile":       profile,
		"access_token":  raw.AccessToken,
		"refresh_token": raw.RefreshToken,
		"id_token":      raw.IDToken,
		"token_type":    raw.TokenType,
		"scope":         raw.Scope,
		"obtained_at":   obtainedAt.Format(time.RFC3339),
	}
	if !expiresAt.IsZero() {
		payload["expires_at"] = expiresAt.Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	sealed, err := encryptOAuthStateBlob(append(data, '\n'))
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "token.json")
	if err := os.WriteFile(path, sealed, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func oauthStateDir(provider OAuthProvider, profile string) (string, error) {
	dirs, err := EnsureStateDirs()
	if err != nil {
		return "", err
	}
	profile = sanitizeSkillName(profile)
	if profile == "" {
		profile = "default"
	}
	base := filepath.Join(dirs.ToolsDir, "auth", string(provider), profile)
	return base, nil
}

func oauthStateFile(provider OAuthProvider, profile, file string) (string, error) {
	dir, err := oauthStateDir(provider, profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, file), nil
}

func normalizeScopes(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			v := strings.TrimSpace(part)
			if v == "" {
				continue
			}
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomB64URL(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func appendOAuthSecurityEvent(eventType string, provider OAuthProvider, profile string, success bool, message string) error {
	return appendSecurityAuditEvent(eventType, map[string]any{
		"provider": string(provider),
		"profile":  sanitizeSkillName(profile),
		"success":  success,
		"message":  strings.TrimSpace(message),
	})
}
