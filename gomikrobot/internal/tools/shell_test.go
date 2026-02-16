package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecTool_Basic(t *testing.T) {
	tool := NewExecTool(5*time.Second, false, "", nil)
	tool.StrictAllowList = false

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("expected 'hello' in output, got '%s'", result)
	}
}

func TestExecTool_Timeout(t *testing.T) {
	tool := NewExecTool(100*time.Millisecond, false, "", nil)
	tool.StrictAllowList = false

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "sleep 10",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "timed out") {
		t.Errorf("expected timeout message, got '%s'", result)
	}
}

func TestExecTool_DenyPatterns(t *testing.T) {
	tool := NewExecTool(5*time.Second, false, "", nil)
	tool.StrictAllowList = false

	dangerousCommands := []string{
		"rm -rf /",
		"rm -rf ~",
		"dd if=/dev/zero of=/dev/sda",
		"chmod -R 777 /",
		"shutdown -h now",
	}

	for _, cmd := range dangerousCommands {
		result, _ := tool.Execute(context.Background(), map[string]any{
			"command": cmd,
		})
		if !strings.Contains(result, "Error") && !strings.Contains(result, "blocked") && !strings.Contains(result, blockedAttackMessage) {
			t.Errorf("expected '%s' to be blocked, got '%s'", cmd, result)
		}
	}
}

func TestExecTool_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	tool := NewExecTool(5*time.Second, true, tmpDir, nil)

	// Path traversal in command should be blocked
	result, _ := tool.Execute(context.Background(), map[string]any{
		"command": "cat ../../../etc/passwd",
	})
	if !strings.Contains(result, "Error") {
		t.Errorf("expected path traversal to be blocked, got '%s'", result)
	}
}

func TestExecTool_WorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	tool := NewExecTool(5*time.Second, false, tmpDir, nil)

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "pwd",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, tmpDir) {
		t.Errorf("expected working dir '%s' in output, got '%s'", tmpDir, result)
	}
}

func TestExecTool_StderrCapture(t *testing.T) {
	tool := NewExecTool(5*time.Second, false, "", nil)
	tool.StrictAllowList = false

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo error >&2",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "STDERR") {
		t.Errorf("expected STDERR in output, got '%s'", result)
	}
}

func TestExecTool_ExitCode(t *testing.T) {
	tool := NewExecTool(5*time.Second, false, "", nil)
	tool.StrictAllowList = false

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "exit 42",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "Exit code: 42") {
		t.Errorf("expected 'Exit code: 42' in output, got '%s'", result)
	}
}
