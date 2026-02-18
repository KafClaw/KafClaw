package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckPrerequisiteGoogleCLI(t *testing.T) {
	res, err := CheckPrerequisite("google-cli")
	if err != nil {
		t.Fatalf("check prerequisite failed: %v", err)
	}
	if res.Name != "google-cli" {
		t.Fatalf("unexpected prerequisite name: %s", res.Name)
	}
}

func TestInstallPrerequisiteDryRun(t *testing.T) {
	res, err := InstallPrerequisite("google-cli", true)
	if err != nil {
		t.Fatalf("dry-run install prerequisite failed: %v", err)
	}
	if res.Name != "google-cli" {
		t.Fatalf("unexpected prerequisite name: %s", res.Name)
	}
	if len(res.Messages) == 0 && !res.Installed {
		t.Fatal("expected dry-run output messages when not installed")
	}
}

func TestRunCommandAndRunCommandWithStdin(t *testing.T) {
	if err := runCommand(2*time.Second, "sh", "-c", "exit 0"); err != nil {
		t.Fatalf("runCommand expected success: %v", err)
	}
	if err := runCommandWithStdin(2*time.Second, []byte("hello"), "sh", "-c", "cat >/dev/null"); err != nil {
		t.Fatalf("runCommandWithStdin expected success: %v", err)
	}
}

func TestFormatCommands(t *testing.T) {
	out := formatCommands([][]string{{"a", "b"}, {"c"}})
	if len(out) != 2 || out[0] != "a b" || out[1] != "c" {
		t.Fatalf("unexpected formatCommands output: %#v", out)
	}
}

func TestInstallGoogleCLIDryRunDarwinPath(t *testing.T) {
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bin, "brew"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write brew: %v", err)
	}
	_ = os.Setenv("PATH", bin+string(os.PathListSeparator)+origPath)
	res, err := installGoogleCLIDarwinBrew(true)
	if err != nil {
		t.Fatalf("dry run should succeed: %v", err)
	}
	if len(res.Messages) == 0 || !strings.Contains(res.Messages[0], "brew update") {
		t.Fatalf("unexpected dry-run messages: %#v", res.Messages)
	}
}
