package skills

import (
	"os"
	"strings"

	"github.com/KafClaw/KafClaw/internal/secrets"
)

// encryptOAuthStateBlob encrypts an OAuth state blob using the shared secrets package.
func encryptOAuthStateBlob(plain []byte) ([]byte, error) {
	return secrets.EncryptBlob(plain)
}

// decryptOAuthStateBlob decrypts an OAuth state blob using the shared secrets package.
func decryptOAuthStateBlob(data []byte) ([]byte, error) {
	return secrets.DecryptBlob(data)
}

// loadOrCreateOAuthMasterKey delegates to the shared secrets package.
func loadOrCreateOAuthMasterKey() ([]byte, error) {
	return secrets.LoadOrCreateMasterKey()
}

// ResolveLocalOAuthTombPath delegates to the shared secrets package.
func ResolveLocalOAuthTombPath() (string, error) {
	return secrets.ResolveLocalTombPath()
}

// decodeMasterKey delegates to the shared secrets package.
func decodeMasterKey(raw string) ([]byte, error) {
	return secrets.DecodeMasterKey(raw)
}

// StoreEnvSecretsInLocalTomb merges sensitive env kv into tomb-managed encrypted payload.
func StoreEnvSecretsInLocalTomb(kv map[string]string) (int, error) {
	if len(kv) == 0 {
		return 0, nil
	}
	tombPath, doc, err := secrets.LoadOrCreateLocalTomb()
	if err != nil {
		return 0, err
	}
	existing, err := secrets.LoadEnvSecretsFromTombDoc(doc)
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
	if err := secrets.SealEnvSecretsIntoTombDoc(doc, existing); err != nil {
		return 0, err
	}
	if err := secrets.WriteLocalTomb(tombPath, doc); err != nil {
		return 0, err
	}
	if err := os.Chmod(tombPath, 0o600); err != nil {
		return 0, err
	}
	return changed, nil
}

// LoadEnvSecretsFromLocalTomb decrypts env secrets previously stored in tomb.
func LoadEnvSecretsFromLocalTomb() (map[string]string, error) {
	tombPath, err := secrets.ResolveLocalTombPath()
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
	doc, err := secrets.DecodeLocalTomb(data)
	if err != nil {
		return nil, err
	}
	return secrets.LoadEnvSecretsFromTombDoc(doc)
}
