package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/cliconfig"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"github.com/spf13/cobra"
)

var updateLatest bool
var updateVersion string
var updateSource bool
var updateSkipBackup bool
var updateBackupDir string
var updateDryRun bool
var updateAllowDowngrade bool
var updateRepoPath string
var updateRollbackPath string
var updateRestoreBinary bool
var updateJSON bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Manage KafClaw updates (plan, apply, backup, rollback)",
}

var updatePlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Print canonical update flow",
	RunE:  runUpdatePlan,
}

var updateBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create pre-update backup snapshot",
	RunE:  runUpdateBackup,
}

var updateApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Run update flow with preflight, backup, apply, and post-checks",
	RunE:  runUpdateApply,
}

var updateRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Restore state from a backup snapshot",
	RunE:  runUpdateRollback,
}

type updateBackupManifest struct {
	CreatedAt      string            `json:"createdAt"`
	CurrentVersion string            `json:"currentVersion"`
	Paths          map[string]string `json:"paths"`
}

var (
	nowFn              = func() time.Time { return time.Now().UTC() }
	runDoctorReportFn  = cliconfig.RunDoctorWithOptions
	runSecurityCheckFn = cliconfig.RunSecurityCheck
	runBinaryUpdateFn  = runBinaryUpdate
	runSourceUpdateFn  = runSourceUpdate
)

func init() {
	updateCmd.AddCommand(updatePlanCmd)
	updateCmd.AddCommand(updateBackupCmd)
	updateCmd.AddCommand(updateApplyCmd)
	updateCmd.AddCommand(updateRollbackCmd)

	updateApplyCmd.Flags().BoolVar(&updateLatest, "latest", false, "Update to latest release (binary flow)")
	updateApplyCmd.Flags().StringVar(&updateVersion, "version", "", "Update to pinned version (for example v2.6.3)")
	updateApplyCmd.Flags().BoolVar(&updateSource, "source", false, "Use source update flow (git pull + build) instead of release binary flow")
	updateApplyCmd.Flags().BoolVar(&updateSkipBackup, "skip-backup", false, "Skip creating pre-update backup snapshot")
	updateApplyCmd.Flags().StringVar(&updateBackupDir, "backup-dir", "", "Backup root directory (default: ~/.kafclaw/backups)")
	updateApplyCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show update actions without executing")
	updateApplyCmd.Flags().BoolVar(&updateAllowDowngrade, "allow-downgrade", false, "Allow pinned target version lower than current")
	updateApplyCmd.Flags().StringVar(&updateRepoPath, "repo-path", "", "Repo path for source updates (default: current working directory)")

	updateBackupCmd.Flags().StringVar(&updateBackupDir, "backup-dir", "", "Backup root directory (default: ~/.kafclaw/backups)")

	updateRollbackCmd.Flags().StringVar(&updateRollbackPath, "backup-path", "", "Backup snapshot path to restore (default: latest)")
	updateRollbackCmd.Flags().StringVar(&updateBackupDir, "backup-dir", "", "Backup root directory (default: ~/.kafclaw/backups)")
	updateRollbackCmd.Flags().BoolVar(&updateRestoreBinary, "restore-binary", false, "Restore backed-up binary to current executable path")
	updateCmd.PersistentFlags().BoolVar(&updateJSON, "json", false, "Output machine-readable JSON")

	rootCmd.AddCommand(updateCmd)
}

func runUpdatePlan(cmd *cobra.Command, args []string) error {
	if updateJSON {
		return printUpdateJSON(cmd, "ok", "plan", map[string]any{
			"steps": []string{
				"preflight",
				"backup",
				"apply",
				"health-gate",
				"rollback",
			},
		}, "")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Canonical update flow:")
	fmt.Fprintln(cmd.OutOrStdout(), "1) preflight: config/timeline compatibility checks")
	fmt.Fprintln(cmd.OutOrStdout(), "2) backup: snapshot config/env/timeline(+binary)")
	fmt.Fprintln(cmd.OutOrStdout(), "3) apply: release binary update (or source update)")
	fmt.Fprintln(cmd.OutOrStdout(), "4) health gate: doctor + security checks")
	fmt.Fprintln(cmd.OutOrStdout(), "5) rollback: restore snapshot if checks fail")
	fmt.Fprintln(cmd.OutOrStdout(), "")
	fmt.Fprintln(cmd.OutOrStdout(), "Examples:")
	fmt.Fprintln(cmd.OutOrStdout(), "  kafclaw update apply --latest")
	fmt.Fprintln(cmd.OutOrStdout(), "  kafclaw update apply --version v2.6.3")
	fmt.Fprintln(cmd.OutOrStdout(), "  kafclaw update rollback --backup-path <snapshot-dir>")
	return nil
}

func runUpdateBackup(cmd *cobra.Command, args []string) error {
	_ = emitLifecycleEvent("update", "backup", "info", "backup requested", nil)
	backupRoot, err := resolveUpdateBackupRoot(updateBackupDir)
	if err != nil {
		_ = emitLifecycleEvent("update", "backup", "error", err.Error(), nil)
		return err
	}
	path, _, err := createUpdateBackup(backupRoot)
	if err != nil {
		_ = emitLifecycleEvent("update", "backup", "error", err.Error(), nil)
		return err
	}
	_ = emitLifecycleEvent("update", "backup", "ok", "backup created", map[string]any{"path": path})
	if updateJSON {
		return printUpdateJSON(cmd, "ok", "backup", map[string]any{"path": path}, "")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Backup created: %s\n", path)
	return nil
}

func runUpdateApply(cmd *cobra.Command, args []string) error {
	_ = emitLifecycleEvent("update", "apply-start", "info", "update apply started", map[string]any{
		"latest":     updateLatest,
		"version":    strings.TrimSpace(updateVersion),
		"source":     updateSource,
		"skipBackup": updateSkipBackup,
	})
	if err := validateUpdateTarget(updateLatest, updateVersion, updateSource); err != nil {
		_ = emitLifecycleEvent("update", "validate-target", "error", err.Error(), nil)
		return err
	}

	current := strings.TrimSpace(version)
	target := normalizeVersion(updateVersion)
	if err := preflightUpdateCompatibility(current, target, updateAllowDowngrade); err != nil {
		_ = emitLifecycleEvent("update", "preflight-compat", "error", err.Error(), nil)
		return err
	}
	if err := preflightUpdateRuntime(); err != nil {
		_ = emitLifecycleEvent("update", "preflight-runtime", "error", err.Error(), nil)
		return err
	}
	_ = emitLifecycleEvent("update", "preflight", "ok", "preflight checks passed", nil)

	beforeCfg, _ := loadConfigMapForDrift()

	backupRoot, err := resolveUpdateBackupRoot(updateBackupDir)
	if err != nil {
		return err
	}
	var backupPath string
	if !updateSkipBackup {
		p, _, bErr := createUpdateBackup(backupRoot)
		if bErr != nil {
			_ = emitLifecycleEvent("update", "backup", "error", bErr.Error(), nil)
			return bErr
		}
		backupPath = p
		_ = emitLifecycleEvent("update", "backup", "ok", "backup created", map[string]any{"path": backupPath})
		if !updateJSON {
			fmt.Fprintf(cmd.OutOrStdout(), "Pre-update backup: %s\n", backupPath)
		}
	}

	if updateDryRun {
		_ = emitLifecycleEvent("update", "apply", "ok", "dry-run completed", nil)
		if updateJSON {
			return printUpdateJSON(cmd, "ok", "apply", map[string]any{
				"dryRun":     true,
				"backupPath": backupPath,
			}, "")
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Dry-run mode: update apply skipped.")
		return nil
	}

	if updateSource {
		repoPath := strings.TrimSpace(updateRepoPath)
		if repoPath == "" {
			cwd, cwdErr := os.Getwd()
			if cwdErr != nil {
				return cwdErr
			}
			repoPath = cwd
		}
		if err := runSourceUpdateFn(repoPath); err != nil {
			_ = emitLifecycleEvent("update", "apply", "error", err.Error(), map[string]any{"path": "source"})
			return fmt.Errorf("source update failed: %w", err)
		}
	} else {
		if err := runBinaryUpdateFn(updateLatest, target); err != nil {
			_ = emitLifecycleEvent("update", "apply", "error", err.Error(), map[string]any{"path": "binary"})
			return fmt.Errorf("binary update failed: %w", err)
		}
	}
	_ = emitLifecycleEvent("update", "apply", "ok", "update apply succeeded", nil)

	failures := 0
	doctorReport, err := runDoctorReportFn(cliconfig.DoctorOptions{})
	if err != nil {
		return fmt.Errorf("post-update doctor failed: %w", err)
	}
	for _, c := range doctorReport.Checks {
		if c.Status == cliconfig.DoctorFail {
			failures++
		}
	}
	securityReport, err := runSecurityCheckFn()
	if err != nil {
		return fmt.Errorf("post-update security check failed: %w", err)
	}
	if securityReport.HasFailures() {
		failures++
	}

	afterCfg, _ := loadConfigMapForDrift()
	drift := detectConfigDrift(beforeCfg, afterCfg)
	if !updateJSON && len(drift) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Config drift detected:")
		for _, d := range drift {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", d)
		}
	} else if !updateJSON {
		fmt.Fprintln(cmd.OutOrStdout(), "Config drift: none")
	}

	if failures > 0 {
		_ = emitLifecycleEvent("update", "health-gate", "error", "post-update health gate failed", map[string]any{"failures": failures})
		if updateJSON {
			return printUpdateJSON(cmd, "error", "apply", map[string]any{
				"backupPath": backupPath,
				"drift":      drift,
				"failures":   failures,
			}, "post-update health gate failed")
		}
		if backupPath != "" {
			return fmt.Errorf("post-update health gate failed; run `kafclaw update rollback --backup-path %s`", backupPath)
		}
		return fmt.Errorf("post-update health gate failed")
	}
	_ = emitLifecycleEvent("update", "health-gate", "ok", "post-update health checks passed", nil)
	_ = emitLifecycleEvent("update", "complete", "ok", "update flow completed", nil)
	if updateJSON {
		return printUpdateJSON(cmd, "ok", "apply", map[string]any{
			"backupPath": backupPath,
			"drift":      drift,
		}, "")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Update completed and health gates passed.")
	return nil
}

func runUpdateRollback(cmd *cobra.Command, args []string) error {
	_ = emitLifecycleEvent("update", "rollback-start", "info", "rollback requested", nil)
	backupRoot, err := resolveUpdateBackupRoot(updateBackupDir)
	if err != nil {
		_ = emitLifecycleEvent("update", "rollback", "error", err.Error(), nil)
		return err
	}
	path := strings.TrimSpace(updateRollbackPath)
	if path == "" {
		path, err = findLatestBackup(backupRoot)
		if err != nil {
			_ = emitLifecycleEvent("update", "rollback", "error", err.Error(), nil)
			return err
		}
	}
	manifest, err := restoreUpdateBackup(path, updateRestoreBinary)
	if err != nil {
		_ = emitLifecycleEvent("update", "rollback", "error", err.Error(), map[string]any{"path": path})
		return err
	}
	_ = emitLifecycleEvent("update", "rollback", "ok", "rollback restored", map[string]any{"path": path})
	if updateJSON {
		return printUpdateJSON(cmd, "ok", "rollback", map[string]any{
			"path":            path,
			"restoredVersion": manifest.CurrentVersion,
		}, "")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Rollback restored from: %s\n", path)
	fmt.Fprintf(cmd.OutOrStdout(), "Restored version context: %s\n", manifest.CurrentVersion)
	return nil
}

func validateUpdateTarget(latest bool, pinned string, source bool) error {
	hasPinned := strings.TrimSpace(pinned) != ""
	if latest && hasPinned {
		return fmt.Errorf("use either --latest or --version, not both")
	}
	if !source && !latest && !hasPinned {
		return fmt.Errorf("binary update requires --latest or --version")
	}
	return nil
}

func preflightUpdateRuntime() error {
	if _, err := config.Load(); err != nil {
		return fmt.Errorf("config compatibility check failed: %w", err)
	}
	home, err := resolveUpdateHome()
	if err != nil {
		return err
	}
	dbPath := filepath.Join(home, ".kafclaw", "timeline.db")
	if _, err := os.Stat(dbPath); err == nil {
		svc, err := timeline.NewTimelineService(dbPath)
		if err != nil {
			return fmt.Errorf("timeline migration check failed: %w", err)
		}
		_ = svc.Close()
	}
	return nil
}

func preflightUpdateCompatibility(current, target string, allowDowngrade bool) error {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	cur, err := parseSemver(current)
	if err != nil {
		return nil
	}
	tgt, err := parseSemver(target)
	if err != nil {
		return fmt.Errorf("invalid target version %q", target)
	}
	if tgt.major-cur.major > 1 {
		return fmt.Errorf("major jump too large: current=%s target=%s", current, target)
	}
	if versionLess(tgt, cur) && !allowDowngrade {
		return fmt.Errorf("target version %s is lower than current %s; pass --allow-downgrade or use rollback", target, current)
	}
	return nil
}

func runBinaryUpdate(latest bool, target string) error {
	script := filepath.Join("scripts", "install.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("installer script not found at %s", script)
	}
	args := []string{script, "--yes"}
	if latest {
		args = append(args, "--latest")
	} else {
		args = append(args, "--version", target)
	}
	cmd := exec.Command("bash", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runSourceUpdate(repoPath string) error {
	if strings.TrimSpace(repoPath) == "" {
		return fmt.Errorf("repo path is required")
	}
	steps := [][]string{
		{"git", "-C", repoPath, "pull", "--ff-only"},
		{"make", "-C", repoPath, "check"},
		{"make", "-C", repoPath, "build"},
	}
	for _, step := range steps {
		cmd := exec.Command(step[0], step[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", strings.Join(step, " "), err)
		}
	}
	return nil
}

func createUpdateBackup(root string) (string, updateBackupManifest, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", updateBackupManifest{}, err
	}
	ts := nowFn().Format("20060102-150405Z")
	path := filepath.Join(root, "update-"+ts)
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", updateBackupManifest{}, err
	}

	home, err := resolveUpdateHome()
	if err != nil {
		return "", updateBackupManifest{}, err
	}
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return "", updateBackupManifest{}, err
	}
	envPath := filepath.Join(home, ".config", "kafclaw", "env")
	timelinePath := filepath.Join(home, ".kafclaw", "timeline.db")

	manifest := updateBackupManifest{
		CreatedAt:      nowFn().Format(time.RFC3339),
		CurrentVersion: version,
		Paths:          map[string]string{},
	}

	if dst, ok, err := backupFile(cfgPath, filepath.Join(path, "config.json")); err != nil {
		return "", updateBackupManifest{}, err
	} else if ok {
		manifest.Paths["config"] = dst
	}
	if dst, ok, err := backupFile(envPath, filepath.Join(path, "env")); err != nil {
		return "", updateBackupManifest{}, err
	} else if ok {
		manifest.Paths["env"] = dst
	}
	if dst, ok, err := backupFile(timelinePath, filepath.Join(path, "timeline.db")); err != nil {
		return "", updateBackupManifest{}, err
	} else if ok {
		manifest.Paths["timeline"] = dst
	}
	if dst, ok, err := backupFile(timelinePath+"-wal", filepath.Join(path, "timeline.db-wal")); err != nil {
		return "", updateBackupManifest{}, err
	} else if ok {
		manifest.Paths["timeline_wal"] = dst
	}
	if dst, ok, err := backupFile(timelinePath+"-shm", filepath.Join(path, "timeline.db-shm")); err != nil {
		return "", updateBackupManifest{}, err
	} else if ok {
		manifest.Paths["timeline_shm"] = dst
	}
	if exe, err := os.Executable(); err == nil {
		if dst, ok, err := backupFile(exe, filepath.Join(path, "kafclaw.binary")); err != nil {
			return "", updateBackupManifest{}, err
		} else if ok {
			manifest.Paths["binary"] = dst
		}
	}

	manifestPath := filepath.Join(path, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", updateBackupManifest{}, err
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		return "", updateBackupManifest{}, err
	}
	manifest.Paths["manifest"] = manifestPath
	return path, manifest, nil
}

func restoreUpdateBackup(path string, restoreBinary bool) (updateBackupManifest, error) {
	manifestPath := filepath.Join(path, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return updateBackupManifest{}, err
	}
	var manifest updateBackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return updateBackupManifest{}, err
	}
	home, err := resolveUpdateHome()
	if err != nil {
		return updateBackupManifest{}, err
	}
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return updateBackupManifest{}, err
	}
	restorePair := []struct {
		key string
		dst string
	}{
		{"config", cfgPath},
		{"env", filepath.Join(home, ".config", "kafclaw", "env")},
		{"timeline", filepath.Join(home, ".kafclaw", "timeline.db")},
		{"timeline_wal", filepath.Join(home, ".kafclaw", "timeline.db-wal")},
		{"timeline_shm", filepath.Join(home, ".kafclaw", "timeline.db-shm")},
	}
	for _, p := range restorePair {
		src := strings.TrimSpace(manifest.Paths[p.key])
		if src == "" {
			continue
		}
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := copyFile(src, p.dst, 0o600); err != nil {
			return updateBackupManifest{}, err
		}
	}
	if restoreBinary {
		src := strings.TrimSpace(manifest.Paths["binary"])
		if src != "" {
			if exe, err := os.Executable(); err == nil {
				if err := copyFile(src, exe, 0o755); err != nil {
					return updateBackupManifest{}, err
				}
			}
		}
	}
	return manifest, nil
}

func backupFile(src, dst string) (string, bool, error) {
	info, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o600
	}
	if err := copyFile(src, dst, mode); err != nil {
		return "", false, err
	}
	return dst, true, nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Chmod(mode)
}

func resolveUpdateHome() (string, error) {
	if h := strings.TrimSpace(os.Getenv("KAFCLAW_HOME")); h != "" {
		return h, nil
	}
	if h := strings.TrimSpace(os.Getenv("MIKROBOT_HOME")); h != "" {
		return h, nil
	}
	return os.UserHomeDir()
}

func resolveUpdateBackupRoot(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override), nil
	}
	home, err := resolveUpdateHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kafclaw", "backups"), nil
}

func findLatestBackup(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	candidates := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), "update-") {
			candidates = append(candidates, filepath.Join(root, e.Name()))
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no backup snapshots found in %s", root)
	}
	sort.Strings(candidates)
	return candidates[len(candidates)-1], nil
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

type semver struct {
	major int
	minor int
	patch int
}

var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

func parseSemver(v string) (semver, error) {
	m := semverRe.FindStringSubmatch(strings.TrimSpace(v))
	if len(m) != 4 {
		return semver{}, fmt.Errorf("invalid semver")
	}
	var out semver
	if _, err := fmt.Sscanf(m[0], "v%d.%d.%d", &out.major, &out.minor, &out.patch); err != nil {
		if _, err := fmt.Sscanf(m[0], "%d.%d.%d", &out.major, &out.minor, &out.patch); err != nil {
			return semver{}, err
		}
	}
	return out, nil
}

func versionLess(a, b semver) bool {
	if a.major != b.major {
		return a.major < b.major
	}
	if a.minor != b.minor {
		return a.minor < b.minor
	}
	return a.patch < b.patch
}

func loadConfigMapForDrift() (map[string]any, error) {
	cfgPath, err := config.ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func detectConfigDrift(before, after map[string]any) []string {
	pre := map[string]string{}
	post := map[string]string{}
	flattenConfig("", before, pre)
	flattenConfig("", after, post)
	keys := map[string]struct{}{}
	for k := range pre {
		keys[k] = struct{}{}
	}
	for k := range post {
		keys[k] = struct{}{}
	}
	out := make([]string, 0)
	for k := range keys {
		if pre[k] != post[k] {
			switch {
			case pre[k] == "":
				out = append(out, k+" (added)")
			case post[k] == "":
				out = append(out, k+" (removed)")
			default:
				out = append(out, k+" (changed)")
			}
		}
	}
	sort.Strings(out)
	return out
}

func flattenConfig(prefix string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			flattenConfig(next, child, out)
		}
	case []any:
		b, _ := json.Marshal(t)
		out[prefix] = string(b)
	default:
		b, _ := json.Marshal(t)
		out[prefix] = string(b)
	}
}

func printUpdateJSON(cmd *cobra.Command, status, action string, result map[string]any, errMsg string) error {
	payload := map[string]any{
		"status":  strings.TrimSpace(status),
		"command": "update",
		"action":  strings.TrimSpace(action),
	}
	if len(result) > 0 {
		payload["result"] = result
	}
	if strings.TrimSpace(errMsg) != "" {
		payload["error"] = strings.TrimSpace(errMsg)
	}
	b, _ := json.MarshalIndent(payload, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(b))
	if strings.EqualFold(status, "error") {
		if errMsg == "" {
			errMsg = "update command failed"
		}
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}
