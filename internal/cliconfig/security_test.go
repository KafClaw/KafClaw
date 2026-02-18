package cliconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestRunSecurityAuditDeepNoInstalledSkills(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"skills":{"enabled":true}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunSecurityAudit(SecurityAuditOptions{Deep: true})
	if err != nil {
		t.Fatalf("run security audit: %v", err)
	}
	found := false
	for _, c := range report.Checks {
		if c.Name == "skills_verify_installed" {
			found = true
			if !strings.Contains(c.Message, "no installed skills") {
				t.Fatalf("unexpected deep audit message: %#v", c)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected skills_verify_installed check, got %#v", report.Checks)
	}
}

func TestRunSecurityCheckReportsHashPinningGap(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"skills":{"enabled":true}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	installed := filepath.Join(cfgDir, "skills", "installed")
	if err := os.MkdirAll(filepath.Join(installed, "demo"), 0o700); err != nil {
		t.Fatalf("mkdir installed skill: %v", err)
	}
	meta := `{"name":"demo","source":"https://example.org/demo.zip","sourceHash":"abc","pinnedHash":""}`
	if err := os.WriteFile(filepath.Join(installed, "demo", ".kafclaw-skill.json"), []byte(meta), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunSecurityCheck()
	if err != nil {
		t.Fatalf("run security check: %v", err)
	}
	found := false
	for _, c := range report.Checks {
		if c.Name == "gap_skill_hash_pinning" {
			found = true
			if c.Status != SecurityWarn {
				t.Fatalf("expected hash pinning warning, got %#v", c)
			}
		}
	}
	if !found {
		t.Fatalf("expected gap_skill_hash_pinning check, got %#v", report.Checks)
	}
}

func TestRunSecurityFixRequiresConfirmation(t *testing.T) {
	if _, err := RunSecurityFix(SecurityFixOptions{}); err == nil {
		t.Fatal("expected confirmation error when --yes is missing")
	}
}

func TestRunSecurityFixAppliesSecureSkillsDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	seed := `{
  "skills": {
    "enabled": true,
    "scope": "all",
    "runtimeIsolation": "auto",
    "linkPolicy": {
      "mode": "",
      "allowHttp": true,
      "maxLinksPerSkill": 0
    }
  }
}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(seed), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunSecurityFix(SecurityFixOptions{Yes: true})
	if err != nil {
		t.Fatalf("RunSecurityFix error: %v", err)
	}
	if report.Mode != "fix" {
		t.Fatalf("expected fix mode report, got %q", report.Mode)
	}
	marshaled := MarshalSecurityReport(report)
	if !strings.Contains(marshaled, "skills_policy_defaults") {
		t.Fatalf("expected skills_policy_defaults check in report: %s", marshaled)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config after fix: %v", err)
	}
	if cfg.Skills.Scope != "selected" {
		t.Fatalf("expected scope=selected, got %q", cfg.Skills.Scope)
	}
	if cfg.Skills.RuntimeIsolation != "strict" {
		t.Fatalf("expected runtimeIsolation=strict, got %q", cfg.Skills.RuntimeIsolation)
	}
	if cfg.Skills.LinkPolicy.Mode != "allowlist" {
		t.Fatalf("expected linkPolicy.mode=allowlist, got %q", cfg.Skills.LinkPolicy.Mode)
	}
	if cfg.Skills.LinkPolicy.AllowHTTP {
		t.Fatalf("expected linkPolicy.allowHttp=false")
	}
	if cfg.Skills.LinkPolicy.MaxLinksPerSkill != 20 {
		t.Fatalf("expected maxLinksPerSkill=20, got %d", cfg.Skills.LinkPolicy.MaxLinksPerSkill)
	}
	if len(cfg.Skills.LinkPolicy.AllowDomains) == 0 {
		t.Fatal("expected allowDomains default to be set")
	}
}

func TestIsEncryptedOAuthBlobFile(t *testing.T) {
	tmpDir := t.TempDir()
	encryptedPath := filepath.Join(tmpDir, "encrypted.json")
	plainPath := filepath.Join(tmpDir, "token.json")

	encryptedBlob := map[string]any{
		"version":    "v1",
		"nonce":      "abc",
		"ciphertext": "def",
	}
	plainBlob := map[string]any{
		"access_token": "token",
		"expires_in":   3600,
	}
	encData, _ := json.Marshal(encryptedBlob)
	plainData, _ := json.Marshal(plainBlob)
	if err := os.WriteFile(encryptedPath, encData, 0o600); err != nil {
		t.Fatalf("write encrypted blob: %v", err)
	}
	if err := os.WriteFile(plainPath, plainData, 0o600); err != nil {
		t.Fatalf("write plain blob: %v", err)
	}

	if !isEncryptedOAuthBlobFile(encryptedPath) {
		t.Fatalf("expected encrypted blob to be detected")
	}
	if isEncryptedOAuthBlobFile(plainPath) {
		t.Fatalf("expected plain oauth token file to not be detected as encrypted")
	}
}

func TestSecurityReportHasFailures(t *testing.T) {
	report := SecurityReport{
		Checks: []SecurityCheck{
			{Name: "ok", Status: SecurityPass},
			{Name: "warn", Status: SecurityWarn},
			{Name: "fail", Status: SecurityFail},
		},
	}
	if !report.HasFailures() {
		t.Fatal("expected HasFailures to return true when at least one check failed")
	}
}

func TestSecurityReportHasFailuresFalse(t *testing.T) {
	report := SecurityReport{
		Checks: []SecurityCheck{
			{Name: "ok", Status: SecurityPass},
			{Name: "warn", Status: SecurityWarn},
		},
	}
	if report.HasFailures() {
		t.Fatal("expected HasFailures to return false when no check failed")
	}
}

func TestRunSecurityAuditDeepChecksInstalledSkills(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"skills":{"enabled":true}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	installedSkillDir := filepath.Join(cfgDir, "skills", "installed", "demo")
	if err := os.MkdirAll(installedSkillDir, 0o700); err != nil {
		t.Fatalf("mkdir installed skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installedSkillDir, ".kafclaw-skill.json"), []byte(`{"name":"demo","source":"https://example.org/demo.zip"}`), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunSecurityAudit(SecurityAuditOptions{Deep: true})
	if err != nil {
		t.Fatalf("run security audit deep: %v", err)
	}
	found := false
	for _, c := range report.Checks {
		if strings.HasPrefix(c.Name, "skills_verify:demo") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected per-skill deep check for demo, got %#v", report.Checks)
	}
}

func TestRunSecurityCheckReportsOAuthEncryptionPartialCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"skills":{"enabled":true}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	plainTokenDir := filepath.Join(cfgDir, "skills", "tools", "auth", "google-workspace", "default")
	encTokenDir := filepath.Join(cfgDir, "skills", "tools", "auth", "m365", "default")
	if err := os.MkdirAll(plainTokenDir, 0o700); err != nil {
		t.Fatalf("mkdir plain token dir: %v", err)
	}
	if err := os.MkdirAll(encTokenDir, 0o700); err != nil {
		t.Fatalf("mkdir encrypted token dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plainTokenDir, "token.json"), []byte(`{"access_token":"plain"}`), 0o600); err != nil {
		t.Fatalf("write plain token: %v", err)
	}
	if err := os.WriteFile(filepath.Join(encTokenDir, "token.json"), []byte(`{"version":"v1","nonce":"a","ciphertext":"b"}`), 0o600); err != nil {
		t.Fatalf("write encrypted token: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunSecurityCheck()
	if err != nil {
		t.Fatalf("run security check: %v", err)
	}
	found := false
	for _, c := range report.Checks {
		if c.Name == "gap_oauth_encryption" {
			found = true
			if c.Status != SecurityWarn {
				t.Fatalf("expected oauth encryption warning for partial coverage, got %#v", c)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected gap_oauth_encryption check, got %#v", report.Checks)
	}
}
