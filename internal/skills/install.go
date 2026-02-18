package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
)

const metadataFileName = ".kafclaw-skill.json"
const snapshotRetentionPerSkill = 5
const clawhubStateLockFile = "lock.json"
const clawhubStateIndexFile = "managed-lock.json"

// InstallResult contains install/update outcome for a single skill.
type InstallResult struct {
	Name         string       `json:"name"`
	Target       string       `json:"target"`
	InstalledAt  time.Time    `json:"installedAt"`
	Updated      bool         `json:"updated"`
	Report       VerifyReport `json:"report"`
	InstallPath  string       `json:"installPath"`
	WarningCount int          `json:"warningCount"`
}

type installedMetadata struct {
	Name           string    `json:"name"`
	Source         string    `json:"source"`
	SourceHash     string    `json:"sourceHash,omitempty"`
	PinnedHash     string    `json:"pinnedHash,omitempty"`
	InstalledAt    time.Time `json:"installedAt"`
	UpdatedAt      time.Time `json:"updatedAt,omitempty"`
	ClawhubSlug    string    `json:"clawhubSlug,omitempty"`
	ClawhubVersion string    `json:"clawhubVersion,omitempty"`
}

type clawhubInstallMetadata struct {
	Slug    string
	Version string
	LockRaw []byte
}

type clawhubStateEntry struct {
	Slug      string    `json:"slug"`
	Version   string    `json:"version,omitempty"`
	SkillName string    `json:"skillName,omitempty"`
	Source    string    `json:"source,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type installAuditEntry struct {
	Time             time.Time `json:"time"`
	Action           string    `json:"action"`
	Target           string    `json:"target"`
	SkillName        string    `json:"skillName,omitempty"`
	Success          bool      `json:"success"`
	ApprovedWarnings bool      `json:"approvedWarnings"`
	Warnings         int       `json:"warnings"`
	Criticals        int       `json:"criticals"`
	Message          string    `json:"message,omitempty"`
	InstallPath      string    `json:"installPath,omitempty"`
	PrevHash         string    `json:"prevHash,omitempty"`
	Hash             string    `json:"hash,omitempty"`
}

// InstallSkill verifies then installs one skill source.
func InstallSkill(cfg *config.Config, target string, approveWarnings bool) (*InstallResult, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	effectiveTarget, sourceOverride, clawhubMeta, cleanup, err := prepareInstallTarget(cfg, target)
	if err != nil {
		_ = appendInstallAudit(installAuditEntry{
			Time:             time.Now().UTC(),
			Action:           "install",
			Target:           target,
			Success:          false,
			ApprovedWarnings: approveWarnings,
			Message:          err.Error(),
		})
		return nil, err
	}
	defer cleanup()

	pkg, report, err := loadSkillPackage(cfg, effectiveTarget)
	if err != nil {
		_ = appendInstallAudit(installAuditEntry{
			Time:             time.Now().UTC(),
			Action:           "install",
			Target:           target,
			Success:          false,
			ApprovedWarnings: approveWarnings,
			Message:          err.Error(),
		})
		return nil, err
	}
	if strings.TrimSpace(sourceOverride) != "" {
		report.ResolvedTarget = sourceOverride
	}
	scanPackage(cfg, pkg, report)
	validateManifestAndFrontmatter(cfg, pkg, report)
	report.OK = report.CriticalCount() == 0
	if report.CriticalCount() > 0 {
		err := fmt.Errorf("verification failed with %d critical finding(s)", report.CriticalCount())
		_ = appendInstallAudit(installAuditEntry{
			Time:             time.Now().UTC(),
			Action:           "install",
			Target:           target,
			SkillName:        pkg.SkillName,
			Success:          false,
			ApprovedWarnings: approveWarnings,
			Warnings:         report.WarningCount(),
			Criticals:        report.CriticalCount(),
			Message:          err.Error(),
		})
		return nil, err
	}
	if report.WarningCount() > 0 && !approveWarnings {
		err := fmt.Errorf("verification produced %d warning(s); re-run with warning approval", report.WarningCount())
		_ = appendInstallAudit(installAuditEntry{
			Time:             time.Now().UTC(),
			Action:           "install",
			Target:           target,
			SkillName:        pkg.SkillName,
			Success:          false,
			ApprovedWarnings: approveWarnings,
			Warnings:         report.WarningCount(),
			Criticals:        report.CriticalCount(),
			Message:          err.Error(),
		})
		return nil, err
	}
	if pkg.SkillName == "" {
		err := fmt.Errorf("skill name missing after verification")
		_ = appendInstallAudit(installAuditEntry{
			Time:             time.Now().UTC(),
			Action:           "install",
			Target:           target,
			Success:          false,
			ApprovedWarnings: approveWarnings,
			Message:          err.Error(),
		})
		return nil, err
	}
	state, err := EnsureStateDirs()
	if err != nil {
		return nil, err
	}
	dest := filepath.Join(state.Installed, pkg.SkillName)
	snapshot := filepath.Join(state.Snapshots, pkg.SkillName, time.Now().UTC().Format("20060102T150405Z"))
	stage := filepath.Join(state.TmpDir, fmt.Sprintf("install-%s-%d", pkg.SkillName, time.Now().UnixNano()))
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return nil, err
	}
	if err := writePackageFiles(stage, pkg.Files); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	prevInstalledAt := now
	if prevMeta, err := readMetadata(filepath.Join(dest, metadataFileName)); err == nil && !prevMeta.InstalledAt.IsZero() {
		prevInstalledAt = prevMeta.InstalledAt
	}
	meta := installedMetadata{
		Name:        pkg.SkillName,
		Source:      report.ResolvedTarget,
		SourceHash:  computeSourceHash(pkg.Files),
		InstalledAt: prevInstalledAt,
	}
	pinnedHash, hasPinnedHash := extractPinnedSourceHash(target)
	if !hasPinnedHash {
		if pinnedFromResolved, ok := extractPinnedSourceHash(report.ResolvedTarget); ok {
			pinnedHash = pinnedFromResolved
			hasPinnedHash = true
		}
	}
	if hasPinnedHash {
		if meta.SourceHash != pinnedHash {
			err := fmt.Errorf("source hash pin mismatch (expected %s got %s)", pinnedHash, meta.SourceHash)
			_ = appendInstallAudit(installAuditEntry{
				Time:             now,
				Action:           "install",
				Target:           target,
				SkillName:        pkg.SkillName,
				Success:          false,
				ApprovedWarnings: approveWarnings,
				Warnings:         report.WarningCount(),
				Criticals:        report.CriticalCount(),
				Message:          err.Error(),
			})
			return nil, err
		}
		meta.PinnedHash = pinnedHash
	}
	if clawhubMeta != nil {
		meta.ClawhubSlug = clawhubMeta.Slug
		meta.ClawhubVersion = clawhubMeta.Version
	}
	updated := false
	if _, err := os.Stat(dest); err == nil {
		updated = true
		meta.UpdatedAt = now
		if err := os.MkdirAll(filepath.Dir(snapshot), 0o700); err != nil {
			return nil, err
		}
		if err := os.Rename(dest, snapshot); err != nil {
			return nil, err
		}
	}

	if err := writeMetadata(filepath.Join(stage, metadataFileName), meta); err != nil {
		return nil, err
	}
	if err := os.Rename(stage, dest); err != nil {
		if updated {
			_ = os.Rename(snapshot, dest)
		}
		_ = appendInstallAudit(installAuditEntry{
			Time:             now,
			Action:           "install",
			Target:           target,
			SkillName:        pkg.SkillName,
			Success:          false,
			ApprovedWarnings: approveWarnings,
			Warnings:         report.WarningCount(),
			Criticals:        report.CriticalCount(),
			Message:          err.Error(),
		})
		return nil, err
	}
	if updated {
		_ = pruneSkillSnapshots(filepath.Join(state.Snapshots, pkg.SkillName), snapshotRetentionPerSkill)
	}
	if clawhubMeta != nil {
		_ = syncClawhubState(state, pkg.SkillName, report.ResolvedTarget, clawhubMeta)
	}
	result := &InstallResult{
		Name:         pkg.SkillName,
		Target:       target,
		InstalledAt:  now,
		Updated:      updated,
		Report:       *report,
		InstallPath:  dest,
		WarningCount: report.WarningCount(),
	}
	_ = appendInstallAudit(installAuditEntry{
		Time:             now,
		Action:           "install",
		Target:           target,
		SkillName:        pkg.SkillName,
		Success:          true,
		ApprovedWarnings: approveWarnings,
		Warnings:         report.WarningCount(),
		Criticals:        report.CriticalCount(),
		InstallPath:      dest,
	})
	return result, nil
}

func prepareInstallTarget(cfg *config.Config, target string) (effectiveTarget string, sourceOverride string, clawhubMeta *clawhubInstallMetadata, cleanup func(), err error) {
	noCleanup := func() {}
	slug, ok := normalizeClawhubTarget(target)
	if !ok {
		return target, "", nil, noCleanup, nil
	}
	if cfg == nil || !cfg.Skills.ExternalInstalls {
		return target, "", nil, noCleanup, nil
	}
	if !HasBinary("clawhub") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(target)), "clawhub:") {
			return "", "", nil, noCleanup, fmt.Errorf("clawhub source requested but `clawhub` is not in PATH")
		}
		return target, "", nil, noCleanup, nil
	}
	dirs, err := EnsureStateDirs()
	if err != nil {
		return "", "", nil, noCleanup, err
	}
	stage := filepath.Join(dirs.Quarantine, fmt.Sprintf("clawhub-%d", time.Now().UnixNano()))
	skillsDir := filepath.Join(stage, "skills")
	if err := os.MkdirAll(skillsDir, 0o700); err != nil {
		return "", "", nil, noCleanup, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	args := []string{"install", slug, "--force", "--no-input", "--workdir", stage, "--dir", "skills"}
	cmd := exec.CommandContext(ctx, "clawhub", args...)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		_ = os.RemoveAll(stage)
		return "", "", nil, noCleanup, fmt.Errorf("clawhub install %q failed: %w (%s)", slug, runErr, strings.TrimSpace(string(out)))
	}
	skillPath, err := detectSingleInstalledSkillDir(skillsDir, slug)
	if err != nil {
		_ = os.RemoveAll(stage)
		return "", "", nil, noCleanup, err
	}
	meta := loadClawhubInstallMetadata(stage, slug)
	return skillPath, "clawhub:" + slug, meta, func() { _ = os.RemoveAll(stage) }, nil
}

func normalizeClawhubTarget(target string) (string, bool) {
	t := strings.TrimSpace(target)
	if t == "" {
		return "", false
	}
	lower := strings.ToLower(t)
	if strings.HasPrefix(lower, "clawhub:") {
		slug := sanitizeSkillName(strings.TrimSpace(t[len("clawhub:"):]))
		return slug, slug != ""
	}
	if strings.Contains(t, "/") || strings.Contains(t, `\`) {
		return "", false
	}
	if isLikelyURL(t) {
		return "", false
	}
	if _, err := os.Stat(t); err == nil {
		return "", false
	}
	slug := sanitizeSkillName(t)
	return slug, slug != ""
}

func isLikelyURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")
}

func detectSingleInstalledSkillDir(root string, slug string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	cands := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			cands = append(cands, e.Name())
		}
	}
	if len(cands) == 0 {
		return "", fmt.Errorf("clawhub install produced no skill directory")
	}
	sort.Strings(cands)
	preferred := sanitizeSkillName(slug)
	for _, c := range cands {
		if sanitizeSkillName(c) == preferred {
			return filepath.Join(root, c), nil
		}
	}
	if len(cands) == 1 {
		return filepath.Join(root, cands[0]), nil
	}
	return "", fmt.Errorf("clawhub install produced multiple skill directories: %s", strings.Join(cands, ", "))
}

func loadClawhubInstallMetadata(stage string, slug string) *clawhubInstallMetadata {
	meta := &clawhubInstallMetadata{Slug: sanitizeSkillName(slug)}
	lockPath := filepath.Join(stage, ".clawhub", clawhubStateLockFile)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return meta
	}
	meta.LockRaw = data
	meta.Version = extractClawhubVersion(data, meta.Slug)
	return meta
}

func extractClawhubVersion(lockRaw []byte, slug string) string {
	var doc any
	if err := json.Unmarshal(lockRaw, &doc); err != nil {
		return ""
	}
	slug = sanitizeSkillName(slug)
	versionFromObj := func(obj map[string]any) string {
		s := sanitizeSkillName(toString(obj["slug"]))
		if s == "" || s != slug {
			return ""
		}
		for _, k := range []string{"version", "resolvedVersion", "tag"} {
			if v := strings.TrimSpace(toString(obj[k])); v != "" {
				return v
			}
		}
		return ""
	}
	switch v := doc.(type) {
	case map[string]any:
		if ver := versionFromObj(v); ver != "" {
			return ver
		}
		if skills, ok := v["skills"].([]any); ok {
			for _, it := range skills {
				if m, ok := it.(map[string]any); ok {
					if ver := versionFromObj(m); ver != "" {
						return ver
					}
				}
			}
		}
	}
	return ""
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprintf("%v", v)
	}
}

func syncClawhubState(state StateDirs, skillName, source string, meta *clawhubInstallMetadata) error {
	if meta == nil || strings.TrimSpace(meta.Slug) == "" {
		return nil
	}
	root := filepath.Join(state.ToolsDir, "clawhub")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	if len(meta.LockRaw) > 0 {
		if err := os.WriteFile(filepath.Join(root, clawhubStateLockFile), meta.LockRaw, 0o600); err != nil {
			return err
		}
	}
	indexPath := filepath.Join(root, clawhubStateIndexFile)
	idx := map[string]clawhubStateEntry{}
	if data, err := os.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(data, &idx)
	}
	idx[meta.Slug] = clawhubStateEntry{
		Slug:      meta.Slug,
		Version:   strings.TrimSpace(meta.Version),
		SkillName: strings.TrimSpace(skillName),
		Source:    strings.TrimSpace(source),
		UpdatedAt: time.Now().UTC(),
	}
	out, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, append(out, '\n'), 0o600)
}

func extractPinnedSourceHash(target string) (string, bool) {
	t := strings.TrimSpace(target)
	if t == "" {
		return "", false
	}
	u, err := url.Parse(t)
	if err != nil || u.Fragment == "" {
		return "", false
	}
	const prefix = "sha256="
	frag := strings.ToLower(strings.TrimSpace(u.Fragment))
	if !strings.HasPrefix(frag, prefix) {
		return "", false
	}
	v := strings.TrimSpace(frag[len(prefix):])
	if len(v) != 64 {
		return "", false
	}
	for _, ch := range v {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return "", false
		}
	}
	return v, true
}

func verifyInstalledSkillIntegrity(root string, meta *installedMetadata) error {
	if meta == nil || strings.TrimSpace(meta.SourceHash) == "" {
		return nil
	}
	files, err := readLocalDir(root)
	if err != nil {
		return err
	}
	filtered := make([]sourceFile, 0, len(files))
	for _, f := range files {
		if filepath.ToSlash(strings.TrimSpace(f.Path)) == metadataFileName {
			continue
		}
		filtered = append(filtered, f)
	}
	current := computeSourceHash(filtered)
	if current == "" {
		return fmt.Errorf("empty computed source hash")
	}
	if current != strings.TrimSpace(meta.SourceHash) {
		return fmt.Errorf("source hash mismatch (expected %s got %s)", meta.SourceHash, current)
	}
	return nil
}

func computeSourceHash(files []sourceFile) string {
	if len(files) == 0 {
		return ""
	}
	normalized := make([]sourceFile, 0, len(files))
	for _, f := range files {
		p := filepath.ToSlash(strings.TrimSpace(f.Path))
		if p == "" {
			continue
		}
		normalized = append(normalized, sourceFile{Path: p, Data: f.Data})
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Path < normalized[j].Path })
	h := sha256.New()
	for _, f := range normalized {
		sum := sha256.Sum256(f.Data)
		_, _ = h.Write([]byte(f.Path))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(hex.EncodeToString(sum[:])))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// UpdateSkills updates one installed skill by name, or all when name is empty.
func UpdateSkills(cfg *config.Config, name string, approveWarnings bool) ([]InstallResult, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	state, err := EnsureStateDirs()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(state.Installed)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no installed skills found")
		}
		return nil, err
	}

	var targets []string
	targetName := sanitizeSkillName(name)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if targetName != "" && e.Name() != targetName {
			continue
		}
		meta, err := readMetadata(filepath.Join(state.Installed, e.Name(), metadataFileName))
		if err != nil {
			continue
		}
		if err := verifyInstalledSkillIntegrity(filepath.Join(state.Installed, e.Name()), meta); err != nil {
			return nil, fmt.Errorf("installed skill %q failed integrity check: %w", e.Name(), err)
		}
		if strings.TrimSpace(meta.Source) == "" {
			continue
		}
		targets = append(targets, meta.Source)
	}
	if len(targets) == 0 {
		if targetName != "" {
			return nil, fmt.Errorf("skill %q not installed or has no update source", targetName)
		}
		return nil, fmt.Errorf("no installed skills with update source metadata")
	}
	sort.Strings(targets)
	out := make([]InstallResult, 0, len(targets))
	for _, target := range targets {
		_ = appendInstallAudit(installAuditEntry{
			Time:             time.Now().UTC(),
			Action:           "update",
			Target:           target,
			Success:          false,
			ApprovedWarnings: approveWarnings,
			Message:          "update started",
		})
		res, err := InstallSkill(cfg, target, approveWarnings)
		if err != nil {
			_ = appendInstallAudit(installAuditEntry{
				Time:             time.Now().UTC(),
				Action:           "update",
				Target:           target,
				SkillName:        sanitizeSkillName(name),
				Success:          false,
				ApprovedWarnings: approveWarnings,
				Message:          err.Error(),
			})
			return nil, err
		}
		out = append(out, *res)
		_ = appendInstallAudit(installAuditEntry{
			Time:             time.Now().UTC(),
			Action:           "update",
			Target:           target,
			SkillName:        res.Name,
			Success:          true,
			ApprovedWarnings: approveWarnings,
			Warnings:         res.WarningCount,
			Criticals:        res.Report.CriticalCount(),
			InstallPath:      res.InstallPath,
		})
	}
	return out, nil
}

func writePackageFiles(root string, files []sourceFile) error {
	for _, f := range files {
		rel := filepath.ToSlash(strings.TrimSpace(f.Path))
		if !isSafeRelativePath(rel) {
			return fmt.Errorf("unsafe file path in package: %s", rel)
		}
		dst := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(dst, f.Data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func writeMetadata(path string, meta installedMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func readMetadata(path string) (*installedMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta installedMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func appendInstallAudit(entry installAuditEntry) error {
	state, err := EnsureStateDirs()
	if err != nil {
		return err
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	if err := os.MkdirAll(state.AuditDir, 0o700); err != nil {
		return err
	}
	installPath := filepath.Join(state.AuditDir, "install-decisions.jsonl")
	if err := appendChainedAuditLine(installPath, &entry); err != nil {
		return err
	}
	securityEvent := map[string]any{
		"action":           entry.Action,
		"target":           entry.Target,
		"skillName":        entry.SkillName,
		"success":          entry.Success,
		"approvedWarnings": entry.ApprovedWarnings,
		"warnings":         entry.Warnings,
		"criticals":        entry.Criticals,
		"message":          entry.Message,
		"installPath":      entry.InstallPath,
	}
	return appendSecurityAuditEvent("skills_install", securityEvent)
}

func pruneSkillSnapshots(snapshotRoot string, keep int) error {
	if keep <= 0 {
		keep = 1
	}
	entries, err := os.ReadDir(snapshotRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) <= keep {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) <= keep {
		return nil
	}
	for _, old := range names[:len(names)-keep] {
		if err := os.RemoveAll(filepath.Join(snapshotRoot, old)); err != nil {
			return err
		}
	}
	return nil
}
