package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/group"
	"github.com/KafClaw/KafClaw/internal/orchestrator"
)

func TestGatewayHelperFunctions(t *testing.T) {
	if got := normalizeWhatsAppJID(""); got != "" {
		t.Fatalf("expected empty jid to stay empty, got %q", got)
	}
	if got := normalizeWhatsAppJID(" 12345 "); got != "12345@s.whatsapp.net" {
		t.Fatalf("unexpected normalized jid: %q", got)
	}
	if got := normalizeWhatsAppJID("a@s.whatsapp.net"); got != "a@s.whatsapp.net" {
		t.Fatalf("jid with domain should stay unchanged: %q", got)
	}

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git", "objects"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "dir", "nested"), 0755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "dir", "nested", "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	items, err := listRepoTree(repo, repo)
	if err != nil {
		t.Fatalf("listRepoTree: %v", err)
	}
	for _, it := range items {
		if strings.HasPrefix(it.Path, ".git") {
			t.Fatalf("expected .git to be skipped, got item: %+v", it)
		}
	}

	inside := filepath.Join(repo, "dir", "nested")
	outside := filepath.Join(filepath.Dir(repo), "outside")
	parent := filepath.Dir(repo)
	if !isWithin(repo, inside) {
		t.Fatal("expected path to be within repo")
	}
	if isWithin(repo, outside) {
		t.Fatal("expected outside path to be rejected")
	}
	if isWithin(repo, parent) {
		t.Fatal("expected direct parent path to be rejected")
	}

	if got := maskSecret("abcd1234"); got != "ab****34" {
		t.Fatalf("unexpected masked value: %q", got)
	}
	if got := maskSecret("abc"); got != "***" {
		t.Fatalf("unexpected masked short value: %q", got)
	}

	if inferTopicCategory("team.tasks.requests") != "tasks" {
		t.Fatal("expected tasks category")
	}
	if inferTopicCategory("team.observe.audit") != "observe" {
		t.Fatal("expected observe category")
	}
	if inferTopicCategory("team.control.onboarding") != "control" {
		t.Fatal("expected control category")
	}
	if inferTopicCategory("team.memory.shared") != "memory" {
		t.Fatal("expected memory category")
	}
	if inferTopicCategory("team.alpha.skill.math.execute") != "skill" {
		t.Fatal("expected skill category")
	}
	if inferTopicCategory("team.skill.sql.requests") != "tasks" {
		t.Fatal("expected tasks category for skill request topic naming")
	}
	if inferTopicCategory("team.random") != "control" {
		t.Fatal("expected control fallback")
	}
}

func TestRunGitAndRunGhRepoValidation(t *testing.T) {
	if _, err := runGit(""); err == nil {
		t.Fatal("expected runGit to reject empty repo")
	}
	if _, err := runGh(""); err == nil {
		t.Fatal("expected runGh to reject empty repo")
	}
}

func TestRunGitDisallowedSubcommand(t *testing.T) {
	repo := t.TempDir()
	_, err := runGit(repo, "rebase")
	if err == nil {
		t.Fatal("expected runGit to reject disallowed subcommand")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunGitNoArgs(t *testing.T) {
	repo := t.TempDir()
	_, err := runGit(repo)
	if err == nil {
		t.Fatal("expected runGit to reject empty args")
	}
}

func TestRunGitUnsafeArg(t *testing.T) {
	repo := t.TempDir()
	_, err := runGit(repo, "status", "$(rm -rf /)")
	if err == nil {
		t.Fatal("expected runGit to reject unsafe arg")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunGitGitNotFound(t *testing.T) {
	repo := t.TempDir()
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", t.TempDir()) // empty dir, no git binary
	_, err := runGit(repo, "status")
	if err == nil {
		t.Fatal("expected runGit to fail when git is not in PATH")
	}
	if !strings.Contains(err.Error(), "git not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunGitCmdFailure(t *testing.T) {
	repo := t.TempDir() // not a git repo, so git status will fail
	_, err := runGit(repo, "status")
	if err == nil {
		t.Fatal("expected runGit to fail on non-git directory")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunGitHappyPath(t *testing.T) {
	repo := t.TempDir()
	// Init a real git repo so the happy path works.
	initCmd := &exec.Cmd{Path: gitBinPath(t), Args: []string{"git", "init"}, Dir: repo}
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", out, err)
	}

	out, err := runGit(repo, "status", "-sb")
	if err != nil {
		t.Fatalf("expected runGit success, got: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output from git status")
	}
}

func gitBinPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found in PATH")
	}
	return p
}

func TestRunGhSuccessAndFailureWithFakeBinary(t *testing.T) {
	repo := t.TempDir()
	binDir := t.TempDir()
	ghPath := filepath.Join(binDir, "gh")
	script := "#!/usr/bin/env bash\nif [ \"$1\" = \"ok\" ]; then echo gh-ok; exit 0; fi\necho gh-fail >&2\nexit 7\n"
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	_ = os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)

	out, err := runGh(repo, "ok")
	if err != nil || !strings.Contains(out, "gh-ok") {
		t.Fatalf("expected runGh success, out=%q err=%v", out, err)
	}
	if _, err := runGh(repo, "nope"); err == nil {
		t.Fatal("expected runGh failure")
	}
}

func TestOrchDiscoveryHandlerAndGroupState(t *testing.T) {
	if h := orchDiscoveryHandler(nil); h != nil {
		t.Fatal("expected nil handler for nil orchestrator")
	}

	mgr := newActiveGroupManagerForGatewayTest(t)
	orch := orchestrator.New(config.OrchestratorConfig{
		Enabled: true,
		Role:    "orchestrator",
		ZoneID:  "public",
	}, mgr, nil)
	h := orchDiscoveryHandler(orch)
	if h == nil {
		t.Fatal("expected non-nil discovery handler")
	}

	// Marshal failure branch (func value in payload cannot be JSON marshaled)
	h(&group.GroupEnvelope{Payload: map[string]any{"bad": func() {}}})
	// Unmarshal failure branch (JSON string into struct)
	h(&group.GroupEnvelope{Payload: "not-an-object"})
	// Success branch
	h(&group.GroupEnvelope{Payload: map[string]any{
		"action": "discover",
		"node": map[string]any{
			"agent_id": "remote-1",
			"role":     "worker",
			"zone_id":  "public",
			"endpoint": "http://127.0.0.1",
			"status":   "active",
		},
	}})

	var gs groupState
	if gs.Consumer() != nil {
		t.Fatal("expected nil consumer initially")
	}
	gs.SetManager(mgr, nil)
	if gs.Manager() == nil {
		t.Fatal("expected manager set")
	}
	if !(&groupTraceAdapter{mgr: mgr}).Active() {
		t.Fatal("expected adapter active when manager is active")
	}
	gs.SetConsumer(dummyConsumer{})
	if gs.Consumer() == nil {
		t.Fatal("expected consumer set")
	}
	sameRoot := t.TempDir()
	if !isWithin(sameRoot, sameRoot) {
		t.Fatal("expected dot-rel branch to be true")
	}
	if isWithin("bad\x00root", "bad\x00path") {
		t.Fatal("expected invalid path to return false")
	}
	canceled := false
	gs.SetManager(mgr, func() { canceled = true })
	gs.Clear()
	if !canceled {
		t.Fatal("expected clear to invoke cancel")
	}
}

func TestGroupTraceAdapterPublishers(t *testing.T) {
	mgr := newActiveGroupManagerForGatewayTest(t)
	adapter := &groupTraceAdapter{mgr: mgr}

	if err := adapter.PublishTrace(context.Background(), group.TracePayload{
		TraceID:  "t-1",
		SpanType: "TOOL",
		Title:    "run",
		Content:  "ok",
	}); err != nil {
		t.Fatalf("publish trace with typed payload: %v", err)
	}

	if err := adapter.PublishTrace(context.Background(), map[string]string{
		"trace_id":  "t-2",
		"span_type": "TOOL",
		"title":     "mapped",
		"content":   "ok",
	}); err != nil {
		t.Fatalf("publish trace with map payload: %v", err)
	}

	if err := adapter.PublishTrace(context.Background(), 123); err == nil {
		t.Fatal("expected unsupported payload type error")
	}

	if err := adapter.PublishAudit(context.Background(), "evt", "trace-x", "details"); err != nil {
		t.Fatalf("publish audit: %v", err)
	}
}

type dummyConsumer struct{}

func (dummyConsumer) Start(context.Context) error            { return nil }
func (dummyConsumer) Messages() <-chan group.ConsumerMessage { return make(chan group.ConsumerMessage) }
func (dummyConsumer) Subscribe(string) error                 { return nil }
func (dummyConsumer) Close() error                           { return nil }

func newActiveGroupManagerForGatewayTest(t *testing.T) *group.Manager {
	t.Helper()
	var produced int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		produced++
		var env group.GroupEnvelope
		_ = json.NewDecoder(r.Body).Decode(&env)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(group.LFSEnvelope{KfsLFS: 1})
	}))
	t.Cleanup(srv.Close)

	mgr := group.NewManager(config.GroupConfig{
		Enabled:        true,
		GroupName:      "gateway-test",
		LFSProxyURL:    srv.URL,
		PollIntervalMs: 10,
	}, nil, group.AgentIdentity{
		AgentID:   "gateway-agent",
		AgentName: "GatewayAgent",
		Model:     "gpt-4o",
		Status:    "active",
	})
	if err := mgr.Join(context.Background()); err != nil {
		t.Fatalf("join group manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Leave(context.Background()) })

	if !mgr.Active() || produced == 0 {
		t.Fatalf("expected active manager after join, active=%v produced=%d", mgr.Active(), produced)
	}
	time.Sleep(10 * time.Millisecond)
	return mgr
}

func TestListRepoTreeErrorPath(t *testing.T) {
	// Non-existent root should return an error from WalkDir.
	if _, err := listRepoTree(filepath.Join(t.TempDir(), "missing"), t.TempDir()); err == nil {
		t.Fatal("expected listRepoTree error for missing root")
	}
}

func TestNewTraceIDFallback(t *testing.T) {
	orig := rand.Reader
	t.Cleanup(func() { rand.Reader = orig })
	rand.Reader = errReader{}
	id := newTraceID()
	if id == "" {
		t.Fatal("expected fallback trace id")
	}
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }
