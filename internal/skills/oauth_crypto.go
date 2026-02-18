package skills

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

	"github.com/zalando/go-keyring"
)

const oauthKeyFileName = "master.key"
const oauthLocalTombFileName = "tomb.rr"
const oauthKeyringService = "kafclaw.skills.oauth"
const oauthKeyringUser = "master-key"
const localTombEnvAAD = "kafclaw-local-env-secrets-v1"

type localTomb struct {
	Version       string `json:"version"`
	MasterKey     string `json:"masterKey"`
	EnvNonce      string `json:"envNonce,omitempty"`
	EnvCiphertext string `json:"envCiphertext,omitempty"`
}

type encryptedOAuthBlob struct {
	Version    string `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func encryptOAuthStateBlob(plain []byte) ([]byte, error) {
	key, err := loadOrCreateOAuthMasterKey()
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
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	out := encryptedOAuthBlob{
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

func decryptOAuthStateBlob(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty oauth state blob")
	}
	var wrapped encryptedOAuthBlob
	if err := json.Unmarshal(data, &wrapped); err != nil {
		// Backward compatibility: old plaintext JSON.
		return data, nil
	}
	if wrapped.Version == "" || wrapped.Nonce == "" || wrapped.Ciphertext == "" {
		// Backward compatibility: plaintext JSON object.
		return data, nil
	}
	if wrapped.Version != "v1" {
		return nil, fmt.Errorf("unsupported oauth state blob version: %s", wrapped.Version)
	}
	key, err := loadOrCreateOAuthMasterKey()
	if err != nil {
		return nil, err
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

func loadOrCreateOAuthMasterKey() ([]byte, error) {
	if envKey := strings.TrimSpace(os.Getenv("KAFCLAW_OAUTH_MASTER_KEY")); envKey != "" {
		key, err := decodeMasterKey(envKey)
		if err != nil {
			return nil, fmt.Errorf("invalid KAFCLAW_OAUTH_MASTER_KEY: %w", err)
		}
		return key, nil
	}

	switch resolveOAuthKeyBackend() {
	case "local":
		return loadOrCreateOAuthMasterKeyLocalOnly()
	case "keyring":
		return loadOrCreateOAuthMasterKeyKeyringOnly()
	case "file":
		return loadOrCreateOAuthMasterKeyFileOnly()
	default:
		if key, err := loadOrCreateOAuthMasterKeyKeyringOnly(); err == nil {
			return key, nil
		}
		if key, err := loadOrCreateOAuthMasterKeyLocalOnly(); err == nil {
			return key, nil
		}
		return loadOrCreateOAuthMasterKeyFileOnly()
	}
}

func resolveOAuthKeyBackend() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("KAFCLAW_OAUTH_KEY_BACKEND")))
	switch v {
	case "local", "keyring", "file", "auto":
		return v
	default:
		return "local"
	}
}

func loadOrCreateOAuthMasterKeyKeyringOnly() ([]byte, error) {
	val, err := keyring.Get(oauthKeyringService, oauthKeyringUser)
	if err == nil {
		return decodeMasterKey(val)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	encoded := base64.RawStdEncoding.EncodeToString(key)
	if setErr := keyring.Set(oauthKeyringService, oauthKeyringUser, encoded); setErr != nil {
		return nil, setErr
	}
	return key, nil
}

func loadOrCreateOAuthMasterKeyFileOnly() ([]byte, error) {
	dirs, err := EnsureStateDirs()
	if err != nil {
		return nil, err
	}
	authRoot := filepath.Join(dirs.ToolsDir, "auth")
	if err := os.MkdirAll(authRoot, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(authRoot, oauthKeyFileName)
	if data, err := os.ReadFile(keyPath); err == nil {
		return decodeMasterKey(strings.TrimSpace(string(data)))
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

// ResolveLocalOAuthTombPath returns the local key tomb file path.
func ResolveLocalOAuthTombPath() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("KAFCLAW_OAUTH_TOMB_FILE")); explicit != "" {
		if strings.HasPrefix(explicit, "~") {
			home, err := resolveOAuthHomeDir()
			if err != nil {
				return "", err
			}
			return expandOAuthTildePath(explicit, home), nil
		}
		return explicit, nil
	}
	home, err := resolveOAuthHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "kafclaw", oauthLocalTombFileName), nil
}

func resolveOAuthHomeDir() (string, error) {
	if h := strings.TrimSpace(os.Getenv("KAFCLAW_HOME")); h != "" {
		if strings.HasPrefix(h, "~") {
			base, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			return expandOAuthTildePath(h, base), nil
		}
		return h, nil
	}
	return os.UserHomeDir()
}

func expandOAuthTildePath(path string, home string) string {
	switch {
	case path == "~":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	default:
		return path
	}
}

func loadOrCreateOAuthMasterKeyLocalOnly() ([]byte, error) {
	tombPath, doc, err := loadOrCreateLocalTomb()
	if err != nil {
		return nil, err
	}
	key, err := decodeMasterKey(doc.MasterKey)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(tombPath, 0o600)
	return key, nil
}

func loadOrCreateLocalTomb() (string, *localTomb, error) {
	tombPath, err := ResolveLocalOAuthTombPath()
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(tombPath), 0o700); err != nil {
		return "", nil, err
	}
	if data, err := os.ReadFile(tombPath); err == nil {
		doc, decErr := decodeLocalTomb(data)
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
	doc := &localTomb{
		Version:   "v1",
		MasterKey: encoded,
	}
	if err := writeLocalTomb(tombPath, doc); err != nil {
		return "", nil, err
	}
	return tombPath, doc, nil
}

func decodeLocalTomb(data []byte) (*localTomb, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, fmt.Errorf("empty tomb file")
	}
	var doc localTomb
	if err := json.Unmarshal([]byte(trimmed), &doc); err == nil && strings.TrimSpace(doc.MasterKey) != "" {
		if strings.TrimSpace(doc.Version) == "" {
			doc.Version = "v1"
		}
		return &doc, nil
	}
	// Backward-compatible: tomb was raw base64 key only.
	if _, err := decodeMasterKey(trimmed); err == nil {
		return &localTomb{
			Version:   "v1",
			MasterKey: trimmed,
		}, nil
	}
	return nil, fmt.Errorf("invalid tomb file format")
}

func writeLocalTomb(path string, doc *localTomb) error {
	if doc == nil {
		return fmt.Errorf("nil tomb payload")
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func loadEnvSecretsFromLocalTombDoc(doc *localTomb) (map[string]string, error) {
	out := map[string]string{}
	if doc == nil || strings.TrimSpace(doc.EnvNonce) == "" || strings.TrimSpace(doc.EnvCiphertext) == "" {
		return out, nil
	}
	key, err := decodeMasterKey(doc.MasterKey)
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

func sealEnvSecretsIntoLocalTombDoc(doc *localTomb, kv map[string]string) error {
	if doc == nil {
		return fmt.Errorf("nil tomb payload")
	}
	if kv == nil {
		kv = map[string]string{}
	}
	key, err := decodeMasterKey(doc.MasterKey)
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

// StoreEnvSecretsInLocalTomb merges sensitive env kv into tomb-managed encrypted payload.
func StoreEnvSecretsInLocalTomb(kv map[string]string) (int, error) {
	if len(kv) == 0 {
		return 0, nil
	}
	tombPath, doc, err := loadOrCreateLocalTomb()
	if err != nil {
		return 0, err
	}
	existing, err := loadEnvSecretsFromLocalTombDoc(doc)
	if err != nil {
		return 0, err
	}
	changed := 0
	for k, v := range kv {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if old, ok := existing[k]; !ok || old != v {
			existing[k] = v
			changed++
		}
	}
	if changed == 0 {
		return 0, nil
	}
	if err := sealEnvSecretsIntoLocalTombDoc(doc, existing); err != nil {
		return 0, err
	}
	if err := writeLocalTomb(tombPath, doc); err != nil {
		return 0, err
	}
	if err := os.Chmod(tombPath, 0o600); err != nil {
		return 0, err
	}
	return changed, nil
}

// LoadEnvSecretsFromLocalTomb decrypts env secrets previously stored in tomb.
func LoadEnvSecretsFromLocalTomb() (map[string]string, error) {
	tombPath, err := ResolveLocalOAuthTombPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(tombPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	doc, err := decodeLocalTomb(data)
	if err != nil {
		return nil, err
	}
	return loadEnvSecretsFromLocalTombDoc(doc)
}

func decodeMasterKey(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	decoded := make([]byte, base64.RawStdEncoding.DecodedLen(len(trimmed)))
	n, err := base64.RawStdEncoding.Decode(decoded, []byte(trimmed))
	if err != nil {
		return nil, err
	}
	if n != 32 {
		return nil, fmt.Errorf("invalid oauth master key length: %d", n)
	}
	return decoded[:n], nil
}
