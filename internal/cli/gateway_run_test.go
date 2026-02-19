package cli

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

var (
	gatewayTestSignalMu sync.Mutex
	gatewayTestSignalCh chan<- os.Signal
)

func stubGatewaySignals(t *testing.T) {
	t.Helper()
	origNotify := gatewaySignalNotify
	origStop := gatewaySignalStop
	gatewaySignalNotify = func(c chan<- os.Signal, _ ...os.Signal) {
		gatewayTestSignalMu.Lock()
		gatewayTestSignalCh = c
		gatewayTestSignalMu.Unlock()
	}
	gatewaySignalStop = func(c chan<- os.Signal) {
		gatewayTestSignalMu.Lock()
		if gatewayTestSignalCh == c {
			gatewayTestSignalCh = nil
		}
		gatewayTestSignalMu.Unlock()
	}
	t.Cleanup(func() {
		gatewaySignalNotify = origNotify
		gatewaySignalStop = origStop
		gatewayTestSignalMu.Lock()
		gatewayTestSignalCh = nil
		gatewayTestSignalMu.Unlock()
	})
}

func sendGatewaySignal(t *testing.T, sig os.Signal) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gatewayTestSignalMu.Lock()
		ch := gatewayTestSignalCh
		gatewayTestSignalMu.Unlock()
		if ch != nil {
			ch <- sig
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for gateway signal channel")
}

func TestRunGatewayBootAndShutdown(t *testing.T) {
	stubGatewaySignals(t)
	// Isolate all HOME-based writes used by config/timeline/gateway startup.
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origHost := os.Getenv("KAFCLAW_GATEWAY_HOST")
	origPort := os.Getenv("KAFCLAW_GATEWAY_PORT")
	origDash := os.Getenv("KAFCLAW_GATEWAY_DASHBOARD_PORT")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("KAFCLAW_GATEWAY_HOST", origHost)
		_ = os.Setenv("KAFCLAW_GATEWAY_PORT", origPort)
		_ = os.Setenv("KAFCLAW_GATEWAY_DASHBOARD_PORT", origDash)
	})
	_ = os.Setenv("HOME", tmpHome)
	_ = os.Setenv("KAFCLAW_GATEWAY_HOST", "127.0.0.1")
	_ = os.Setenv("KAFCLAW_GATEWAY_PORT", "0")
	_ = os.Setenv("KAFCLAW_GATEWAY_DASHBOARD_PORT", "0")

	if err := os.MkdirAll(filepath.Join(tmpHome, ".kafclaw"), 0755); err != nil {
		t.Fatalf("mkdir home .kafclaw: %v", err)
	}

	done := make(chan struct{})
	go func() {
		runGateway(nil, nil)
		close(done)
	}()

	// Let startup progress through initialization, handlers, and server boot.
	time.Sleep(800 * time.Millisecond)
	sendGatewaySignal(t, syscall.SIGTERM)

	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("gateway did not shut down after SIGTERM")
	}
}

func TestRunGatewayServesDashboardEndpoints(t *testing.T) {
	stubGatewaySignals(t)
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origHost := os.Getenv("KAFCLAW_GATEWAY_HOST")
	origPort := os.Getenv("KAFCLAW_GATEWAY_PORT")
	origDash := os.Getenv("KAFCLAW_GATEWAY_DASHBOARD_PORT")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("KAFCLAW_GATEWAY_HOST", origHost)
		_ = os.Setenv("KAFCLAW_GATEWAY_PORT", origPort)
		_ = os.Setenv("KAFCLAW_GATEWAY_DASHBOARD_PORT", origDash)
	})
	_ = os.Setenv("HOME", tmpHome)
	_ = os.Setenv("KAFCLAW_GATEWAY_HOST", "127.0.0.1")
	_ = os.Setenv("KAFCLAW_GATEWAY_PORT", freePort(t))
	_ = os.Setenv("KAFCLAW_GATEWAY_DASHBOARD_PORT", freePort(t))

	if err := os.MkdirAll(filepath.Join(tmpHome, ".kafclaw"), 0755); err != nil {
		t.Fatalf("mkdir home .kafclaw: %v", err)
	}

	done := make(chan struct{})
	go func() {
		runGateway(nil, nil)
		close(done)
	}()

	dashBase := "http://127.0.0.1:" + os.Getenv("KAFCLAW_GATEWAY_DASHBOARD_PORT")
	waitForHTTP(t, dashBase+"/api/v1/status")

	client := &http.Client{Timeout: 2 * time.Second}
	getStatus := func(path string) int {
		t.Helper()
		resp, err := client.Get(dashBase + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	call := func(method, path, body string) {
		t.Helper()
		var reqBody *bytes.Buffer
		if body == "" {
			reqBody = bytes.NewBuffer(nil)
		} else {
			reqBody = bytes.NewBufferString(body)
		}
		req, err := http.NewRequest(method, dashBase+path, reqBody)
		if err != nil {
			t.Fatalf("new request %s %s: %v", method, path, err)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
	}

	if got := getStatus("/"); got != http.StatusOK {
		t.Fatalf("expected / status 200, got %d", got)
	}
	if got := getStatus("/timeline"); got != http.StatusOK {
		t.Fatalf("expected /timeline status 200, got %d", got)
	}
	if got := getStatus("/approvals"); got != http.StatusOK {
		t.Fatalf("expected /approvals status 200, got %d", got)
	}

	call(http.MethodGet, "/api/v1/status", "")
	call(http.MethodPost, "/api/v1/auth/verify", "{}")
	call(http.MethodGet, "/api/v1/orchestrator/status", "")
	call(http.MethodGet, "/api/v1/orchestrator/hierarchy", "")
	call(http.MethodGet, "/api/v1/orchestrator/zones", "")
	call(http.MethodGet, "/api/v1/orchestrator/agents", "")
	call(http.MethodPost, "/api/v1/orchestrator/dispatch", `{"description":"x","target_zone":"z"}`)
	call(http.MethodGet, "/api/v1/timeline?limit=10", "")
	call(http.MethodGet, "/api/v1/trace/trace-x", "")
	call(http.MethodGet, "/api/v1/policy-decisions", "")
	call(http.MethodGet, "/api/v1/trace-graph/trace-x", "")
	call(http.MethodGet, "/api/v1/group/status", "")
	call(http.MethodGet, "/api/v1/group/members", "")
	call(http.MethodPost, "/api/v1/group/join", `{"group_name":"g1","kafka_brokers":"127.0.0.1:9092","consumer_group":"cg","agent_id":"a1","lfs_proxy_url":"http://127.0.0.1:8080"}`)
	call(http.MethodPost, "/api/v1/group/leave", "{}")
	call(http.MethodPost, "/api/v1/group/config", `{"group_name":"g2"}`)
	call(http.MethodPost, "/api/v1/group/tasks/submit", `{"description":"do x","target_agent":"a2"}`)
	call(http.MethodGet, "/api/v1/group/tasks", "")
	call(http.MethodGet, "/api/v1/group/traces", "")
	call(http.MethodGet, "/api/v1/group/memory", "")
	call(http.MethodPost, "/api/v1/group/memory", `{"item_id":"m1","author_id":"a1","title":"n","content_type":"text/plain","tags":"[]","metadata":"{}"}`)
	call(http.MethodGet, "/api/v1/group/skills", "")
	call(http.MethodPost, "/api/v1/group/skills", `{"skill_name":"sql","group_name":"g1","requests_topic":"g1.skill.sql.req","responses_topic":"g1.skill.sql.resp","registered_by":"a1"}`)
	call(http.MethodPost, "/api/v1/group/skills/task", `{"skill_name":"sql","group_name":"g1","title":"run","content":"go","requester_id":"a1"}`)
	call(http.MethodPost, "/api/v1/group/onboard", `{"agent_id":"a2","group_name":"g1","action":"invite"}`)
	call(http.MethodGet, "/api/v1/group/membership/history", "")
	call(http.MethodGet, "/api/v1/group/members/previous", "")
	call(http.MethodPost, "/api/v1/group/rejoin", "{}")
	call(http.MethodGet, "/api/v1/group/stats", "")
	call(http.MethodGet, "/api/v1/group/audit", "")
	call(http.MethodGet, "/api/v1/group/manifest", "")
	call(http.MethodGet, "/api/v1/group/topics", "")
	call(http.MethodGet, "/api/v1/group/topics/flow", "")
	call(http.MethodGet, "/api/v1/group/topics/health", "")
	call(http.MethodPost, "/api/v1/group/topics/ensure", `{"group_name":"g1"}`)
	call(http.MethodGet, "/api/v1/group/topics/xp", "")
	call(http.MethodGet, "/api/v1/group/topics/density?topic=team.tasks&hours=6", "")
	call(http.MethodGet, "/api/v1/settings", "")
	call(http.MethodPost, "/api/v1/settings", `{"key":"silent_mode","value":"false"}`)
	call(http.MethodGet, "/api/v1/memory/status", "")
	call(http.MethodPost, "/api/v1/memory/reset", "{}")
	call(http.MethodGet, "/api/v1/memory/config", "")
	call(http.MethodPost, "/api/v1/memory/config", `{"enabled":true}`)
	call(http.MethodPost, "/api/v1/memory/prune", `{"days":1}`)
	call(http.MethodGet, "/api/v1/workrepo", "")
	call(http.MethodPost, "/api/v1/workrepo", `{"path":"`+tmpHome+`"}`)
	call(http.MethodGet, "/api/v1/repo/tree", "")
	call(http.MethodGet, "/api/v1/repo/file?path=README.md", "")
	call(http.MethodGet, "/api/v1/repo/status", "")
	call(http.MethodGet, "/api/v1/repo/search?q=kaf", "")
	call(http.MethodGet, "/api/v1/repo/gh-auth", "")
	call(http.MethodGet, "/api/v1/repo/branches", "")
	call(http.MethodPost, "/api/v1/repo/checkout", `{"branch":"main"}`)
	call(http.MethodGet, "/api/v1/repo/log", "")
	call(http.MethodGet, "/api/v1/repo/diff-file?path=README.md", "")
	call(http.MethodGet, "/api/v1/repo/diff", "")
	call(http.MethodPost, "/api/v1/repo/commit", `{"message":"test"}`)
	call(http.MethodPost, "/api/v1/repo/pull", "{}")
	call(http.MethodPost, "/api/v1/repo/push", "{}")
	call(http.MethodPost, "/api/v1/repo/init", `{"path":"`+tmpHome+`/repo-init"}`)
	call(http.MethodPost, "/api/v1/repo/pr", `{"title":"t","body":"b","head":"h","base":"main"}`)
	call(http.MethodGet, "/api/v1/webusers", "")
	call(http.MethodPost, "/api/v1/webusers", `{"name":"tester"}`)
	call(http.MethodPost, "/api/v1/webusers/force", `{"id":1,"force_send":true}`)
	call(http.MethodGet, "/api/v1/weblinks", "")
	call(http.MethodPost, "/api/v1/weblinks", `{"web_user_id":1,"whatsapp_jid":"1@s.whatsapp.net"}`)
	call(http.MethodPost, "/api/v1/webchat/send", `{"content":"hello","web_user_id":1}`)
	call(http.MethodGet, "/api/v1/tasks", "")
	call(http.MethodGet, "/api/v1/tasks/nope", "")
	call(http.MethodGet, "/api/v1/approvals/pending", "")
	call(http.MethodPost, "/api/v1/approvals/nope", `{"status":"approved"}`)
	call(http.MethodGet, "/timeline", "")
	call(http.MethodGet, "/group", "")
	call(http.MethodGet, "/approvals", "")
	call(http.MethodGet, "/", "")

	sendGatewaySignal(t, syscall.SIGTERM)

	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("gateway did not shut down after endpoint exercise")
	}
}

func TestRunGatewayOrchestratorModeBranches(t *testing.T) {
	stubGatewaySignals(t)
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origHost := os.Getenv("KAFCLAW_GATEWAY_HOST")
	origPort := os.Getenv("KAFCLAW_GATEWAY_PORT")
	origDash := os.Getenv("KAFCLAW_GATEWAY_DASHBOARD_PORT")
	origAuth := os.Getenv("KAFCLAW_GATEWAY_AUTH_TOKEN")
	origGroupEnabled := os.Getenv("KAFCLAW_GROUP_ENABLED")
	origGroupName := os.Getenv("KAFCLAW_GROUP_GROUP_NAME")
	origOrchEnabled := os.Getenv("KAFCLAW_ORCHESTRATOR_ENABLED")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("KAFCLAW_GATEWAY_HOST", origHost)
		_ = os.Setenv("KAFCLAW_GATEWAY_PORT", origPort)
		_ = os.Setenv("KAFCLAW_GATEWAY_DASHBOARD_PORT", origDash)
		_ = os.Setenv("KAFCLAW_GATEWAY_AUTH_TOKEN", origAuth)
		_ = os.Setenv("KAFCLAW_GROUP_ENABLED", origGroupEnabled)
		_ = os.Setenv("KAFCLAW_GROUP_GROUP_NAME", origGroupName)
		_ = os.Setenv("KAFCLAW_ORCHESTRATOR_ENABLED", origOrchEnabled)
	})

	_ = os.Setenv("HOME", tmpHome)
	_ = os.Setenv("KAFCLAW_GATEWAY_HOST", "127.0.0.1")
	_ = os.Setenv("KAFCLAW_GATEWAY_PORT", freePort(t))
	_ = os.Setenv("KAFCLAW_GATEWAY_DASHBOARD_PORT", freePort(t))
	_ = os.Setenv("KAFCLAW_GATEWAY_AUTH_TOKEN", "token123")
	_ = os.Setenv("KAFCLAW_GROUP_ENABLED", "true")
	_ = os.Setenv("KAFCLAW_GROUP_GROUP_NAME", "g-orch")
	_ = os.Setenv("KAFCLAW_ORCHESTRATOR_ENABLED", "true")

	if err := os.MkdirAll(filepath.Join(tmpHome, ".kafclaw"), 0755); err != nil {
		t.Fatalf("mkdir home .kafclaw: %v", err)
	}

	done := make(chan struct{})
	go func() {
		runGateway(nil, nil)
		close(done)
	}()

	dashBase := "http://127.0.0.1:" + os.Getenv("KAFCLAW_GATEWAY_DASHBOARD_PORT")
	apiBase := "http://127.0.0.1:" + os.Getenv("KAFCLAW_GATEWAY_PORT")
	waitForHTTP(t, dashBase+"/api/v1/status")
	waitForHTTP(t, apiBase+"/chat?message=hello&session=test")

	client := &http.Client{Timeout: 3 * time.Second}
	do := func(method, url, authToken, body string) {
		t.Helper()
		req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("new request %s %s: %v", method, url, err)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if authToken != "" {
			req.Header.Set("Authorization", "Bearer "+authToken)
		}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
	}

	do(http.MethodPost, dashBase+"/api/v1/auth/verify", "", "{}")
	do(http.MethodPost, dashBase+"/api/v1/auth/verify", "token123", "{}")
	do(http.MethodOptions, dashBase+"/api/v1/orchestrator/dispatch", "", "")
	do(http.MethodPost, dashBase+"/api/v1/orchestrator/dispatch", "", `{"description":"test","target_zone":"public"}`)
	do(http.MethodGet, dashBase+"/api/v1/orchestrator/status", "", "")
	do(http.MethodGet, dashBase+"/api/v1/orchestrator/hierarchy", "", "")
	do(http.MethodGet, dashBase+"/api/v1/orchestrator/zones", "", "")
	do(http.MethodGet, dashBase+"/api/v1/orchestrator/agents", "", "")

	do(http.MethodPost, apiBase+"/chat?message=hello+there&session=s1", "", "")

	sendGatewaySignal(t, syscall.SIGTERM)

	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("gateway did not shut down in orchestrator branch test")
	}
}

func TestRunGatewayChatAuthEnforcedWhenTokenConfigured(t *testing.T) {
	stubGatewaySignals(t)
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	origHost := os.Getenv("KAFCLAW_GATEWAY_HOST")
	origPort := os.Getenv("KAFCLAW_GATEWAY_PORT")
	origDash := os.Getenv("KAFCLAW_GATEWAY_DASHBOARD_PORT")
	origAuth := os.Getenv("KAFCLAW_GATEWAY_AUTH_TOKEN")
	t.Cleanup(func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("KAFCLAW_GATEWAY_HOST", origHost)
		_ = os.Setenv("KAFCLAW_GATEWAY_PORT", origPort)
		_ = os.Setenv("KAFCLAW_GATEWAY_DASHBOARD_PORT", origDash)
		_ = os.Setenv("KAFCLAW_GATEWAY_AUTH_TOKEN", origAuth)
	})

	_ = os.Setenv("HOME", tmpHome)
	_ = os.Setenv("KAFCLAW_GATEWAY_HOST", "127.0.0.1")
	_ = os.Setenv("KAFCLAW_GATEWAY_PORT", freePort(t))
	_ = os.Setenv("KAFCLAW_GATEWAY_DASHBOARD_PORT", freePort(t))
	_ = os.Setenv("KAFCLAW_GATEWAY_AUTH_TOKEN", "token123")

	if err := os.MkdirAll(filepath.Join(tmpHome, ".kafclaw"), 0755); err != nil {
		t.Fatalf("mkdir home .kafclaw: %v", err)
	}

	done := make(chan struct{})
	go func() {
		runGateway(nil, nil)
		close(done)
	}()

	dashBase := "http://127.0.0.1:" + os.Getenv("KAFCLAW_GATEWAY_DASHBOARD_PORT")
	apiBase := "http://127.0.0.1:" + os.Getenv("KAFCLAW_GATEWAY_PORT")
	waitForHTTP(t, dashBase+"/api/v1/status")
	waitForHTTP(t, apiBase+"/chat")

	client := &http.Client{Timeout: 3 * time.Second}
	doChat := func(token string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, apiBase+"/chat?message=hello&session=s1", bytes.NewBuffer(nil))
		if err != nil {
			t.Fatalf("new /chat request: %v", err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("do /chat request: %v", err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if code := doChat(""); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", code)
	}
	if code := doChat("wrong"); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", code)
	}
	if code := doChat("token123"); code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 with correct token, got %d", code)
	}

	sendGatewaySignal(t, syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("gateway did not shut down after auth test")
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port %q: %v", addr, err)
	}
	return port
}

func waitForHTTP(t *testing.T, url string) {
	t.Helper()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}
