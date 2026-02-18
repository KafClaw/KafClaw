package skills

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
)

// StateDirs contains the private filesystem locations used by the skills runtime.
type StateDirs struct {
	Root       string
	TmpDir     string
	ToolsDir   string
	Quarantine string
	Installed  string
	Snapshots  string
	AuditDir   string
}

func resolveStateRoot() (string, error) {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(cfgPath), "skills"), nil
}

// ResolveStateDirs computes the private skill runtime directories.
func ResolveStateDirs() (StateDirs, error) {
	root, err := resolveStateRoot()
	if err != nil {
		return StateDirs{}, err
	}
	return StateDirs{
		Root:       root,
		TmpDir:     filepath.Join(root, "tmp"),
		ToolsDir:   filepath.Join(root, "tools"),
		Quarantine: filepath.Join(root, "quarantine"),
		Installed:  filepath.Join(root, "installed"),
		Snapshots:  filepath.Join(root, "snapshots"),
		AuditDir:   filepath.Join(root, "audit"),
	}, nil
}

// EnsureStateDirs creates required private skill directories with 0700 permissions.
func EnsureStateDirs() (StateDirs, error) {
	dirs, err := ResolveStateDirs()
	if err != nil {
		return StateDirs{}, err
	}
	for _, dir := range []string{dirs.Root, dirs.TmpDir, dirs.ToolsDir, dirs.Quarantine, dirs.Installed, dirs.Snapshots, dirs.AuditDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return StateDirs{}, err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return StateDirs{}, err
		}
	}
	return dirs, nil
}

// EnsureNVMRC writes a pinned Node version to .nvmrc when missing.
func EnsureNVMRC(workRepo string, nodeMajor string) (string, error) {
	repo := strings.TrimSpace(workRepo)
	if repo == "" {
		return "", nil
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return "", err
	}
	nvmrc := filepath.Join(repo, ".nvmrc")
	if _, err := os.Stat(nvmrc); err == nil {
		return nvmrc, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	val := strings.TrimSpace(nodeMajor)
	if val == "" {
		val = "20"
	}
	if err := os.WriteFile(nvmrc, []byte(val+"\n"), 0o644); err != nil {
		return "", err
	}
	return nvmrc, nil
}

// HasBinary returns whether a command exists in PATH.
func HasBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// EnsureClawhub verifies clawhub exists, optionally installing it with npm.
func EnsureClawhub(installIfMissing bool) error {
	if HasBinary("clawhub") {
		return nil
	}
	if !installIfMissing {
		return fmt.Errorf("clawhub not found in PATH; install with: npm install -g --ignore-scripts clawhub")
	}
	if !HasBinary("npm") {
		return fmt.Errorf("npm not found in PATH; install Node.js/npm first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npm", "install", "-g", "--ignore-scripts", "clawhub")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install clawhub with npm: %w", err)
	}
	if !HasBinary("clawhub") {
		return fmt.Errorf("clawhub install completed but binary is not discoverable in PATH")
	}
	return nil
}

// EffectiveSkillEnabled resolves a skill toggle from config entries + bundled defaults.
func EffectiveSkillEnabled(cfg *config.Config, skillName string) bool {
	if cfg == nil || !cfg.Skills.Enabled {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Skills.Scope), "all") {
		return true
	}
	if entry, ok := cfg.Skills.Entries[skillName]; ok {
		return entry.Enabled
	}
	for _, bundled := range BundledCatalog {
		if bundled.Name == skillName {
			return bundled.DefaultEnabled
		}
	}
	return false
}
