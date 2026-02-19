package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runInstallScript(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	scriptPath := filepath.Join("..", "..", "scripts", "install.sh")
	cmd := exec.Command("bash", append([]string{scriptPath}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallScriptRejectsConflictingReleaseSelectors(t *testing.T) {
	out, err := runInstallScript(t, nil, "--version", "v2.6.3", "--latest")
	if err == nil {
		t.Fatal("expected install script to fail")
	}
	if !strings.Contains(out, "INSTALL_ARGS_INVALID") {
		t.Fatalf("expected INSTALL_ARGS_INVALID error code, got:\n%s", out)
	}
}

func TestInstallScriptRejectsUnattendedWithoutVersionSelector(t *testing.T) {
	out, err := runInstallScript(t, nil, "--unattended")
	if err == nil {
		t.Fatal("expected install script to fail")
	}
	if !strings.Contains(out, "INSTALL_ARGS_INVALID") {
		t.Fatalf("expected INSTALL_ARGS_INVALID error code, got:\n%s", out)
	}
}

func TestInstallScriptReportsMissingPrerequisiteWithCode(t *testing.T) {
	fakeBin := t.TempDir()

	// Keep OS detection working with controlled PATH while omitting curl.
	realUname, err := exec.LookPath("uname")
	if err != nil {
		t.Fatalf("look path uname: %v", err)
	}
	realTr, err := exec.LookPath("tr")
	if err != nil {
		t.Fatalf("look path tr: %v", err)
	}

	writeForwarder := func(name, target string) {
		t.Helper()
		content := "#!/bin/sh\nexec " + target + " \"$@\"\n"
		p := filepath.Join(fakeBin, name)
		if wErr := os.WriteFile(p, []byte(content), 0o755); wErr != nil {
			t.Fatalf("write forwarder %s: %v", name, wErr)
		}
	}

	writeForwarder("uname", realUname)
	writeForwarder("tr", realTr)

	env := []string{
		"PATH=" + fakeBin,
	}
	out, runErr := runInstallScript(t, env, "--latest")
	if runErr == nil {
		t.Fatal("expected install script to fail")
	}
	if !strings.Contains(out, "INSTALL_PREREQ_MISSING") {
		t.Fatalf("expected INSTALL_PREREQ_MISSING error code, got:\n%s", out)
	}
	if !strings.Contains(out, "missing required command: curl") {
		t.Fatalf("expected missing curl detail, got:\n%s", out)
	}
}
