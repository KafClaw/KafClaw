package cli

import (
	"strings"
	"testing"
)

func TestCompletionCommandForSupportedShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			out, err := runRootCommand(t, "completion", shell)
			if err != nil {
				t.Fatalf("completion %s failed: %v", shell, err)
			}
			if !strings.Contains(out, "kafclaw") {
				t.Fatalf("expected completion output to mention kafclaw, got %q", out)
			}
		})
	}
}

func TestCompletionCommandRejectsUnknownShell(t *testing.T) {
	if _, err := runRootCommand(t, "completion", "tcsh"); err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}
