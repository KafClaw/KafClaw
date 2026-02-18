package skills

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/KafClaw/KafClaw/internal/config"
)

const (
	maxRemoteSkillBytes = 20 << 20 // 20 MiB
	maxSkillFiles       = 1500
	maxFileBytes        = 1 << 20 // 1 MiB per scanned file
	maxArchiveEntryBytes = 5 << 20 // 5 MiB per archive entry
	maxArchiveTotalBytes = 40 << 20 // 40 MiB total extracted bytes
	maxDownloadRedirects = 10
)

// Severity indicates a verification finding level.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarn     Severity = "warn"
	SeverityInfo     Severity = "info"
)

// Finding represents a single verification result.
type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	File     string   `json:"file,omitempty"`
}

// VerifyReport summarizes verification checks for a skill source.
type VerifyReport struct {
	OK             bool      `json:"ok"`
	Target         string    `json:"target"`
	ResolvedTarget string    `json:"resolvedTarget,omitempty"`
	SourceType     string    `json:"sourceType"`
	SkillName      string    `json:"skillName,omitempty"`
	FileCount      int       `json:"fileCount"`
	LinkCount      int       `json:"linkCount"`
	Findings       []Finding `json:"findings,omitempty"`
}

func (r *VerifyReport) appendFinding(sev Severity, code, msg, file string) {
	r.Findings = append(r.Findings, Finding{
		Severity: sev,
		Code:     code,
		Message:  msg,
		File:     file,
	})
}

// CriticalCount returns number of critical findings.
func (r *VerifyReport) CriticalCount() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityCritical {
			n++
		}
	}
	return n
}

// WarningCount returns number of warning findings.
func (r *VerifyReport) WarningCount() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == SeverityWarn {
			n++
		}
	}
	return n
}

type sourceFile struct {
	Path string
	Data []byte
}

type sourcePackage struct {
	SourceType     string
	Target         string
	ResolvedTarget string
	SkillName      string
	Files          []sourceFile
}

var (
	urlRegex            = regexp.MustCompile(`https?://[^\s"'<>]+`)
	frontmatterNameExpr = regexp.MustCompile(`(?m)^name:\s*([a-zA-Z0-9_.-]+)\s*$`)
	frontmatterDescExpr = regexp.MustCompile(`(?m)^description:\s*(.+)\s*$`)
)

type skillFrontmatter struct {
	Name        string
	Description string
}

type skillPolicyManifest struct {
	Version   string `json:"version"`
	Execution struct {
		Network           *bool    `json:"network,omitempty"`
		ReadOnlyWorkspace *bool    `json:"readOnlyWorkspace,omitempty"`
		AllowCommands     []string `json:"allowCommands,omitempty"`
		DenyCommands      []string `json:"denyCommands,omitempty"`
		TimeoutSeconds    int      `json:"timeoutSeconds,omitempty"`
		MaxOutputBytes    int      `json:"maxOutputBytes,omitempty"`
	} `json:"execution,omitempty"`
	LinkPolicy struct {
		AllowDomains []string `json:"allowDomains,omitempty"`
		DenyDomains  []string `json:"denyDomains,omitempty"`
		AllowHTTP    *bool    `json:"allowHttp,omitempty"`
	} `json:"linkPolicy,omitempty"`
}

var suspiciousPatterns = []struct {
	re       *regexp.Regexp
	code     string
	severity Severity
	message  string
}{
	{regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n]{0,200}\|\s*(sh|bash)\b`), "pipe_shell_exec", SeverityCritical, "Potential remote script execution pattern"},
	{regexp.MustCompile(`(?i)\bpowershell\b[^\n]{0,200}-enc(odedcommand)?\b`), "encoded_powershell", SeverityCritical, "Potential encoded PowerShell execution pattern"},
	{regexp.MustCompile(`(?i)\beval\s*\(`), "eval_usage", SeverityWarn, "Dynamic eval usage detected"},
	{regexp.MustCompile(`(?i)exec\.Command\(`), "exec_command_usage", SeverityWarn, "Command execution usage detected"},
	{regexp.MustCompile(`(?i)\bos/exec\b`), "os_exec_import", SeverityWarn, "os/exec import detected"},
	{regexp.MustCompile(`(?i)\bsubprocess\b`), "subprocess_usage", SeverityWarn, "subprocess usage detected"},
	{regexp.MustCompile(`(?i)base64[^\n]{0,100}(decode|DecodeString)`), "base64_decode", SeverityWarn, "Base64 decode usage detected"},
}

// VerifySkillSource runs static security checks for local/remote skill sources.
func VerifySkillSource(cfg *config.Config, target string) (*VerifyReport, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	pkg, report, err := loadSkillPackage(cfg, target)
	if err != nil {
		return nil, err
	}
	report.SkillName = pkg.SkillName
	report.FileCount = len(pkg.Files)
	scanPackage(cfg, pkg, report)
	validateManifestAndFrontmatter(cfg, pkg, report)
	report.OK = report.CriticalCount() == 0
	return report, nil
}

func loadSkillPackage(cfg *config.Config, target string) (*sourcePackage, *VerifyReport, error) {
	resolved := resolveTarget(strings.TrimSpace(target))
	if resolved == "" {
		return nil, nil, fmt.Errorf("target is required")
	}
	report := &VerifyReport{Target: target, ResolvedTarget: resolved}

	if isHTTPURL(resolved) {
		report.SourceType = "url"
		if !cfg.Skills.ExternalInstalls {
			return nil, nil, fmt.Errorf("external skill installs are disabled by config")
		}
		if err := validatePolicyURL(cfg, resolved); err != nil {
			return nil, nil, err
		}
		b, finalURL, err := downloadSkillArchive(context.Background(), resolved, cfg)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(finalURL) != "" {
			resolved = finalURL
			report.ResolvedTarget = finalURL
		}
		files, stype, err := unpackArchive(resolved, b)
		if err != nil {
			return nil, nil, err
		}
		report.SourceType = stype
		return normalizePackage(&sourcePackage{
			SourceType:     stype,
			Target:         target,
			ResolvedTarget: resolved,
			Files:          files,
		}, cfg, report)
	}

	fi, err := os.Stat(resolved)
	if err != nil {
		return nil, nil, err
	}
	if fi.IsDir() {
		report.SourceType = "dir"
		files, err := readLocalDir(resolved)
		if err != nil {
			return nil, nil, err
		}
		return normalizePackage(&sourcePackage{
			SourceType:     "dir",
			Target:         target,
			ResolvedTarget: resolved,
			Files:          files,
		}, cfg, report)
	}

	report.SourceType = "archive"
	b, err := os.ReadFile(resolved)
	if err != nil {
		return nil, nil, err
	}
	files, stype, err := unpackArchive(resolved, b)
	if err != nil {
		return nil, nil, err
	}
	report.SourceType = stype
	return normalizePackage(&sourcePackage{
		SourceType:     stype,
		Target:         target,
		ResolvedTarget: resolved,
		Files:          files,
	}, cfg, report)
}

func normalizePackage(pkg *sourcePackage, cfg *config.Config, report *VerifyReport) (*sourcePackage, *VerifyReport, error) {
	if len(pkg.Files) == 0 {
		return nil, nil, errors.New("skill source is empty")
	}
	if len(pkg.Files) > maxSkillFiles {
		return nil, nil, fmt.Errorf("skill source contains too many files (%d > %d)", len(pkg.Files), maxSkillFiles)
	}

	prefix, err := detectSkillRootPrefix(pkg.Files)
	if err != nil {
		return nil, nil, err
	}
	trimmed := make([]sourceFile, 0, len(pkg.Files))
	for _, f := range pkg.Files {
		p := filepath.ToSlash(strings.TrimSpace(f.Path))
		if prefix != "" {
			if p == strings.TrimSuffix(prefix, "/") {
				continue
			}
			if !strings.HasPrefix(p, prefix) {
				continue
			}
			p = strings.TrimPrefix(p, prefix)
		}
		p = strings.TrimPrefix(p, "./")
		p = strings.TrimLeft(p, "/")
		if p == "" {
			continue
		}
		trimmed = append(trimmed, sourceFile{Path: p, Data: f.Data})
	}
	if len(trimmed) == 0 {
		return nil, nil, errors.New("skill source root did not contain files")
	}
	pkg.Files = trimmed

	skillData, ok := findFile(pkg.Files, "SKILL.md")
	if !ok {
		return nil, nil, errors.New("SKILL.md not found in skill source root")
	}
	fm, _ := parseSkillFrontmatter(skillData)
	name := fm.Name
	if name == "" && prefix != "" {
		name = strings.TrimSuffix(prefix, "/")
	}
	if name == "" {
		base := filepath.Base(pkg.ResolvedTarget)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		name = sanitizeSkillName(base)
	}
	if name == "" {
		return nil, nil, errors.New("could not determine skill name")
	}
	pkg.SkillName = name
	report.SkillName = name
	return pkg, report, nil
}

func scanPackage(cfg *config.Config, pkg *sourcePackage, report *VerifyReport) {
	linkCount := 0
	maxLinks := cfg.Skills.LinkPolicy.MaxLinksPerSkill
	if maxLinks <= 0 {
		maxLinks = 20
	}

	for _, f := range pkg.Files {
		if len(f.Data) > maxFileBytes {
			report.appendFinding(SeverityWarn, "large_file", fmt.Sprintf("Large file skipped after %d bytes", maxFileBytes), f.Path)
		}
		data := f.Data
		if len(data) > maxFileBytes {
			data = data[:maxFileBytes]
		}
		if !isTextContent(data) {
			continue
		}
		text := string(data)
		for _, p := range suspiciousPatterns {
			if p.re.MatchString(text) {
				report.appendFinding(p.severity, p.code, p.message, f.Path)
			}
		}

		matches := urlRegex.FindAllString(text, -1)
		for _, raw := range matches {
			linkCount++
			if linkCount > maxLinks {
				report.appendFinding(SeverityCritical, "max_links_exceeded", fmt.Sprintf("Skill contains too many links (%d > %d)", linkCount, maxLinks), f.Path)
				break
			}
			if err := validatePolicyURL(cfg, raw); err != nil {
				report.appendFinding(SeverityCritical, "link_policy_block", err.Error(), f.Path)
			}
		}
	}
	report.LinkCount = linkCount
}

func detectSkillRootPrefix(files []sourceFile) (string, error) {
	if _, ok := findFile(files, "SKILL.md"); ok {
		return "", nil
	}
	candidates := map[string]struct{}{}
	for _, f := range files {
		p := filepath.ToSlash(f.Path)
		if strings.HasSuffix(p, "/SKILL.md") {
			dir := strings.TrimSuffix(p, "SKILL.md")
			candidates[dir] = struct{}{}
		}
	}
	if len(candidates) == 1 {
		for k := range candidates {
			return k, nil
		}
	}
	return "", errors.New("unable to locate unique skill root containing SKILL.md")
}

func findFile(files []sourceFile, rel string) ([]byte, bool) {
	rel = filepath.ToSlash(rel)
	for _, f := range files {
		if filepath.ToSlash(f.Path) == rel {
			return f.Data, true
		}
	}
	return nil, false
}

func parseSkillName(skillMD []byte) string {
	fm, _ := parseSkillFrontmatter(skillMD)
	return fm.Name
}

func parseSkillFrontmatter(skillMD []byte) (skillFrontmatter, bool) {
	text := string(skillMD)
	if !strings.HasPrefix(text, "---\n") {
		return skillFrontmatter{}, false
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return skillFrontmatter{}, false
	}
	block := text[4 : 4+end]
	var out skillFrontmatter
	if nameMatch := frontmatterNameExpr.FindStringSubmatch(block); len(nameMatch) >= 2 {
		out.Name = sanitizeSkillName(nameMatch[1])
	}
	if descMatch := frontmatterDescExpr.FindStringSubmatch(block); len(descMatch) >= 2 {
		out.Description = strings.TrimSpace(descMatch[1])
		out.Description = strings.Trim(out.Description, `"'`)
	}
	return out, true
}

func sanitizeSkillName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, " ", "-")
	var out strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			out.WriteRune(r)
		}
	}
	name := strings.Trim(out.String(), "-_.")
	return name
}

func readLocalDir(root string) ([]sourceFile, error) {
	var files []sourceFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed in local skill source: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !isSafeRelativePath(rel) {
			return fmt.Errorf("unsafe path in skill source: %s", rel)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, sourceFile{Path: rel, Data: b})
		return nil
	})
	return files, err
}

func unpackArchive(name string, data []byte) ([]sourceFile, string, error) {
	if len(data) == 0 {
		return nil, "", errors.New("empty archive")
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		files, err := unpackZIP(data)
		return files, "zip", err
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		files, err := unpackTarGZ(data)
		return files, "tar.gz", err
	default:
		if files, err := unpackZIP(data); err == nil {
			return files, "zip", nil
		}
		if files, err := unpackTarGZ(data); err == nil {
			return files, "tar.gz", nil
		}
		return nil, "", errors.New("unsupported archive format (expected .zip or .tar.gz/.tgz)")
	}
}

func unpackZIP(data []byte) ([]sourceFile, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := make([]sourceFile, 0, len(r.File))
	var totalRead int64
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink entry not allowed: %s", f.Name)
		}
		if f.UncompressedSize64 > uint64(maxArchiveEntryBytes) {
			return nil, fmt.Errorf("archive entry exceeds max size (%d bytes): %s", maxArchiveEntryBytes, f.Name)
		}
		totalRead += int64(f.UncompressedSize64)
		if totalRead > maxArchiveTotalBytes {
			return nil, fmt.Errorf("archive extracted size exceeds max total (%d bytes)", maxArchiveTotalBytes)
		}
		name := filepath.ToSlash(strings.TrimPrefix(f.Name, "./"))
		if !isSafeRelativePath(name) {
			return nil, fmt.Errorf("unsafe archive path: %s", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxArchiveEntryBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if len(b) > maxArchiveEntryBytes {
			return nil, fmt.Errorf("archive entry exceeds max size while reading: %s", f.Name)
		}
		files = append(files, sourceFile{Path: name, Data: b})
	}
	return files, nil
}

func unpackTarGZ(data []byte) ([]sourceFile, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var files []sourceFile
	var totalRead int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeSymlink, tar.TypeLink:
			return nil, fmt.Errorf("link entry not allowed: %s", hdr.Name)
		case tar.TypeReg, tar.TypeRegA:
			if hdr.Size < 0 {
				return nil, fmt.Errorf("invalid tar entry size for %s", hdr.Name)
			}
			if hdr.Size > maxArchiveEntryBytes {
				return nil, fmt.Errorf("archive entry exceeds max size (%d bytes): %s", maxArchiveEntryBytes, hdr.Name)
			}
			totalRead += hdr.Size
			if totalRead > maxArchiveTotalBytes {
				return nil, fmt.Errorf("archive extracted size exceeds max total (%d bytes)", maxArchiveTotalBytes)
			}
			name := filepath.ToSlash(strings.TrimPrefix(hdr.Name, "./"))
			if !isSafeRelativePath(name) {
				return nil, fmt.Errorf("unsafe archive path: %s", hdr.Name)
			}
			b, err := io.ReadAll(io.LimitReader(tr, maxArchiveEntryBytes+1))
			if err != nil {
				return nil, err
			}
			if len(b) > maxArchiveEntryBytes {
				return nil, fmt.Errorf("archive entry exceeds max size while reading: %s", hdr.Name)
			}
			files = append(files, sourceFile{Path: name, Data: b})
		default:
			return nil, fmt.Errorf("unsupported tar entry type for %s", hdr.Name)
		}
	}
	return files, nil
}

func isSafeRelativePath(p string) bool {
	p = filepath.ToSlash(strings.TrimSpace(p))
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "/") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return false
	}
	return true
}

func isTextContent(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	return utf8.Valid(b)
}

func downloadSkillArchive(ctx context.Context, rawURL string, cfg *config.Config) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxDownloadRedirects {
				return fmt.Errorf("too many redirects while downloading skill archive")
			}
			if cfg != nil {
				if err := validatePolicyURL(cfg, req.URL.String()); err != nil {
					return fmt.Errorf("redirect blocked by link policy: %w", err)
				}
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download failed: %s", resp.Status)
	}
	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	if cfg != nil {
		if err := validatePolicyURL(cfg, finalURL); err != nil {
			return nil, "", fmt.Errorf("resolved URL blocked by link policy: %w", err)
		}
	}
	reader := io.LimitReader(resp.Body, maxRemoteSkillBytes+1)
	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > maxRemoteSkillBytes {
		return nil, "", fmt.Errorf("download exceeds max size (%d bytes)", maxRemoteSkillBytes)
	}
	return b, finalURL, nil
}

func validatePolicyURL(cfg *config.Config, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL: %q", raw)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("unsupported URL scheme: %s", scheme)
	}
	if scheme == "http" && !cfg.Skills.LinkPolicy.AllowHTTP {
		return fmt.Errorf("http URL is blocked by policy: %s", raw)
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return fmt.Errorf("URL missing host: %s", raw)
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Skills.LinkPolicy.Mode))
	allow := normalizeDomains(cfg.Skills.LinkPolicy.AllowDomains)
	deny := normalizeDomains(cfg.Skills.LinkPolicy.DenyDomains)
	switch mode {
	case "", "allowlist":
		if len(allow) == 0 {
			return nil
		}
		if !matchesAnyDomain(host, allow) {
			return fmt.Errorf("domain blocked by allowlist policy: %s", host)
		}
	case "denylist":
		if matchesAnyDomain(host, deny) {
			return fmt.Errorf("domain blocked by denylist policy: %s", host)
		}
	case "open":
		// no domain restrictions
	default:
		return fmt.Errorf("unknown link policy mode: %s", mode)
	}
	return nil
}

func normalizeDomains(in []string) []string {
	out := make([]string, 0, len(in))
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(d, ".")
		if d != "" {
			out = append(out, d)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func matchesAnyDomain(host string, domains []string) bool {
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func isHTTPURL(target string) bool {
	u, err := url.Parse(target)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")
}

func resolveTarget(target string) string {
	if target == "" {
		return ""
	}
	if isHTTPURL(target) {
		return target
	}
	if fi, err := os.Stat(target); err == nil {
		if abs, err := filepath.Abs(target); err == nil {
			return abs
		}
		if fi.Mode().IsRegular() || fi.IsDir() {
			return target
		}
	}
	// Slug fallback for clawhub catalog.
	if !strings.Contains(target, "/") && !strings.Contains(target, `\`) {
		return fmt.Sprintf("https://clawhub.ai/skills/%s.zip", target)
	}
	return target
}

func validateManifestAndFrontmatter(cfg *config.Config, pkg *sourcePackage, report *VerifyReport) {
	skillData, ok := findFile(pkg.Files, "SKILL.md")
	if !ok {
		report.appendFinding(SeverityCritical, "missing_skill_md", "SKILL.md missing from package root", "")
		return
	}
	fm, hasFM := parseSkillFrontmatter(skillData)
	if !hasFM {
		report.appendFinding(SeverityCritical, "missing_frontmatter", "SKILL.md must include YAML frontmatter", "SKILL.md")
	} else {
		if fm.Name == "" {
			report.appendFinding(SeverityCritical, "frontmatter_name_missing", "SKILL.md frontmatter missing `name`", "SKILL.md")
		}
		if fm.Description == "" {
			report.appendFinding(SeverityCritical, "frontmatter_description_missing", "SKILL.md frontmatter missing `description`", "SKILL.md")
		}
		if fm.Name != "" && report.SkillName != "" && fm.Name != report.SkillName {
			report.appendFinding(SeverityWarn, "frontmatter_name_mismatch", fmt.Sprintf("frontmatter name %q differs from resolved skill name %q", fm.Name, report.SkillName), "SKILL.md")
		}
	}

	policyData, hasPolicy := findFile(pkg.Files, "SKILL-POLICY.json")
	if !hasPolicy {
		report.appendFinding(SeverityWarn, "policy_manifest_missing", "SKILL-POLICY.json not found; using global policy defaults", "")
		return
	}
	var policy skillPolicyManifest
	if err := json.Unmarshal(policyData, &policy); err != nil {
		report.appendFinding(SeverityCritical, "policy_manifest_invalid_json", fmt.Sprintf("invalid SKILL-POLICY.json: %v", err), "SKILL-POLICY.json")
		return
	}
	if strings.TrimSpace(policy.Version) == "" {
		report.appendFinding(SeverityCritical, "policy_manifest_version_missing", "SKILL-POLICY.json requires non-empty `version`", "SKILL-POLICY.json")
	}
	for _, d := range append([]string{}, policy.LinkPolicy.AllowDomains...) {
		if strings.TrimSpace(d) == "" || strings.Contains(strings.TrimSpace(d), " ") {
			report.appendFinding(SeverityCritical, "policy_manifest_invalid_domain", fmt.Sprintf("invalid allow domain entry: %q", d), "SKILL-POLICY.json")
		}
	}
	for _, d := range append([]string{}, policy.LinkPolicy.DenyDomains...) {
		if strings.TrimSpace(d) == "" || strings.Contains(strings.TrimSpace(d), " ") {
			report.appendFinding(SeverityCritical, "policy_manifest_invalid_domain", fmt.Sprintf("invalid deny domain entry: %q", d), "SKILL-POLICY.json")
		}
	}
	if policy.LinkPolicy.AllowHTTP != nil && !cfg.Skills.LinkPolicy.AllowHTTP && *policy.LinkPolicy.AllowHTTP {
		report.appendFinding(SeverityWarn, "policy_manifest_allowhttp_overrides_global", "SKILL-POLICY allowHttp=true conflicts with stricter global policy", "SKILL-POLICY.json")
	}
	if policy.Execution.TimeoutSeconds < 0 {
		report.appendFinding(SeverityCritical, "policy_manifest_timeout_invalid", "execution.timeoutSeconds must be >= 0", "SKILL-POLICY.json")
	}
	if policy.Execution.MaxOutputBytes < 0 {
		report.appendFinding(SeverityCritical, "policy_manifest_output_limit_invalid", "execution.maxOutputBytes must be >= 0", "SKILL-POLICY.json")
	}
	for _, c := range append([]string{}, policy.Execution.AllowCommands...) {
		if strings.TrimSpace(c) == "" {
			report.appendFinding(SeverityCritical, "policy_manifest_command_invalid", "execution.allowCommands cannot include empty values", "SKILL-POLICY.json")
			break
		}
	}
	for _, c := range append([]string{}, policy.Execution.DenyCommands...) {
		if strings.TrimSpace(c) == "" {
			report.appendFinding(SeverityCritical, "policy_manifest_command_invalid", "execution.denyCommands cannot include empty values", "SKILL-POLICY.json")
			break
		}
	}
}
