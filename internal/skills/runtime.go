package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
)

const (
	defaultExecTimeout    = 30 * time.Second
	defaultMaxOutputBytes = 64 * 1024
)

var defaultNetworkDeniedCommands = []string{
	"curl", "wget", "nc", "ncat", "netcat", "ssh", "scp", "sftp", "ftp", "telnet", "dig", "nslookup", "ping", "traceroute",
	"git", "node", "npm", "npx", "pnpm", "yarn", "python", "python3", "pip", "pip3", "go", "cargo",
}

var blockedInterpreterCommands = []string{
	"sh", "bash", "zsh", "fish", "dash", "ksh", "csh", "tcsh", "cmd", "powershell", "pwsh",
}

// SkillExecResult contains runtime execution details.
type SkillExecResult struct {
	SkillName       string        `json:"skillName"`
	Command         []string      `json:"command"`
	ScratchDir      string        `json:"scratchDir"`
	Duration        time.Duration `json:"duration"`
	ExitCode        int           `json:"exitCode"`
	Stdout          string        `json:"stdout,omitempty"`
	Stderr          string        `json:"stderr,omitempty"`
	OutputTruncated bool          `json:"outputTruncated"`
}

type runtimePolicy struct {
	AllowCommands     []string
	DenyCommands      []string
	Network           bool
	ReadOnlyWorkspace bool
	Timeout           time.Duration
	MaxOutputBytes    int
}

// ExecuteSkillCommand runs one command in a skill sandbox with policy checks.
func ExecuteSkillCommand(cfg *config.Config, skillName string, command []string) (*SkillExecResult, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if len(command) == 0 {
		return nil, fmt.Errorf("command is required")
	}
	skillName = sanitizeSkillName(skillName)
	if skillName == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	if !EffectiveSkillEnabled(cfg, skillName) {
		return nil, fmt.Errorf("skill %q is not enabled", skillName)
	}

	dirs, err := EnsureStateDirs()
	if err != nil {
		return nil, err
	}
	skillRoot := filepath.Join(dirs.Installed, skillName)
	if st, err := os.Stat(skillRoot); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("skill %q is not installed", skillName)
	}

	policy, err := loadRuntimePolicy(skillRoot)
	if err != nil {
		return nil, err
	}
	if err := enforceRuntimePolicy(policy, cfg.Paths.WorkRepoPath, command); err != nil {
		return nil, err
	}

	scratch, err := prepareSkillScratch(dirs.TmpDir, skillName)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), policy.Timeout)
	defer cancel()
	stdoutBuf := newLimitedBuffer(policy.MaxOutputBytes)
	stderrBuf := newLimitedBuffer(policy.MaxOutputBytes)
	err = runSkillCommandWithIsolation(ctx, cfg, skillRoot, scratch, command, stdoutBuf, stderrBuf)
	dur := time.Since(start)
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
		if ctx.Err() == context.DeadlineExceeded {
			return &SkillExecResult{
				SkillName:       skillName,
				Command:         append([]string{}, command...),
				ScratchDir:      scratch,
				Duration:        dur,
				ExitCode:        exitCode,
				Stdout:          stdoutBuf.String(),
				Stderr:          stderrBuf.String(),
				OutputTruncated: stdoutBuf.Truncated() || stderrBuf.Truncated(),
			}, fmt.Errorf("skill command timed out after %s", policy.Timeout)
		}
	}

	return &SkillExecResult{
		SkillName:       skillName,
		Command:         append([]string{}, command...),
		ScratchDir:      scratch,
		Duration:        dur,
		ExitCode:        exitCode,
		Stdout:          stdoutBuf.String(),
		Stderr:          stderrBuf.String(),
		OutputTruncated: stdoutBuf.Truncated() || stderrBuf.Truncated(),
	}, err
}

func runSkillCommandWithIsolation(ctx context.Context, cfg *config.Config, skillRoot, scratch string, command []string, stdout io.Writer, stderr io.Writer) error {
	mode := "auto"
	if cfg != nil {
		mode = strings.ToLower(strings.TrimSpace(cfg.Skills.RuntimeIsolation))
		if mode == "" {
			mode = "auto"
		}
	}
	switch mode {
	case "host":
		return runHostCommand(ctx, scratch, command, stdout, stderr)
	case "strict":
		runtimeBin, err := StrictIsolationPreflight()
		if err != nil {
			return err
		}
		return runContainerIsolatedCommand(ctx, runtimeBin, skillRoot, scratch, command, stdout, stderr)
	case "auto":
		if runtimeBin, ok := detectContainerRuntime(); ok {
			if err := runContainerIsolatedCommand(ctx, runtimeBin, skillRoot, scratch, command, stdout, stderr); err == nil {
				return nil
			}
		}
		return runHostCommand(ctx, scratch, command, stdout, stderr)
	default:
		return runHostCommand(ctx, scratch, command, stdout, stderr)
	}
}

func runHostCommand(ctx context.Context, scratch string, command []string, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = scratch
	cmd.Env = minimalRuntimeEnv(scratch)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func detectContainerRuntime() (string, bool) {
	for _, name := range []string{"docker", "podman"} {
		if HasBinary(name) {
			return name, true
		}
	}
	return "", false
}

// StrictIsolationPreflight validates strict runtime isolation prerequisites.
func StrictIsolationPreflight() (string, error) {
	runtimeBin, ok := detectContainerRuntime()
	if !ok {
		return "", fmt.Errorf("strict runtime isolation enabled but no container runtime (docker/podman) found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, runtimeBin, "version")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("strict runtime isolation requires %s to be usable by current user: %w", runtimeBin, err)
	}
	return runtimeBin, nil
}

func runContainerIsolatedCommand(ctx context.Context, runtimeBin, skillRoot, scratch string, command []string, stdout io.Writer, stderr io.Writer) error {
	if len(command) == 0 {
		return fmt.Errorf("invalid command")
	}
	image := strings.TrimSpace(os.Getenv("KAFCLAW_SKILL_SANDBOX_IMAGE"))
	if image == "" {
		image = "alpine:3.20"
	}
	args := []string{
		"run", "--rm",
		"--network", "none",
		"--read-only",
		"--pids-limit", "64",
		"--memory", "256m",
		"--cpus", "1.0",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--user", "65534:65534",
		"-v", skillRoot + ":/skill:ro",
		"-v", scratch + ":/work:rw",
		"-w", "/work",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"-e", "HOME=/work",
		"-e", "TMPDIR=/tmp",
		"-e", "KAFCLAW_SKILL_NETWORK=disabled",
		image,
	}
	args = append(args, command...)
	cmd := exec.CommandContext(ctx, runtimeBin, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func loadRuntimePolicy(skillRoot string) (runtimePolicy, error) {
	p := runtimePolicy{
		Network:           false,
		ReadOnlyWorkspace: true,
		Timeout:           defaultExecTimeout,
		MaxOutputBytes:    defaultMaxOutputBytes,
	}
	data, err := os.ReadFile(filepath.Join(skillRoot, "SKILL-POLICY.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return p, err
	}
	var manifest skillPolicyManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return p, fmt.Errorf("invalid SKILL-POLICY.json: %w", err)
	}
	if manifest.Execution.Network != nil {
		p.Network = *manifest.Execution.Network
	}
	if manifest.Execution.ReadOnlyWorkspace != nil {
		p.ReadOnlyWorkspace = *manifest.Execution.ReadOnlyWorkspace
	}
	if manifest.Execution.TimeoutSeconds > 0 {
		p.Timeout = time.Duration(manifest.Execution.TimeoutSeconds) * time.Second
	}
	if manifest.Execution.MaxOutputBytes > 0 {
		p.MaxOutputBytes = manifest.Execution.MaxOutputBytes
	}
	p.AllowCommands = normalizeCommandList(manifest.Execution.AllowCommands)
	p.DenyCommands = normalizeCommandList(manifest.Execution.DenyCommands)
	return p, nil
}

func normalizeCommandList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		v = filepath.Base(v)
		if v != "" {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func enforceRuntimePolicy(p runtimePolicy, workspace string, command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("invalid command")
	}
	baseCmd := strings.ToLower(filepath.Base(strings.TrimSpace(command[0])))
	if baseCmd == "" || baseCmd == "." {
		return fmt.Errorf("invalid command")
	}
	if len(p.AllowCommands) > 0 && !slices.Contains(p.AllowCommands, baseCmd) {
		return fmt.Errorf("command %q blocked by allowlist policy", baseCmd)
	}
	if len(p.DenyCommands) > 0 && slices.Contains(p.DenyCommands, baseCmd) {
		return fmt.Errorf("command %q blocked by denylist policy", baseCmd)
	}
	if slices.Contains(blockedInterpreterCommands, baseCmd) {
		return fmt.Errorf("command %q blocked: interpreter shells are not allowed in skills runtime", baseCmd)
	}
	if !p.Network && slices.Contains(defaultNetworkDeniedCommands, baseCmd) {
		return fmt.Errorf("command %q blocked by no-network policy", baseCmd)
	}
	if p.ReadOnlyWorkspace {
		ws := strings.TrimSpace(workspace)
		if ws != "" {
			if abs, err := filepath.Abs(ws); err == nil {
				ws = abs
			}
			for _, arg := range command[1:] {
				arg = strings.TrimSpace(arg)
				if arg == "" || !filepath.IsAbs(arg) {
					continue
				}
				clean := filepath.Clean(arg)
				if clean == ws || strings.HasPrefix(clean, ws+string(os.PathSeparator)) {
					return fmt.Errorf("workspace path argument blocked by read-only policy: %s", arg)
				}
			}
		}
	}
	return nil
}

func prepareSkillScratch(tmpRoot, skill string) (string, error) {
	dir := filepath.Join(tmpRoot, skill, fmt.Sprintf("run-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func minimalRuntimeEnv(scratch string) []string {
	path := os.Getenv("PATH")
	if strings.TrimSpace(path) == "" {
		path = "/usr/bin:/bin"
	}
	return []string{
		"PATH=" + path,
		"HOME=" + scratch,
		"TMPDIR=" + scratch,
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"HTTP_PROXY=",
		"HTTPS_PROXY=",
		"ALL_PROXY=",
		"NO_PROXY=*",
		"KAFCLAW_SKILL_NETWORK=disabled",
	}
}

type limitedBuffer struct {
	buf       bytes.Buffer
	maxBytes  int
	truncated bool
}

func newLimitedBuffer(max int) *limitedBuffer {
	return &limitedBuffer{maxBytes: max}
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.maxBytes <= 0 {
		l.truncated = true
		return len(p), nil
	}
	remaining := l.maxBytes - l.buf.Len()
	if remaining <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		l.truncated = true
		_, _ = l.buf.Write(p[:remaining])
		return len(p), nil
	}
	_, err := l.buf.Write(p)
	return len(p), err
}

func (l *limitedBuffer) String() string {
	return l.buf.String()
}

func (l *limitedBuffer) Truncated() bool {
	return l.truncated
}

var _ io.Writer = (*limitedBuffer)(nil)
