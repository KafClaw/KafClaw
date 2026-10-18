package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/skills"
)

type toolTransport func(*http.Request) (*http.Response, error)

func (f toolTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQueryGraphContracts(t *testing.T) {
	mode := "ok"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == "status" {
			w.WriteHeader(503)
			return
		}
		if mode == "decode" {
			w.Write([]byte("invalid"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/edges") {
			w.Write([]byte(`[{"label":"TAGGED","toId":"tag"},{"label":"OTHER","toId":"category"}]`))
			return
		}
		switch r.URL.Query().Get("label") {
		case "MemoryItem":
			json.NewEncoder(w).Encode([]kgNode{{ID: "node", Properties: map[string]any{"memory_id": "id1", "title": "Runbook", "text": strings.Repeat("Kafka notes ", 60), "category": "ops"}}, {ID: "other", Properties: map[string]any{"memory_id": "id2", "text": "another memory"}}})
		case "Tag":
			w.Write([]byte(`[{"id":"tag","properties":{"name":"Kafka"}}]`))
		case "Category":
			w.Write([]byte(`[{"id":"category","properties":{"name":"operations"}}]`))
		default:
			w.Write([]byte(`[{"id":"ctx","properties":{"ctx_id":"context"}}]`))
		}
	}))
	defer server.Close()
	if NewQueryGraphTool(" ") != nil {
		t.Fatal("blank URL accepted")
	}
	tool := NewQueryGraphTool(server.URL + "/")
	if tool.Name() != "query_graph" || tool.Tier() != TierReadOnly || tool.Description() == "" || len(tool.Parameters()) == 0 {
		t.Fatal("invalid tool metadata")
	}
	for _, tc := range []struct {
		params map[string]any
		want   string
	}{{map[string]any{"memory_id": "id1"}, "tags: Kafka"}, {map[string]any{"memory_id": "missing"}, "No MemoryItem"}, {map[string]any{"contains": "KAFKA", "limit": 1}, "1 match(es)"}, {map[string]any{"contains": "missing"}, "No MemoryItem matched"}, {map[string]any{"limit": 0}, "2 match(es)"}} {
		got, err := tool.Execute(context.Background(), tc.params)
		if err != nil || !strings.Contains(got, tc.want) {
			t.Fatalf("%v: %s %v", tc.params, got, err)
		}
	}
	for _, m := range []string{"status", "decode"} {
		mode = m
		got, err := tool.Execute(context.Background(), nil)
		if err != nil || !strings.Contains(got, "Error querying graph") {
			t.Fatalf("query failure: %s %v", got, err)
		}
		if len(tool.idNameMap()) != 0 {
			t.Fatal("failed names should be skipped")
		}
	}
	server.Close()
	if _, err := tool.nodes("MemoryItem"); err == nil {
		t.Fatal("offline query accepted")
	}
	if propStr(map[string]any{"number": 1}, "number") != "" || kgTrunc("  short  ", 10) != "short" {
		t.Fatal("property or trim contract")
	}
}

func TestWorkspaceOAuthReadContracts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("KAFCLAW_HOME", root)
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(root, "config.json"))
	t.Setenv("KAFCLAW_OAUTH_MASTER_KEY", base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	cfg := config.DefaultConfig()
	cfg.Skills.Enabled = true
	cfg.Skills.Entries = map[string]config.SkillEntryConfig{"google-workspace": {Enabled: true}, "m365": {Enabled: true}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	dirs, err := skills.EnsureStateDirs()
	if err != nil {
		t.Fatal(err)
	}
	writeToken := func(id, scope string) {
		t.Helper()
		dir := filepath.Join(dirs.ToolsDir, "auth", id, "default")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(map[string]any{"access_token": "fixture-token", "scope": scope, "expires_at": "2100-01-01T00:00:00Z"})
		if err := os.WriteFile(filepath.Join(dir, "token.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	old := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = old })
	calls := 0
	http.DefaultTransport = toolTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Errorf("OAuth request contract: %s", r.Method)
		}
		if r.URL.Host != "gmail.googleapis.com" && r.URL.Host != "www.googleapis.com" && r.URL.Host != "graph.microsoft.com" {
			t.Errorf("unexpected API host: %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"items":[{"id":"fixture"}]}`))}, nil
	})
	google := NewGoogleWorkspaceReadTool()
	ms := NewM365ReadTool()
	if google.Description() == "" || ms.Description() == "" || google.Parameters() == nil || ms.Parameters() == nil {
		t.Fatal("missing metadata")
	}
	tests := []struct {
		id     string
		tool   Tool
		scopes string
		ops    []string
	}{{"google-workspace", google, "https://www.googleapis.com/auth/gmail.readonly https://www.googleapis.com/auth/calendar.readonly https://www.googleapis.com/auth/drive.readonly", []string{"gmail_list_messages", "calendar_list_events", "drive_list_files"}}, {"m365", ms, "Mail.Read Calendars.Read Files.Read", []string{"mail_list_messages", "calendar_list_events", "onedrive_list_children"}}}
	for _, tc := range tests {
		if got, err := tc.tool.Execute(context.Background(), nil); err != nil || !strings.Contains(got, "operation is required") {
			t.Fatalf("required op: %s %v", got, err)
		}
		if got, err := tc.tool.Execute(context.Background(), map[string]any{"operation": tc.ops[0]}); err != nil || !strings.Contains(got, "Error:") {
			t.Fatalf("missing token: %s %v", got, err)
		}
		writeToken(tc.id, "")
		for _, op := range tc.ops {
			got, err := tc.tool.Execute(context.Background(), map[string]any{"operation": op})
			if err != nil || !strings.Contains(got, "scope missing") {
				t.Fatalf("missing scope %s: %s %v", op, got, err)
			}
		}
		writeToken(tc.id, tc.scopes)
		for _, op := range tc.ops {
			for _, custom := range []bool{false, true} {
				params := map[string]any{"operation": op}
				if custom {
					params["profile"] = " "
					params["calendar_id"] = " "
					params["drive_item_id"] = "custom-id"
					params["query"] = "from:fixture"
					params["time_min"] = "2026-01-01T00:00:00Z"
					params["time_max"] = "2026-12-31T00:00:00Z"
					params["drive_query"] = " "
					params["select"] = " "
					params["max_results"] = 100
				}
				got, err := tc.tool.Execute(context.Background(), params)
				if err != nil || !strings.Contains(got, "fixture") {
					t.Fatalf("%s: %s %v", op, got, err)
				}
			}
		}
		if got, err := tc.tool.Execute(context.Background(), map[string]any{"operation": "unknown"}); err != nil || !strings.Contains(got, "unsupported") {
			t.Fatalf("unknown op: %s %v", got, err)
		}
	}
	if calls != 12 {
		t.Fatalf("API calls: %d", calls)
	}
	cfg.Skills.Enabled = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if got, _ := google.Execute(context.Background(), nil); !strings.Contains(got, "disabled") {
		t.Fatal("disabled skill executed")
	}
	if got, _ := ms.Execute(context.Background(), nil); !strings.Contains(got, "disabled") {
		t.Fatal("disabled skill executed")
	}
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureSkillEnabled("google-workspace"); err == nil {
		t.Fatal("invalid config accepted")
	}
}

func TestOAuthHTTPResponses(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, want string
	}{{200, `{"ok":true}`, `"ok": true`}, {200, "plain", "plain"}, {403, "denied", "Error: provider API status 403: denied"}} {
		client := &http.Client{Transport: toolTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}
		got, err := oauthGETJSON(context.Background(), client, "https://example.invalid", "test")
		if err != nil || !strings.Contains(got, tc.want) {
			t.Fatalf("response: %s %v", got, err)
		}
	}
	if _, err := oauthGETJSON(context.Background(), http.DefaultClient, ":invalid", "test"); err == nil {
		t.Fatal("invalid URL accepted")
	}
	client := &http.Client{Transport: toolTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
	if _, err := oauthGETJSON(context.Background(), client, "https://example.invalid", "test"); err == nil {
		t.Fatal("transport error hidden")
	}
	if !scopeHasAny("") || scopeHasAny("", "missing") {
		t.Fatal("empty scope contract")
	}
}
