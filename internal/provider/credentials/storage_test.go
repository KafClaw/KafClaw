package credentials

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/secrets"
)

func isolateStorage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("KAFCLAW_HOME", root)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(root, "config.json"))
	t.Setenv("KAFCLAW_OAUTH_TOMB_FILE", filepath.Join(root, "tomb.rr"))
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	return root
}

func TestTokenStorageRoundTrip(t *testing.T) {
	root := isolateStorage(t)
	want := &OAuthToken{Access: "test-access", Refresh: "test-refresh", Expires: 2000000000, Email: "test@example.invalid"}
	if err := SaveToken("example", want); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "skills/tools/auth/providers/example/token.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(want.Access)) {
		t.Fatal("token persisted in plaintext")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v", info.Mode())
	}
	got, err := LoadToken("example")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	// Legacy plaintext tokens remain readable.
	if err := os.WriteFile(path, []byte(`{"access_token":"legacy","expires_at":2000000000}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = LoadToken("example")
	if err != nil || got.Access != "legacy" {
		t.Fatalf("legacy token: %+v %v", got, err)
	}
}

func TestTokenStorageFailures(t *testing.T) {
	root := isolateStorage(t)
	if err := SaveToken("example", nil); err == nil {
		t.Fatal("nil token accepted")
	}
	if _, err := LoadToken("missing"); err == nil || !strings.Contains(err.Error(), "load token") {
		t.Fatalf("missing token: %v", err)
	}
	if err := SaveToken("example", &OAuthToken{Access: "test"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "skills/tools/auth/providers/example/token.json")
	for _, tc := range []struct{ body, want string }{{"not json", "parse token"}, {`{"version":"v2","nonce":"AA","ciphertext":"AA"}`, "decrypt token"}} {
		if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadToken("example"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected %s: %v", tc.want, err)
		}
	}
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", "invalid")
	if err := SaveToken("example", &OAuthToken{}); err == nil || !strings.Contains(err.Error(), "encrypt token") {
		t.Fatalf("invalid encryption key: %v", err)
	}
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(path, "config.json"))
	if err := SaveToken("example", &OAuthToken{}); err == nil {
		t.Fatal("non-directory auth path accepted")
	}
}

func TestAPIKeyStoragePreservesOtherProviders(t *testing.T) {
	isolateStorage(t)
	if _, err := LoadAPIKey("OpenAI"); err == nil {
		t.Fatal("missing tomb accepted")
	}
	if err := SaveAPIKey("OpenAI", "test-openai"); err != nil {
		t.Fatal(err)
	}
	if err := SaveAPIKey("Other", "test-other"); err != nil {
		t.Fatal(err)
	}
	if err := SaveAPIKey("OPENAI", "replacement"); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"openai": "replacement", "other": "test-other"} {
		got, err := LoadAPIKey(id)
		if err != nil || got != want {
			t.Fatalf("%s lookup: %q %v", id, got, err)
		}
	}
	if _, err := LoadAPIKey("missing"); err == nil {
		t.Fatal("missing provider accepted")
	}
	if err := SaveAPIKey("empty", " "); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAPIKey("empty"); err == nil {
		t.Fatal("blank key accepted")
	}
	path, err := secrets.ResolveLocalTombPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v", info.Mode())
	}
}

func TestAPIKeyStorageCorruption(t *testing.T) {
	root := isolateStorage(t)
	path := filepath.Join(root, "tomb.rr")
	for _, body := range []string{"invalid", `{"masterKey":"bad","envNonce":"AA","envCiphertext":"AA"}`} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadAPIKey("test"); err == nil {
			t.Fatal("corrupt tomb accepted on load")
		}
		if err := SaveAPIKey("test", "value"); err == nil {
			t.Fatal("corrupt tomb accepted on save")
		}
	}
	t.Setenv("KAFCLAW_OAUTH_TOMB_FILE", root)
	if _, err := LoadAPIKey("test"); err == nil {
		t.Fatal("directory tomb accepted")
	}
	if err := SaveAPIKey("test", "value"); err == nil {
		t.Fatal("directory tomb accepted on save")
	}
}
