package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBridgeHandlerValidationContracts(t *testing.T) {
	b := newTestBridge("http://127.0.0.1:1")
	for name, handler := range map[string]http.HandlerFunc{"slack-out": b.handleSlackOutbound, "teams-out": b.handleTeamsOutbound, "slack-users": b.handleSlackResolveUsers, "slack-channels": b.handleSlackResolveChannels, "teams-users": b.handleTeamsResolveUsers, "teams-channels": b.handleTeamsResolveChannels, "slack-events": b.handleSlackEvents, "slack-commands": b.handleSlackCommands, "slack-interactions": b.handleSlackInteractions, "teams-messages": b.handleTeamsMessages} {
		t.Run(name, func(t *testing.T) {
			for _, method := range []string{"GET", "PUT"} {
				w := httptest.NewRecorder()
				handler(w, httptest.NewRequest(method, "/", nil))
				if w.Code != 405 {
					t.Errorf("method %s: %d", method, w.Code)
				}
			}
			w := httptest.NewRecorder()
			handler(w, httptest.NewRequest("POST", "/", strings.NewReader("invalid")))
			if w.Code < 400 && name != "slack-commands" {
				t.Errorf("invalid body accepted: %d", w.Code)
			}
		})
	}
	for _, handler := range []http.HandlerFunc{b.handleSlackOutbound, b.handleTeamsOutbound} {
		for _, body := range []string{`{}`, `{"chat_id":"chat"}`} {
			w := httptest.NewRecorder()
			handler(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
			if w.Code != 400 {
				t.Errorf("missing required fields: %d", w.Code)
			}
		}
	}
	for _, handler := range []http.HandlerFunc{b.handleSlackResolveUsers, b.handleSlackResolveChannels, b.handleTeamsResolveUsers, b.handleTeamsResolveChannels} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"entries":["unknown"]}`)))
		if w.Code != 502 {
			t.Errorf("missing credentials: %d", w.Code)
		}
	}
	b.startSlackSocketMode()
	b.cfg.SlackAppToken = "fixture"
	b.startSlackSocketMode()
	if _, err := b.slackClientWithAppToken("fixture"); err == nil {
		t.Fatal("missing bot token accepted")
	}
	b.cfg.SlackBotToken = "fixture"
	if _, err := b.slackClientWithAppToken(""); err == nil {
		t.Fatal("missing app token accepted")
	}
	if _, err := b.slackClientWithAppToken("fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestSlackActionsAndOutboundContracts(t *testing.T) {
	mode := "ok"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if mode == "error" {
			w.Write([]byte(`{"ok":false,"error":"fixture_denied"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "conversations.open") {
			w.Write([]byte(`{"ok":true,"channel":{"id":"D123"}}`))
			return
		}
		w.Write([]byte(`{"ok":true,"channel":"C123","ts":"1.0","text":"updated","messages":[{"text":"fixture"}],"user":"U123","team":"Test","channels":[{"id":"C123","name":"general","is_member":true}],"response_metadata":{"next_cursor":""}}`))
	}))
	defer server.Close()
	b := newTestBridge(server.URL)
	b.cfg.SlackAPIBase = server.URL
	b.cfg.SlackBotToken = "fixture"
	for _, action := range []string{"react", "edit", "delete", "pin", "unpin", "read"} {
		params := map[string]any{"message_id": "1.0", "emoji": "thumbsup", "text": "updated", "limit": float64(1)}
		got, err := b.slackHandleAction(action, "C123", "1.0", "", params)
		if err != nil || got["ok"] != true {
			t.Fatalf("%s: %+v %v", action, got, err)
		}
		if action != "read" {
			if _, err := b.slackHandleAction(action, "C123", "", "", nil); err == nil {
				t.Fatalf("%s missing args accepted", action)
			}
		}
		mode = "error"
		if _, err := b.slackHandleAction(action, "C123", "1.0", "updated", params); err == nil {
			t.Fatalf("%s API error hidden", action)
		}
		mode = "ok"
	}
	if _, err := b.slackHandleAction("unknown", "C123", "", "", nil); err == nil {
		t.Fatal("unknown action accepted")
	}
	if got, err := b.resolveSlackChannelID("U123"); err != nil || got != "D123" {
		t.Fatalf("named channel: %q %v", got, err)
	}
	if _, err := b.resolveSlackChannelID(""); err == nil {
		t.Fatal("missing channel accepted")
	}
	for _, body := range []string{`{"chat_id":"C123","action":"react","action_params":{"message_id":"1.0","emoji":"thumbsup"}}`, `{"chat_id":"C123","card":{"blocks":[]},"content":"card"}`, `{"chat_id":"C123","content":"fixture","native_streaming":false,"stream_chunk_chars":10}`} {
		w := httptest.NewRecorder()
		b.handleSlackOutbound(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if w.Code != 200 {
			t.Fatalf("outbound: %d %s", w.Code, w.Body)
		}
	}
	mode = "error"
	for _, body := range []string{`{"chat_id":"C123","action":"react","action_params":{"message_id":"1.0","emoji":"thumbsup"}}`, `{"chat_id":"C123","card":{"blocks":[]},"content":"card"}`, `{"chat_id":"C123","content":"fixture","native_streaming":true}`, `{"chat_id":"U123","content":"fixture"}`} {
		w := httptest.NewRecorder()
		b.handleSlackOutbound(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if w.Code != 502 {
			t.Fatalf("outbound failure: %d %s", w.Code, w.Body)
		}
	}
	for _, handler := range []http.HandlerFunc{b.handleSlackResolveUsers, b.handleSlackResolveChannels, b.handleSlackProbe} {
		w := httptest.NewRecorder()
		method, body := "POST", `{"entries":["unknown"]}`
		if fmt.Sprintf("%p", handler) == fmt.Sprintf("%p", b.handleSlackProbe) {
			method = "GET"
		}
		handler(w, httptest.NewRequest(method, "/", strings.NewReader(body)))
		if w.Code < 400 {
			t.Fatal("API failure hidden")
		}
	}
}

func TestTeamsTokenAcquisitionContracts(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		wantErr bool
	}{{200, `{"access_token":"fixture-token","expires_in":3600}`, false}, {400, `{"error":"invalid_client"}`, true}, {400, `{}`, true}} {
		t.Run(fmt.Sprint(tc.status, tc.body), func(t *testing.T) {
			b := newTestBridge("")
			b.cfg.MSTeamsAppID = "fixture"
			b.cfg.MSTeamsAppPassword = "fixture"
			b.cfg.MSTeamsTenantID = "fixture"
			b.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if r.Form.Get("client_id") != "fixture" || r.Form.Get("grant_type") != "client_credentials" {
					t.Error("invalid token request")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			for _, graph := range []bool{false, true} {
				got, err := b.getTeamsTokenForScope("scope", graph)
				if (err != nil) != tc.wantErr {
					t.Fatalf("token: %q %v", got, err)
				}
				if !tc.wantErr && got != "fixture-token" {
					t.Fatal("incorrect token")
				}
			}
		})
	}
	b := newTestBridge("")
	if _, err := b.getTeamsAccessToken(); err == nil {
		t.Fatal("missing app credentials accepted")
	}
}

func TestTeamsGraphFailures(t *testing.T) {
	for _, mode := range []string{"status", "decode", "transport", "url"} {
		t.Run(mode, func(t *testing.T) {
			b := newTestBridge("")
			b.cfg.MSTeamsGraphBase = "https://example.invalid"
			if mode == "url" {
				b.cfg.MSTeamsGraphBase = ":invalid"
			}
			b.teamsGraphToken = tokenCache{accessToken: "fixture", expiresAt: time.Now().Add(time.Hour)}
			b.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if mode == "transport" {
					return nil, errors.New("offline")
				}
				status := 200
				if mode == "status" {
					status = 403
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("invalid"))}, nil
			})}
			if _, err := b.teamsGraphUsers(); err == nil {
				t.Fatal("users failure hidden")
			}
			if _, err := b.teamsGraphTeams(); err == nil {
				t.Fatal("teams failure hidden")
			}
			if _, err := b.teamsGraphTeamChannels("team"); err == nil {
				t.Fatal("channels failure hidden")
			}
			if mode != "decode" {
				if _, _, err := b.graphProbeGET("fixture", "/users"); err == nil {
					t.Fatal("graph probe failure hidden")
				}
			}
		})
	}
	if graphFirstValueID([]byte("broken")) != "" || graphFirstValueID(nil) != "" {
		t.Fatal("invalid graph values accepted")
	}
	if graphCapabilityResult(403, errors.New("forbidden"), "Mail.Read")["remediation_hint"] == nil {
		t.Fatal("missing remediation")
	}
}

func TestBridgeScalarContracts(t *testing.T) {
	for _, tc := range []struct {
		raw            string
		fallback, want bool
	}{{"", true, true}, {"yes", false, false}, {"true", false, true}, {"false", true, false}} {
		t.Setenv("BRIDGE_TEST_BOOL", tc.raw)
		if got := parseBoolDefault("BRIDGE_TEST_BOOL", tc.fallback); got != tc.want {
			t.Errorf("bool %q: %v", tc.raw, got)
		}
	}
	for _, v := range []any{int(2), int64(2), float64(2), json.Number("2"), "2", nil} {
		_ = intFromAny(v, 1)
		_ = normalizePollSelections(v)
	}
	for _, s := range []string{"", "slack:channel:C123", "slack:user:U123", "channel:C123", "user:U123", "#general"} {
		_ = normalizeSlackTarget(s)
	}
	for _, s := range []string{"user:id", "msteams:user:id", "conversation:id", "id"} {
		if normalizeTeamsTarget(s) != "id" {
			t.Fatal("teams target normalization")
		}
	}
}
