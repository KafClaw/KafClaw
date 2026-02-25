package tools

import (
	"strings"
	"testing"
	"time"
)

func FuzzGuardCommand_NoPanicAndTraversalBlocked(f *testing.F) {
	f.Add("echo hello", "")
	f.Add("cat ../../../etc/passwd", "")
	f.Add("rm -rf /", "")
	f.Add("git status", ".")

	f.Fuzz(func(t *testing.T, command, workingDir string) {
		tool := NewExecTool(50*time.Millisecond, true, t.TempDir(), nil)
		tool.StrictAllowList = false

		err := tool.guardCommand(command, workingDir)

		lc := strings.ToLower(command)
		if strings.Contains(command, "../") || strings.Contains(command, `..\`) {
			if err == nil {
				t.Fatalf("expected traversal-like command to be blocked: %q", command)
			}
		}
		if destructiveRMRootRegex.MatchString(lc) && err == nil {
			t.Fatalf("expected destructive rm pattern blocked: %q", command)
		}
	})
}

func FuzzGuardCommand_StrictAllowList(f *testing.F) {
	f.Add("git status")
	f.Add("echo hi")
	f.Add("python -c 'print(1)'")
	f.Add("")

	f.Fuzz(func(t *testing.T, command string) {
		tool := NewExecTool(50*time.Millisecond, false, "", nil)
		tool.StrictAllowList = true

		err := tool.guardCommand(command, "")
		allowed := false
		for _, re := range tool.allowRegexes {
			if re.MatchString(command) {
				allowed = true
				break
			}
		}
		if !allowed && err == nil {
			t.Fatalf("expected non-allow-listed command blocked: %q", command)
		}
	})
}
