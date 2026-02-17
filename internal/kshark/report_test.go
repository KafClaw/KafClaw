package kshark

import (
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

func TestReportHelpersAndWriters(t *testing.T) {
	t.Chdir(t.TempDir())

	r := &Report{StartedAt: time.Now(), FinishedAt: time.Now()}
	addRow(r, Row{Layer: L7, Status: OK})
	addRow(r, Row{Layer: L7, Status: FAIL})
	summarize(r)
	if !r.HasFailed || r.Summary[string(L7)].FAIL != 1 {
		t.Fatalf("unexpected summary: %+v", r.Summary)
	}
	if truncate("abcdef", 4) != "abc..." {
		t.Fatalf("truncate mismatch: %q", truncate("abcdef", 4))
	}

	if _, err := createSafeReportPath("", "reports"); err == nil {
		t.Fatal("expected empty path error")
	}
	if _, err := createSafeReportPath("..", "reports"); err == nil {
		t.Fatal("expected invalid filename error")
	}

	jsonPath, err := WriteJSON("scan.json", r)
	if err != nil {
		t.Fatalf("write json: %v", err)
	}
	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("expected json report written: %v", err)
	}

	tplPath := filepath.Join(t.TempDir(), "report.html.tmpl")
	tpl := `<html><body>{{len .Report.Rows}}</body></html>`
	if err := os.WriteFile(tplPath, []byte(tpl), 0644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	htmlPath, err := WriteHTMLReport(r, tplPath)
	if err != nil {
		t.Fatalf("write html: %v", err)
	}
	if _, err := os.Stat(htmlPath); err != nil {
		t.Fatalf("expected html report written: %v", err)
	}

	sha, err := fileSHA256(jsonPath)
	if err != nil || sha == "" {
		t.Fatalf("sha256: %q %v", sha, err)
	}
	md5sum, err := fileMD5(jsonPath)
	if err != nil || md5sum == "" {
		t.Fatalf("md5: %q %v", md5sum, err)
	}

	if _, _, err := WriteReportMD5(""); err == nil {
		t.Fatal("expected empty md5 path error")
	}
	md5Path, hash, err := WriteReportMD5(jsonPath)
	if err != nil || hash == "" {
		t.Fatalf("write md5: path=%q hash=%q err=%v", md5Path, hash, err)
	}
	b, _ := os.ReadFile(md5Path)
	if !strings.Contains(string(b), hash) {
		t.Fatalf("expected md5 hash in file, got: %s", string(b))
	}
}

func TestKsharkUtilityHelpers(t *testing.T) {
	if firstHost("host1:9092,host2:9092") != "host1" {
		t.Fatal("unexpected first host")
	}
	if !containsAny("abc TOPIC_AUTHORIZATION_FAILED", "authorization") {
		t.Fatal("containsAny should match case-insensitively")
	}
	if !isTimeout(contextDeadlineErr{}) {
		t.Fatal("expected timeout true")
	}
	if _, err := parseStartOffset("bad"); err == nil {
		t.Fatal("expected invalid start offset error")
	}
	if off, _ := parseStartOffset("latest"); off != kafka.LastOffset {
		t.Fatalf("unexpected latest offset: %d", off)
	}
	if !isValidHostname("broker-1.example.com") || isValidHostname("bad;rm -rf") {
		t.Fatal("hostname validation mismatch")
	}
	if trimLines("a\nb\nc", 2) != "a\nb\n..." {
		t.Fatalf("unexpected trim lines: %q", trimLines("a\nb\nc", 2))
	}
	if extractHost("https://127.0.0.1:8081/subjects") != "127.0.0.1" {
		t.Fatal("unexpected extracted host")
	}
	if len(ParseTopics("a, b,,c")) != 3 {
		t.Fatal("expected 3 parsed topics")
	}
	if policyHint("Produce", kafka.TopicAuthorizationFailed) == "" {
		t.Fatal("expected policy hint for kafka error")
	}
	if hint(kafka.SASLAuthenticationFailed) == "" {
		t.Fatal("expected hint for kafka auth error")
	}
	if _, ok := kafkaErrorCode(kafka.NotLeaderForPartition); !ok {
		t.Fatal("expected kafka error code extraction")
	}
	if _, ok := kafkaErrorCode(errors.New("nope")); ok {
		t.Fatal("expected non-kafka error")
	}

	r := &Report{}
	if wrapTLS(r, nil, nil, "kafka", "addr") != nil {
		t.Fatal("expected nil base conn to remain nil in plaintext branch")
	}

	hc := httpClientFromTLS(&tls.Config{}, time.Second)
	if hc == nil || hc.Timeout != time.Second {
		t.Fatalf("unexpected http client: %+v", hc)
	}
}

type contextDeadlineErr struct{}

func (contextDeadlineErr) Error() string   { return "deadline exceeded" }
func (contextDeadlineErr) Timeout() bool   { return true }
func (contextDeadlineErr) Temporary() bool { return true }
