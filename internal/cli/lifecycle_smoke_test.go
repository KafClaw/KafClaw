package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/cliconfig"
)

func TestLifecycleFullFlowSmoke(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origKHome := os.Getenv("KAFCLAW_HOME")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("KAFCLAW_HOME", origKHome)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("KAFCLAW_HOME", tmpDir)

	origRunBinary := runBinaryUpdateFn
	origRunDoctor := runDoctorReportFn
	origRunSecurity := runSecurityCheckFn
	defer func() {
		runBinaryUpdateFn = origRunBinary
		runDoctorReportFn = origRunDoctor
		runSecurityCheckFn = origRunSecurity
	}()
	runBinaryUpdateFn = func(_ bool, _ string) error { return nil }
	runDoctorReportFn = func(_ cliconfig.DoctorOptions) (cliconfig.DoctorReport, error) {
		return cliconfig.DoctorReport{
			Checks: []cliconfig.DoctorCheck{
				{Name: "config_load", Status: cliconfig.DoctorPass},
			},
		}, nil
	}
	runSecurityCheckFn = func() (cliconfig.SecurityReport, error) {
		return cliconfig.SecurityReport{
			Checks: []cliconfig.SecurityCheck{
				{Name: "security", Status: cliconfig.SecurityPass},
			},
		}, nil
	}

	if _, err := runRootCommand(t, "onboard", "--non-interactive", "--accept-risk", "--mode=local", "--llm=skip", "--skip-skills", "--gateway-port=19890"); err != nil {
		t.Fatalf("onboard failed: %v", err)
	}

	backupRoot := filepath.Join(tmpDir, "backups")
	if _, err := runRootCommand(t, "update", "apply", "--latest", "--backup-dir", backupRoot); err != nil {
		t.Fatalf("update apply failed: %v", err)
	}

	snapshot, err := findLatestBackup(backupRoot)
	if err != nil {
		t.Fatalf("find latest backup: %v", err)
	}

	if _, err := runRootCommand(t, "update", "rollback", "--backup-path", snapshot); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	eventsPath := filepath.Join(tmpDir, ".kafclaw", "lifecycle-events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read lifecycle events: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"command":"onboard"`) {
		t.Fatalf("expected onboard lifecycle events, got: %s", text)
	}
	if !strings.Contains(text, `"command":"update"`) {
		t.Fatalf("expected update lifecycle events, got: %s", text)
	}
}
