package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestSourceArchiveAndManifestContracts(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Skills.ExternalInstalls = true
	cfg.Skills.LinkPolicy.AllowHTTP = true
	cfg.Skills.LinkPolicy.AllowDomains = []string{"example.com"}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	w, err := zw.Create("root/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("---\nname: archived\ndescription: archive fixture\n---\nRead notes."))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	file := filepath.Join(root, "fixture.zip")
	if err := os.WriteFile(file, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if report, err := VerifySkillSource(cfg, file); err != nil || report.SkillName != "archived" || !report.OK {
		t.Fatalf("archive: %+v %v", report, err)
	}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(archive.Bytes())), Header: http.Header{}}, nil
	})
	if report, err := VerifySkillSource(cfg, "https://example.com/fixture.zip"); err != nil || report.SourceType != "zip" || report.SkillName != "archived" {
		t.Fatalf("remote archive: %+v %v", report, err)
	}
	if _, err := VerifySkillSource(nil, ""); err == nil {
		t.Fatal("empty source accepted")
	}
	if _, err := VerifySkillSource(config.DefaultConfig(), "https://example.com/fixture.zip"); err == nil {
		t.Fatal("disabled remote accepted")
	}
	if _, _, err := unpackArchive("unknown", []byte("invalid")); err == nil {
		t.Fatal("invalid archive accepted")
	}
	if _, _, err := unpackArchive("empty", nil); err == nil {
		t.Fatal("empty archive accepted")
	}
	if parseSkillName([]byte("---\nname: Parsed\ndescription: test\n---\n")) != "parsed" {
		t.Fatal("name not parsed")
	}
	for _, files := range [][]sourceFile{nil, make([]sourceFile, maxSkillFiles+1), {{Path: "a.txt"}}, {{Path: "a/SKILL.md"}, {Path: "b/SKILL.md"}}} {
		if _, _, err := normalizePackage(&sourcePackage{Files: files}, cfg, &VerifyReport{}); err == nil {
			t.Fatal("invalid package accepted")
		}
	}
	for _, policy := range []string{`{}`, `{"version":"v1","execution":{"timeoutSeconds":-1,"maxOutputBytes":-1,"allowCommands":[""],"denyCommands":[""]},"linkPolicy":{"allowDomains":["bad domain"],"denyDomains":[""],"allowHttp":true}}`} {
		report := &VerifyReport{SkillName: "different"}
		pkg := &sourcePackage{Files: []sourceFile{{Path: "SKILL.md", Data: []byte("---\nname: fixture\ndescription: test\n---\n")}, {Path: "SKILL-POLICY.json", Data: []byte(policy)}}}
		validateManifestAndFrontmatter(config.DefaultConfig(), pkg, report)
		if report.CriticalCount() == 0 {
			t.Fatal("invalid policy accepted")
		}
	}
	for _, skill := range []string{"plain markdown", "---\ndescription: only\n---\n", "---\nname: fixture\n---\n"} {
		report := &VerifyReport{}
		validateManifestAndFrontmatter(cfg, &sourcePackage{Files: []sourceFile{{Path: "SKILL.md", Data: []byte(skill)}}}, report)
		if report.CriticalCount() == 0 {
			t.Fatal("invalid frontmatter accepted")
		}
	}
	report := &VerifyReport{}
	validateManifestAndFrontmatter(cfg, &sourcePackage{}, report)
	if report.CriticalCount() != 1 {
		t.Fatal("missing SKILL.md not reported")
	}
	cfg.Skills.LinkPolicy.MaxLinksPerSkill = 1
	report = &VerifyReport{}
	scanPackage(cfg, &sourcePackage{Files: []sourceFile{{Path: "large.txt", Data: []byte(strings.Repeat("a", maxFileBytes+1))}, {Path: "binary", Data: []byte{0}}, {Path: "links", Data: []byte("https://example.com/one https://example.com/two")}}}, report)
	if report.WarningCount() == 0 || report.CriticalCount() == 0 {
		t.Fatal("size/link limits not enforced")
	}
	if !isTextContent(nil) || isTextContent([]byte{255}) {
		t.Fatal("text classification")
	}
}

func TestRapidSkillUpdatesRetainSnapshots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, "config.json"))
	cfg := config.DefaultConfig()
	source := filepath.Join(home, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: rapid\ndescription: Snapshot regression\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		res, err := InstallSkill(cfg, source, true)
		if err != nil {
			t.Fatalf("update %d: %v", i, err)
		}
		if res.Updated != (i > 0) {
			t.Fatal("update state incorrect")
		}
	}
	dirs, err := EnsureStateDirs()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dirs.Snapshots, "rapid"))
	if err != nil || len(entries) != snapshotRetentionPerSkill {
		t.Fatalf("snapshot retention: %d %v", len(entries), err)
	}
	if err := pruneSkillSnapshots(filepath.Join(dirs.Snapshots, "rapid"), 0); err != nil {
		t.Fatal(err)
	}
	entries, _ = os.ReadDir(filepath.Join(dirs.Snapshots, "rapid"))
	if len(entries) != 1 {
		t.Fatal("minimum retention not enforced")
	}
	if err := pruneSkillSnapshots(filepath.Join(home, "missing"), 2); err != nil {
		t.Fatal(err)
	}
	if err := writePackageFiles(t.TempDir(), []sourceFile{{Path: "../escape", Data: []byte("x")}}); err == nil {
		t.Fatal("unsafe output path accepted")
	}
	if _, err := readMetadata(filepath.Join(source, "SKILL.md")); err == nil {
		t.Fatal("invalid metadata accepted")
	}
}

func TestSkillContainerRuntimeContracts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root)
	cfg := config.DefaultConfig()
	cfg.Skills.RuntimeIsolation = "strict"
	script := filepath.Join(root, "docker")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n[ \"$1\" = version ] && exit 0\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if runtime, err := StrictIsolationPreflight(); err != nil || runtime != "docker" {
		t.Fatalf("preflight: %s %v", runtime, err)
	}
	var out bytes.Buffer
	if err := runSkillCommandWithIsolation(context.Background(), cfg, root, root, []string{"printf", "fixture"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--network\nnone", "--read-only", "--cap-drop\nALL", "no-new-privileges"} {
		if !strings.Contains(out.String(), flag) {
			t.Fatalf("missing isolation argument: %s", flag)
		}
	}
	cfg.Skills.RuntimeIsolation = "auto"
	out.Reset()
	if err := runSkillCommandWithIsolation(context.Background(), cfg, root, root, []string{"printf", "fixture"}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := runContainerIsolatedCommand(context.Background(), script, root, root, nil, io.Discard, io.Discard); err == nil {
		t.Fatal("empty command accepted")
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := StrictIsolationPreflight(); err == nil {
		t.Fatal("unusable runtime accepted")
	}
	cfg.Skills.RuntimeIsolation = "unknown"
	if err := runSkillCommandWithIsolation(context.Background(), cfg, root, root, []string{"/usr/bin/printf", "fixture"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestOAuthInvalidStateAndTokenContracts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KAFCLAW_HOME", root)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(root, "config.json"))
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	for _, in := range []OAuthStartInput{{Provider: "unknown"}, {Provider: ProviderM365}, {Provider: ProviderM365, ClientID: "fixture", ClientSecret: "fixture", RedirectURI: ":invalid"}} {
		if _, err := StartOAuthFlow(in); err == nil {
			t.Fatal("invalid flow accepted")
		}
	}
	if _, _, err := ParseOAuthCallbackURL(":invalid"); err == nil {
		t.Fatal("invalid URL accepted")
	}
	if _, _, err := ParseOAuthCallbackURL("https://example.invalid?error=denied"); err == nil {
		t.Fatal("provider error accepted")
	}
	if _, err := CompleteOAuthFlow(OAuthCompleteInput{}); err == nil {
		t.Fatal("empty completion accepted")
	}
	for _, raw := range []string{"", "invalid", `{"access_token":""}`, `{"access_token":"fixture"}`} {
		path, err := oauthStateFile(ProviderM365, "default", "token.json")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := loadOAuthStoredToken(ProviderM365, "default")
		if raw == `{"access_token":"fixture"}` {
			if err != nil || got.Provider != ProviderM365 || got.Profile != "default" {
				t.Fatalf("legacy token defaults: %+v %v", got, err)
			}
		} else if err == nil {
			t.Fatal("invalid token accepted")
		}
	}
	if _, err := parseOptionalRFC3339("invalid"); err == nil {
		t.Fatal("invalid expiry accepted")
	}
	now := time.Now()
	pending := &oauthPending{Provider: ProviderM365, Profile: "default", ClientID: "fixture", ClientSecret: "fixture", RedirectURI: "http://localhost", CreatedAt: now, State: "state"}
	if err := savePendingOAuth(pending); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadPendingOAuth(ProviderM365, "")
	if err != nil || loaded.Profile != "default" {
		t.Fatalf("pending: %+v %v", loaded, err)
	}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	for _, body := range []string{"invalid", `{"error":"denied"}`, `{"access_token":""}`} {
		http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		if _, err := exchangeCodeForToken(pending, "fixture"); err == nil {
			t.Fatal("token exchange failure hidden")
		}
		if _, err := refreshOAuthToken(&oauthStoredToken{Provider: ProviderM365, ClientID: "fixture", ClientSecret: "fixture", RefreshToken: "fixture"}); err == nil {
			t.Fatal("refresh failure hidden")
		}
	}
	raw, _ := json.Marshal(pending)
	if len(raw) == 0 {
		t.Fatal("pending not serializable")
	}
}
