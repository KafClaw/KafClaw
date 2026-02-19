package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/cliconfig"
	"github.com/KafClaw/KafClaw/internal/config"
)

func TestPreflightUpdateCompatibility(t *testing.T) {
	if err := preflightUpdateCompatibility("2.6.3", "v2.7.0", false); err != nil {
		t.Fatalf("expected compatible version, got %v", err)
	}
	if err := preflightUpdateCompatibility("2.6.3", "v4.0.0", false); err == nil {
		t.Fatal("expected major jump failure")
	}
	if err := preflightUpdateCompatibility("2.6.3", "v2.5.9", false); err == nil {
		t.Fatal("expected downgrade failure without allow flag")
	}
	if err := preflightUpdateCompatibility("2.6.3", "v2.5.9", true); err != nil {
		t.Fatalf("expected allowed downgrade, got %v", err)
	}
	if err := preflightUpdateCompatibility("dev", "v2.6.3", false); err != nil {
		t.Fatalf("expected current-version parse fallback path to pass, got %v", err)
	}
	if err := preflightUpdateCompatibility("2.6.3", "not-semver", false); err == nil {
		t.Fatal("expected invalid target version error")
	}
}

func TestCreateAndRestoreUpdateBackup(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origKHome := os.Getenv("KAFCLAW_HOME")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKHome)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("KAFCLAW_HOME", tmpDir)

	cfgPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	cfgDir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir cfg dir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"gateway":{"port":18790}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	envPath := filepath.Join(tmpDir, ".config", "kafclaw", "env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		t.Fatalf("mkdir env dir: %v", err)
	}
	if err := os.WriteFile(envPath, []byte("A=1\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	dbPath := filepath.Join(tmpDir, ".kafclaw", "timeline.db")
	if err := os.WriteFile(dbPath, []byte("db"), 0o600); err != nil {
		t.Fatalf("write db: %v", err)
	}

	origNow := nowFn
	nowFn = func() time.Time { return time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC) }
	defer func() { nowFn = origNow }()

	backupRoot := filepath.Join(tmpDir, "backups")
	snapshot, _, err := createUpdateBackup(backupRoot)
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snapshot, "manifest.json")); err != nil {
		t.Fatalf("missing manifest: %v", err)
	}

	if err := os.WriteFile(cfgPath, []byte(`{"gateway":{"port":19999}}`), 0o600); err != nil {
		t.Fatalf("mutate config: %v", err)
	}
	if err := os.WriteFile(envPath, []byte("A=2\n"), 0o600); err != nil {
		t.Fatalf("mutate env: %v", err)
	}

	if _, err := restoreUpdateBackup(snapshot, false); err != nil {
		t.Fatalf("restore backup: %v", err)
	}

	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfgData), "18790") {
		t.Fatalf("expected config restored, got %s", string(cfgData))
	}
	envData, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	if strings.TrimSpace(string(envData)) != "A=1" {
		t.Fatalf("expected env restored, got %s", string(envData))
	}
}

func TestDetectConfigDrift(t *testing.T) {
	before := map[string]any{
		"gateway": map[string]any{
			"port": float64(18790),
			"host": "127.0.0.1",
		},
	}
	after := map[string]any{
		"gateway": map[string]any{
			"port": float64(19990),
		},
		"group": map[string]any{
			"enabled": true,
		},
	}
	drift := detectConfigDrift(before, after)
	if len(drift) < 3 {
		t.Fatalf("expected at least 3 drift entries, got %v", drift)
	}
}

func TestLifecycleUpdateApplyAndRollbackWithFailureInjection(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origKHome := os.Getenv("KAFCLAW_HOME")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKHome)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("KAFCLAW_HOME", tmpDir)

	cfgPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatalf("mkdir cfg dir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"gateway":{"host":"127.0.0.1","port":18790,"dashboardPort":18791}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".kafclaw", "timeline.db"), []byte("db"), 0o600); err != nil {
		t.Fatalf("write timeline: %v", err)
	}

	origRunBinary := runBinaryUpdateFn
	origRunSource := runSourceUpdateFn
	origDoctor := runDoctorReportFn
	origSecurity := runSecurityCheckFn
	defer func() {
		runBinaryUpdateFn = origRunBinary
		runSourceUpdateFn = origRunSource
		runDoctorReportFn = origDoctor
		runSecurityCheckFn = origSecurity
	}()
	runSourceUpdateFn = func(_ string) error { return nil }
	runDoctorReportFn = func(_ cliconfig.DoctorOptions) (cliconfig.DoctorReport, error) {
		return cliconfig.DoctorReport{Checks: []cliconfig.DoctorCheck{{Name: "config_load", Status: cliconfig.DoctorPass}}}, nil
	}
	runSecurityCheckFn = func() (cliconfig.SecurityReport, error) {
		return cliconfig.SecurityReport{Checks: []cliconfig.SecurityCheck{{Name: "security", Status: cliconfig.SecurityPass}}}, nil
	}

	backupRoot := filepath.Join(tmpDir, "backups")
	if _, err := runRootCommand(t, "update", "backup", "--backup-dir", backupRoot); err != nil {
		t.Fatalf("create update backup command failed: %v", err)
	}
	knownBackup, err := findLatestBackup(backupRoot)
	if err != nil {
		t.Fatalf("expected backup snapshot before failure injection: %v", err)
	}

	updateLatest = true
	updateVersion = ""
	updateSource = false
	updateSkipBackup = false
	updateBackupDir = backupRoot
	updateDryRun = false
	updateAllowDowngrade = false
	updateRepoPath = ""
	runBinaryUpdateFn = func(_ bool, _ string) error {
		return errors.New("injected update failure")
	}

	if _, err := runRootCommand(t, "update", "apply", "--latest", "--backup-dir", backupRoot); err == nil {
		t.Fatal("expected update apply to fail with injected updater failure")
	}

	if _, err := os.Stat(filepath.Join(knownBackup, "manifest.json")); err != nil {
		t.Fatalf("missing backup manifest: %v", err)
	}

	if err := os.WriteFile(cfgPath, []byte(`{"gateway":{"host":"0.0.0.0","port":19990,"dashboardPort":18791}}`), 0o600); err != nil {
		t.Fatalf("mutate config: %v", err)
	}

	if _, err := runRootCommand(t, "update", "rollback", "--backup-path", knownBackup); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(cfgData), "19990") {
		t.Fatalf("expected rollback to restore pre-update config, got %s", string(cfgData))
	}
	if !strings.Contains(string(cfgData), "18790") {
		t.Fatalf("expected original config port after rollback, got %s", string(cfgData))
	}
}

func TestUpdatePlanJSONOutput(t *testing.T) {
	out, err := runRootCommand(t, "update", "plan", "--json")
	if err != nil {
		t.Fatalf("update plan --json failed: %v", err)
	}
	if !strings.Contains(out, `"command": "update"`) || !strings.Contains(out, `"action": "plan"`) {
		t.Fatalf("expected update plan json output, got %q", out)
	}
}

func TestUpdatePlanTextOutput(t *testing.T) {
	origJSON := updateJSON
	defer func() { updateJSON = origJSON }()
	updateJSON = false

	out, err := runRootCommand(t, "update", "plan")
	if err != nil {
		t.Fatalf("update plan failed: %v", err)
	}
	if !strings.Contains(out, "Canonical update flow:") || !strings.Contains(out, "kafclaw update apply --latest") {
		t.Fatalf("expected update plan text output, got %q", out)
	}
}

func TestValidateUpdateTarget(t *testing.T) {
	if err := validateUpdateTarget(true, "v2.6.3", false); err == nil {
		t.Fatal("expected conflict when --latest and --version are both set")
	}
	if err := validateUpdateTarget(false, "", false); err == nil {
		t.Fatal("expected binary target validation error")
	}
	if err := validateUpdateTarget(false, "", true); err != nil {
		t.Fatalf("expected source mode to allow no explicit version, got %v", err)
	}
	if err := validateUpdateTarget(true, "", false); err != nil {
		t.Fatalf("expected latest mode to validate, got %v", err)
	}
}

func TestPreflightUpdateRuntimeConfigError(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origKHome := os.Getenv("KAFCLAW_HOME")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKHome)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("KAFCLAW_HOME", tmpDir)

	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir cfg dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	if err := preflightUpdateRuntime(); err == nil {
		t.Fatal("expected preflight runtime to fail on invalid config")
	}
}

func TestPreflightUpdateRuntimeTimelineError(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origKHome := os.Getenv("KAFCLAW_HOME")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKHome)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("KAFCLAW_HOME", tmpDir)

	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	timelinePath := filepath.Join(tmpDir, ".kafclaw", "timeline.db")
	if err := os.MkdirAll(timelinePath, 0o700); err != nil {
		t.Fatalf("create invalid timeline path: %v", err)
	}

	if err := preflightUpdateRuntime(); err == nil {
		t.Fatal("expected timeline migration preflight error")
	}
}

func TestPreflightUpdateRuntimeWithoutTimelineDB(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origKHome := os.Getenv("KAFCLAW_HOME")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKHome)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("KAFCLAW_HOME", tmpDir)

	if err := config.Save(config.DefaultConfig()); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := preflightUpdateRuntime(); err != nil {
		t.Fatalf("expected preflight to pass without timeline db, got %v", err)
	}
}

func TestUpdateRollbackUsesLatestSnapshotJSON(t *testing.T) {
	origJSON := updateJSON
	origRollbackPath := updateRollbackPath
	origBackupDir := updateBackupDir
	defer func() {
		updateJSON = origJSON
		updateRollbackPath = origRollbackPath
		updateBackupDir = origBackupDir
	}()
	updateJSON = false
	updateRollbackPath = ""
	updateBackupDir = ""

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origKHome := os.Getenv("KAFCLAW_HOME")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKHome)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("KAFCLAW_HOME", tmpDir)

	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	cfgPath, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"gateway":{"port":18790}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	backupRoot := filepath.Join(tmpDir, "backups")
	if _, _, err := createUpdateBackup(backupRoot); err != nil {
		t.Fatalf("create backup: %v", err)
	}

	out, err := runRootCommand(t, "update", "rollback", "--backup-dir", backupRoot, "--json")
	if err != nil {
		t.Fatalf("rollback --json failed: %v", err)
	}
	if !strings.Contains(out, `"action": "rollback"`) || !strings.Contains(out, `"restoredVersion"`) {
		t.Fatalf("expected rollback json output, got %q", out)
	}
}

func TestUpdateRollbackErrors(t *testing.T) {
	origRollbackPath := updateRollbackPath
	origBackupDir := updateBackupDir
	defer func() {
		updateRollbackPath = origRollbackPath
		updateBackupDir = origBackupDir
	}()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origKHome := os.Getenv("KAFCLAW_HOME")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKHome)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("KAFCLAW_HOME", tmpDir)

	backupRoot := filepath.Join(tmpDir, "backups")
	if _, err := runRootCommand(t, "update", "rollback", "--backup-dir", backupRoot); err == nil {
		t.Fatal("expected rollback to fail when no snapshots exist")
	}
	if _, err := runRootCommand(t, "update", "rollback", "--backup-path", filepath.Join(tmpDir, "missing-snapshot")); err == nil {
		t.Fatal("expected rollback to fail for missing snapshot path")
	}
}

func TestRunBinaryAndSourceUpdateErrors(t *testing.T) {
	if err := runSourceUpdate(""); err == nil {
		t.Fatal("expected source update to fail when repo path is empty")
	}

	tmpDir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := runBinaryUpdate(true, ""); err == nil {
		t.Fatal("expected binary update to fail when installer script is missing from cwd")
	}
}

func TestRunSourceUpdateCommandFailure(t *testing.T) {
	tmpDir := t.TempDir()
	if err := runSourceUpdate(tmpDir); err == nil {
		t.Fatal("expected source update command failure in non-git directory")
	}
}

func TestRunBinaryUpdateScriptFailure(t *testing.T) {
	tmpDir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := os.MkdirAll(filepath.Join("scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join("scripts", "install.sh"), []byte("#!/usr/bin/env sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write install script: %v", err)
	}

	if err := runBinaryUpdate(true, ""); err == nil {
		t.Fatal("expected binary update command failure")
	}
}
