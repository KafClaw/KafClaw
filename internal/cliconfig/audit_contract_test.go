package cliconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/skills"
)

func TestInstalledSkillAuditStates(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("KAFCLAW_HOME", root)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(root, "config.json"))
	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	dirs, err := skills.EnsureStateDirs()
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"missing": "", "warn": "---\nname: warn\ndescription: warning fixture\n---\n", "critical": "---\nname: critical\n---\n", "valid": "---\nname: valid\ndescription: valid fixture\n---\n"} {
		dir := filepath.Join(dirs.Installed, name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if body != "" {
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if name == "valid" {
			if err := os.WriteFile(filepath.Join(dir, "SKILL-POLICY.json"), []byte(`{"version":"v1"}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	report := &SecurityReport{}
	appendDeepSkillAuditChecks(cfg, report)
	states := map[string]SecurityStatus{}
	for _, c := range report.Checks {
		states[c.Name] = c.Status
	}
	for name, want := range map[string]SecurityStatus{"missing": SecurityFail, "warn": SecurityWarn, "critical": SecurityFail, "valid": SecurityPass} {
		if states["skills_verify:"+name] != want {
			t.Fatalf("%s audit state: %+v", name, states)
		}
	}
	appendDeepSkillAuditChecks(nil, report)
	appendDeepSkillAuditChecks(cfg, nil)
	// The auditor must report an unreadable installed root rather than claiming no skills.
	for _, name := range []string{"missing", "warn", "critical", "valid"} {
		if err := os.RemoveAll(filepath.Join(dirs.Installed, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(dirs.Installed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dirs.Installed, []byte("blocker"), 0600); err != nil {
		t.Fatal(err)
	}
	report = &SecurityReport{}
	appendDeepSkillAuditChecks(cfg, report)
	if !report.HasFailures() {
		t.Fatal("invalid installed root not failed")
	}
}

func TestDoctorMissingHomeAndInvalidSecretPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("KAFCLAW_HOME", root)
	t.Setenv("MIKROBOT_HOME", "")
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(root, "config.json"))
	t.Setenv("MIKROBOT_CONFIG", "")
	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	dirs, err := skills.EnsureStateDirs()
	if err != nil {
		t.Fatal(err)
	}
	// Secret-file permission checks cover pending OAuth state as well as tokens.
	auth := filepath.Join(dirs.ToolsDir, "auth", "m365", "default")
	if err := os.MkdirAll(auth, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pending.json", "token.json"} {
		if err := os.WriteFile(filepath.Join(auth, name), []byte(`{"access_token":"fixture"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	checks, _ := checkSkillsSecretFilePermissions(dirs.ToolsDir)
	if len(checks) == 0 {
		t.Fatal("no secret permission checks")
	}
	if err := fixSkillsPermissions(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pending.json", "token.json"} {
		info, err := os.Stat(filepath.Join(auth, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("secret permissions not repaired: %v", err)
		}
	}
	t.Setenv("KAFCLAW_HOME", "")
	t.Setenv("KAFCLAW_CONFIG", "")
	t.Setenv("HOME", "")
	report, err := RunDoctorWithOptions(DoctorOptions{})
	if err != nil || !report.HasFailures() {
		t.Fatalf("missing home: %+v %v", report, err)
	}
	if len(report.Checks) != 1 || !strings.Contains(report.Checks[0].Name, "config_path") {
		t.Fatalf("unexpected missing-home report: %+v", report)
	}
}
