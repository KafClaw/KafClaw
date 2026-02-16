package kshark

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ---------- Formatting Constants ----------

const (
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorReset  = "\033[0m"
	IconOK      = "\u2705"
	IconWarn    = "\u26a0\ufe0f"
	IconFail    = "\u274c"
	IconSkip    = "\u26aa"
)

// ---------- Models ----------

// CheckStatus represents the result of a diagnostic check.
type CheckStatus string

const (
	OK   CheckStatus = "OK"
	WARN CheckStatus = "WARN"
	FAIL CheckStatus = "FAIL"
	SKIP CheckStatus = "SKIP"
)

// Layer represents the network/protocol layer being checked.
type Layer string

const (
	L3   Layer = "L3-Network"
	L4   Layer = "L4-TCP"
	L56  Layer = "L5-6-TLS"
	L7   Layer = "L7-Kafka"
	HTTP Layer = "L7-HTTP"
	DIAG Layer = "Diag"
)

// Row is a single diagnostic check result.
type Row struct {
	Component string      `json:"component"`
	Target    string      `json:"target"`
	Layer     Layer       `json:"layer"`
	Status    CheckStatus `json:"status"`
	Detail    string      `json:"detail"`
	Hint      string      `json:"hint,omitempty"`
}

// Report collects all diagnostic results.
type Report struct {
	Rows       []Row                 `json:"rows"`
	Summary    map[string]CheckStats `json:"summary"`
	StartedAt  time.Time             `json:"started_at"`
	FinishedAt time.Time             `json:"finished_at"`
	ConfigEcho map[string]string     `json:"config_echo,omitempty"`
	HasFailed  bool                  `json:"-"`
}

// CheckStats counts results per layer.
type CheckStats struct {
	OK   int `json:"ok"`
	WARN int `json:"warn"`
	FAIL int `json:"fail"`
	SKIP int `json:"skip"`
}

// ---------- Report methods ----------

func addRow(r *Report, row Row) {
	if row.Status == FAIL {
		r.HasFailed = true
	}
	r.Rows = append(r.Rows, row)
	logf("row component=%s target=%s layer=%s status=%s detail=%q hint=%q",
		row.Component, row.Target, row.Layer, row.Status, row.Detail, row.Hint)
}

func summarize(r *Report) {
	r.Summary = map[string]CheckStats{}
	for _, row := range r.Rows {
		cs := r.Summary[string(row.Layer)]
		switch row.Status {
		case OK:
			cs.OK++
		case WARN:
			cs.WARN++
		case FAIL:
			cs.FAIL++
		case SKIP:
			cs.SKIP++
		}
		r.Summary[string(row.Layer)] = cs
	}
}

// PrintPretty prints the report to stdout with ANSI colors.
func PrintPretty(r *Report) {
	fmt.Printf("\nKafka Wire Health Report  (%s -> %s)  on %s\n",
		r.StartedAt.Format(time.RFC3339), r.FinishedAt.Format(time.RFC3339), runtime.GOOS)
	fmt.Println(strings.Repeat("-", 92))
	fmt.Printf("%-4s %-14s %-26s %-12s %s\n", " ", "Component", "Target", "Layer", "Detail")
	fmt.Println(strings.Repeat("-", 92))

	for _, row := range r.Rows {
		var color, icon string
		switch row.Status {
		case OK:
			color = ColorGreen
			icon = IconOK
		case WARN:
			color = ColorYellow
			icon = IconWarn
		case FAIL:
			color = ColorRed
			icon = IconFail
		case SKIP:
			color = ""
			icon = IconSkip
		}

		fmt.Printf("%s%-4s %-14s %-26s %-12s %s%s\n",
			color, icon, row.Component, truncate(row.Target, 26), row.Layer, row.Detail, ColorReset)

		if row.Hint != "" && row.Status != OK {
			fmt.Printf("  %s-> Hint: %s%s\n", ColorYellow, row.Hint, ColorReset)
		}
	}
	fmt.Println(strings.Repeat("-", 92))
	for layer, s := range r.Summary {
		fmt.Printf("%-12s  %sOK:%d%s  %sWARN:%d%s  %sFAIL:%d%s  SKIP:%d\n",
			layer,
			ColorGreen, s.OK, ColorReset,
			ColorYellow, s.WARN, ColorReset,
			ColorRed, s.FAIL, ColorReset,
			s.SKIP)
	}
	fmt.Println()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "..."
}

// ---------- JSON export ----------

func createSafeReportPath(userInputPath string, safeSubDir string) (string, error) {
	if userInputPath == "" {
		return "", errors.New("output path cannot be empty")
	}
	cleanFilename := filepath.Base(userInputPath)
	if cleanFilename == "." || cleanFilename == "/" || cleanFilename == ".." {
		return "", fmt.Errorf("invalid filename provided: %s", userInputPath)
	}
	if err := os.MkdirAll(safeSubDir, 0755); err != nil {
		return "", fmt.Errorf("could not create reports directory '%s': %w", safeSubDir, err)
	}
	safePath := filepath.Join(safeSubDir, cleanFilename)
	return safePath, nil
}

// WriteJSON writes the report as JSON to the given path under a "reports" subdirectory.
func WriteJSON(path string, r *Report) (string, error) {
	if path == "" {
		return "", nil
	}
	safePath, err := createSafeReportPath(path, "reports")
	if err != nil {
		return "", fmt.Errorf("invalid output path: %w", err)
	}
	f, err := os.Create(safePath)
	if err != nil {
		return safePath, err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return safePath, enc.Encode(r)
}

// ---------- HTML report ----------

// WriteHTMLReport renders the report as an HTML file using the embedded template.
func WriteHTMLReport(r *Report, templatePath string) (string, error) {
	funcMap := template.FuncMap{
		"ToLower": func(s CheckStatus) string {
			return strings.ToLower(string(s))
		},
		"Icon": func(status CheckStatus) string {
			switch status {
			case OK:
				return IconOK
			case WARN:
				return IconWarn
			case FAIL:
				return IconFail
			case SKIP:
				return IconSkip
			default:
				return ""
			}
		},
	}

	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(funcMap).ParseFiles(templatePath)
	if err != nil {
		return "", fmt.Errorf("could not parse html template: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	hostname = strings.Split(hostname, ".")[0]
	timestamp := time.Now().Format("20060102_150405")
	reportDir := "reports"
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return "", fmt.Errorf("could not create reports directory: %w", err)
	}

	reportPath := filepath.Join(reportDir, fmt.Sprintf("kshark_report_%s_%s.html", hostname, timestamp))
	file, err := os.Create(reportPath)
	if err != nil {
		return "", fmt.Errorf("could not create html report file: %w", err)
	}
	defer file.Close()

	type TemplateData struct {
		Report *Report
	}
	if err := tmpl.Execute(file, TemplateData{Report: r}); err != nil {
		return "", fmt.Errorf("could not execute html template: %w", err)
	}
	return reportPath, nil
}

// ---------- Hash utilities ----------

func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func fileMD5(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:]), nil
}

// WriteReportMD5 writes an MD5 checksum file for the given report path.
func WriteReportMD5(path string) (string, string, error) {
	if path == "" {
		return "", "", errors.New("empty report path")
	}
	hash, err := fileMD5(path)
	if err != nil {
		return "", "", err
	}
	timestamp := time.Now().Format("20060102_150405")
	dir := filepath.Dir(path)
	md5Path := filepath.Join(dir, fmt.Sprintf("report_md5_%s.txt", timestamp))
	content := fmt.Sprintf("%s\t%s\t%s\n", timestamp, path, hash)
	if err := os.WriteFile(md5Path, []byte(content), 0644); err != nil {
		return "", "", err
	}
	return md5Path, hash, nil
}
