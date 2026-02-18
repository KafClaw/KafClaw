package channels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

var pairingCodeRe = regexp.MustCompile(`Pairing code:\s+([A-Z0-9]+)`)

func extractPairingCode(t *testing.T, msg string) string {
	t.Helper()
	m := pairingCodeRe.FindStringSubmatch(msg)
	if len(m) < 2 {
		t.Fatalf("pairing code not found in message: %q", msg)
	}
	return m[1]
}

func TestSlackHandleInboundRequiresPairing(t *testing.T) {
	msgBus := bus.NewMessageBus()
	db := filepath.Join(t.TempDir(), "timeline.db")
	timeSvc, err := timeline.NewTimelineService(db)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	defer timeSvc.Close()

	ch := NewSlackChannel(config.SlackConfig{
		Enabled:        true,
		DmPolicy:       config.DmPolicyPairing,
		GroupPolicy:    config.GroupPolicyAllowlist,
		RequireMention: true,
	}, msgBus, timeSvc)

	out := make(chan *bus.OutboundMessage, 1)
	msgBus.Subscribe("slack", func(msg *bus.OutboundMessage) { out <- msg })
	go msgBus.DispatchOutbound(t.Context())

	if err := ch.HandleInbound("U999", "D123", "", "m1", "hello", false, false); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}

	select {
	case got := <-out:
		if got.ChatID != "D123" {
			t.Fatalf("unexpected chat id: %s", got.ChatID)
		}
		if got.Content == "" {
			t.Fatal("expected pairing instruction message")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected pairing outbound message")
	}
}

func TestMSTeamsHandleInboundAllowlistedPublishesInbound(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:        true,
		AllowFrom:      []string{"A123"},
		DmPolicy:       config.DmPolicyAllowlist,
		GroupPolicy:    config.GroupPolicyAllowlist,
		RequireMention: true,
	}, msgBus, nil)

	if err := ch.HandleInbound("A123", "conv1", "thread1", "m1", "ping", false, false); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}

	msg, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	if msg.Channel != "msteams" || msg.Content != "ping" || msg.ThreadID != "thread1" {
		t.Fatalf("unexpected inbound message: %+v", msg)
	}
	scope, _ := msg.Metadata[bus.MetaKeySessionScope].(string)
	if scope != "msteams:default:conv1" {
		t.Fatalf("unexpected session scope: %q", scope)
	}
}

func TestSlackHandleInboundSetsRoomSessionScope(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewSlackChannel(config.SlackConfig{
		Enabled:        true,
		AllowFrom:      []string{"U123"},
		DmPolicy:       config.DmPolicyAllowlist,
		GroupPolicy:    config.GroupPolicyAllowlist,
		RequireMention: true,
	}, msgBus, nil)

	if err := ch.HandleInbound("U123", "C100", "t1", "m1", "hello", false, false); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}

	msg, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	scope, _ := msg.Metadata[bus.MetaKeySessionScope].(string)
	if scope != "slack:default:C100" {
		t.Fatalf("unexpected session scope: %q", scope)
	}
}

func TestSlackSendUsesOutboundBridge(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewSlackChannel(config.SlackConfig{
		Enabled:          true,
		OutboundURL:      srv.URL,
		BotToken:         "xoxb-test",
		NativeStreaming:  true,
		StreamMode:       "append",
		StreamChunkChars: 180,
	}, bus.NewMessageBus(), nil)

	err := ch.Send(context.Background(), &bus.OutboundMessage{
		Channel:   "slack",
		ChatID:    "C123",
		ThreadID:  "1717000000.000001",
		Content:   "hello",
		MediaURLs: []string{"https://files.example.com/a.png"},
		Card: map[string]any{
			"type": "adaptive",
		},
		TraceID: "trace-1",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if got["chat_id"] != "C123" || got["content"] != "hello" || got["thread_id"] != "1717000000.000001" {
		t.Fatalf("unexpected outbound payload: %#v", got)
	}
	if _, ok := got["media_urls"]; !ok {
		t.Fatalf("expected media_urls in payload: %#v", got)
	}
	if _, ok := got["card"]; !ok {
		t.Fatalf("expected card in payload: %#v", got)
	}
	if got["native_streaming"] != true {
		t.Fatalf("expected native_streaming=true, got %#v", got["native_streaming"])
	}
	if got["stream_mode"] != "append" {
		t.Fatalf("expected stream_mode=append, got %#v", got["stream_mode"])
	}
	if got["stream_chunk_chars"] != float64(180) {
		t.Fatalf("expected stream_chunk_chars=180, got %#v", got["stream_chunk_chars"])
	}
}

func TestSlackSendUsesAccountStreamingOverrides(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	disableStreaming := false
	ch := NewSlackChannel(config.SlackConfig{
		Enabled:          true,
		OutboundURL:      "http://base.invalid",
		NativeStreaming:  true,
		StreamMode:       "replace",
		StreamChunkChars: 320,
		Accounts: []config.SlackAccountConfig{
			{
				ID:               "acct1",
				OutboundURL:      srv.URL,
				NativeStreaming:  &disableStreaming,
				StreamMode:       "status_final",
				StreamChunkChars: 64,
			},
		},
	}, bus.NewMessageBus(), nil)

	err := ch.Send(context.Background(), &bus.OutboundMessage{
		Channel: "slack",
		ChatID:  "acct://acct1|C123",
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if got["account_id"] != "acct1" {
		t.Fatalf("expected account_id acct1, got %#v", got["account_id"])
	}
	if got["native_streaming"] != false {
		t.Fatalf("expected native_streaming=false override, got %#v", got["native_streaming"])
	}
	if got["stream_mode"] != "status_final" {
		t.Fatalf("expected stream_mode=status_final, got %#v", got["stream_mode"])
	}
	if got["stream_chunk_chars"] != float64(64) {
		t.Fatalf("expected stream_chunk_chars=64, got %#v", got["stream_chunk_chars"])
	}
}

func TestMSTeamsSendUsesOutboundBridge(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:     true,
		OutboundURL: srv.URL,
		AppPassword: "secret",
	}, bus.NewMessageBus(), nil)

	err := ch.Send(context.Background(), &bus.OutboundMessage{
		Channel:   "msteams",
		ChatID:    "conv-1",
		ThreadID:  "activity-1",
		Content:   "hello",
		MediaURLs: []string{"https://files.example.com/b.pdf"},
		Card: map[string]any{
			"type": "AdaptiveCard",
		},
		TraceID: "trace-2",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if got["chat_id"] != "conv-1" || got["content"] != "hello" || got["thread_id"] != "activity-1" {
		t.Fatalf("unexpected outbound payload: %#v", got)
	}
	if _, ok := got["media_urls"]; !ok {
		t.Fatalf("expected media_urls in payload: %#v", got)
	}
	if _, ok := got["card"]; !ok {
		t.Fatalf("expected card in payload: %#v", got)
	}
}

func TestSlackPairingApproveThenAllowedIntegration(t *testing.T) {
	msgBus := bus.NewMessageBus()
	db := filepath.Join(t.TempDir(), "timeline.db")
	timeSvc, err := timeline.NewTimelineService(db)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	defer timeSvc.Close()

	ch := NewSlackChannel(config.SlackConfig{
		Enabled:        true,
		DmPolicy:       config.DmPolicyPairing,
		GroupPolicy:    config.GroupPolicyAllowlist,
		RequireMention: true,
	}, msgBus, timeSvc)
	svc := NewPairingService(timeSvc)
	cfg := config.DefaultConfig()

	out := make(chan *bus.OutboundMessage, 4)
	msgBus.Subscribe("slack", func(msg *bus.OutboundMessage) { out <- msg })
	go msgBus.DispatchOutbound(t.Context())

	if err := ch.HandleInbound("U900", "D900", "", "m1", "hello", false, false); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	var pairMsg *bus.OutboundMessage
	select {
	case pairMsg = <-out:
	case <-time.After(2 * time.Second):
		t.Fatal("expected pairing message")
	}
	code := extractPairingCode(t, pairMsg.Content)
	if _, err := svc.Approve(cfg, "slack", code); err != nil {
		t.Fatalf("approve: %v", err)
	}
	ch = NewSlackChannel(cfg.Channels.Slack, msgBus, timeSvc)

	if err := ch.HandleInbound("U900", "D900", "t1", "m2", "allowed now", false, false); err != nil {
		t.Fatalf("handle inbound after approve: %v", err)
	}
	in, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	if in.Content != "allowed now" || in.Channel != "slack" {
		t.Fatalf("unexpected inbound after approve: %+v", in)
	}
}

func TestMSTeamsPairingApproveThenAllowedIntegration(t *testing.T) {
	msgBus := bus.NewMessageBus()
	db := filepath.Join(t.TempDir(), "timeline.db")
	timeSvc, err := timeline.NewTimelineService(db)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	defer timeSvc.Close()

	ch := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:        true,
		DmPolicy:       config.DmPolicyPairing,
		GroupPolicy:    config.GroupPolicyAllowlist,
		RequireMention: true,
	}, msgBus, timeSvc)
	svc := NewPairingService(timeSvc)
	cfg := config.DefaultConfig()

	out := make(chan *bus.OutboundMessage, 4)
	msgBus.Subscribe("msteams", func(msg *bus.OutboundMessage) { out <- msg })
	go msgBus.DispatchOutbound(t.Context())

	if err := ch.HandleInbound("A900", "conv900", "", "m1", "hello", false, false); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	var pairMsg *bus.OutboundMessage
	select {
	case pairMsg = <-out:
	case <-time.After(2 * time.Second):
		t.Fatal("expected pairing message")
	}
	code := extractPairingCode(t, pairMsg.Content)
	if _, err := svc.Approve(cfg, "msteams", code); err != nil {
		t.Fatalf("approve: %v", err)
	}
	ch = NewMSTeamsChannel(cfg.Channels.MSTeams, msgBus, timeSvc)

	if err := ch.HandleInbound("A900", "conv900", "t1", "m2", "allowed now", false, false); err != nil {
		t.Fatalf("handle inbound after approve: %v", err)
	}
	in, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	if in.Content != "allowed now" || in.Channel != "msteams" {
		t.Fatalf("unexpected inbound after approve: %+v", in)
	}
}

func TestMentionGatingIntegrationForGroupContexts(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewSlackChannel(config.SlackConfig{
		Enabled:        true,
		DmPolicy:       config.DmPolicyAllowlist,
		GroupPolicy:    config.GroupPolicyOpen,
		RequireMention: true,
	}, msgBus, nil)

	if err := ch.HandleInbound("U1", "C1", "", "m1", "hello", true, false); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	if _, err := msgBus.ConsumeInbound(ctx); err == nil {
		t.Fatal("expected no inbound due to mention gating")
	}

	if err := ch.HandleInbound("U1", "C1", "", "m2", "@bot hello", true, true); err != nil {
		t.Fatalf("handle inbound mentioned: %v", err)
	}
	in, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	if in.MessageID != "m2" {
		t.Fatalf("unexpected inbound: %+v", in)
	}
}

func TestThreadContinuityIntegrationSlackAndTeams(t *testing.T) {
	msgBus := bus.NewMessageBus()
	slack := NewSlackChannel(config.SlackConfig{
		Enabled:     true,
		OutboundURL: "http://example.invalid/slack",
		AllowFrom:   []string{"U1"},
		DmPolicy:    config.DmPolicyAllowlist,
		GroupPolicy: config.GroupPolicyAllowlist,
	}, msgBus, nil)
	teams := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:     true,
		OutboundURL: "http://example.invalid/teams",
		AllowFrom:   []string{"A1"},
		DmPolicy:    config.DmPolicyAllowlist,
		GroupPolicy: config.GroupPolicyAllowlist,
	}, msgBus, nil)

	if err := slack.HandleInbound("U1", "C1", "th-slack", "m1", "hello", false, false); err != nil {
		t.Fatalf("slack inbound: %v", err)
	}
	inSlack, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume slack inbound: %v", err)
	}
	if inSlack.ThreadID != "th-slack" {
		t.Fatalf("expected slack thread id, got %+v", inSlack)
	}

	if err := teams.HandleInbound("A1", "conv1", "th-teams", "m2", "hello", false, false); err != nil {
		t.Fatalf("teams inbound: %v", err)
	}
	inTeams, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume teams inbound: %v", err)
	}
	if inTeams.ThreadID != "th-teams" {
		t.Fatalf("expected teams thread id, got %+v", inTeams)
	}
}

func TestIsolationScopesAcrossRoomsAndChannels(t *testing.T) {
	msgBus := bus.NewMessageBus()
	slack := NewSlackChannel(config.SlackConfig{
		Enabled:     true,
		AllowFrom:   []string{"U1"},
		DmPolicy:    config.DmPolicyAllowlist,
		GroupPolicy: config.GroupPolicyAllowlist,
	}, msgBus, nil)
	teams := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:     true,
		AllowFrom:   []string{"U1"},
		DmPolicy:    config.DmPolicyAllowlist,
		GroupPolicy: config.GroupPolicyAllowlist,
	}, msgBus, nil)

	if err := slack.HandleInbound("U1", "room-a", "th-1", "m1", "hello a", true, true); err != nil {
		t.Fatalf("slack room-a inbound: %v", err)
	}
	inA, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume room-a inbound: %v", err)
	}
	scopeA, _ := inA.Metadata[bus.MetaKeySessionScope].(string)
	if scopeA != "slack:default:room-a" {
		t.Fatalf("unexpected room-a scope: %q", scopeA)
	}

	if err := slack.HandleInbound("U1", "room-b", "th-2", "m2", "hello b", true, true); err != nil {
		t.Fatalf("slack room-b inbound: %v", err)
	}
	inB, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume room-b inbound: %v", err)
	}
	scopeB, _ := inB.Metadata[bus.MetaKeySessionScope].(string)
	if scopeB != "slack:default:room-b" {
		t.Fatalf("unexpected room-b scope: %q", scopeB)
	}
	if scopeA == scopeB {
		t.Fatalf("expected isolated scopes per room, got same scope %q", scopeA)
	}

	if err := teams.HandleInbound("U1", "room-a", "th-3", "m3", "hello teams", true, true); err != nil {
		t.Fatalf("teams inbound: %v", err)
	}
	inTeams, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume teams inbound: %v", err)
	}
	scopeTeams, _ := inTeams.Metadata[bus.MetaKeySessionScope].(string)
	if scopeTeams != "msteams:default:room-a" {
		t.Fatalf("unexpected teams scope: %q", scopeTeams)
	}
	if scopeTeams == scopeA {
		t.Fatalf("expected channel isolation across same chat id, got same scope %q", scopeTeams)
	}
}

func TestSlackMultiAccountInboundAndOutbound(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	msgBus := bus.NewMessageBus()
	ch := NewSlackChannel(config.SlackConfig{
		Enabled:     true,
		OutboundURL: "http://default.invalid",
		BotToken:    "xoxb-default",
		AllowFrom:   []string{"U-A"},
		DmPolicy:    config.DmPolicyAllowlist,
		GroupPolicy: config.GroupPolicyAllowlist,
		Accounts: []config.SlackAccountConfig{
			{
				ID:           "acct-a",
				Enabled:      true,
				BotToken:     "xoxb-a",
				InboundToken: "in-a",
				OutboundURL:  srv.URL,
				AllowFrom:    []string{"U-A"},
				DmPolicy:     config.DmPolicyAllowlist,
				GroupPolicy:  config.GroupPolicyAllowlist,
			},
		},
	}, msgBus, nil)

	if err := ch.HandleInboundWithAccount("acct-a", "U-A", "C-A", "th", "m1", "hello", false, false); err != nil {
		t.Fatalf("handle inbound with account: %v", err)
	}
	in, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	scope, _ := in.Metadata[bus.MetaKeySessionScope].(string)
	if scope != "slack:acct-a:C-A" {
		t.Fatalf("unexpected session scope: %q", scope)
	}
	if in.ChatID != "acct://acct-a|C-A" {
		t.Fatalf("expected account-scoped chat id, got %q", in.ChatID)
	}

	if err := ch.Send(t.Context(), &bus.OutboundMessage{
		Channel: "slack",
		ChatID:  in.ChatID,
		Content: "response",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got == nil {
		t.Fatal("expected outbound payload")
	}
	if got["account_id"] != "acct-a" || got["chat_id"] != "C-A" {
		t.Fatalf("unexpected outbound payload: %#v", got)
	}
}

func TestMSTeamsMultiAccountInboundAndOutbound(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	msgBus := bus.NewMessageBus()
	ch := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:     true,
		OutboundURL: "http://default.invalid",
		AppPassword: "default-secret",
		AllowFrom:   []string{"A-A"},
		DmPolicy:    config.DmPolicyAllowlist,
		GroupPolicy: config.GroupPolicyAllowlist,
		Accounts: []config.MSTeamsAccountConfig{
			{
				ID:           "acct-a",
				Enabled:      true,
				AppPassword:  "secret-a",
				InboundToken: "in-a",
				OutboundURL:  srv.URL,
				AllowFrom:    []string{"A-A"},
				DmPolicy:     config.DmPolicyAllowlist,
				GroupPolicy:  config.GroupPolicyAllowlist,
			},
		},
	}, msgBus, nil)

	if err := ch.HandleInboundWithAccount("acct-a", "A-A", "conv-A", "th", "m1", "hello", false, false); err != nil {
		t.Fatalf("handle inbound with account: %v", err)
	}
	in, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	scope, _ := in.Metadata[bus.MetaKeySessionScope].(string)
	if scope != "msteams:acct-a:conv-A" {
		t.Fatalf("unexpected session scope: %q", scope)
	}
	if in.ChatID != "acct://acct-a|conv-A" {
		t.Fatalf("expected account-scoped chat id, got %q", in.ChatID)
	}

	if err := ch.Send(t.Context(), &bus.OutboundMessage{
		Channel: "msteams",
		ChatID:  in.ChatID,
		Content: "response",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got == nil {
		t.Fatal("expected outbound payload")
	}
	if got["account_id"] != "acct-a" || got["chat_id"] != "conv-A" {
		t.Fatalf("unexpected outbound payload: %#v", got)
	}
}

func TestMSTeamsGroupTargetAllowlistParity(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:        true,
		AllowFrom:      []string{"A1"},
		GroupAllowFrom: []string{"team-1/channel-1"},
		DmPolicy:       config.DmPolicyAllowlist,
		GroupPolicy:    config.GroupPolicyAllowlist,
		RequireMention: false,
	}, msgBus, nil)

	if err := ch.HandleInboundWithContext("default", "A1", "conv1", "th1", "m1", "hello", true, true, "team-1", "channel-1"); err != nil {
		t.Fatalf("handle inbound allowed: %v", err)
	}
	in, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	if in.Content != "hello" {
		t.Fatalf("expected allowed inbound, got %+v", in)
	}
}

func TestMSTeamsGroupTargetAllowlistBlocksMismatchedChannel(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:        true,
		AllowFrom:      []string{"A1"},
		GroupAllowFrom: []string{"team-1/channel-1"},
		DmPolicy:       config.DmPolicyAllowlist,
		GroupPolicy:    config.GroupPolicyAllowlist,
		RequireMention: false,
	}, msgBus, nil)

	if err := ch.HandleInboundWithContext("default", "A1", "conv1", "th1", "m1", "hello", true, true, "team-1", "channel-2"); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	if _, err := msgBus.ConsumeInbound(ctx); err == nil {
		t.Fatal("expected inbound blocked by team/channel allowlist")
	}
}

func TestMSTeamsGroupSenderAllowlistBackwardCompatibility(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:        true,
		GroupAllowFrom: []string{"A1"},
		AllowFrom:      []string{"A1"},
		DmPolicy:       config.DmPolicyAllowlist,
		GroupPolicy:    config.GroupPolicyAllowlist,
		RequireMention: false,
	}, msgBus, nil)

	if err := ch.HandleInboundWithContext("default", "A1", "conv1", "th1", "m1", "hello", true, true, "team-x", "channel-x"); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	if _, err := msgBus.ConsumeInbound(t.Context()); err != nil {
		t.Fatalf("expected sender allowlist to remain valid: %v", err)
	}
}

func TestSlackSessionScopeThreadMode(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewSlackChannel(config.SlackConfig{
		Enabled:      true,
		AllowFrom:    []string{"U1"},
		DmPolicy:     config.DmPolicyAllowlist,
		GroupPolicy:  config.GroupPolicyAllowlist,
		SessionScope: "thread",
	}, msgBus, nil)
	if err := ch.HandleInboundWithAccount("default", "U1", "C1", "th1", "m1", "hello", false, false); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	in, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	scope, _ := in.Metadata[bus.MetaKeySessionScope].(string)
	if scope != "slack:default:C1:th1" {
		t.Fatalf("unexpected scope: %q", scope)
	}
}

func TestMSTeamsSessionScopeUserMode(t *testing.T) {
	msgBus := bus.NewMessageBus()
	ch := NewMSTeamsChannel(config.MSTeamsConfig{
		Enabled:      true,
		AllowFrom:    []string{"A1"},
		DmPolicy:     config.DmPolicyAllowlist,
		GroupPolicy:  config.GroupPolicyAllowlist,
		SessionScope: "user",
	}, msgBus, nil)
	if err := ch.HandleInboundWithContext("default", "A1", "conv1", "th1", "m1", "hello", false, false, "", ""); err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	in, err := msgBus.ConsumeInbound(t.Context())
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	scope, _ := in.Metadata[bus.MetaKeySessionScope].(string)
	if scope != "msteams:default:A1" {
		t.Fatalf("unexpected scope: %q", scope)
	}
}
