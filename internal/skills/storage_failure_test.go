package skills

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestSkillStatePathFailures(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KAFCLAW_HOME", root)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(root, "config.json"))
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureNVMRC("", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureNVMRC(blocker, ""); err == nil {
		t.Fatal("file workspace accepted")
	}
	dirs, err := EnsureStateDirs()
	if err != nil {
		t.Fatal(err)
	}
	pending := &oauthPending{Provider: ProviderM365, Profile: "default", ClientID: "fixture", ClientSecret: "fixture", RedirectURI: "http://localhost"}
	path, err := oauthStateFile(ProviderM365, "default", "pending.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadPendingOAuth(ProviderM365, "default"); err == nil {
		t.Fatal("missing pending state accepted")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPendingOAuth(ProviderM365, "default"); err == nil {
		t.Fatal("corrupt pending accepted")
	}
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", "invalid")
	if _, err := loadPendingOAuth(ProviderM365, "default"); err == nil {
		t.Fatal("invalid decryption key accepted")
	}
	if err := savePendingOAuth(pending); err == nil {
		t.Fatal("invalid encryption key accepted")
	}
	if _, err := saveOAuthToken(pending, &oauthTokenRaw{AccessToken: "fixture"}, time.Now(), time.Time{}); err == nil {
		t.Fatal("invalid token encryption key accepted")
	}
	if err := saveRefreshedOAuthToken(&oauthStoredToken{Provider: ProviderM365, Profile: "default"}, &oauthTokenRaw{AccessToken: "fixture"}, time.Now()); err == nil {
		t.Fatal("invalid refresh encryption key accepted")
	}
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	// A file in place of the state root must fail all state-writing workflows.
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(blocker, "config.json"))
	if _, err := EnsureStateDirs(); err == nil {
		t.Fatal("invalid state root accepted")
	}
	if _, err := oauthStateDir(ProviderM365, "default"); err == nil {
		t.Fatal("invalid auth root accepted")
	}
	if err := removePendingOAuth(ProviderM365, "default"); err == nil {
		t.Fatal("invalid pending root accepted")
	}
	if _, err := loadPendingOAuth(ProviderM365, "default"); err == nil {
		t.Fatal("invalid pending root accepted")
	}
	if err := savePendingOAuth(pending); err == nil {
		t.Fatal("invalid pending root accepted")
	}
	if _, err := saveOAuthToken(pending, &oauthTokenRaw{}, time.Now(), time.Time{}); err == nil {
		t.Fatal("invalid token root accepted")
	}
	if err := saveRefreshedOAuthToken(&oauthStoredToken{Provider: ProviderM365}, &oauthTokenRaw{}, time.Now()); err == nil {
		t.Fatal("invalid refresh root accepted")
	}
	if err := appendInstallAudit(installAuditEntry{}); err == nil {
		t.Fatal("invalid audit root accepted")
	}
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(root, "config.json"))
	cfg := config.DefaultConfig()
	if EffectiveSkillEnabled(nil, "fixture") {
		t.Fatal("nil config enabled skill")
	}
	cfg.Skills.Enabled = true
	cfg.Skills.Scope = "all"
	if !EffectiveSkillEnabled(cfg, "fixture") {
		t.Fatal("all scope not honored")
	}
	cfg.Skills.Scope = "selected"
	if EffectiveSkillEnabled(cfg, "missing") {
		t.Fatal("unknown skill enabled")
	}
	if _, err := loadOrCreateOAuthMasterKey(); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeMasterKey("bad"); err == nil {
		t.Fatal("bad encoded key accepted")
	}
	t.Setenv("KAFCLAW_OAUTH_TOMB_FILE", filepath.Join(root, "tomb.rr"))
	if n, err := StoreEnvSecretsInLocalTomb(nil); err != nil || n != 0 {
		t.Fatal("empty secrets")
	}
	if _, err := LoadEnvSecretsFromLocalTomb(); err != nil {
		t.Fatal("missing tomb should be empty")
	}
	if n, err := StoreEnvSecretsInLocalTomb(map[string]string{" ": "ignored", "fixture": "value"}); err != nil || n != 1 {
		t.Fatalf("secrets: %d %v", n, err)
	}
	if n, err := StoreEnvSecretsInLocalTomb(map[string]string{"fixture": "value"}); err != nil || n != 0 {
		t.Fatal("unchanged secret counted")
	}
	if err := os.WriteFile(filepath.Join(root, "tomb.rr"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEnvSecretsFromLocalTomb(); err == nil {
		t.Fatal("corrupt tomb accepted")
	}
	if _, err := StoreEnvSecretsInLocalTomb(map[string]string{"fixture": "value"}); err == nil {
		t.Fatal("corrupt tomb overwritten")
	}
	if err := writePackageFiles(blocker, []sourceFile{{Path: "child/file", Data: []byte("fixture")}}); err == nil {
		t.Fatal("non-directory install root accepted")
	}
	if err := writeMetadata(filepath.Join(blocker, "metadata"), installedMetadata{}); err == nil {
		t.Fatal("non-directory metadata root accepted")
	}
	if err := pruneSkillSnapshots(blocker, 2); err == nil {
		t.Fatal("non-directory snapshots accepted")
	}
	if _, err := detectSingleInstalledSkillDir(filepath.Join(root, "missing"), "test"); err == nil {
		t.Fatal("missing clawhub output accepted")
	}
	if _, err := detectSingleInstalledSkillDir(dirs.TmpDir, "test"); err == nil {
		t.Fatal("empty clawhub output accepted")
	}
}

func TestOAuthExchangeHTTPFailures(t *testing.T) {
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	pending := &oauthPending{Provider: ProviderGoogleWorkspace, ClientID: "fixture", RedirectURI: "http://localhost"}
	stored := &oauthStoredToken{Provider: ProviderM365, ClientID: "fixture", RefreshToken: "fixture"}
	for _, tc := range []struct {
		status int
		body   string
		err    error
	}{{200, "invalid", nil}, {200, `{}`, nil}, {400, `{}`, nil}, {0, "", errors.New("offline")}} {
		http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			if tc.err != nil {
				return nil, tc.err
			}
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(bytes.NewBufferString(tc.body))}, nil
		})
		raw, err := exchangeCodeForToken(pending, "fixture")
		if tc.status == 200 && tc.body == `{}` {
			if err != nil || raw.AccessToken != "" {
				t.Fatal("empty exchange response changed")
			}
		} else if err == nil {
			t.Fatal("exchange failure hidden")
		}
		if _, err := refreshOAuthToken(stored); err == nil {
			t.Fatal("refresh failure hidden")
		}
	}
	if _, err := exchangeCodeForToken(&oauthPending{Provider: "invalid"}, "fixture"); err == nil {
		t.Fatal("unknown provider accepted")
	}
	if _, err := refreshOAuthToken(&oauthStoredToken{Provider: "invalid"}); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestArchiveSizeAndTypeLimits(t *testing.T) {
	for _, header := range []*tar.Header{{Name: "huge", Size: maxArchiveEntryBytes + 1, Mode: 0600, Typeflag: tar.TypeReg}, {Name: "fifo", Typeflag: tar.TypeFifo, Mode: 0600}, {Name: "/absolute", Typeflag: tar.TypeReg, Mode: 0600}, {Name: "dir/", Typeflag: tar.TypeDir, Mode: 0700}} {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		_ = tw.Close()
		gz.Close()
		_, err := unpackTarGZ(b.Bytes())
		if header.Typeflag != tar.TypeDir && err == nil {
			t.Fatalf("invalid tar entry accepted: %+v", header)
		}
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, err := zw.Create("huge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(bytes.Repeat([]byte("x"), maxArchiveEntryBytes+1)); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	if _, err := unpackZIP(b.Bytes()); err == nil {
		t.Fatal("oversized zip accepted")
	}
	root := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := readLocalDir(root); err == nil {
		t.Fatal("symlink accepted")
	}
}
