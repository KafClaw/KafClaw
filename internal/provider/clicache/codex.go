package clicache

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/KafClaw/KafClaw/internal/provider/credentials"
)

// codexCLICreds mirrors the JSON structure of ~/.codex/auth.json.
type codexCLICreds struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	Email        string `json:"email"`
}

// ReadCodexCLICredential reads the Codex CLI's cached OAuth credential.
// If the credential is expired, it shells out to `codex auth` to refresh.
func ReadCodexCLICredential() (*credentials.OAuthToken, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	credsPath := filepath.Join(home, ".codex", "auth.json")
	tok, err := readCodexCredsFile(credsPath)
	if err != nil {
		return nil, err
	}
	if !credentials.IsExpired(tok) {
		return tok, nil
	}
	if refreshErr := runCodexAuthRefresh(); refreshErr != nil {
		return nil, fmt.Errorf("codex credential expired and refresh failed: %w", refreshErr)
	}
	return readCodexCredsFile(credsPath)
}

func readCodexCredsFile(path string) (*credentials.OAuthToken, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read codex credentials at %s: %w (run 'codex auth' first)", path, err)
	}
	var creds codexCLICreds
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse codex credentials: %w", err)
	}
	expires := creds.ExpiresAt
	if expires == 0 {
		expires = time.Now().Add(time.Hour).Unix()
	}
	return &credentials.OAuthToken{
		Access:  creds.AccessToken,
		Refresh: creds.RefreshToken,
		Expires: expires,
		Email:   creds.Email,
	}, nil
}

func runCodexAuthRefresh() error {
	cmd := exec.Command("codex", "auth")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
