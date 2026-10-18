package clicache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/provider/credentials"
)

func TestCLICredentialCache(t *testing.T) {
	for _, tc := range []struct {
		name, dir, file, expiry string
		read                    func() (*credentials.OAuthToken, error)
		readFile                func(string) (*credentials.OAuthToken, error)
	}{
		{"codex", ".codex", "auth.json", "expires_at", ReadCodexCLICredential, readCodexCredsFile},
		{"gemini", ".gemini", "oauth_creds.json", "expires_at_seconds", ReadGeminiCLICredential, readGeminiCredsFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("PATH", root)
			path := filepath.Join(root, tc.dir, tc.file)
			if _, err := tc.read(); err == nil {
				t.Fatal("missing cache accepted")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := tc.readFile(path); err == nil {
				t.Fatal("invalid JSON accepted")
			}
			for _, expiry := range []string{"", `,"` + tc.expiry + `":4102444800`} {
				if err := os.WriteFile(path, []byte(`{"access_token":"test-access","refresh_token":"test-refresh","email":"test@example.invalid"`+expiry+`}`), 0600); err != nil {
					t.Fatal(err)
				}
				got, err := tc.read()
				if err != nil || got.Access != "test-access" || got.Refresh != "test-refresh" || got.Email != "test@example.invalid" || got.Expires <= time.Now().Unix() {
					t.Fatalf("cache: %+v %v", got, err)
				}
			}
			expired := `{"access_token":"expired","` + tc.expiry + `":1}`
			if err := os.WriteFile(path, []byte(expired), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := tc.read(); err == nil || !strings.Contains(err.Error(), "refresh failed") {
				t.Fatalf("refresh failure: %v", err)
			}
			script := "#!/bin/sh\n[ \"$1\" = auth ] || exit 2\nprintf '%s' '{\"access_token\":\"refreshed\",\"" + tc.expiry + "\":4102444800}' > '" + path + "'\n"
			if err := os.WriteFile(filepath.Join(root, tc.name), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			got, err := tc.read()
			if err != nil || got.Access != "refreshed" {
				t.Fatalf("refresh success: %+v %v", got, err)
			}
		})
	}
}
