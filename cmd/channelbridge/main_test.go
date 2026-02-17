package main

import (
	"bytes"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

func newTestBridge(baseURL string) *bridge {
	return &bridge{
		cfg: config{
			KafclawBase: baseURL,
		},
		client:            &http.Client{Timeout: 2 * time.Second},
		teamsConvByID:     map[string]teamsConversationRef{},
		teamsConvByUserID: map[string]teamsConversationRef{},
		inboundSeen:       map[string]time.Time{},
		inboundTTL:        10 * time.Minute,
		teamsPolls:        map[string]map[string]any{},
		metrics: bridgeMetrics{
			StartedAt: time.Now().UTC(),
		},
	}
}

func TestSlackEventsDedupesByEventID(t *testing.T) {
	var forwards int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/slack/inbound" {
			atomic.AddInt32(&forwards, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)

	payload := map[string]any{
		"type":     "event_callback",
		"event_id": "Ev123",
		"event": map[string]any{
			"type":         "message",
			"channel":      "C123",
			"user":         "U123",
			"text":         "hello",
			"channel_type": "channel",
			"ts":           "1700000.001",
		},
	}
	body, _ := json.Marshal(payload)

	req1 := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w1 := httptest.NewRecorder()
	b.handleSlackEvents(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status=%d", w1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w2 := httptest.NewRecorder()
	b.handleSlackEvents(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second request status=%d", w2.Code)
	}

	if got := atomic.LoadInt32(&forwards); got != 1 {
		t.Fatalf("expected 1 forward, got %d", got)
	}
}

func TestSlackEventsAppMentionForwards(t *testing.T) {
	var got map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/slack/inbound" {
			defer r.Body.Close()
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	payload := map[string]any{
		"type":     "event_callback",
		"event_id": "EvMention",
		"event": map[string]any{
			"type":      "app_mention",
			"channel":   "C555",
			"user":      "U999",
			"text":      "<@Ubot> hello",
			"thread_ts": "171.100",
			"ts":        "171.101",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got == nil || strings.TrimSpace(asString(got["chat_id"])) != "C555" || strings.TrimSpace(asString(got["sender_id"])) != "U999" {
		t.Fatalf("unexpected forwarded payload: %#v", got)
	}
	if was, _ := got["was_mentioned"].(bool); !was {
		t.Fatalf("expected was_mentioned=true, got %#v", got["was_mentioned"])
	}
}

func TestSlackEventsMessageChangedForwardsNormalized(t *testing.T) {
	var got map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/slack/inbound" {
			defer r.Body.Close()
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	b.cfg.SlackBotUserID = "Ubot"
	payload := map[string]any{
		"type":     "event_callback",
		"event_id": "EvChanged",
		"event": map[string]any{
			"type":         "message",
			"subtype":      "message_changed",
			"channel":      "C123",
			"channel_type": "channel",
			"message": map[string]any{
				"user":      "U123",
				"text":      "hi <@Ubot>",
				"thread_ts": "171.200",
				"ts":        "171.201",
			},
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got == nil {
		t.Fatal("expected forwarded payload")
	}
	if strings.TrimSpace(asString(got["chat_id"])) != "C123" || strings.TrimSpace(asString(got["sender_id"])) != "U123" {
		t.Fatalf("unexpected forwarded payload: %#v", got)
	}
	if strings.TrimSpace(asString(got["message_id"])) != "171.201" || strings.TrimSpace(asString(got["thread_id"])) != "171.200" {
		t.Fatalf("unexpected message/thread ids: %#v", got)
	}
	if was, _ := got["was_mentioned"].(bool); !was {
		t.Fatalf("expected mention detection, got %#v", got["was_mentioned"])
	}
}

func TestSlackEventsFileShareSubtypeForwardsFallbackText(t *testing.T) {
	var got map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/slack/inbound" {
			defer r.Body.Close()
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	payload := map[string]any{
		"type":     "event_callback",
		"event_id": "EvFile",
		"event": map[string]any{
			"type":         "message",
			"subtype":      "file_share",
			"channel":      "C777",
			"channel_type": "channel",
			"user":         "U777",
			"ts":           "171.300",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got == nil {
		t.Fatal("expected forwarded payload")
	}
	if strings.TrimSpace(asString(got["text"])) != "[file shared]" {
		t.Fatalf("expected fallback text, got %#v", got["text"])
	}
}

func TestSlackEventsMessageDeletedForwardsTombstone(t *testing.T) {
	var got map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/slack/inbound" {
			defer r.Body.Close()
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	payload := map[string]any{
		"type":     "event_callback",
		"event_id": "EvDelete",
		"event": map[string]any{
			"type":         "message",
			"subtype":      "message_deleted",
			"channel":      "C888",
			"channel_type": "channel",
			"previous_message": map[string]any{
				"user": "U888",
			},
			"user":       "U888",
			"deleted_ts": "171.400",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got == nil {
		t.Fatal("expected forwarded payload")
	}
	if strings.TrimSpace(asString(got["text"])) != "[message deleted]" {
		t.Fatalf("expected delete tombstone text, got %#v", got["text"])
	}
	if strings.TrimSpace(asString(got["message_id"])) != "171.400" {
		t.Fatalf("unexpected message id: %#v", got["message_id"])
	}
}

func TestSlackEventsBotMessagesAreIgnored(t *testing.T) {
	var forwards int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/slack/inbound" {
			atomic.AddInt32(&forwards, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	payload := map[string]any{
		"type":     "event_callback",
		"event_id": "EvBot",
		"event": map[string]any{
			"type":    "message",
			"subtype": "bot_message",
			"channel": "C999",
			"user":    "U999",
			"text":    "ignore me",
			"ts":      "171.500",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleSlackEvents(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := atomic.LoadInt32(&forwards); got != 0 {
		t.Fatalf("expected no forwards for bot message, got %d", got)
	}
}

func TestSlackCommandsForwardInbound(t *testing.T) {
	var got map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/slack/inbound" {
			defer r.Body.Close()
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	form := url.Values{}
	form.Set("channel_id", "C111")
	form.Set("user_id", "U111")
	form.Set("command", "/ask")
	form.Set("text", "status")
	form.Set("trigger_id", "trig-1")
	req := httptest.NewRequest(http.MethodPost, "/slack/commands", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	b.handleSlackCommands(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got == nil {
		t.Fatal("expected forwarded inbound payload")
	}
	if strings.TrimSpace(asString(got["chat_id"])) != "C111" || strings.TrimSpace(asString(got["sender_id"])) != "U111" {
		t.Fatalf("unexpected forwarded payload: %#v", got)
	}
	if strings.TrimSpace(asString(got["text"])) != "/ask status" {
		t.Fatalf("unexpected text: %#v", got["text"])
	}
}

func TestSlackInteractionsForwardInbound(t *testing.T) {
	var got map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/slack/inbound" {
			defer r.Body.Close()
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	payload, _ := json.Marshal(map[string]any{
		"type": "block_actions",
		"user": map[string]any{"id": "U22"},
		"channel": map[string]any{
			"id": "C22",
		},
		"container": map[string]any{
			"thread_ts": "171.222",
		},
		"action_ts": "171.223",
		"actions": []map[string]any{
			{"action_id": "approve", "value": "yes"},
		},
	})
	form := url.Values{}
	form.Set("payload", string(payload))
	req := httptest.NewRequest(http.MethodPost, "/slack/interactions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	b.handleSlackInteractions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got == nil {
		t.Fatal("expected forwarded inbound payload")
	}
	if strings.TrimSpace(asString(got["chat_id"])) != "C22" || strings.TrimSpace(asString(got["sender_id"])) != "U22" {
		t.Fatalf("unexpected forwarded payload: %#v", got)
	}
	if strings.TrimSpace(asString(got["thread_id"])) != "171.222" {
		t.Fatalf("unexpected thread_id: %#v", got["thread_id"])
	}
}

func TestTeamsInboundRequiresBearerWhenConfigured(t *testing.T) {
	var forwards int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/msteams/inbound" {
			atomic.AddInt32(&forwards, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	b.cfg.KafclawMSTeamsInboundToken = "bridge-token"
	b.cfg.MSTeamsInboundBearer = "secret"

	payload := map[string]any{
		"type":      "message",
		"id":        "activity-1",
		"text":      "hello",
		"from":      map[string]any{"id": "user-1"},
		"recipient": map[string]any{"id": "bot-1"},
		"conversation": map[string]any{
			"id":               "conv-1",
			"conversationType": "personal",
		},
		"serviceUrl": "https://smba.trafficmanager.net/emea",
	}
	body, _ := json.Marshal(payload)

	reqNoAuth := httptest.NewRequest(http.MethodPost, "/teams/messages", bytes.NewReader(body))
	wNoAuth := httptest.NewRecorder()
	b.handleTeamsMessages(wNoAuth, reqNoAuth)
	if wNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", wNoAuth.Code)
	}

	reqAuth := httptest.NewRequest(http.MethodPost, "/teams/messages", bytes.NewReader(body))
	reqAuth.Header.Set("Authorization", "Bearer secret")
	wAuth := httptest.NewRecorder()
	b.handleTeamsMessages(wAuth, reqAuth)
	if wAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 with auth, got %d", wAuth.Code)
	}

	if got := atomic.LoadInt32(&forwards); got != 1 {
		t.Fatalf("expected 1 forward, got %d", got)
	}
}

func TestTeamsInboundDedupesByMessageID(t *testing.T) {
	var forwards int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/msteams/inbound" {
			atomic.AddInt32(&forwards, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)

	payload := map[string]any{
		"type":      "message",
		"id":        "activity-2",
		"text":      "hello",
		"from":      map[string]any{"id": "user-1"},
		"recipient": map[string]any{"id": "bot-1"},
		"conversation": map[string]any{
			"id":               "conv-1",
			"conversationType": "personal",
		},
		"serviceUrl": "https://smba.trafficmanager.net/emea",
	}
	body, _ := json.Marshal(payload)

	req1 := httptest.NewRequest(http.MethodPost, "/teams/messages", bytes.NewReader(body))
	w1 := httptest.NewRecorder()
	b.handleTeamsMessages(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request status=%d", w1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/teams/messages", bytes.NewReader(body))
	w2 := httptest.NewRecorder()
	b.handleTeamsMessages(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second request status=%d", w2.Code)
	}

	if got := atomic.LoadInt32(&forwards); got != 1 {
		t.Fatalf("expected 1 forward, got %d", got)
	}
}

func TestTeamsInboundJWTValidation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	kid := "kid-1"
	issuer := "https://api.botframework.com"
	appID := "app-123"

	var openid *httptest.Server
	openid = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":   issuer,
				"jwks_uri": openid.URL + "/keys",
			})
		case "/keys":
			n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
			e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())
			_ = json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]any{
					{"kid": kid, "kty": "RSA", "n": n, "e": e},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer openid.Close()

	var forwards int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/msteams/inbound" {
			atomic.AddInt32(&forwards, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	b.cfg.MSTeamsOpenIDConfig = openid.URL + "/.well-known/openid"
	b.cfg.MSTeamsAppID = appID
	b.jwt = newTeamsJWTVerifier(b.client, b.cfg.MSTeamsOpenIDConfig, b.cfg.MSTeamsAppID)

	goodJWT := buildTestJWT(t, key, kid, map[string]any{
		"iss":        issuer,
		"aud":        appID,
		"serviceurl": "https://smba.trafficmanager.net/emea",
		"exp":        time.Now().Add(5 * time.Minute).Unix(),
		"nbf":        time.Now().Add(-1 * time.Minute).Unix(),
	})
	badJWT := buildTestJWT(t, key, kid, map[string]any{
		"iss": issuer,
		"aud": "wrong-app",
		"exp": time.Now().Add(5 * time.Minute).Unix(),
		"nbf": time.Now().Add(-1 * time.Minute).Unix(),
	})

	payload := map[string]any{
		"type":      "message",
		"id":        "activity-3",
		"text":      "hello",
		"from":      map[string]any{"id": "user-1"},
		"recipient": map[string]any{"id": "bot-1"},
		"conversation": map[string]any{
			"id":               "conv-1",
			"conversationType": "personal",
		},
		"serviceUrl": "https://smba.trafficmanager.net/emea",
	}
	body, _ := json.Marshal(payload)

	reqBad := httptest.NewRequest(http.MethodPost, "/teams/messages", bytes.NewReader(body))
	reqBad.Header.Set("Authorization", "Bearer "+badJWT)
	wBad := httptest.NewRecorder()
	b.handleTeamsMessages(wBad, reqBad)
	if wBad.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad jwt, got %d", wBad.Code)
	}

	reqGood := httptest.NewRequest(http.MethodPost, "/teams/messages", bytes.NewReader(body))
	reqGood.Header.Set("Authorization", "Bearer "+goodJWT)
	wGood := httptest.NewRecorder()
	b.handleTeamsMessages(wGood, reqGood)
	if wGood.Code != http.StatusOK {
		t.Fatalf("expected 200 for good jwt, got %d", wGood.Code)
	}
	if got := atomic.LoadInt32(&forwards); got != 1 {
		t.Fatalf("expected 1 forward, got %d", got)
	}

	badServiceJWT := buildTestJWT(t, key, kid, map[string]any{
		"iss":        issuer,
		"aud":        appID,
		"serviceurl": "https://evil.example.com",
		"exp":        time.Now().Add(5 * time.Minute).Unix(),
		"nbf":        time.Now().Add(-1 * time.Minute).Unix(),
	})
	reqSvc := httptest.NewRequest(http.MethodPost, "/teams/messages", bytes.NewReader(body))
	reqSvc.Header.Set("Authorization", "Bearer "+badServiceJWT)
	wSvc := httptest.NewRecorder()
	b.handleTeamsMessages(wSvc, reqSvc)
	if wSvc.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for serviceurl mismatch jwt, got %d", wSvc.Code)
	}

	evilBody, _ := json.Marshal(map[string]any{
		"type":      "message",
		"id":        "activity-4",
		"text":      "hello",
		"from":      map[string]any{"id": "user-1"},
		"recipient": map[string]any{"id": "bot-1"},
		"conversation": map[string]any{
			"id":               "conv-1",
			"conversationType": "personal",
		},
		"serviceUrl": "https://evil.example.com",
	})
	reqEvil := httptest.NewRequest(http.MethodPost, "/teams/messages", bytes.NewReader(evilBody))
	reqEvil.Header.Set("Authorization", "Bearer "+goodJWT)
	wEvil := httptest.NewRecorder()
	b.handleTeamsMessages(wEvil, reqEvil)
	if wEvil.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for untrusted serviceurl host, got %d", wEvil.Code)
	}
}

func TestTeamsInboundNormalizationIncludesChannelMetadata(t *testing.T) {
	var got map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/channels/msteams/inbound" {
			defer r.Body.Close()
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer api.Close()

	b := newTestBridge(api.URL)
	payload := map[string]any{
		"type": "message",
		"id":   "activity-meta-1",
		"text": "<at>KafClaw</at> status",
		"from": map[string]any{
			"id":          "29:user-1",
			"aadObjectId": "aad-user-1",
		},
		"recipient": map[string]any{
			"id":   "28:bot-1",
			"name": "KafClaw",
		},
		"conversation": map[string]any{
			"id":               "19:conv-1",
			"conversationType": "channel",
		},
		"channelData": map[string]any{
			"team":    map[string]any{"id": "team-1"},
			"channel": map[string]any{"id": "channel-1"},
			"tenant":  map[string]any{"id": "tenant-1"},
		},
		"serviceUrl": "https://smba.trafficmanager.net/emea",
		"replyToId":  "thread-123",
		"entities": []map[string]any{
			{
				"type":      "mention",
				"mentioned": map[string]any{"id": "28:bot-1", "name": "KafClaw"},
			},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/teams/messages", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleTeamsMessages(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got == nil {
		t.Fatal("expected forwarded payload")
	}
	if strings.TrimSpace(asString(got["text"])) != "status" {
		t.Fatalf("expected mention to be stripped, got %#v", got["text"])
	}
	if strings.TrimSpace(asString(got["group_id"])) != "team-1" || strings.TrimSpace(asString(got["channel_id"])) != "channel-1" {
		t.Fatalf("expected channel metadata, got %#v", got)
	}
	if strings.TrimSpace(asString(got["tenant_id"])) != "tenant-1" {
		t.Fatalf("expected tenant_id, got %#v", got["tenant_id"])
	}
	if was, _ := got["was_mentioned"].(bool); !was {
		t.Fatalf("expected was_mentioned=true, got %#v", got["was_mentioned"])
	}
	if isGroup, _ := got["is_group"].(bool); !isGroup {
		t.Fatalf("expected is_group=true, got %#v", got["is_group"])
	}
}

func TestInboundDedupPersistenceAcrossReload(t *testing.T) {
	statePath := t.TempDir() + "/state.json"
	b := newTestBridge("http://example.invalid")
	b.cfg.StatePath = statePath

	if b.seenInboundEvent("slack:event:Ev9", time.Now()) {
		t.Fatal("first seen should not dedupe")
	}
	if err := b.saveState(); err != nil {
		t.Fatalf("save state: %v", err)
	}

	b2 := newTestBridge("http://example.invalid")
	b2.cfg.StatePath = statePath
	if err := b2.loadState(); err != nil {
		t.Fatalf("load state: %v", err)
	}
	if !b2.seenInboundEvent("slack:event:Ev9", time.Now()) {
		t.Fatal("expected dedupe hit after reload")
	}
}

func TestSlackOutboundMediaUpload(t *testing.T) {
	var uploaded int32
	var mediaServed int32

	mediaSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media/file.txt" {
			atomic.AddInt32(&mediaServed, 1)
			_, _ = w.Write([]byte("hello media"))
			return
		}
		http.NotFound(w, r)
	}))
	defer mediaSrv.Close()

	var slackAPI *httptest.Server
	slackAPI = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files.uploadV2":
			atomic.AddInt32(&uploaded, 1)
			_ = r.ParseMultipartForm(2 << 20)
			if got := r.FormValue("channel_id"); got != "C123" {
				t.Fatalf("channel_id=%q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/chat.postMessage":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer slackAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.SlackAPIBase = slackAPI.URL
	b.cfg.SlackBotToken = "xoxb-test"
	mediaBase, err := url.Parse(mediaSrv.URL)
	if err != nil {
		t.Fatalf("parse media server url: %v", err)
	}
	baseTransport := mediaSrv.Client().Transport
	b.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.EqualFold(req.URL.Hostname(), "files.slack.com") {
				clone := req.Clone(req.Context())
				clone.URL.Scheme = mediaBase.Scheme
				clone.URL.Host = mediaBase.Host
				return baseTransport.RoundTrip(clone)
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	reqBody, _ := json.Marshal(map[string]any{
		"chat_id":    "C123",
		"content":    "caption",
		"media_urls": []string{"https://files.slack.com/media/file.txt"},
	})
	req := httptest.NewRequest(http.MethodPost, "/slack/outbound", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	b.handleSlackOutbound(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if atomic.LoadInt32(&mediaServed) != 1 || atomic.LoadInt32(&uploaded) != 1 {
		t.Fatalf("expected media served=1 and uploaded=1, got %d/%d", mediaServed, uploaded)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTeamsOutboundCardAndAttachment(t *testing.T) {
	var payload map[string]any
	teamsAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer teamsAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.MSTeamsAPIBase = teamsAPI.URL
	b.teamsMu.Lock()
	b.teamsConvByID["conv-1"] = teamsConversationRef{
		ServiceURL:     teamsAPI.URL,
		ConversationID: "conv-1",
		UserID:         "u1",
	}
	b.teamsToken = tokenCache{accessToken: "token", expiresAt: time.Now().Add(30 * time.Minute)}
	b.teamsMu.Unlock()

	reqBody, _ := json.Marshal(map[string]any{
		"chat_id":    "conv-1",
		"thread_id":  "r1",
		"content":    "hello",
		"media_urls": []string{"https://files.example.com/doc.pdf"},
		"card": map[string]any{
			"type": "AdaptiveCard",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/teams/outbound", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	b.handleTeamsOutbound(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	atts, _ := payload["attachments"].([]any)
	if len(atts) != 2 {
		t.Fatalf("expected 2 attachments, got %#v", payload["attachments"])
	}
}

func buildTestJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": kid}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	p1 := base64.RawURLEncoding.EncodeToString(hb)
	p2 := base64.RawURLEncoding.EncodeToString(cb)
	signingInput := p1 + "." + p2
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return fmt.Sprintf("%s.%s.%s", p1, p2, base64.RawURLEncoding.EncodeToString(sig))
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter("2"); d != 2*time.Second {
		t.Fatalf("expected 2s, got %v", d)
	}
	future := time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(future); d <= 0 {
		t.Fatalf("expected positive duration for http date, got %v", d)
	}
	if d := parseRetryAfter("invalid"); d != 0 {
		t.Fatalf("expected 0 for invalid value, got %v", d)
	}
}

func TestSlackResolveUsersAndChannels(t *testing.T) {
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users.list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"members": []map[string]any{
					{"id": "U123", "name": "alice", "real_name": "Alice Doe", "profile": map[string]any{"display_name": "alice"}},
				},
			})
		case "/conversations.list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"channels": []map[string]any{
					{"id": "C111", "name": "eng"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer slackAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.SlackAPIBase = slackAPI.URL
	b.cfg.SlackBotToken = "xoxb-test"

	usersReq, _ := json.Marshal(map[string]any{"entries": []string{"alice"}})
	uw := httptest.NewRecorder()
	ur := httptest.NewRequest(http.MethodPost, "/slack/resolve/users", bytes.NewReader(usersReq))
	b.handleSlackResolveUsers(uw, ur)
	if uw.Code != http.StatusOK {
		t.Fatalf("users status=%d body=%s", uw.Code, uw.Body.String())
	}
	var uresp map[string]any
	_ = json.Unmarshal(uw.Body.Bytes(), &uresp)
	results, _ := uresp["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("unexpected users results: %#v", uresp)
	}

	chReq, _ := json.Marshal(map[string]any{"entries": []string{"eng"}})
	cw := httptest.NewRecorder()
	cr := httptest.NewRequest(http.MethodPost, "/slack/resolve/channels", bytes.NewReader(chReq))
	b.handleSlackResolveChannels(cw, cr)
	if cw.Code != http.StatusOK {
		t.Fatalf("channels status=%d body=%s", cw.Code, cw.Body.String())
	}
}

func TestTeamsResolveUsersAndChannels(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/users"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"id": "uid-1", "displayName": "Alex Doe", "mail": "alex@example.com", "userPrincipalName": "alex@example.com"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/teams/") && strings.HasSuffix(r.URL.Path, "/channels"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"id": "ch-1", "displayName": "general"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/teams"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"id": "team-1", "displayName": "eng"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer graph.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.MSTeamsGraphBase = graph.URL
	b.teamsMu.Lock()
	b.teamsGraphToken = tokenCache{accessToken: "graph-token", expiresAt: time.Now().Add(10 * time.Minute)}
	b.teamsMu.Unlock()

	usersReq, _ := json.Marshal(map[string]any{"entries": []string{"alex@example.com"}})
	uw := httptest.NewRecorder()
	ur := httptest.NewRequest(http.MethodPost, "/teams/resolve/users", bytes.NewReader(usersReq))
	b.handleTeamsResolveUsers(uw, ur)
	if uw.Code != http.StatusOK {
		t.Fatalf("users status=%d body=%s", uw.Code, uw.Body.String())
	}

	chReq, _ := json.Marshal(map[string]any{"entries": []string{"eng/general"}})
	cw := httptest.NewRecorder()
	cr := httptest.NewRequest(http.MethodPost, "/teams/resolve/channels", bytes.NewReader(chReq))
	b.handleTeamsResolveChannels(cw, cr)
	if cw.Code != http.StatusOK {
		t.Fatalf("channels status=%d body=%s", cw.Code, cw.Body.String())
	}
}

func TestSlackOutboundActionReact(t *testing.T) {
	var reactCalled int32
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/reactions.add" {
			atomic.AddInt32(&reactCalled, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
		http.NotFound(w, r)
	}))
	defer slackAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.SlackAPIBase = slackAPI.URL
	b.cfg.SlackBotToken = "xoxb-test"

	body, _ := json.Marshal(map[string]any{
		"chat_id": "channel:C111",
		"action":  "react",
		"action_params": map[string]any{
			"emoji":      "thumbsup",
			"message_id": "123.456",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/slack/outbound", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleSlackOutbound(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if atomic.LoadInt32(&reactCalled) != 1 {
		t.Fatalf("expected reactions.add call")
	}
}

func TestSlackOutboundCardBlocks(t *testing.T) {
	var sawBlocks bool
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat.postMessage" {
			_ = r.ParseForm()
			blocks := strings.TrimSpace(r.FormValue("blocks"))
			sawBlocks = blocks != ""
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1"})
			return
		}
		http.NotFound(w, r)
	}))
	defer slackAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.SlackAPIBase = slackAPI.URL
	b.cfg.SlackBotToken = "xoxb-test"

	body, _ := json.Marshal(map[string]any{
		"chat_id": "C111",
		"content": "card text",
		"card": map[string]any{
			"blocks": []map[string]any{{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": "hello"}}},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/slack/outbound", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleSlackOutbound(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !sawBlocks {
		t.Fatal("expected blocks in chat.postMessage payload")
	}
}

func TestSlackReplyModeOffSuppressesThread(t *testing.T) {
	var gotThreadTS string
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat.postMessage" {
			_ = r.ParseForm()
			gotThreadTS = strings.TrimSpace(r.FormValue("thread_ts"))
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1"})
			return
		}
		http.NotFound(w, r)
	}))
	defer slackAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.SlackAPIBase = slackAPI.URL
	b.cfg.SlackBotToken = "xoxb-test"

	body, _ := json.Marshal(map[string]any{
		"chat_id":    "C111",
		"thread_id":  "171.1",
		"reply_mode": "off",
		"content":    "hello",
	})
	req := httptest.NewRequest(http.MethodPost, "/slack/outbound", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleSlackOutbound(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if gotThreadTS != "" {
		t.Fatalf("expected no thread_ts for off mode, got %q", gotThreadTS)
	}
}

func TestSlackReplyModeFirstUsesThreadOnce(t *testing.T) {
	got := make([]string, 0, 2)
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat.postMessage" {
			_ = r.ParseForm()
			got = append(got, strings.TrimSpace(r.FormValue("thread_ts")))
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1"})
			return
		}
		http.NotFound(w, r)
	}))
	defer slackAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.SlackAPIBase = slackAPI.URL
	b.cfg.SlackBotToken = "xoxb-test"

	send := func() {
		body, _ := json.Marshal(map[string]any{
			"chat_id":    "C111",
			"thread_id":  "171.2",
			"reply_mode": "first",
			"content":    "hello",
		})
		req := httptest.NewRequest(http.MethodPost, "/slack/outbound", bytes.NewReader(body))
		w := httptest.NewRecorder()
		b.handleSlackOutbound(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
	send()
	send()
	if len(got) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(got))
	}
	if got[0] == "" || got[1] != "" {
		t.Fatalf("expected first threaded, second not threaded; got %#v", got)
	}
}

func TestTeamsOutboundPollStoresState(t *testing.T) {
	teamsAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer teamsAPI.Close()

	state := t.TempDir() + "/state.json"
	b := newTestBridge("http://example.invalid")
	b.cfg.StatePath = state
	b.cfg.MSTeamsAPIBase = teamsAPI.URL
	b.teamsMu.Lock()
	b.teamsConvByID["conv-1"] = teamsConversationRef{ServiceURL: teamsAPI.URL, ConversationID: "conv-1", UserID: "u1"}
	b.teamsToken = tokenCache{accessToken: "token", expiresAt: time.Now().Add(10 * time.Minute)}
	b.teamsMu.Unlock()

	body, _ := json.Marshal(map[string]any{
		"chat_id":             "conversation:conv-1",
		"poll_question":       "Lunch?",
		"poll_options":        []string{"Sushi", "Pizza"},
		"poll_max_selections": 1,
	})
	req := httptest.NewRequest(http.MethodPost, "/teams/outbound", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleTeamsOutbound(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	b2 := newTestBridge("http://example.invalid")
	b2.cfg.StatePath = state
	if err := b2.loadState(); err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(b2.teamsPolls) == 0 {
		t.Fatal("expected poll persisted in state")
	}
}

func TestTeamsReplyModeOffSuppressesReplyPath(t *testing.T) {
	var replyToIDs []string
	teamsAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&payload)
		replyToIDs = append(replyToIDs, strings.TrimSpace(asString(payload["replyToId"])))
		w.WriteHeader(http.StatusOK)
	}))
	defer teamsAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.MSTeamsAPIBase = teamsAPI.URL
	b.teamsMu.Lock()
	b.teamsConvByID["conv-1"] = teamsConversationRef{ServiceURL: teamsAPI.URL, ConversationID: "conv-1", UserID: "u1"}
	b.teamsToken = tokenCache{accessToken: "token", expiresAt: time.Now().Add(10 * time.Minute)}
	b.teamsMu.Unlock()

	body, _ := json.Marshal(map[string]any{
		"chat_id":    "conversation:conv-1",
		"thread_id":  "reply-1",
		"reply_mode": "off",
		"content":    "hello",
	})
	req := httptest.NewRequest(http.MethodPost, "/teams/outbound", bytes.NewReader(body))
	w := httptest.NewRecorder()
	b.handleTeamsOutbound(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(replyToIDs) != 1 || replyToIDs[0] != "" {
		t.Fatalf("expected no replyToId for off mode, got %#v", replyToIDs)
	}
}

func TestTeamsReplyModeFirstUsesReplyPathOnce(t *testing.T) {
	var replyToIDs []string
	teamsAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&payload)
		replyToIDs = append(replyToIDs, strings.TrimSpace(asString(payload["replyToId"])))
		w.WriteHeader(http.StatusOK)
	}))
	defer teamsAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.MSTeamsAPIBase = teamsAPI.URL
	b.teamsMu.Lock()
	b.teamsConvByID["conv-1"] = teamsConversationRef{ServiceURL: teamsAPI.URL, ConversationID: "conv-1", UserID: "u1"}
	b.teamsToken = tokenCache{accessToken: "token", expiresAt: time.Now().Add(10 * time.Minute)}
	b.teamsMu.Unlock()

	send := func() {
		body, _ := json.Marshal(map[string]any{
			"chat_id":    "conversation:conv-1",
			"thread_id":  "reply-2",
			"reply_mode": "first",
			"content":    "hello",
		})
		req := httptest.NewRequest(http.MethodPost, "/teams/outbound", bytes.NewReader(body))
		w := httptest.NewRecorder()
		b.handleTeamsOutbound(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
	send()
	send()
	if len(replyToIDs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(replyToIDs))
	}
	if replyToIDs[0] != "reply-2" || replyToIDs[1] != "" {
		t.Fatalf("expected first replyToId and second empty, got %#v", replyToIDs)
	}
}

func TestTeamsInboundPollVoteRecorded(t *testing.T) {
	b := newTestBridge("http://example.invalid")
	b.teamsPolls["poll-1"] = map[string]any{
		"chat_id":  "conv-1",
		"question": "Lunch?",
		"options":  []string{"Sushi", "Pizza"},
	}
	ok := b.handleTeamsPollVote("conv-1", "user-1", map[string]any{
		"value": map[string]any{
			"poll_choice": "Sushi",
		},
	})
	if !ok {
		t.Fatal("expected poll vote recorded")
	}
	p := b.teamsPolls["poll-1"]
	votes, _ := p["votes"].(map[string]any)
	if votes == nil || votes["user-1"] != "Sushi" {
		t.Fatalf("unexpected votes: %#v", votes)
	}
}

func TestSlackProbe(t *testing.T) {
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth.test" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "team": "T", "user": "U"})
			return
		}
		http.NotFound(w, r)
	}))
	defer slackAPI.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.SlackAPIBase = slackAPI.URL
	b.cfg.SlackBotToken = "xoxb-test"

	req := httptest.NewRequest(http.MethodGet, "/slack/probe", nil)
	w := httptest.NewRecorder()
	b.handleSlackProbe(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestTeamsProbe(t *testing.T) {
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/users"):
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"id": "u1"}}})
		case strings.HasPrefix(r.URL.Path, "/teams/") && strings.Contains(r.URL.Path, "/channels"):
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"id": "ch1"}}})
		case strings.HasPrefix(r.URL.Path, "/teams"):
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"id": "team1"}}})
		case strings.HasPrefix(r.URL.Path, "/organization"):
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []map[string]any{{"id": "org1"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer graph.Close()

	b := newTestBridge("http://example.invalid")
	b.cfg.MSTeamsGraphBase = graph.URL
	b.teamsMu.Lock()
	b.teamsToken = tokenCache{accessToken: buildUnsignedTestJWT(map[string]any{"aud": "api.botframework.com"}), expiresAt: time.Now().Add(10 * time.Minute)}
	b.teamsGraphToken = tokenCache{accessToken: buildUnsignedTestJWT(map[string]any{
		"aud":   "graph.microsoft.com",
		"scp":   "User.Read.All Team.ReadBasic.All Channel.ReadBasic.All Organization.Read.All",
		"tid":   "tenant-1",
		"appid": "app-1",
	}), expiresAt: time.Now().Add(10 * time.Minute)}
	b.teamsMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/teams/probe", nil)
	w := httptest.NewRecorder()
	b.handleTeamsProbe(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	bot, _ := body["bot"].(map[string]any)
	if bot == nil {
		t.Fatalf("missing bot diagnostics: %s", w.Body.String())
	}
	diag, _ := bot["diagnostics"].(map[string]any)
	if diag == nil {
		t.Fatalf("missing bot diagnostics map: %s", w.Body.String())
	}
	if _, ok := diag["audience_matches"]; !ok {
		t.Fatalf("expected audience_matches in diagnostics: %#v", diag)
	}
	graphBlock, _ := body["graph"].(map[string]any)
	if graphBlock == nil {
		t.Fatalf("missing graph diagnostics block: %s", w.Body.String())
	}
	graphDiag, _ := graphBlock["diagnostics"].(map[string]any)
	if graphDiag == nil {
		t.Fatalf("missing graph diagnostics map: %s", w.Body.String())
	}
	caps, _ := graphDiag["capabilities"].(map[string]any)
	if caps == nil {
		t.Fatalf("missing capabilities diagnostics: %#v", graphDiag)
	}
	users, _ := caps["users"].(map[string]any)
	if users == nil || users["ok"] != true {
		t.Fatalf("expected users capability ok, got %#v", users)
	}
}

func TestVerifySlackSignatureBranches(t *testing.T) {
	secret := "s3cret"
	body := []byte(`{"type":"event_callback"}`)

	req := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	if err := verifySlackSignature(body, req, ""); err != nil {
		t.Fatalf("expected empty secret to bypass verify: %v", err)
	}

	if err := verifySlackSignature(body, req, secret); err == nil {
		t.Fatal("expected missing headers error")
	}

	reqBadTS := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	reqBadTS.Header.Set("X-Slack-Request-Timestamp", "not-a-number")
	reqBadTS.Header.Set("X-Slack-Signature", "v0=abcd")
	if err := verifySlackSignature(body, reqBadTS, secret); err == nil {
		t.Fatal("expected invalid timestamp error")
	}

	oldTS := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
	reqOld := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	reqOld.Header.Set("X-Slack-Request-Timestamp", oldTS)
	reqOld.Header.Set("X-Slack-Signature", "v0=abcd")
	if err := verifySlackSignature(body, reqOld, secret); err == nil {
		t.Fatal("expected out of range timestamp error")
	}

	ts := fmt.Sprintf("%d", time.Now().Unix())
	reqMismatch := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	reqMismatch.Header.Set("X-Slack-Request-Timestamp", ts)
	reqMismatch.Header.Set("X-Slack-Signature", "v0=deadbeef")
	if err := verifySlackSignature(body, reqMismatch, secret); err == nil {
		t.Fatal("expected signature mismatch error")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + ts + ":" + string(body)))
	validSig := "v0=" + hex.EncodeToString(mac.Sum(nil))
	reqOK := httptest.NewRequest(http.MethodPost, "/slack/events", bytes.NewReader(body))
	reqOK.Header.Set("X-Slack-Request-Timestamp", ts)
	reqOK.Header.Set("X-Slack-Signature", validSig)
	if err := verifySlackSignature(body, reqOK, secret); err != nil {
		t.Fatalf("expected valid signature: %v", err)
	}
}

func TestVerifyBearer(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/teams/messages", nil)
	if !verifyBearer(req, "") {
		t.Fatal("expected empty expected token to allow request")
	}
	if verifyBearer(req, "x") {
		t.Fatal("expected missing auth header to fail")
	}
	req.Header.Set("Authorization", "Bearer abc")
	if !verifyBearer(req, "abc") {
		t.Fatal("expected matching bearer to pass")
	}
	if verifyBearer(req, "def") {
		t.Fatal("expected mismatched bearer to fail")
	}
}

func TestVerifyTeamsJWTRequestBranches(t *testing.T) {
	b := newTestBridge("http://example.invalid")

	if err := b.verifyTeamsJWTRequest(httptest.NewRequest(http.MethodPost, "/teams/messages", nil), "https://service"); err != nil {
		t.Fatalf("expected bypass when jwt/app id unset: %v", err)
	}

	b.cfg.MSTeamsAppID = "app-id"
	b.jwt = &teamsJWTVerifier{appID: "app-id"}
	req := httptest.NewRequest(http.MethodPost, "/teams/messages", nil)
	if err := b.verifyTeamsJWTRequest(req, "https://service"); err == nil {
		t.Fatal("expected missing authorization error")
	}
}

func TestReplyRoutingHelpers(t *testing.T) {
	if got := normalizeReplyMode("off"); got != "off" {
		t.Fatalf("normalize off mismatch: %q", got)
	}
	if got := normalizeReplyMode("first"); got != "first" {
		t.Fatalf("normalize first mismatch: %q", got)
	}
	if got := normalizeReplyMode("weird"); got != "all" {
		t.Fatalf("normalize default mismatch: %q", got)
	}

	b := newTestBridge("http://example.invalid")
	if got := b.resolveReplyThread("slack", "acct", "C1", "", "all", "all"); got != "" {
		t.Fatalf("expected empty reply thread when no thread id, got %q", got)
	}
	if got := b.resolveReplyThread("slack", "acct", "C1", "T1", "off", "all"); got != "" {
		t.Fatalf("expected off mode to suppress thread, got %q", got)
	}
	if got := b.resolveReplyThread("slack", "acct", "C1", "T1", "", "all"); got != "T1" {
		t.Fatalf("expected default mode all to keep thread, got %q", got)
	}
	if got := b.resolveReplyThread("slack", "acct", "C1", "T1", "first", "all"); got != "T1" {
		t.Fatalf("expected first call in first mode to keep thread, got %q", got)
	}
	if got := b.resolveReplyThread("slack", "acct", "C1", "T1", "first", "all"); got != "" {
		t.Fatalf("expected second call in first mode to suppress thread, got %q", got)
	}
	if got := b.resolveReplyThread("slack", "", "C1", "T1", "bogus", "all"); got != "T1" {
		t.Fatalf("expected invalid mode to fallback to all, got %q", got)
	}

	if got := bridgeAccountIDOrDefault(""); got != "default" {
		t.Fatalf("expected default account id, got %q", got)
	}
	if got := bridgeAccountIDOrDefault("acct-a"); got != "acct-a" {
		t.Fatalf("expected explicit account id, got %q", got)
	}
	if got := asString(nil); got != "" {
		t.Fatalf("asString nil mismatch: %q", got)
	}
	if got := asString("x"); got != "x" {
		t.Fatalf("asString string mismatch: %q", got)
	}
	if got := firstNonEmpty(" ", "", "x", "y"); got != "x" {
		t.Fatalf("firstNonEmpty mismatch: %q", got)
	}
}

func TestEnvAndConfigHelpers(t *testing.T) {
	t.Setenv("CHANNEL_BRIDGE_ADDR", " :19999 ")
	t.Setenv("KAFCLAW_BASE_URL", " http://127.0.0.1:19991 ")
	t.Setenv("SLACK_ACCOUNT_ID", " acct-a ")
	t.Setenv("SLACK_REPLY_MODE", " first ")
	t.Setenv("MSTEAMS_ACCOUNT_ID", " acct-b ")
	t.Setenv("MSTEAMS_REPLY_MODE", " off ")
	t.Setenv("CHANNEL_BRIDGE_STATE", " /tmp/state.json ")

	cfg := loadConfig()
	if cfg.ListenAddr != ":19999" || cfg.KafclawBase != "http://127.0.0.1:19991" {
		t.Fatalf("unexpected config base/listen: %#v", cfg)
	}
	if cfg.SlackAccountID != "acct-a" || cfg.SlackReplyMode != "first" {
		t.Fatalf("unexpected slack config: %#v", cfg)
	}
	if cfg.MSTeamsAccountID != "acct-b" || cfg.MSTeamsReplyMode != "off" {
		t.Fatalf("unexpected teams config: %#v", cfg)
	}
	if cfg.StatePath != "/tmp/state.json" {
		t.Fatalf("unexpected state path: %q", cfg.StatePath)
	}

	t.Setenv("FOO_EMPTY", " ")
	if got := getEnvDefault("FOO_EMPTY", "d"); got != "d" {
		t.Fatalf("getEnvDefault expected fallback, got %q", got)
	}
	t.Setenv("FOO_SET", " x ")
	if got := getEnvDefault("FOO_SET", "d"); got != "x" {
		t.Fatalf("getEnvDefault expected trimmed env, got %q", got)
	}
}

func TestStatusAndMetricsHelpers(t *testing.T) {
	b := newTestBridge("http://example.invalid")
	now := time.Now().Add(5 * time.Minute)
	b.teamsMu.Lock()
	b.teamsConvByID["c1"] = teamsConversationRef{ConversationID: "c1"}
	b.teamsConvByUserID["u1"] = teamsConversationRef{UserID: "u1"}
	b.teamsToken = tokenCache{accessToken: "token", expiresAt: now}
	b.teamsMu.Unlock()
	b.cfg.MSTeamsInboundBearer = "required"
	b.inboundMu.Lock()
	b.inboundSeen["old"] = time.Now().Add(-2 * b.inboundTTL)
	b.inboundSeen["new"] = time.Now().Add(2 * time.Minute)
	b.inboundMu.Unlock()

	b.noteInboundForward(false, fmt.Errorf("forward failed"))
	b.noteInboundForward(true, nil)
	if b.metrics.InboundForwardErrors == 0 || b.metrics.LastError == "" || b.metrics.LastErrorAt == "" {
		t.Fatalf("expected inbound error metrics to update, got %#v", b.metrics)
	}

	w := httptest.NewRecorder()
	b.handleStatus(w, httptest.NewRequest(http.MethodGet, "/status", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status code=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	teams, _ := body["teams"].(map[string]any)
	if teams == nil {
		t.Fatalf("missing teams payload: %#v", body)
	}
	if int(teams["conversation_refs"].(float64)) != 1 || int(teams["user_refs"].(float64)) != 1 {
		t.Fatalf("unexpected ref counts: %#v", teams)
	}
	if !teams["token_cached"].(bool) || !teams["inbound_bearer_required"].(bool) {
		t.Fatalf("expected token cached + bearer required true: %#v", teams)
	}
	if int(body["inbound_dedupe_cache"].(float64)) != 1 {
		t.Fatalf("expected dedupe cache size 1 after prune, got %#v", body["inbound_dedupe_cache"])
	}
}

func TestRetryAndJWTUtilityHelpers(t *testing.T) {
	b := newTestBridge("http://example.invalid")
	retry, err := b.slackRetryDecision(nil)
	if retry || err != nil {
		t.Fatalf("expected nil error to be non-retry, got retry=%v err=%v", retry, err)
	}
	retry, err = b.slackRetryDecision(fmt.Errorf("x"))
	if retry || err == nil {
		t.Fatalf("expected non-rate-limit error passthrough, got retry=%v err=%v", retry, err)
	}
	retry, err = b.slackRetryDecision(&slack.RateLimitedError{RetryAfter: 0})
	if !retry || err == nil {
		t.Fatalf("expected rate-limit error retry=true, got retry=%v err=%v", retry, err)
	}

	if matchesJWTAudience("app", "app") != true {
		t.Fatal("expected string audience match")
	}
	if matchesJWTAudience([]any{"x", "app"}, "app") != true {
		t.Fatal("expected array audience match")
	}
	if matchesJWTAudience([]any{"x"}, "app") != false {
		t.Fatal("expected audience mismatch")
	}
	if matchesJWTAudience("x", "") != true {
		t.Fatal("expected empty app id to allow")
	}

	if anyToUnix(float64(12)) != 12 || anyToUnix(int64(13)) != 13 || anyToUnix(int(14)) != 14 {
		t.Fatal("anyToUnix numeric conversions failed")
	}
	if anyToUnix(json.Number("15")) != 15 {
		t.Fatal("anyToUnix json.Number conversion failed")
	}
	if anyToUnix("x") != 0 {
		t.Fatal("anyToUnix invalid input should be 0")
	}

	now := time.Unix(1000, 0).UTC()
	diag := tokenDiagnostics(map[string]any{
		"iss":        "issuer",
		"aud":        []any{"api://bot"},
		"serviceUrl": "https://smba.trafficmanager.net/teams/",
		"scp":        "User.Read",
		"roles":      []any{"Chat.Read", ""},
		"exp":        float64(1100),
		"nbf":        float64(900),
	}, now, "api://")
	if diag["audience"] != "api://bot" || diag["issuer"] != "issuer" {
		t.Fatalf("unexpected token diagnostics basics: %#v", diag)
	}
	if diag["expired"].(bool) || !diag["active"].(bool) || !diag["audience_matches"].(bool) {
		t.Fatalf("unexpected token diagnostics flags: %#v", diag)
	}
	roles, _ := diag["roles"].([]string)
	if len(roles) != 1 || roles[0] != "Chat.Read" {
		t.Fatalf("unexpected roles diagnostics: %#v", diag["roles"])
	}
}

func TestValidateMediaDownloadURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "allow slack host", raw: "https://files.slack.com/path/file.bin"},
		{name: "reject non-https", raw: "http://files.slack.com/path/file.bin", wantErr: true},
		{name: "reject unknown host", raw: "https://example.com/file.bin", wantErr: true},
		{name: "reject userinfo", raw: "https://user:pass@files.slack.com/file.bin", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateMediaDownloadURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got url=%v", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.raw, err)
			}
			if got == nil || strings.TrimSpace(got.String()) == "" {
				t.Fatalf("expected normalized URL for %q", tt.raw)
			}
			if got.Host != "files.slack.com" {
				t.Fatalf("expected normalized host files.slack.com, got %q", got.Host)
			}
		})
	}
}

func buildUnsignedTestJWT(claims map[string]any) string {
	header := map[string]any{"alg": "none", "typ": "JWT"}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb) + "."
}
