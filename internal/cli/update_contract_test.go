package cli

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/cliconfig"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/spf13/cobra"
)

func TestUpdateApplyHealthContracts(t *testing.T) {
	root := commandHome(t)
	oldBinary, oldSource, oldDoctor, oldSecurity := runBinaryUpdateFn, runSourceUpdateFn, runDoctorReportFn, runSecurityCheckFn
	oldLatest, oldVersion, oldIsSource, oldSkip, oldBackup, oldDry, oldJSON, oldRepo := updateLatest, updateVersion, updateSource, updateSkipBackup, updateBackupDir, updateDryRun, updateJSON, updateRepoPath
	t.Cleanup(func() {
		runBinaryUpdateFn, runSourceUpdateFn, runDoctorReportFn, runSecurityCheckFn = oldBinary, oldSource, oldDoctor, oldSecurity
		updateLatest, updateVersion, updateSource, updateSkipBackup, updateBackupDir, updateDryRun, updateJSON, updateRepoPath = oldLatest, oldVersion, oldIsSource, oldSkip, oldBackup, oldDry, oldJSON, oldRepo
	})
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	updateLatest = true
	updateVersion = ""
	updateSource = false
	updateSkipBackup = true
	updateBackupDir = filepath.Join(root, "backups")
	updateDryRun = false
	updateRepoPath = ""
	for _, mode := range []string{"ok", "binary-failure", "source-failure", "doctor-error", "doctor-fail", "security-error", "security-fail", "drift"} {
		for _, asJSON := range []bool{false, true} {
			updateJSON = asJSON
			out.Reset()
			updateSource = mode == "source-failure"
			updateLatest = !updateSource
			runBinaryUpdateFn = func(bool, string) error {
				if mode == "binary-failure" {
					return errors.New("fixture binary failure")
				}
				if mode == "drift" {
					cfg, err := config.Load()
					if err != nil {
						return err
					}
					cfg.Gateway.Port++
					return config.Save(cfg)
				}
				return nil
			}
			runSourceUpdateFn = func(string) error { return errors.New("fixture source failure") }
			runDoctorReportFn = func(cliconfig.DoctorOptions) (cliconfig.DoctorReport, error) {
				if mode == "doctor-error" {
					return cliconfig.DoctorReport{}, errors.New("fixture doctor failure")
				}
				report := cliconfig.DoctorReport{}
				if mode == "doctor-fail" {
					report.Checks = []cliconfig.DoctorCheck{{Status: cliconfig.DoctorFail}}
				}
				return report, nil
			}
			runSecurityCheckFn = func() (cliconfig.SecurityReport, error) {
				if mode == "security-error" {
					return cliconfig.SecurityReport{}, errors.New("fixture security failure")
				}
				report := cliconfig.SecurityReport{}
				if mode == "security-fail" {
					report.Checks = []cliconfig.SecurityCheck{{Status: cliconfig.SecurityFail}}
				}
				return report, nil
			}
			err := runUpdateApply(cmd, nil)
			if mode == "ok" || mode == "drift" {
				if err != nil {
					t.Fatal(err)
				}
				if !asJSON && !strings.Contains(out.String(), "health gates passed") {
					t.Fatal(out.String())
				}
			} else if err == nil {
				t.Fatalf("%s was not propagated", mode)
			}
		}
	}
	updateJSON = false
	updateSkipBackup = false
	updateDryRun = true
	updateSource = false
	updateLatest = true
	if err := runUpdateApply(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if err := runUpdateBackup(cmd, nil); err != nil {
		t.Fatal(err)
	}
}
