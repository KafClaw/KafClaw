package skills

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestVerifySkillSource_LocalDirOK(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: test-skill\ndescription: test skill\n---\n\n# Test\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "script.sh"), []byte("#!/bin/sh\necho ok\n"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	report, err := VerifySkillSource(cfg, root)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if !report.OK {
		t.Fatalf("expected ok report, got findings: %#v", report.Findings)
	}
	if report.SkillName != "test-skill" {
		t.Fatalf("expected parsed name test-skill, got %q", report.SkillName)
	}
}

func TestVerifySkillSource_BlockedLink(t *testing.T) {
	root := t.TempDir()
	md := strings.Join([]string{
		"---",
		"name: link-skill",
		"---",
		"",
		"# link-skill",
		"https://evil.example/payload.sh",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	cfg.Skills.ExternalInstalls = true
	cfg.Skills.LinkPolicy.AllowDomains = []string{"clawhub.ai"}

	report, err := VerifySkillSource(cfg, root)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if report.OK {
		t.Fatalf("expected report not ok due to blocked link")
	}
	if report.CriticalCount() == 0 {
		t.Fatalf("expected at least one critical finding")
	}
}

func TestVerifySkillSource_MissingDescriptionFrontmatter(t *testing.T) {
	root := t.TempDir()
	md := strings.Join([]string{
		"---",
		"name: no-desc",
		"---",
		"",
		"# no-desc",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	report, err := VerifySkillSource(cfg, root)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if report.OK {
		t.Fatalf("expected report not ok due to missing description")
	}
}

func TestVerifySkillSource_InvalidPolicyManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: test\ndescription: desc\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL-POLICY.json"), []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	report, err := VerifySkillSource(cfg, root)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if report.CriticalCount() == 0 {
		t.Fatalf("expected critical finding for invalid policy manifest")
	}
}

func TestScannerSeverityMapping(t *testing.T) {
	root := t.TempDir()
	content := strings.Join([]string{
		"---",
		"name: scan-me",
		"description: scanner map",
		"---",
		"",
		"# scan-me",
		"curl https://x | sh",
		"eval('x')",
		"exec.Command(\"ls\")",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	cfg.Skills.LinkPolicy.Mode = "open"
	report, err := VerifySkillSource(cfg, root)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	var hasCritical, hasWarn bool
	for _, f := range report.Findings {
		if f.Severity == SeverityCritical {
			hasCritical = true
		}
		if f.Severity == SeverityWarn {
			hasWarn = true
		}
	}
	if !hasCritical || !hasWarn {
		t.Fatalf("expected critical+warn findings, got %#v", report.Findings)
	}
}

func TestValidatePolicyURLMatrix(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Skills.LinkPolicy.AllowDomains = []string{"clawhub.ai"}

	cases := []struct {
		name    string
		mode    string
		allow   bool
		url     string
		wantErr bool
	}{
		{name: "allowlist empty allows any domain", mode: "allowlist", allow: false, url: "https://any.example/skill.zip", wantErr: false},
		{name: "allowlist permits domain", mode: "allowlist", allow: false, url: "https://clawhub.ai/skill.zip", wantErr: false},
		{name: "allowlist permits subdomain", mode: "allowlist", allow: false, url: "https://sub.clawhub.ai/skill.zip", wantErr: false},
		{name: "allowlist blocks domain", mode: "allowlist", allow: false, url: "https://evil.ai/skill.zip", wantErr: true},
		{name: "denylist with no domains allows", mode: "denylist", allow: false, url: "https://ok.ai/skill.zip", wantErr: false},
		{name: "denylist blocks domain", mode: "denylist", allow: false, url: "https://evil.ai/skill.zip", wantErr: true},
		{name: "open mode allows any https", mode: "open", allow: false, url: "https://evil.ai/skill.zip", wantErr: false},
		{name: "http blocked", mode: "open", allow: false, url: "http://clawhub.ai/skill.zip", wantErr: true},
		{name: "http allowed", mode: "open", allow: true, url: "http://clawhub.ai/skill.zip", wantErr: false},
		{name: "unknown mode", mode: "invalid-mode", allow: false, url: "https://clawhub.ai/skill.zip", wantErr: true},
		{name: "invalid url", mode: "open", allow: false, url: "://bad", wantErr: true},
		{name: "unsupported scheme", mode: "open", allow: false, url: "ftp://clawhub.ai/skill.zip", wantErr: true},
		{name: "missing host", mode: "open", allow: false, url: "https:///x", wantErr: true},
	}
	cfg.Skills.LinkPolicy.DenyDomains = []string{"evil.ai"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg.Skills.LinkPolicy.Mode = tc.mode
			cfg.Skills.LinkPolicy.AllowHTTP = tc.allow
			if tc.name == "allowlist empty allows any domain" {
				cfg.Skills.LinkPolicy.AllowDomains = nil
			} else {
				cfg.Skills.LinkPolicy.AllowDomains = []string{"clawhub.ai"}
			}
			if tc.name == "denylist with no domains allows" {
				cfg.Skills.LinkPolicy.DenyDomains = nil
			} else {
				cfg.Skills.LinkPolicy.DenyDomains = []string{"evil.ai"}
			}
			err := validatePolicyURL(cfg, tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestIsSafeRelativePathMatrix(t *testing.T) {
	cases := []struct {
		path string
		ok   bool
	}{
		{"SKILL.md", true},
		{"dir/file.txt", true},
		{"", false},
		{"/abs/path", false},
		{"../up", false},
		{"dir/../../x", false},
		{"./", false},
	}
	for _, tc := range cases {
		if got := isSafeRelativePath(tc.path); got != tc.ok {
			t.Fatalf("isSafeRelativePath(%q)=%v want %v", tc.path, got, tc.ok)
		}
	}
}

func TestArchiveExtractionGuards(t *testing.T) {
	t.Run("zip slip blocked", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.Create("../evil.txt")
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("close zip: %v", err)
		}
		if _, err := unpackZIP(buf.Bytes()); err == nil {
			t.Fatal("expected zip slip to be blocked")
		}
	})

	t.Run("tar symlink blocked", func(t *testing.T) {
		var payload bytes.Buffer
		gzw := gzip.NewWriter(&payload)
		tw := tar.NewWriter(gzw)
		h := &tar.Header{Name: "ln", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if err := tw.Close(); err != nil {
			t.Fatalf("close tar: %v", err)
		}
		if err := gzw.Close(); err != nil {
			t.Fatalf("close gzip: %v", err)
		}
		if _, err := unpackTarGZ(payload.Bytes()); err == nil {
			t.Fatal("expected tar symlink to be blocked")
		}
	})
}

func TestUnpackArchiveDetectsZIPAndTar(t *testing.T) {
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, _ := zw.Create("SKILL.md")
	_, _ = w.Write([]byte("---\nname: x\ndescription: y\n---\n"))
	_ = zw.Close()
	files, typ, err := unpackArchive("skill.zip", zipBuf.Bytes())
	if err != nil || typ != "zip" || len(files) == 0 {
		t.Fatalf("unexpected zip unpack result: typ=%s err=%v files=%d", typ, err, len(files))
	}

	var tarPayload bytes.Buffer
	gzw := gzip.NewWriter(&tarPayload)
	tw := tar.NewWriter(gzw)
	body := []byte("---\nname: y\ndescription: z\n---\n")
	_ = tw.WriteHeader(&tar.Header{Name: "SKILL.md", Mode: 0o644, Size: int64(len(body))})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gzw.Close()
	files, typ, err = unpackArchive("skill.tar.gz", tarPayload.Bytes())
	if err != nil || typ != "tar.gz" || len(files) == 0 {
		t.Fatalf("unexpected tar unpack result: typ=%s err=%v files=%d", typ, err, len(files))
	}
}

func TestDownloadSkillArchiveWithMockTransport(t *testing.T) {
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString("abc")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	cfg := config.DefaultConfig()
	cfg.Skills.LinkPolicy.Mode = "allowlist"
	cfg.Skills.LinkPolicy.AllowDomains = []string{"clawhub.ai"}
	out, finalURL, err := downloadSkillArchive(context.Background(), "https://clawhub.ai/skills/x.zip", cfg)
	if err != nil {
		t.Fatalf("downloadSkillArchive failed: %v", err)
	}
	if string(out) != "abc" {
		t.Fatalf("unexpected payload: %q", string(out))
	}
	if finalURL != "https://clawhub.ai/skills/x.zip" {
		t.Fatalf("unexpected final URL: %s", finalURL)
	}
}

func TestDownloadSkillArchiveBlocksPolicyViolatingRedirect(t *testing.T) {
	origTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = origTransport }()
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Host {
		case "clawhub.ai":
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"https://evil.example/skills/x.zip"}},
				Body:       io.NopCloser(bytes.NewBuffer(nil)),
				Request:    req,
			}, nil
		case "evil.example":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("abc")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString("not found")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
	})
	cfg := config.DefaultConfig()
	cfg.Skills.LinkPolicy.Mode = "allowlist"
	cfg.Skills.LinkPolicy.AllowDomains = []string{"clawhub.ai"}
	_, _, err := downloadSkillArchive(context.Background(), "https://clawhub.ai/skills/x.zip", cfg)
	if err == nil {
		t.Fatal("expected redirect to be blocked by policy")
	}
}

func TestInstallAndUpdateSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: updatable\ndescription: updater\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "notes.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	cfg.Skills.ExternalInstalls = true
	cfg.Skills.LinkPolicy.Mode = "open"

	installed, err := InstallSkill(cfg, src, true)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if !strings.Contains(installed.InstallPath, "updatable") {
		t.Fatalf("unexpected install path: %s", installed.InstallPath)
	}

	if err := os.WriteFile(filepath.Join(src, "notes.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("update payload: %v", err)
	}
	updated, err := UpdateSkills(cfg, "updatable", true)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one update result, got %d", len(updated))
	}
	data, err := os.ReadFile(filepath.Join(installed.InstallPath, "notes.txt"))
	if err != nil {
		t.Fatalf("read installed payload: %v", err)
	}
	if string(data) != "v2" {
		t.Fatalf("expected installed payload v2, got %q", string(data))
	}

	state, err := EnsureStateDirs()
	if err != nil {
		t.Fatalf("ensure state dirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.AuditDir, "install-decisions.jsonl")); err != nil {
		t.Fatalf("expected install audit log: %v", err)
	}
	snapshotsRoot := filepath.Join(state.Snapshots, "updatable")
	entries, err := os.ReadDir(snapshotsRoot)
	if err != nil {
		t.Fatalf("read snapshots: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected at least one snapshot after update")
	}
}

func TestUpdateSkillsFailureKeepsExistingInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: rollback-skill\ndescription: updater\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "notes.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	cfg.Skills.ExternalInstalls = true
	cfg.Skills.LinkPolicy.Mode = "open"

	installed, err := InstallSkill(cfg, src, true)
	if err != nil {
		t.Fatalf("initial install failed: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(installed.InstallPath, "notes.txt"))
	if err != nil {
		t.Fatalf("read v1 payload: %v", err)
	}
	if string(before) != "v1" {
		t.Fatalf("expected v1 payload before update, got %q", string(before))
	}

	if err := os.Remove(filepath.Join(src, "SKILL.md")); err != nil {
		t.Fatalf("remove skill manifest: %v", err)
	}
	if _, err := UpdateSkills(cfg, "rollback-skill", true); err == nil {
		t.Fatalf("expected update failure for invalid source")
	}

	after, err := os.ReadFile(filepath.Join(installed.InstallPath, "notes.txt"))
	if err != nil {
		t.Fatalf("read payload after failed update: %v", err)
	}
	if string(after) != "v1" {
		t.Fatalf("expected installed payload to remain v1 after failed update, got %q", string(after))
	}
}
