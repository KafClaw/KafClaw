// Package secrets provides shared encryption primitives for KafClaw.
// It extracts the AES-256-GCM blob encryption originally in internal/skills/oauth_crypto.go
// so that both internal/skills and internal/provider can use it without import cycles.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/zalando/go-keyring"
)

const keyFileName = "master.key"
const localTombFileName = "tomb.rr"
const keyringService = "kafclaw.skills.oauth"
const keyringUser = "master-key"
const localTombEnvAAD = "kafclaw-local-env-secrets-v1"

// LocalTomb is the on-disk encrypted key store structure.
type LocalTomb struct {
	Version       string `json:"version"`
	MasterKey     string `json:"masterKey"`
	EnvNonce      string `json:"envNonce,omitempty"`
	EnvCiphertext string `json:"envCiphertext,omitempty"`
}

type encryptedBlob struct {
	Version    string `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// EncryptBlob encrypts plain bytes using AES-256-GCM with the shared master key.
func EncryptBlob(plain []byte) ([]byte, error) {
	key, err := LoadOrCreateMasterKey()
	if err != nil {
		return nil, err
	}
	return EncryptBlobWithKey(plain, key)
}

// EncryptBlobWithKey encrypts plain bytes using the given 32-byte AES key.
func EncryptBlobWithKey(plain, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	out := encryptedBlob{
		Version:    "v1",
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecryptBlob decrypts an AES-256-GCM encrypted blob using the shared master key.
// For backward compatibility, plaintext JSON is returned as-is.
func DecryptBlob(data []byte) ([]byte, error) {
	key, err := LoadOrCreateMasterKey()
	if err != nil {
		return nil, err
	}
	return DecryptBlobWithKey(data, key)
}

// DecryptBlobWithKey decrypts an encrypted blob using the given 32-byte AES key.
// For backward compatibility, plaintext JSON is returned as-is.
func DecryptBlobWithKey(data, key []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty encrypted blob")
	}
	var wrapped encryptedBlob
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return data, nil
	}
	if wrapped.Version == "" || wrapped.Nonce == "" || wrapped.Ciphertext == "" {
		return data, nil
	}
	if wrapped.Version != "v1" {
		return nil, fmt.Errorf("unsupported blob version: %s", wrapped.Version)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(wrapped.Nonce))
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(wrapped.Ciphertext))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

// LoadOrCreateMasterKey returns the 32-byte AES master key, creating one if necessary.
// Priority: KAFCLAW_OAUTH_MASTER_KEY env → backend resolver (keyring / local tomb / file).
func LoadOrCreateMasterKey() ([]byte, error) {
	if envKey := strings.TrimSpace(os.Getenv("KAFCLAW_OAUTH_MASTER_KEY")); envKey != "" {
		key, err := DecodeMasterKey(envKey)
		if err != nil {
			return nil, fmt.Errorf("invalid KAFCLAW_OAUTH_MASTER_KEY: %w", err)
		}
		return key, nil
	}

	switch resolveKeyBackend() {
	case "local":
		return loadOrCreateMasterKeyLocalOnly()
	case "keyring":
		return loadOrCreateMasterKeyKeyringOnly()
	case "file":
		return loadOrCreateMasterKeyFileOnly()
	default:
		if key, err := loadOrCreateMasterKeyKeyringOnly(); err == nil {
			return key, nil
		}
		if key, err := loadOrCreateMasterKeyLocalOnly(); err == nil {
			return key, nil
		}
		return loadOrCreateMasterKeyFileOnly()
	}
}

// DecodeMasterKey base64-decodes a master key and validates its length (32 bytes).
func DecodeMasterKey(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	decoded := make([]byte, base64.RawStdEncoding.DecodedLen(len(trimmed)))
	n, err := base64.RawStdEncoding.Decode(decoded, []byte(trimmed))
	if err != nil {
		return nil, err
	}
	if n != 32 {
		return nil, fmt.Errorf("invalid master key length: %d", n)
	}
	return decoded[:n], nil
}

// ResolveLocalTombPath returns the local key tomb file path.
func ResolveLocalTombPath() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("KAFCLAW_OAUTH_TOMB_FILE")); explicit != "" {
		if strings.HasPrefix(explicit, "~") {
			home, err := resolveHomeDir()
			if err != nil {
				return "", err
			}
			return expandTildePath(explicit, home), nil
		}
		return explicit, nil
	}
	home, err := resolveHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "kafclaw", localTombFileName), nil
}

// LoadOrCreateLocalTomb loads or creates the local tomb file.
func LoadOrCreateLocalTomb() (string, *LocalTomb, error) {
	tombPath, err := ResolveLocalTombPath()
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(tombPath), 0o700); err != nil {
		return "", nil, err
	}
	if data, err := os.ReadFile(tombPath); err == nil {
		doc, decErr := DecodeLocalTomb(data)
		if decErr != nil {
			return "", nil, decErr
		}
		return tombPath, doc, nil
	} else if !os.IsNotExist(err) {
		return "", nil, err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", nil, err
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)
	doc := &LocalTomb{
		Version:   "v1",
		MasterKey: encoded,
	}
	if err := WriteLocalTomb(tombPath, doc); err != nil {
		return "", nil, err
	}
	return tombPath, doc, nil
}

// DecodeLocalTomb parses a tomb file from raw bytes.
func DecodeLocalTomb(data []byte) (*LocalTomb, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, fmt.Errorf("empty tomb file")
	}
	var doc LocalTomb
	if err := json.Unmarshal([]byte(trimmed), &doc); err == nil && strings.TrimSpace(doc.MasterKey) != "" {
		if strings.TrimSpace(doc.Version) == "" {
			doc.Version = "v1"
		}
		return &doc, nil
	}
	if _, err := DecodeMasterKey(trimmed); err == nil {
		return &LocalTomb{
			Version:   "v1",
			MasterKey: trimmed,
		}, nil
	}
	return nil, fmt.Errorf("invalid tomb file format")
}

// WriteLocalTomb writes a tomb document to disk.
func WriteLocalTomb(path string, doc *LocalTomb) error {
	if doc == nil {
		return fmt.Errorf("nil tomb payload")
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// LoadEnvSecretsFromTombDoc decrypts env secrets from a tomb document.
func LoadEnvSecretsFromTombDoc(doc *LocalTomb) (map[string]string, error) {
	out := map[string]string{}
	if doc == nil || strings.TrimSpace(doc.EnvNonce) == "" || strings.TrimSpace(doc.EnvCiphertext) == "" {
		return out, nil
	}
	key, err := DecodeMasterKey(doc.MasterKey)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(doc.EnvNonce))
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(doc.EnvCiphertext))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(localTombEnvAAD))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SealEnvSecretsIntoTombDoc encrypts env secrets into a tomb document.
func SealEnvSecretsIntoTombDoc(doc *LocalTomb, kv map[string]string) error {
	if doc == nil {
		return fmt.Errorf("nil tomb payload")
	}
	if kv == nil {
		kv = map[string]string{}
	}
	key, err := DecodeMasterKey(doc.MasterKey)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(kv)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, []byte(localTombEnvAAD))
	doc.EnvNonce = base64.RawStdEncoding.EncodeToString(nonce)
	doc.EnvCiphertext = base64.RawStdEncoding.EncodeToString(ciphertext)
	return nil
}

// --- internal helpers ---

func resolveKeyBackend() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("KAFCLAW_OAUTH_KEY_BACKEND")))
	switch v {
	case "local", "keyring", "file", "auto":
		return v
	default:
		return "local"
	}
}

func loadOrCreateMasterKeyKeyringOnly() ([]byte, error) {
	val, err := keyring.Get(keyringService, keyringUser)
	if err == nil {
		return DecodeMasterKey(val)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)
	if setErr := keyring.Set(keyringService, keyringUser, encoded); setErr != nil {
		return nil, setErr
	}
	return key, nil
}

func loadOrCreateMasterKeyFileOnly() ([]byte, error) {
	authRoot, err := resolveAuthRoot()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(authRoot, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(authRoot, keyFileName)
	if data, err := os.ReadFile(keyPath); err == nil {
		return DecodeMasterKey(strings.TrimSpace(string(data)))
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)
	if err := os.WriteFile(keyPath, []byte(encoded+"\n"), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func loadOrCreateMasterKeyLocalOnly() ([]byte, error) {
	tombPath, doc, err := LoadOrCreateLocalTomb()
	if err != nil {
		return nil, err
	}
	key, err := DecodeMasterKey(doc.MasterKey)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(tombPath, 0o600)
	return key, nil
}

func resolveHomeDir() (string, error) {
	if h := strings.TrimSpace(os.Getenv("KAFCLAW_HOME")); h != "" {
		if strings.HasPrefix(h, "~") {
			base, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			return expandTildePath(h, base), nil
		}
		return h, nil
	}
	return os.UserHomeDir()
}

func expandTildePath(path string, home string) string {
	switch {
	case path == "~":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	default:
		return path
	}
}

// resolveAuthRoot returns the auth directory path.
// This mirrors the original skills.EnsureStateDirs().ToolsDir + "auth" logic:
// <configDir>/skills/tools/auth
func resolveAuthRoot() (string, error) {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), "skills", "tools", "auth"), nil
}
