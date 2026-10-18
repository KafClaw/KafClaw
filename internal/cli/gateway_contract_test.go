package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

func TestGatewayDashboardContracts(t *testing.T) {
	stubGatewaySignals(t)
	root := commandHome(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"offset":1,"status":"ok","uri":"lfs://test/memory"}`))
	}))
	defer proxy.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/embeddings":
			w.Write([]byte(`{"data":[{"embedding":[1,0]}]}`))
			return
		case "/user/access":
			w.Write([]byte(`{"ctx_id":"fixture"}`))
			return
		case "/memory/fixture":
			w.Write([]byte(`{"memories":[]}`))
			return
		case "/api/v1/nodes":
			w.Write([]byte(`[]`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"fixture answer"},"finish_reason":"stop"}],"usage":{"total_tokens":1}}`))
	}))
	defer api.Close()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	repo := cfg.Paths.WorkRepoPath
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	gitFixture := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %s %v", out, err)
		}
	}
	gitFixture("init", "-b", "main")
	gitFixture("config", "user.email", "fixture@example.invalid")
	gitFixture("config", "user.name", "Fixture")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("initial fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	gitFixture("add", "README.md")
	gitFixture("commit", "-m", "Initial fixture")
	gitFixture("branch", "test-branch")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nprintf 'fixture gh result\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cfg.Group.Enabled = true
	cfg.Group.GroupName = "contracts"
	cfg.Group.LFSProxyURL = proxy.URL
	cfg.Group.KafkaBrokers = "127.0.0.1:1"
	cfg.Group.AgentID = "self"
	cfg.Orchestrator.Enabled = true
	cfg.Observer.Enabled = true
	cfg.ER1.URL = api.URL
	cfg.ER1.SyncInterval = time.Hour
	cfg.KafGraph.URL = api.URL
	cfg.KafGraph.SyncInterval = time.Hour
	cfg.Memory.Embedding.Dimension = 2
	cfg.Providers.OpenAI.APIKey = "fixture"
	cfg.Providers.OpenAI.APIBase = api.URL
	cfg.Model.Name = "openai/test"
	cfg.Channels.Slack.InboundToken = "slack-fixture"
	cfg.Channels.MSTeams.InboundToken = "teams-fixture"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAFCLAW_GATEWAY_HOST", "127.0.0.1")
	t.Setenv("KAFCLAW_GATEWAY_PORT", freePort(t))
	t.Setenv("KAFCLAW_GATEWAY_DASHBOARD_PORT", freePort(t))
	done := make(chan struct{})
	go func() { defer close(done); runGateway(nil, nil) }()
	t.Cleanup(func() {
		sendGatewaySignal(t, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Error("gateway did not stop")
		}
	})
	base := "http://127.0.0.1:" + os.Getenv("KAFCLAW_GATEWAY_DASHBOARD_PORT")
	waitForHTTP(t, base+"/api/v1/status")
	client := &http.Client{Timeout: 3 * time.Second}
	call := func(method, path, body string, want int) []byte {
		t.Helper()
		r, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(r)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Errorf("%s %s: got %d want %d: %s", method, path, resp.StatusCode, want, data)
		}
		return data
	}
	// Every mutation must reject unsupported methods and answer CORS preflights.
	mutations := []string{"orchestrator/dispatch", "group/join", "group/leave", "group/tasks/submit", "group/skills/task", "group/onboard", "group/rejoin", "group/topics/ensure", "memory/reset", "memory/config", "memory/prune", "memory/embedding/install", "memory/embedding/reindex", "webusers/force", "webchat/send"}
	for _, path := range mutations {
		call("OPTIONS", "/api/v1/"+path, "", 200)
		call("PUT", "/api/v1/"+path, "", 405)
	}
	for _, path := range []string{"group/config", "workrepo", "webusers", "weblinks"} {
		call("OPTIONS", "/api/v1/"+path, "", 200)
		call("PUT", "/api/v1/"+path, "", 405)
	}
	for _, path := range []string{"orchestrator/dispatch", "group/join", "group/config", "group/tasks/submit", "group/memory", "group/skills", "group/skills/task", "group/rejoin", "group/topics/ensure", "settings", "memory/reset", "memory/config", "memory/embedding/reindex", "workrepo", "webusers", "webusers/force", "weblinks", "webchat/send"} {
		call("POST", "/api/v1/"+path, "invalid", 400)
	}
	for _, path := range []string{"group/join", "group/tasks/submit", "group/skills", "group/skills/task", "group/rejoin", "group/topics/ensure", "workrepo", "webusers/force", "weblinks", "webchat/send"} {
		call("POST", "/api/v1/"+path, "{}", 400)
	}
	for _, path := range []string{"orchestrator/status", "orchestrator/hierarchy", "orchestrator/zones", "orchestrator/agents", "group/config", "group/members", "group/topics", "group/topics/flow", "group/topics/health", "group/manifest", "group/stats", "group/audit", "group/skills", "group/topics/xp", "memory/status", "memory/metrics", "settings"} {
		data := call("GET", "/api/v1/"+path, "", 200)
		if !json.Valid(data) {
			t.Errorf("%s not JSON: %s", path, data)
		}
	}
	call("POST", "/api/v1/orchestrator/dispatch", `{"description":"test","target_zone":"missing"}`, 500)
	call("POST", "/api/v1/orchestrator/dispatch", `{"description":"test"}`, 200)
	call("POST", "/api/v1/group/tasks/submit", `{"description":"test task","content":"payload"}`, 200)
	if data := call("GET", "/api/v1/group/tasks?direction=outgoing&status=pending&limit=10", "", 200); !strings.Contains(string(data), "test task") {
		t.Fatal("submitted task not listed")
	}
	call("POST", "/api/v1/group/memory", `{"title":"shared","content":"a shared memory","tags":["test"]}`, 200)
	call("POST", "/api/v1/group/skills", `{"skill_name":"test"}`, 200)
	call("POST", "/api/v1/group/skills/task", `{"skill_name":"test","description":"skill task","content":"payload"}`, 200)
	call("POST", "/api/v1/group/onboard", `{}`, 500)
	call("POST", "/api/v1/group/rejoin", `{"agent_id":"unknown"}`, 404)
	call("POST", "/api/v1/group/config", `{"lfs_proxy_url":"`+proxy.URL+`","api_key":"fixture","kafka_brokers":"127.0.0.1:1","consumer_group":"test","agent_id":"self","poll_interval_ms":"100"}`, 200)
	call("POST", "/api/v1/memory/config", `{"enabled":true,"max_chunks":500}`, 200)
	call("POST", "/api/v1/settings", `{"key":"silent_mode","value":"true"}`, 200)
	// Seed persisted trace records and verify the dashboard renders each span type.
	tl, err := timeline.NewTimelineService(filepath.Join(root, ".kafclaw/timeline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	for i, classification := range []string{"INBOUND", "OUTBOUND", "LLM", "TOOL", "EVENT"} {
		meta, _ := json.Marshal(map[string]any{"duration_ms": 12, "response_text": strings.Repeat("answer", 50), "tool_name": "lookup"})
		if err := tl.AddEvent(&timeline.TimelineEvent{EventID: fmt.Sprintf("contract-%d", i), TraceID: "contract-trace", Timestamp: time.Now(), SenderID: "test", EventType: "TEXT", Classification: classification, ContentText: "fixture", Metadata: string(meta)}); err != nil {
			t.Fatal(err)
		}
	}
	data := call("GET", "/api/v1/trace/contract-trace", "", 200)
	for _, s := range []string{"INBOUND", "OUTBOUND", "LLM", "TOOL", "12ms"} {
		if !strings.Contains(string(data), s) {
			t.Errorf("missing %s in trace", s)
		}
	}
	call("GET", "/api/v1/policy-decisions?trace_id=contract-trace", "", 200)
	call("GET", "/api/v1/trace/", "", 400)
	call("GET", "/api/v1/trace-graph/", "", 400)
	call("GET", "/api/v1/trace-graph/contract-trace", "", 200)

	if _, err := tl.DB().Exec(`INSERT INTO working_memory(resource_id,thread_id,content,updated_at) VALUES ('fixture','','` + strings.Repeat("preview", 100) + `',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := tl.DB().Exec(`INSERT INTO observations(session_id,content,priority,observed_at,referenced_at) VALUES ('fixture','observation fixture','high',datetime('now'),datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if data := call("GET", "/api/v1/memory/status", "", 200); !strings.Contains(string(data), "observation fixture") || !strings.Contains(string(data), "preview") {
		t.Fatal("memory dashboard omitted persisted entries")
	}
	// Web-user linking is reversible and never sends to a real channel.
	var user timeline.WebUser
	if err := json.Unmarshal(call("POST", "/api/v1/webusers", `{"name":"Fixture User"}`, 200), &user); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprint(user.ID)
	call("GET", "/api/v1/webusers", "", 200)
	call("GET", "/api/v1/weblinks?web_user_id="+id, "", 200)
	call("POST", "/api/v1/webusers/force", `{"web_user_id":`+id+`,"force_send":false}`, 200)
	call("POST", "/api/v1/weblinks", `{"web_user_id":`+id+`,"whatsapp_jid":"12345"}`, 200)
	if data := call("GET", "/api/v1/weblinks?web_user_id="+id, "", 200); !strings.Contains(string(data), "12345") {
		t.Fatal("link not persisted")
	}
	call("POST", "/api/v1/webchat/send", `{"web_user_id":`+id+`,"message":"a fixture message"}`, 200)
	call("POST", "/api/v1/weblinks", `{"web_user_id":`+id+`}`, 200)
	call("POST", "/api/v1/webchat/send", `{"web_user_id":99999,"message":"unknown user"}`, 400)
	for _, layer := range []string{"soul", "conversation", "tool", "group", "er1", "observation", "working_memory", "all"} {
		call("POST", "/api/v1/memory/reset", `{"layer":"`+layer+`"}`, 200)
	}
	call("POST", "/api/v1/memory/prune", `{}`, 200)
	// Inbound authentication must precede channel processing.
	for _, channel := range []string{"slack", "msteams"} {
		path := "/api/v1/channels/" + channel + "/inbound"
		call("OPTIONS", path, "", 200)
		call("GET", path, "", 405)
		call("POST", path, "broken", 400)
		call("POST", path, `{"sender_id":"user","chat_id":"chat"}`, 401)
	}

	// Repository endpoints operate only on this test's throwaway Git repository.
	for _, path := range []string{"repo/tree", "repo/file?path=README.md", "repo/status", "repo/branches", "repo/log", "repo/diff", "repo/diff-file?path=README.md", "repo/gh-auth"} {
		call("GET", "/api/v1/"+path, "", 200)
	}
	call("POST", "/api/v1/repo/checkout", `{"branch":"test-branch"}`, 200)
	call("POST", "/api/v1/repo/checkout", `{"branch":"missing"}`, 500)
	for _, path := range []string{"repo/checkout", "repo/commit", "repo/init", "repo/pr"} {
		call("OPTIONS", "/api/v1/"+path, "", 200)
		call("POST", "/api/v1/"+path, "broken", 400)
	}
	call("POST", "/api/v1/repo/checkout", `{"branch":"--unsafe"}`, 400)
	call("POST", "/api/v1/repo/commit", `{}`, 400)
	call("POST", "/api/v1/repo/pr", `{}`, 400)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if data := call("GET", "/api/v1/repo/diff-file?path=README.md", "", 200); !strings.Contains(string(data), "changed fixture") {
		t.Fatal("file diff omitted change")
	}
	call("POST", "/api/v1/repo/commit", `{"message":"Update fixture"}`, 200)
	call("POST", "/api/v1/repo/pr", `{"title":"Fixture","body":"Fixture body","base":"main","head":"test-branch","draft":true}`, 200)
	call("POST", "/api/v1/repo/init", `{}`, 200)
	call("POST", "/api/v1/repo/pull", `{}`, 500)
	call("POST", "/api/v1/repo/push", `{}`, 500)
	call("GET", "/api/v1/repo/log?limit=invalid", "", 400)
	call("GET", "/api/v1/repo/diff-file?path=-unsafe", "", 400)
	call("GET", "/api/v1/repo/file?path=../outside", "", 400)
	if err := os.WriteFile(filepath.Join(repo, "binary"), []byte{255, 0}, 0600); err != nil {
		t.Fatal(err)
	}
	if data := call("GET", "/api/v1/repo/file?path=binary", "", 200); !strings.Contains(string(data), "binary file") {
		t.Fatal("binary content not marked")
	}
	if err := os.WriteFile(filepath.Join(repo, "large"), []byte(strings.Repeat("x", 200001)), 0600); err != nil {
		t.Fatal(err)
	}
	if data := call("GET", "/api/v1/repo/file?path=large", "", 200); !strings.Contains(string(data), "truncated") {
		t.Fatal("large content not truncated")
	}
	if err := tl.SetSetting("default_repo_search_path", root); err != nil {
		t.Fatal(err)
	}
	if data := call("GET", "/api/v1/repo/search", "", 200); !strings.Contains(string(data), "workspace") {
		t.Fatal("repo search omitted fixture")
	}
	call("POST", "/api/v1/group/leave", `{}`, 200)
	// Storage failures must become HTTP errors instead of successful empty results.
	for _, table := range []string{"timeline_events", "group_tasks", "group_traces", "group_memory_items", "group_skill_channels", "group_members", "group_membership_history", "web_users", "web_links", "approval_requests"} {
		if _, err := tl.DB().Exec("DROP TABLE IF EXISTS " + table); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"group/tasks", "group/traces", "group/memory", "group/skills", "group/members/previous", "group/membership/history", "webusers", "weblinks?web_user_id=1"} {
		call("GET", "/api/v1/"+path, "", 500)
	}

}
