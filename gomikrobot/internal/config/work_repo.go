package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnsureWorkRepo ensures the work repo exists and is git-initialized (best effort).
// Returns a warning string if git is unavailable or init fails.
func EnsureWorkRepo(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("work repo path is empty")
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	// Ensure standard artifact directories exist.
	_ = os.MkdirAll(filepath.Join(path, "requirements"), 0755)
	_ = os.MkdirAll(filepath.Join(path, "tasks"), 0755)
	_ = os.MkdirAll(filepath.Join(path, "docs"), 0755)
	_ = os.MkdirAll(filepath.Join(path, "memory"), 0755)

	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return "", nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return "git not found; work repo created without git history", nil
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Sprintf("git init failed: %v (%s)", err, string(out)), nil
	}

	return "", nil
}

// ResolveArtifactPath returns a path under the work repo for the given kind.
func ResolveArtifactPath(workRepoPath, kind, filename string) (string, error) {
	if workRepoPath == "" {
		return "", fmt.Errorf("work repo path is empty")
	}
	k := filepath.Clean(strings.TrimSpace(kind))
	if k == "." || k == "" {
		k = "docs"
	}
	switch k {
	case "requirements", "tasks", "docs":
	default:
		k = "docs"
	}
	if filename == "" {
		filename = "artifact.md"
	}
	return filepath.Join(workRepoPath, k, filename), nil
}
