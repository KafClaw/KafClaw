package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestExecuteSkillCommand_AllowlistBlocks(t *testing.T) {
	cfg, skillName := setupRuntimeSkill(t, `{
  "version": "1",
  "execution": {
    "allowCommands": ["echo"]
  }
}`)
	_, err := ExecuteSkillCommand(cfg, skillName, []string{"ls"})
	if err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("expected allowlist block error, got: %v", err)
	}
}

func TestExecuteSkillCommand_NoNetworkDefaultBlocksCurl(t *testing.T) {
	cfg, skillName := setupRuntimeSkill(t, "")
	_, err := ExecuteSkillCommand(cfg, skillName, []string{"curl", "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "no-network") {
		t.Fatalf("expected no-network block, got: %v", err)
	}
}

func TestExecuteSkillCommand_ReadOnlyWorkspaceBlocksAbsolutePathArg(t *testing.T) {
	cfg, skillName := setupRuntimeSkill(t, `{"version":"1","execution":{"allowCommands":["echo"]}}`)
	target := filepath.Join(cfg.Paths.WorkRepoPath, "foo.txt")
	_, err := ExecuteSkillCommand(cfg, skillName, []string{"echo", target})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only workspace block, got: %v", err)
	}
}

func TestExecuteSkillCommand_OutputCapAndScratch(t *testing.T) {
	cfg, skillName := setupRuntimeSkill(t, `{
  "version": "1",
  "execution": {
    "allowCommands": ["printf"],
    "maxOutputBytes": 8
  }
}`)
	res, err := ExecuteSkillCommand(cfg, skillName, []string{"printf", "1234567890"})
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	if !res.OutputTruncated {
		t.Fatalf("expected output truncation")
	}
	if res.ScratchDir == "" {
		t.Fatalf("expected scratch dir")
	}
	if !strings.HasPrefix(res.ScratchDir, filepath.Join(filepath.Dir(mustConfigPath(t)), "skills", "tmp", skillName)+string(os.PathSeparator)) {
		t.Fatalf("unexpected scratch dir: %s", res.ScratchDir)
	}
}

func TestExecuteSkillCommand_TimeoutCap(t *testing.T) {
	cfg, skillName := setupRuntimeSkill(t, `{
  "version": "1",
  "execution": {
    "allowCommands": ["sleep"],
    "timeoutSeconds": 1
  }
}`)
	_, err := ExecuteSkillCommand(cfg, skillName, []string{"sleep", "2"})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestExecuteSkillCommand_BlocksShellInterpreter(t *testing.T) {
	cfg, skillName := setupRuntimeSkill(t, `{"version":"1","execution":{"allowCommands":["bash"]}}`)
	_, err := ExecuteSkillCommand(cfg, skillName, []string{"bash", "-lc", "echo hi"})
	if err == nil || !strings.Contains(err.Error(), "interpreter shells") {
		t.Fatalf("expected interpreter block, got: %v", err)
	}
}

func TestEnforceRuntimePolicyMatrix(t *testing.T) {
	workspace := t.TempDir()
	cases := []struct {
		name    string
		policy  runtimePolicy
		cmd     []string
		wantErr bool
	}{
		{name: "missing command vector", policy: runtimePolicy{}, cmd: []string{}, wantErr: true},
		{name: "invalid command", policy: runtimePolicy{}, cmd: []string{""}, wantErr: true},
		{name: "dot command invalid", policy: runtimePolicy{}, cmd: []string{"."}, wantErr: true},
		{name: "allowlist pass", policy: runtimePolicy{AllowCommands: []string{"echo"}}, cmd: []string{"echo", "ok"}, wantErr: false},
		{name: "allowlist block", policy: runtimePolicy{AllowCommands: []string{"echo"}}, cmd: []string{"ls"}, wantErr: true},
		{name: "denylist block", policy: runtimePolicy{DenyCommands: []string{"ls"}}, cmd: []string{"ls"}, wantErr: true},
		{name: "network block", policy: runtimePolicy{Network: false}, cmd: []string{"curl", "https://x"}, wantErr: true},
		{name: "network allow", policy: runtimePolicy{Network: true}, cmd: []string{"curl", "https://x"}, wantErr: false},
		{name: "workspace readonly block", policy: runtimePolicy{ReadOnlyWorkspace: true}, cmd: []string{"echo", filepath.Join(workspace, "a.txt")}, wantErr: true},
		{name: "workspace readonly pass outside", policy: runtimePolicy{ReadOnlyWorkspace: true}, cmd: []string{"echo", "/tmp/outside.txt"}, wantErr: false},
		{name: "workspace readonly ignores relative", policy: runtimePolicy{ReadOnlyWorkspace: true}, cmd: []string{"echo", "relative.txt"}, wantErr: false},
		{name: "workspace readonly disabled", policy: runtimePolicy{ReadOnlyWorkspace: false}, cmd: []string{"echo", filepath.Join(workspace, "a.txt")}, wantErr: false},
		{name: "workspace empty passthrough", policy: runtimePolicy{ReadOnlyWorkspace: true}, cmd: []string{"echo", filepath.Join(workspace, "a.txt")}, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := workspace
			if tc.name == "workspace empty passthrough" {
				ws = ""
			}
			err := enforceRuntimePolicy(tc.policy, ws, tc.cmd)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestStrictIsolationPreflightNoRuntime(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)
	_, err := StrictIsolationPreflight()
	if err == nil || !strings.Contains(err.Error(), "no container runtime") {
		t.Fatalf("expected strict preflight runtime error, got: %v", err)
	}
}

func setupRuntimeSkill(t *testing.T, policy string) (*config.Config, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("KAFCLAW_HOME", home)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(home, ".kafclaw", "config.json"))
	work := filepath.Join(home, "workspace")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Paths.WorkRepoPath = work
	cfg.Skills.Enabled = true
	cfg.Skills.RuntimeIsolation = "host"
	skillName := "runtime-test-skill"
	cfg.Skills.Entries = map[string]config.SkillEntryConfig{
		skillName: {Enabled: true},
	}
	dirs, err := EnsureStateDirs()
	if err != nil {
		t.Fatalf("ensure state dirs: %v", err)
	}
	root := filepath.Join(dirs.Installed, skillName)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("mkdir skill root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: runtime-test-skill\ndescription: rt\n---\n"), 0o600); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if strings.TrimSpace(policy) != "" {
		if err := os.WriteFile(filepath.Join(root, "SKILL-POLICY.json"), []byte(policy), 0o600); err != nil {
			t.Fatalf("write policy: %v", err)
		}
	}
	return cfg, skillName
}

func mustConfigPath(t *testing.T) string {
	t.Helper()
	p, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	return p
}
