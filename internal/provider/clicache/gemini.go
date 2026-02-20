// Package clicache reads OAuth credentials cached by external CLI tools.
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

// geminiCLICreds mirrors the JSON structure of ~/.gemini/oauth_creds.json.
type geminiCLICreds struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at_seconds"`
	Email        string `json:"email"`
}

// ReadGeminiCLICredential reads the Gemini CLI's cached OAuth credential.
// If the credential is expired, it shells out to `gemini auth` to refresh.
func ReadGeminiCLICredential() (*credentials.OAuthToken, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	credsPath := filepath.Join(home, ".gemini", "oauth_creds.json")
	tok, err := readGeminiCredsFile(credsPath)
	if err != nil {
		return nil, err
	}
	if !credentials.IsExpired(tok) {
		return tok, nil
	}
	// Attempt refresh via CLI.
	if refreshErr := runGeminiAuthRefresh(); refreshErr != nil {
		return nil, fmt.Errorf("gemini credential expired and refresh failed: %w", refreshErr)
	}
	return readGeminiCredsFile(credsPath)
}

func readGeminiCredsFile(path string) (*credentials.OAuthToken, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read gemini credentials at %s: %w (run 'gemini auth login' first)", path, err)
	}
	var creds geminiCLICreds
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse gemini credentials: %w", err)
	}
	expires := creds.ExpiresAt
	if expires == 0 {
		// Some versions use seconds-from-now instead of absolute timestamp.
		expires = time.Now().Add(time.Hour).Unix()
	}
	return &credentials.OAuthToken{
		Access:  creds.AccessToken,
		Refresh: creds.RefreshToken,
		Expires: expires,
		Email:   creds.Email,
	}, nil
}

func runGeminiAuthRefresh() error {
	cmd := exec.Command("gemini", "auth", "login", "--no-browser")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
