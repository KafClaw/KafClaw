package tools

import (
	"strings"
	"testing"
	"time"
)

func TestGuardCommandStrictAllowListBranches(t *testing.T) {
	tool := NewExecTool(5*time.Second, false, "", nil)
	tool.StrictAllowList = true

	if err := tool.guardCommand("echo hello", ""); err != nil {
		t.Fatalf("expected allow-listed command accepted, got %v", err)
	}
	if err := tool.guardCommand("python -c 'print(1)'", ""); err == nil {
		t.Fatal("expected non allow-listed command to be blocked")
	}
}

func TestGuardCommandDenyAndTraversalBranches(t *testing.T) {
	workspace := t.TempDir()
	repo := t.TempDir()
	tool := NewExecTool(5*time.Second, true, workspace, func() string { return repo })
	tool.StrictAllowList = false

	if err := tool.guardCommand("rm -rf /", workspace); err == nil {
		t.Fatal("expected deny-pattern command blocked")
	}
	if err := tool.guardCommand("Rm -rf /", workspace); err == nil {
		t.Fatal("expected mixed-case deny-pattern command blocked")
	}
	if err := tool.guardCommand("0rm -rf /", workspace); err == nil {
		t.Fatal("expected prefixed destructive rm command blocked")
	}
	if err := tool.guardCommand("cat ../../../etc/passwd", workspace); err == nil {
		t.Fatal("expected traversal command blocked")
	}
	if err := tool.guardCommand("echo ok", t.TempDir()); err == nil {
		t.Fatal("expected outside working dir blocked")
	}
	if err := tool.guardCommand("echo ok", workspace); err != nil {
		t.Fatalf("expected workspace working dir allowed, got %v", err)
	}
	if err := tool.guardCommand("echo ok", repo); err != nil {
		t.Fatalf("expected repo working dir allowed, got %v", err)
	}
}

func TestGuardCommandNonAbsoluteWorkingDirHandling(t *testing.T) {
	tool := NewExecTool(5*time.Second, true, t.TempDir(), nil)
	tool.StrictAllowList = false
	err := tool.guardCommand("echo ok", ".")
	if err != nil && !strings.Contains(err.Error(), blockedAttackMessage) {
		t.Fatalf("unexpected error for relative working dir: %v", err)
	}
}
