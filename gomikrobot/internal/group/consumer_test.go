package group

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/config"
)

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

	// Start consuming inbound
	var received *bus.InboundMessage
	go func() {
		msg, _ := msgBus.ConsumeInbound(ctx)
		received = msg
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

	// Wait for routing
	time.Sleep(50 * time.Millisecond)

	if received == nil {
		t.Fatal("expected inbound message on bus")
	}
	if received.Channel != "group" {
		t.Errorf("expected channel group, got %s", received.Channel)
	}
	if received.Content != "Please review this code" {
		t.Errorf("unexpected content: %s", received.Content)
	}

	cancel()
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
