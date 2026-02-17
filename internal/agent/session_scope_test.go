package agent

import (
	"context"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/policy"
)

func TestProcessMessageUsesSessionScopeMetadata(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	msgBus := bus.NewMessageBus()
	policyEngine := policy.NewDefaultEngine()
	policyEngine.MaxAutoTier = 2

	loop := NewLoop(LoopOptions{
		Bus:           msgBus,
		Provider:      &mockProvider{},
		Policy:        policyEngine,
		Workspace:     t.TempDir(),
		WorkRepo:      t.TempDir(),
		Model:         "mock-model",
		MaxIterations: 2,
	})

	ctx := context.Background()
	msgA := &bus.InboundMessage{
		Channel:   "slack",
		SenderID:  "U1",
		ChatID:    "C-room",
		Content:   "hello A",
		Timestamp: time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType:  bus.MessageTypeExternal,
			bus.MetaKeySessionScope: "slack:C-room",
		},
	}
	msgB := &bus.InboundMessage{
		Channel:   "slack",
		SenderID:  "U2",
		ChatID:    "C-room",
		Content:   "hello B",
		Timestamp: time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType:  bus.MessageTypeExternal,
			bus.MetaKeySessionScope: "slack:C-other-room",
		},
	}

	if _, _, err := loop.processMessage(ctx, msgA); err != nil {
		t.Fatalf("process message A: %v", err)
	}
	if _, _, err := loop.processMessage(ctx, msgB); err != nil {
		t.Fatalf("process message B: %v", err)
	}

	infos := loop.sessions.List()
	if len(infos) != 2 {
		t.Fatalf("expected 2 isolated sessions, got %d", len(infos))
	}
	foundRoom := false
	foundOther := false
	for _, info := range infos {
		if info.Key == "slack:C-room" {
			foundRoom = true
		}
		if info.Key == "slack:C-other-room" {
			foundOther = true
		}
	}
	if !foundRoom || !foundOther {
		t.Fatalf("expected both session keys present, got %+v", infos)
	}
}

func TestProcessMessageDefaultCrossChannelIsolation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	msgBus := bus.NewMessageBus()
	policyEngine := policy.NewDefaultEngine()
	policyEngine.MaxAutoTier = 2

	loop := NewLoop(LoopOptions{
		Bus:           msgBus,
		Provider:      &mockProvider{},
		Policy:        policyEngine,
		Workspace:     t.TempDir(),
		WorkRepo:      t.TempDir(),
		Model:         "mock-model",
		MaxIterations: 2,
	})

	ctx := context.Background()
	msgA := &bus.InboundMessage{
		Channel:   "slack",
		SenderID:  "user-1",
		ChatID:    "same-chat",
		Content:   "hello from slack",
		Timestamp: time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType: bus.MessageTypeExternal,
		},
	}
	msgB := &bus.InboundMessage{
		Channel:   "msteams",
		SenderID:  "user-1",
		ChatID:    "same-chat",
		Content:   "hello from teams",
		Timestamp: time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType: bus.MessageTypeExternal,
		},
	}

	if _, _, err := loop.processMessage(ctx, msgA); err != nil {
		t.Fatalf("process message A: %v", err)
	}
	if _, _, err := loop.processMessage(ctx, msgB); err != nil {
		t.Fatalf("process message B: %v", err)
	}

	infos := loop.sessions.List()
	if len(infos) != 2 {
		t.Fatalf("expected 2 isolated sessions, got %d", len(infos))
	}
}

func TestProcessMessageThreadIsolationViaSessionScopeOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	msgBus := bus.NewMessageBus()
	policyEngine := policy.NewDefaultEngine()
	policyEngine.MaxAutoTier = 2

	loop := NewLoop(LoopOptions{
		Bus:           msgBus,
		Provider:      &mockProvider{},
		Policy:        policyEngine,
		Workspace:     t.TempDir(),
		WorkRepo:      t.TempDir(),
		Model:         "mock-model",
		MaxIterations: 2,
	})

	ctx := context.Background()
	msgA := &bus.InboundMessage{
		Channel:   "slack",
		SenderID:  "U1",
		ChatID:    "C-room",
		ThreadID:  "t1",
		Content:   "thread 1",
		Timestamp: time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType:  bus.MessageTypeExternal,
			bus.MetaKeySessionScope: "slack:C-room:t1",
		},
	}
	msgB := &bus.InboundMessage{
		Channel:   "slack",
		SenderID:  "U1",
		ChatID:    "C-room",
		ThreadID:  "t2",
		Content:   "thread 2",
		Timestamp: time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType:  bus.MessageTypeExternal,
			bus.MetaKeySessionScope: "slack:C-room:t2",
		},
	}

	if _, _, err := loop.processMessage(ctx, msgA); err != nil {
		t.Fatalf("process message A: %v", err)
	}
	if _, _, err := loop.processMessage(ctx, msgB); err != nil {
		t.Fatalf("process message B: %v", err)
	}

	infos := loop.sessions.List()
	if len(infos) != 2 {
		t.Fatalf("expected 2 isolated thread sessions, got %d", len(infos))
	}
}

func TestProcessMessageIsolationMatrixChannelRoomThread(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	msgBus := bus.NewMessageBus()
	policyEngine := policy.NewDefaultEngine()
	policyEngine.MaxAutoTier = 2

	loop := NewLoop(LoopOptions{
		Bus:           msgBus,
		Provider:      &mockProvider{},
		Policy:        policyEngine,
		Workspace:     t.TempDir(),
		WorkRepo:      t.TempDir(),
		Model:         "mock-model",
		MaxIterations: 2,
	})

	ctx := context.Background()
	tests := []*bus.InboundMessage{
		{
			Channel:   "slack",
			SenderID:  "same-user",
			ChatID:    "room-a",
			ThreadID:  "thread-1",
			Content:   "a",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				bus.MetaKeyMessageType:  bus.MessageTypeExternal,
				bus.MetaKeySessionScope: "slack:room-a:thread-1",
			},
		},
		{
			Channel:   "slack",
			SenderID:  "same-user",
			ChatID:    "room-a",
			ThreadID:  "thread-2",
			Content:   "b",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				bus.MetaKeyMessageType:  bus.MessageTypeExternal,
				bus.MetaKeySessionScope: "slack:room-a:thread-2",
			},
		},
		{
			Channel:   "slack",
			SenderID:  "same-user",
			ChatID:    "room-b",
			Content:   "c",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				bus.MetaKeyMessageType:  bus.MessageTypeExternal,
				bus.MetaKeySessionScope: "slack:room-b",
			},
		},
		{
			Channel:   "msteams",
			SenderID:  "same-user",
			ChatID:    "room-a",
			Content:   "d",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				bus.MetaKeyMessageType:  bus.MessageTypeExternal,
				bus.MetaKeySessionScope: "msteams:room-a",
			},
		},
	}
	for i, msg := range tests {
		if _, _, err := loop.processMessage(ctx, msg); err != nil {
			t.Fatalf("process message %d: %v", i, err)
		}
	}

	infos := loop.sessions.List()
	if len(infos) != 4 {
		t.Fatalf("expected 4 isolated sessions, got %d", len(infos))
	}
}

func TestProcessMessageAntiLeakageMatrixIncludesWhatsApp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	msgBus := bus.NewMessageBus()
	policyEngine := policy.NewDefaultEngine()
	policyEngine.MaxAutoTier = 2

	loop := NewLoop(LoopOptions{
		Bus:           msgBus,
		Provider:      &mockProvider{},
		Policy:        policyEngine,
		Workspace:     t.TempDir(),
		WorkRepo:      t.TempDir(),
		Model:         "mock-model",
		MaxIterations: 2,
	})

	ctx := context.Background()
	tests := []*bus.InboundMessage{
		{
			Channel:   "whatsapp",
			SenderID:  "alice@s.whatsapp.net",
			ChatID:    "grp-1@g.us",
			Content:   "wa room 1",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				bus.MetaKeyMessageType:  bus.MessageTypeExternal,
				bus.MetaKeySessionScope: "whatsapp:default:grp-1@g.us",
			},
		},
		{
			Channel:   "whatsapp",
			SenderID:  "alice@s.whatsapp.net",
			ChatID:    "grp-2@g.us",
			Content:   "wa room 2",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				bus.MetaKeyMessageType:  bus.MessageTypeExternal,
				bus.MetaKeySessionScope: "whatsapp:default:grp-2@g.us",
			},
		},
		{
			Channel:   "slack",
			SenderID:  "U1",
			ChatID:    "grp-1@g.us",
			Content:   "slack same id text",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				bus.MetaKeyMessageType:  bus.MessageTypeExternal,
				bus.MetaKeySessionScope: "slack:default:grp-1@g.us",
			},
		},
		{
			Channel:   "msteams",
			SenderID:  "A1",
			ChatID:    "grp-1@g.us",
			ThreadID:  "th-1",
			Content:   "teams same id text",
			Timestamp: time.Now(),
			Metadata: map[string]any{
				bus.MetaKeyMessageType:  bus.MessageTypeExternal,
				bus.MetaKeySessionScope: "msteams:default:grp-1@g.us:th-1",
			},
		},
	}
	for i, msg := range tests {
		if _, _, err := loop.processMessage(ctx, msg); err != nil {
			t.Fatalf("process message %d: %v", i, err)
		}
	}

	infos := loop.sessions.List()
	if len(infos) != 4 {
		t.Fatalf("expected 4 isolated sessions, got %d", len(infos))
	}
	want := map[string]bool{
		"whatsapp:default:grp-1@g.us":     false,
		"whatsapp:default:grp-2@g.us":     false,
		"slack:default:grp-1@g.us":        false,
		"msteams:default:grp-1@g.us:th-1": false,
	}
	for _, info := range infos {
		if _, ok := want[info.Key]; ok {
			want[info.Key] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Fatalf("expected isolated session key %q in %+v", k, infos)
		}
	}
}
