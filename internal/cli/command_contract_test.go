package cli

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
	"github.com/KafClaw/KafClaw/internal/provider/credentials"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

func commandHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("KAFCLAW_HOME", root)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(root, ".kafclaw/config.json"))
	t.Setenv("KAFCLAW_OAUTH_TOMB_FILE", filepath.Join(root, "tomb.rr"))
	t.Setenv("KAFCLAW_OAUTH_KEY_BACKEND", "local")
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", "")
	cfg := config.DefaultConfig()
	cfg.Paths.Workspace = filepath.Join(root, "workspace")
	cfg.Paths.WorkRepoPath = cfg.Paths.Workspace
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return root
}

func captureCommandOutput(t *testing.T, fn func()) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old }()
	fn()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestModelsCommands(t *testing.T) {
	root := commandHome(t)
	login, key, value, base, days, asJSON := authLoginProvider, setKeyProvider, setKeyValue, setKeyBase, statsDays, statsJSON
	t.Cleanup(func() {
		authLoginProvider, setKeyProvider, setKeyValue, setKeyBase, statsDays, statsJSON = login, key, value, base, days, asJSON
	})
	for _, name := range []string{"gemini", "codex"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/sh\n[ \"$1\" = auth ] || exit 2\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", root)
	for _, id := range []string{"gemini", "gemini-cli", "openai-codex", "codex"} {
		authLoginProvider = id
		out := captureCommandOutput(t, func() { runModelsAuthLogin(nil, nil) })
		if !strings.Contains(out, "authentication complete") {
			t.Fatalf("login %s: %s", id, out)
		}
	}
	for _, id := range []string{"openai", "scalytics-copilot", "vllm"} {
		setKeyProvider = id
		setKeyValue = "test-fixture-key"
		setKeyBase = "https://example.invalid/v1"
		out := captureCommandOutput(t, func() { runModelsAuthSetKey(nil, nil) })
		if !strings.Contains(out, "stored successfully") {
			t.Fatal(out)
		}
		got, err := credentials.LoadAPIKey(id)
		if err != nil || got != setKeyValue {
			t.Fatalf("stored API key: %q %v", got, err)
		}
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers.VLLM.APIBase != setKeyBase || cfg.Providers.ScalyticsCopilot.APIBase != setKeyBase {
		t.Fatal("provider base not saved")
	}
	out := captureCommandOutput(t, func() { runModelsList(nil, nil) })
	if !strings.Contains(out, "Configured Providers:") || !strings.Contains(out, "Global model:") || !strings.Contains(out, "OAuth") {
		t.Fatal(out)
	}
	tl, err := loadGroupTimeline()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tl.DB().Exec(`INSERT INTO tasks (task_id,channel,chat_id,content_in,status,total_tokens,provider_id,cost_usd) VALUES ('stats-task','cli','test','test','completed',42,'openai',0.25)`); err != nil {
		t.Fatal(err)
	}
	tl.Close()
	tokens, reqs, limit := 12, 3, 100
	reset := time.Now().Add(time.Hour)
	provider.UpdateRateLimitCache("test", &provider.Usage{RemainingTokens: &tokens, RemainingRequests: &reqs, LimitTokens: &limit, ResetAt: &reset})
	for _, d := range []int{0, 7} {
		for _, j := range []bool{false, true} {
			statsDays = d
			statsJSON = j
			out := captureCommandOutput(t, func() { runModelsStats(nil, nil) })
			if !strings.Contains(out, "openai") {
				t.Fatalf("stats days=%d json=%v: %s", d, j, out)
			}
		}
	}
}

func TestWhatsAppApprovalCommands(t *testing.T) {
	commandHome(t)
	approve, deny, list := waApproveJID, waDenyJID, waListAuth
	t.Cleanup(func() { waApproveJID, waDenyJID, waListAuth = approve, deny, list })
	tl, err := loadGroupTimeline()
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	if err := tl.SetSetting("whatsapp_pending", "alice,bob, alice\r\ncarol"); err != nil {
		t.Fatal(err)
	}
	if getSetting(tl, "missing") != "" {
		t.Fatal("missing setting not empty")
	}
	if got := parseList("alice,bob, alice\r\ncarol"); !reflect.DeepEqual(got, []string{"alice", "bob", "carol"}) {
		t.Fatalf("normalized list: %v", got)
	}
	if parseList(" ") != nil {
		t.Fatal("empty list not nil")
	}
	for _, jid := range []string{"", "missing"} {
		if err := moveJID(tl, jid, "whatsapp_pending", "whatsapp_allowlist"); err == nil {
			t.Fatal("invalid JID accepted")
		}
	}
	waListAuth = true
	waApproveJID = ""
	waDenyJID = ""
	out := captureCommandOutput(t, func() { runWhatsAppAuth(nil, nil) })
	if !strings.Contains(out, "Pending:") {
		t.Fatal(out)
	}
	waListAuth = false
	out = captureCommandOutput(t, func() { runWhatsAppAuth(nil, nil) })
	if !strings.Contains(out, "Provide --approve") {
		t.Fatal(out)
	}
	waApproveJID = "alice"
	waDenyJID = "bob"
	out = captureCommandOutput(t, func() { runWhatsAppAuth(nil, nil) })
	if !strings.Contains(out, "Approved: alice") || !strings.Contains(out, "Denied: bob") {
		t.Fatal(out)
	}
	if getSetting(tl, "whatsapp_pending") != "carol" || getSetting(tl, "whatsapp_allowlist") != "alice" || getSetting(tl, "whatsapp_denylist") != "bob" {
		t.Fatal("approval transition not persisted")
	}
	waApproveJID = "missing"
	out = captureCommandOutput(t, func() { runWhatsAppAuth(nil, nil) })
	if !strings.Contains(out, "Approve error") {
		t.Fatal(out)
	}
	waApproveJID = ""
	waDenyJID = "missing"
	out = captureCommandOutput(t, func() { runWhatsAppAuth(nil, nil) })
	if !strings.Contains(out, "Deny error") {
		t.Fatal(out)
	}
	if err := tl.SetSetting("whatsapp_pending", "alice"); err != nil {
		t.Fatal(err)
	}
	if err := moveJID(tl, "alice", "whatsapp_pending", "whatsapp_allowlist"); err != nil {
		t.Fatal(err)
	}
	if getSetting(tl, "whatsapp_allowlist") != "alice" {
		t.Fatal("duplicate allowed JID")
	}
	tl.Close()
	if getSetting(tl, "key") != "" {
		t.Fatal("closed db setting not empty")
	}
}

func TestWhatsAppSetupInteractive(t *testing.T) {
	root := commandHome(t)
	input, err := os.CreateTemp(root, "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if _, err := input.WriteString("yes\ntest-pairing-token\nalice,bob\neve\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = old }()
	out := captureCommandOutput(t, func() { runWhatsAppSetup(nil, nil) })
	if !strings.Contains(out, "Config updated.") {
		t.Fatal(out)
	}
	cfg, err := config.Load()
	if err != nil || !cfg.Channels.WhatsApp.Enabled {
		t.Fatal("channel not enabled")
	}
	tl, err := loadGroupTimeline()
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	if getSetting(tl, "whatsapp_pair_token") != "test-pairing-token" || getSetting(tl, "whatsapp_allowlist") != "alice,bob" || getSetting(tl, "whatsapp_denylist") != "eve" {
		t.Fatal("setup settings not saved")
	}
	if readYesNo(bufio.NewReader(strings.NewReader("no\n"))) {
		t.Fatal("no accepted as yes")
	}
}

func TestGroupInspectionCommands(t *testing.T) {
	commandHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"offset":1}`))
	}))
	defer server.Close()
	for _, fn := range []func(){func() { runGroupStatus(nil, nil) }, func() { runGroupMembers(nil, nil) }, func() { runGroupLeave(nil, nil) }} {
		out := captureCommandOutput(t, fn)
		if !strings.Contains(out, "Not in a group") {
			t.Fatal(out)
		}
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	tl, err := loadGroupTimeline()
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	if err := tl.SetSetting("group_name", "test"); err != nil {
		t.Fatal(err)
	}
	if err := tl.SetSetting("kafscale_lfs_proxy_url", server.URL); err != nil {
		t.Fatal(err)
	}
	mgr := buildGroupManager(cfg, tl)
	if mgr.AgentID() == "" {
		t.Fatal("fallback identity empty")
	}
	out := captureCommandOutput(t, func() { runGroupStatus(nil, nil) })
	if !strings.Contains(out, "Group:       test") {
		t.Fatal(out)
	}
	out = captureCommandOutput(t, func() { runGroupMembers(nil, nil) })
	if !strings.Contains(out, "No members") {
		t.Fatal(out)
	}
	if err := tl.UpsertGroupMember(&timeline.GroupMemberRecord{AgentID: "peer", AgentName: "Peer", Model: "test", Capabilities: `["a-very-long-capability-name-that-is-truncated-at-forty-characters"]`, Status: "active", LastSeen: time.Now()}); err != nil {
		t.Fatal(err)
	}
	out = captureCommandOutput(t, func() { runGroupMembers(nil, nil) })
	if !strings.Contains(out, "peer") || !strings.Contains(out, "...") {
		t.Fatal(out)
	}
	joinOut := captureCommandOutput(t, func() { runGroupJoin(nil, []string{"test"}) })
	if !strings.Contains(joinOut, "Joined group: test") {
		t.Fatal(joinOut)
	}
	out = captureCommandOutput(t, func() { runGroupLeave(nil, nil) })
	if !strings.Contains(out, "Left group: test") {
		t.Fatal(out)
	}
	if getSetting(tl, "group_active") != "false" {
		t.Fatal("leave not persisted")
	}
}
