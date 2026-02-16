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

func TestShareMemory(t *testing.T) {
	var produced []GroupEnvelope

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env GroupEnvelope
		json.NewDecoder(r.Body).Decode(&env)
		produced = append(produced, env)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{
			KfsLFS: 1,
			Bucket: "test-bucket",
			Key:    "test-key-123",
		})
	}))
	defer server.Close()

	cfg := config.GroupConfig{
		Enabled:        true,
		GroupName:      "test-group",
		LFSProxyURL:    server.URL,
		PollIntervalMs: 100,
	}
	identity := AgentIdentity{
		AgentID:      "mem-agent",
		AgentName:    "MemBot",
		Capabilities: []string{"memory"},
		Status:       "active",
	}
	mgr := NewManager(cfg, nil, identity)
	if err := mgr.Join(context.Background()); err != nil {
		t.Fatalf("join failed: %v", err)
	}

	produced = nil
	err := mgr.ShareMemory(context.Background(), "Research notes", "text/plain",
		[]byte("These are my research notes about Go testing patterns"),
		[]string{"research", "go", "testing"})
	if err != nil {
		t.Fatalf("ShareMemory failed: %v", err)
	}

	// Should have produced at least 2 messages: one for the LFS content, one for the envelope
	if len(produced) < 2 {
		t.Fatalf("expected at least 2 produced messages, got %d", len(produced))
	}

	// Find the memory envelope
	foundMemory := false
	for _, env := range produced {
		if env.Type == EnvelopeMemory {
			foundMemory = true
			data, _ := json.Marshal(env.Payload)
			var item MemoryItem
			json.Unmarshal(data, &item)

			if item.Title != "Research notes" {
				t.Errorf("expected title 'Research notes', got '%s'", item.Title)
			}
			if item.AuthorID != "mem-agent" {
				t.Errorf("expected author mem-agent, got %s", item.AuthorID)
			}
			if item.ContentType != "text/plain" {
				t.Errorf("expected content_type text/plain, got %s", item.ContentType)
			}
			if len(item.Tags) != 3 {
				t.Errorf("expected 3 tags, got %d", len(item.Tags))
			}
		}
	}
	if !foundMemory {
		t.Error("expected memory envelope to be produced")
	}
}

func TestShareMemory_NotActive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	cfg := config.GroupConfig{
		Enabled:     true,
		GroupName:   "test-group",
		LFSProxyURL: server.URL,
	}
	mgr := NewManager(cfg, nil, AgentIdentity{AgentID: "test"})

	err := mgr.ShareMemory(context.Background(), "test", "text/plain", []byte("content"), nil)
	if err == nil {
		t.Error("expected error when not active")
	}
}

func TestHandleMemoryItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	cfg := config.GroupConfig{
		Enabled:     true,
		GroupName:   "test-group",
		LFSProxyURL: server.URL,
	}
	mgr := NewManager(cfg, nil, AgentIdentity{AgentID: "local-agent"})

	// Simulate receiving a memory item from another agent
	item := MemoryItem{
		ItemID:      "mem-item-1",
		AuthorID:    "remote-agent",
		Title:       "Shared knowledge",
		ContentType: "application/json",
		Tags:        []string{"shared", "knowledge"},
		LFSEnvelope: &LFSEnvelope{
			Bucket: "data-bucket",
			Key:    "artifacts/mem-1.json",
		},
		CreatedAt: time.Now(),
	}

	data, _ := json.Marshal(item)
	var raw any
	json.Unmarshal(data, &raw)

	env := &GroupEnvelope{
		Type:          EnvelopeMemory,
		CorrelationID: "mem-item-1",
		SenderID:      "remote-agent",
		Timestamp:     time.Now(),
		Payload:       raw,
	}

	// Should not panic even without timeline DB
	mgr.HandleMemoryItem(env)
}

func TestMemory_RouterIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	cfg := config.GroupConfig{
		Enabled:     true,
		GroupName:   "test-group",
		LFSProxyURL: server.URL,
	}
	mgr := NewManager(cfg, nil, AgentIdentity{AgentID: "local-agent"})
	msgBus := bus.NewMessageBus()
	consumer := NewChannelConsumer()
	router := NewGroupRouter(mgr, msgBus, consumer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go router.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Send a memory item via the router
	item := MemoryItem{
		ItemID:   "router-mem-1",
		AuthorID: "remote-agent",
		Title:    "Router test memory",
		Tags:     []string{"test"},
	}
	data, _ := json.Marshal(item)
	var raw any
	json.Unmarshal(data, &raw)

	env := GroupEnvelope{
		Type:          EnvelopeMemory,
		CorrelationID: "router-mem-1",
		SenderID:      "remote-agent",
		Timestamp:     time.Now(),
		Payload:       raw,
	}
	envData, _ := json.Marshal(env)

	ext := ExtendedTopics("test-group")
	consumer.Send(ConsumerMessage{
		Topic: ext.MemoryShared,
		Value: envData,
	})

	time.Sleep(50 * time.Millisecond)
	// No crash = pass (handler runs without timeline DB)
}
