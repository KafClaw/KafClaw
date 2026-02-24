package group

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/config"
)

type fakeKnowledgeHandler struct {
	mu    sync.Mutex
	calls int
	last  string
}

func (f *fakeKnowledgeHandler) Process(topic string, _ []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = topic
	return nil
}

func (f *fakeKnowledgeHandler) Snapshot() (int, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.last
}

func TestGroupRouter_RouteAnnounce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	cfg := config.GroupConfig{
		Enabled:     true,
		GroupName:   "test",
		LFSProxyURL: server.URL,
	}
	identity := AgentIdentity{
		AgentID:   "local-agent",
		AgentName: "LocalBot",
		Status:    "active",
	}

	mgr := NewManager(cfg, nil, identity)
	msgBus := bus.NewMessageBus()
	consumer := NewChannelConsumer()
	router := NewGroupRouter(mgr, msgBus, consumer)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		router.Run(ctx)
	}()

	// Give the router time to start
	time.Sleep(10 * time.Millisecond)

	// Send an announce from a remote agent
	env := GroupEnvelope{
		Type:     EnvelopeAnnounce,
		SenderID: "remote-agent",
		Payload: AnnouncePayload{
			Action: "join",
			Identity: AgentIdentity{
				AgentID:   "remote-agent",
				AgentName: "RemoteBot",
				Status:    "active",
			},
		},
	}
	data, _ := json.Marshal(env)
	consumer.Send(ConsumerMessage{
		Topic: mgr.Topics().Announce,
		Value: data,
	})

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	members := mgr.Members()
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].AgentID != "remote-agent" {
		t.Errorf("expected remote-agent, got %s", members[0].AgentID)
	}

	cancel()
}

func TestGroupRouter_RouteTaskRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	cfg := config.GroupConfig{
		Enabled:     true,
		GroupName:   "test",
		LFSProxyURL: server.URL,
	}
	identity := AgentIdentity{AgentID: "local-agent"}
	mgr := NewManager(cfg, nil, identity)
	msgBus := bus.NewMessageBus()
	consumer := NewChannelConsumer()
	router := NewGroupRouter(mgr, msgBus, consumer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start consuming inbound
	receivedCh := make(chan *bus.InboundMessage, 1)
	go func() {
		msg, _ := msgBus.ConsumeInbound(ctx)
		if msg != nil {
			receivedCh <- msg
		}
	}()

	go func() {
		router.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	// Send a task request
	env := GroupEnvelope{
		Type:          EnvelopeRequest,
		CorrelationID: "trace-abc",
		SenderID:      "remote-agent",
		Payload: TaskRequestPayload{
			TaskID:      "task-1",
			Description: "Help with code",
			Content:     "Please review this code",
			RequesterID: "remote-agent",
		},
	}
	data, _ := json.Marshal(env)
	consumer.Send(ConsumerMessage{
		Topic: mgr.Topics().Requests,
		Value: data,
	})

	var received *bus.InboundMessage
	select {
	case received = <-receivedCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected inbound message on bus")
	}
	if received.Channel != "group" {
		t.Errorf("expected channel group, got %s", received.Channel)
	}
	if received.Content != "Please review this code" {
		t.Errorf("unexpected content: %s", received.Content)
	}
}

func TestGroupRouter_SkipOwnMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	cfg := config.GroupConfig{
		Enabled:     true,
		GroupName:   "test",
		LFSProxyURL: server.URL,
	}
	identity := AgentIdentity{AgentID: "local-agent"}
	mgr := NewManager(cfg, nil, identity)
	msgBus := bus.NewMessageBus()
	consumer := NewChannelConsumer()
	router := NewGroupRouter(mgr, msgBus, consumer)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		router.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	// Send a message from our own agent - should be skipped
	env := GroupEnvelope{
		Type:     EnvelopeAnnounce,
		SenderID: "local-agent", // same as our ID
		Payload: AnnouncePayload{
			Action:   "join",
			Identity: AgentIdentity{AgentID: "local-agent"},
		},
	}
	data, _ := json.Marshal(env)
	consumer.Send(ConsumerMessage{
		Topic: mgr.Topics().Announce,
		Value: data,
	})

	time.Sleep(50 * time.Millisecond)

	// Should not have added to roster since it's our own message
	if mgr.MemberCount() != 0 {
		t.Errorf("expected 0 members (own message skipped), got %d", mgr.MemberCount())
	}

	cancel()
}

func TestChannelConsumer(t *testing.T) {
	c := NewChannelConsumer()
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	go func() {
		c.Send(ConsumerMessage{Topic: "test", Value: []byte("hello")})
	}()

	msg := <-c.Messages()
	if msg.Topic != "test" {
		t.Errorf("expected topic test, got %s", msg.Topic)
	}
	if string(msg.Value) != "hello" {
		t.Errorf("expected value hello, got %s", string(msg.Value))
	}
}

func TestGroupRouter_RouteKnowledgeTopic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	cfg := config.GroupConfig{Enabled: true, GroupName: "test", LFSProxyURL: server.URL}
	mgr := NewManager(cfg, nil, AgentIdentity{AgentID: "local-agent"})
	msgBus := bus.NewMessageBus()
	consumer := NewChannelConsumer()
	router := NewGroupRouter(mgr, msgBus, consumer)

	fh := &fakeKnowledgeHandler{}
	kTopic := "group.test.knowledge.proposals"
	router.SetKnowledgeHandler(fh, []string{kTopic})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = router.Run(ctx) }()
	time.Sleep(10 * time.Millisecond)

	consumer.Send(ConsumerMessage{
		Topic: kTopic,
		Value: []byte(`{"schemaVersion":"v1","type":"proposal","traceId":"t1","timestamp":"2026-02-24T00:00:00Z","idempotencyKey":"i1","clawId":"remote","instanceId":"n1","payload":{"proposalId":"p1","group":"g","statement":"s"}}`),
	})
	time.Sleep(50 * time.Millisecond)

	calls, last := fh.Snapshot()
	if calls != 1 {
		t.Fatalf("expected knowledge handler call count 1, got %d", calls)
	}
	if last != kTopic {
		t.Fatalf("expected knowledge handler topic %s, got %s", kTopic, last)
	}
}
