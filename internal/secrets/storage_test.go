package secrets

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zalando/go-keyring"
)

func isolateKeys(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("KAFCLAW_HOME", root)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(root, "config.json"))
	t.Setenv("KAFCLAW_OAUTH_TOMB_FILE", filepath.Join(root, "tomb.rr"))
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", "")
	return root
}

func TestMasterKeyBackends(t *testing.T) {
	for _, backend := range []string{"local", "file", "keyring", "auto", "unknown"} {
		t.Run(backend, func(t *testing.T) {
			isolateKeys(t)
			keyring.MockInit()
			t.Cleanup(keyring.MockInit)
			t.Setenv("KAFCLAW_OAUTH_KEY_BACKEND", backend)
			first, err := LoadOrCreateMasterKey()
			if err != nil {
				t.Fatal(err)
			}
			second, err := LoadOrCreateMasterKey()
			if err != nil {
				t.Fatal(err)
			}
			if len(first) != 32 || !bytes.Equal(first, second) {
				t.Fatal("master key not stable")
			}
			sealed, err := EncryptBlob([]byte("roundtrip"))
			if err != nil {
				t.Fatal(err)
			}
			plain, err := DecryptBlob(sealed)
			if err != nil || string(plain) != "roundtrip" {
				t.Fatalf("roundtrip: %q %v", plain, err)
			}
		})
	}
}

func TestMasterKeyFallbacksAndErrors(t *testing.T) {
	root := isolateKeys(t)
	keyring.MockInitWithError(errors.New("keyring unavailable"))
	t.Cleanup(keyring.MockInit)
	t.Setenv("KAFCLAW_OAUTH_KEY_BACKEND", "keyring")
	if _, err := LoadOrCreateMasterKey(); err == nil {
		t.Fatal("keyring error hidden")
	}
	t.Setenv("KAFCLAW_OAUTH_KEY_BACKEND", "auto")
	if _, err := LoadOrCreateMasterKey(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAFCLAW_OAUTH_TOMB_FILE", root) // local fallback fails; file backend must succeed
	if _, err := LoadOrCreateMasterKey(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAFCLAW_OAUTH_KEY_BACKEND", "local")
	if _, err := LoadOrCreateMasterKey(); err == nil {
		t.Fatal("directory tomb accepted")
	}
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", "invalid")
	if _, err := EncryptBlob([]byte("test")); err == nil {
		t.Fatal("invalid env key accepted")
	}
	if _, err := DecryptBlob([]byte("test")); err == nil {
		t.Fatal("invalid env key accepted for decryption")
	}
	key := bytes.Repeat([]byte{5}, 32)
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", base64.RawStdEncoding.EncodeToString(key))
	got, err := LoadOrCreateMasterKey()
	if err != nil || !bytes.Equal(got, key) {
		t.Fatalf("env override: %v", err)
	}
}

func TestFileMasterKeyFailures(t *testing.T) {
	root := isolateKeys(t)
	t.Setenv("KAFCLAW_OAUTH_KEY_BACKEND", "file")
	auth, err := resolveAuthRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(auth, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(auth, keyFileName)
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateMasterKey(); err == nil {
		t.Fatal("invalid saved key accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateMasterKey(); err == nil {
		t.Fatal("directory key accepted")
	}
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(blocker, "config.json"))
	if _, err := LoadOrCreateMasterKey(); err == nil {
		t.Fatal("non-directory config accepted")
	}
	t.Setenv("KAFCLAW_OAUTH_TOMB_FILE", filepath.Join(blocker, "tomb.rr"))
	if _, _, err := LoadOrCreateLocalTomb(); err == nil {
		t.Fatal("non-directory tomb parent accepted")
	}
}

func TestTombPersistenceAndPaths(t *testing.T) {
	root := isolateKeys(t)
	for _, tc := range []struct{ home, path, want string }{{root, "~/keys.rr", filepath.Join(root, "keys.rr")}, {"~/alternate", "", filepath.Join(root, "alternate/.config/kafclaw/tomb.rr")}, {"", "", filepath.Join(root, ".config/kafclaw/tomb.rr")}} {
		t.Setenv("KAFCLAW_HOME", tc.home)
		t.Setenv("KAFCLAW_OAUTH_TOMB_FILE", tc.path)
		got, err := ResolveLocalTombPath()
		if err != nil || got != tc.want {
			t.Fatalf("path %q: %q %v", tc.path, got, err)
		}
	}
	t.Setenv("KAFCLAW_HOME", root)
	t.Setenv("KAFCLAW_OAUTH_TOMB_FILE", filepath.Join(root, "keys/tomb.rr"))
	path, doc, err := LoadOrCreateLocalTomb()
	if err != nil {
		t.Fatal(err)
	}
	if err := SealEnvSecretsIntoTombDoc(doc, nil); err != nil {
		t.Fatal(err)
	}
	if err := WriteLocalTomb(path, doc); err != nil {
		t.Fatal(err)
	}
	_, again, err := LoadOrCreateLocalTomb()
	if err != nil || !reflect.DeepEqual(again, doc) {
		t.Fatalf("tomb persistence: %v", err)
	}
	if err := WriteLocalTomb(path, nil); err == nil {
		t.Fatal("nil tomb accepted")
	}
	if err := WriteLocalTomb(root, doc); err == nil {
		t.Fatal("directory tomb accepted")
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadOrCreateLocalTomb(); err == nil {
		t.Fatal("corruption accepted")
	}
	if _, err := LoadEnvSecretsFromTombDoc(nil); err != nil {
		t.Fatal(err)
	}
	if err := SealEnvSecretsIntoTombDoc(&LocalTomb{MasterKey: "invalid"}, nil); err == nil {
		t.Fatal("invalid master key accepted")
	}
	raw := []byte(`{"masterKey":"` + base64.RawStdEncoding.EncodeToString(make([]byte, 32)) + `"}`)
	parsed, err := DecodeLocalTomb(raw)
	if err != nil || parsed.Version != "v1" {
		t.Fatalf("legacy version: %+v %v", parsed, err)
	}
	for _, p := range []string{"~", "~/child", "~other", "absolute"} {
		want := p
		if p == "~" {
			want = root
		}
		if p == "~/child" {
			want = filepath.Join(root, "child")
		}
		if got := expandTildePath(p, root); got != want {
			t.Fatalf("expand %q: %q", p, got)
		}
	}
}

func TestEncryptedBlobCorruption(t *testing.T) {
	key := make([]byte, 32)
	if _, err := EncryptBlobWithKey([]byte("test"), []byte("short")); err == nil {
		t.Fatal("invalid AES key accepted")
	}
	for _, tc := range []struct {
		blob string
		key  []byte
	}{{`{"version":"v2","nonce":"AA","ciphertext":"AA"}`, key}, {`{"version":"v1","nonce":"!","ciphertext":"AA"}`, key}, {`{"version":"v1","nonce":"AA","ciphertext":"!"}`, key}, {`{"version":"v1","nonce":"AA","ciphertext":"AA"}`, []byte("short")}} {
		if _, err := DecryptBlobWithKey([]byte(tc.blob), tc.key); err == nil {
			t.Fatal("corrupt blob accepted")
		}
	}
	got, err := DecryptBlobWithKey([]byte("legacy bytes"), key)
	if err != nil || string(got) != "legacy bytes" {
		t.Fatal("legacy passthrough changed")
	}
	doc := &LocalTomb{MasterKey: base64.RawStdEncoding.EncodeToString(key)}
	if err := SealEnvSecretsIntoTombDoc(doc, map[string]string{"test": "value"}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"key", "nonce", "ciphertext", "tamper"} {
		copy := *doc
		switch field {
		case "key":
			copy.MasterKey = "!"
		case "nonce":
			copy.EnvNonce = "!"
		case "ciphertext":
			copy.EnvCiphertext = "!"
		case "tamper":
			b, _ := base64.RawStdEncoding.DecodeString(copy.EnvCiphertext)
			b[0] ^= 1
			copy.EnvCiphertext = base64.RawStdEncoding.EncodeToString(b)
		}
		if _, err := LoadEnvSecretsFromTombDoc(&copy); err == nil {
			b, _ := json.Marshal(copy)
			t.Fatalf("corrupt %s accepted: %s", field, b)
		}
	}
}
