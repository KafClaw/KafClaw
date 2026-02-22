// Package credentials manages provider credential storage and retrieval.
// OAuth tokens are stored as encrypted blobs in <ToolsDir>/auth/providers/<id>/token.json.
// Static API keys are stored in the local tomb env map keyed as provider.apikey.<id>.
package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/secrets"
)

// OAuthToken represents a stored OAuth credential.
type OAuthToken struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token,omitempty"`
	Expires int64  `json:"expires_at"`
	Email   string `json:"email,omitempty"`
}

// IsExpired reports whether the token is expired (with a 60-second grace margin).
func IsExpired(t *OAuthToken) bool {
	if t == nil || t.Expires == 0 {
		return true
	}
	return time.Now().Unix() >= t.Expires-60
}

// providerAuthDir returns the directory for a provider's credential files.
func providerAuthDir(providerID string) (string, error) {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), "skills", "tools", "auth", "providers", providerID), nil
}

// LoadToken reads and decrypts an OAuth token for the given provider.
func LoadToken(providerID string) (*OAuthToken, error) {
	dir, err := providerAuthDir(providerID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "token.json"))
	if err != nil {
		return nil, fmt.Errorf("load token for provider %s: %w", providerID, err)
	}
	plain, err := secrets.DecryptBlob(data)
	if err != nil {
		return nil, fmt.Errorf("decrypt token for provider %s: %w", providerID, err)
	}
	var tok OAuthToken
	if err := json.Unmarshal(plain, &tok); err != nil {
		return nil, fmt.Errorf("parse token for provider %s: %w", providerID, err)
	}
	return &tok, nil
}

// SaveToken encrypts and saves an OAuth token for the given provider.
func SaveToken(providerID string, t *OAuthToken) error {
	if t == nil {
		return fmt.Errorf("nil token")
	}
	dir, err := providerAuthDir(providerID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	plain, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	sealed, err := secrets.EncryptBlob(append(plain, '\n'))
	if err != nil {
		return fmt.Errorf("encrypt token for provider %s: %w", providerID, err)
	}
	return os.WriteFile(filepath.Join(dir, "token.json"), sealed, 0o600)
}

// apiKeyTombKey returns the tomb env map key for a provider's API key.
func apiKeyTombKey(providerID string) string {
	return "provider.apikey." + strings.ToLower(providerID)
}

// LoadAPIKey reads a static API key for the given provider from the tomb env map.
func LoadAPIKey(providerID string) (string, error) {
	tombPath, err := secrets.ResolveLocalTombPath()
	if err != nil {
		return "", fmt.Errorf("resolve tomb for provider %s: %w", providerID, err)
	}
	data, err := os.ReadFile(tombPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no API key stored for provider %s", providerID)
		}
		return "", fmt.Errorf("load tomb for provider %s: %w", providerID, err)
	}
	doc, err := secrets.DecodeLocalTomb(data)
	if err != nil {
		return "", fmt.Errorf("decode tomb for provider %s: %w", providerID, err)
	}
	envMap, err := secrets.LoadEnvSecretsFromTombDoc(doc)
	if err != nil {
		return "", fmt.Errorf("load env secrets for provider %s: %w", providerID, err)
	}
	key, ok := envMap[apiKeyTombKey(providerID)]
	if !ok || strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("no API key stored for provider %s", providerID)
	}
	return key, nil
}

// SaveAPIKey stores a static API key for the given provider in the tomb env map.
func SaveAPIKey(providerID string, key string) error {
	tombPath, doc, err := secrets.LoadOrCreateLocalTomb()
	if err != nil {
		return fmt.Errorf("load tomb for provider %s: %w", providerID, err)
	}
	existing, err := secrets.LoadEnvSecretsFromTombDoc(doc)
	if err != nil {
		return fmt.Errorf("load env secrets for provider %s: %w", providerID, err)
	}
	existing[apiKeyTombKey(providerID)] = key
	if err := secrets.SealEnvSecretsIntoTombDoc(doc, existing); err != nil {
		return fmt.Errorf("seal env secrets for provider %s: %w", providerID, err)
	}
	if err := secrets.WriteLocalTomb(tombPath, doc); err != nil {
		return fmt.Errorf("write tomb for provider %s: %w", providerID, err)
	}
	return os.Chmod(tombPath, 0o600)
}
