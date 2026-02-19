package cli

import (
	"os"
	"strings"
	"testing"
)

func TestInstallJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	out, err := runRootCommand(t, "install", "--json")
	if err != nil {
		t.Fatalf("install --json failed: %v", err)
	}
	if !strings.Contains(out, `"command": "install"`) || !strings.Contains(out, `"status": "ok"`) {
		t.Fatalf("expected install json output, got %q", out)
	}
}
