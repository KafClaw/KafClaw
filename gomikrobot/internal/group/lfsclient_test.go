package group

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLFSClient_Produce(t *testing.T) {
	expectedTopic := "group.test.announce"
	expectedReqID := "trace-123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/lfs/produce" {
			t.Errorf("expected /lfs/produce, got %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Kafka-Topic"); got != expectedTopic {
			t.Errorf("expected topic %q, got %q", expectedTopic, got)
		}
		if got := r.Header.Get("X-Request-ID"); got != expectedReqID {
			t.Errorf("expected request-id %q, got %q", expectedReqID, got)
		}
		if got := r.Header.Get("X-API-Key"); got != "test-key" {
			t.Errorf("expected api key %q, got %q", "test-key", got)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{
			KfsLFS:    1,
			Bucket:    "kafscale-lfs",
			Key:       "default/group.test.announce/lfs/2026/02/14/abc",
			Size:      42,
			CreatedAt: "2026-02-14T10:00:00Z",
		})
	}))
	defer server.Close()

	client := NewLFSClient(server.URL, "test-key")
	env, err := client.Produce(context.Background(), expectedTopic, expectedReqID, []byte(`{"test": true}`))
	if err != nil {
		t.Fatalf("Produce failed: %v", err)
	}
	if env.Bucket != "kafscale-lfs" {
		t.Errorf("expected bucket kafscale-lfs, got %s", env.Bucket)
	}
	if env.Size != 42 {
		t.Errorf("expected size 42, got %d", env.Size)
	}
}

func TestLFSClient_ProduceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":"missing_topic","message":"missing topic"}`))
	}))
	defer server.Close()

	client := NewLFSClient(server.URL, "")
	_, err := client.Produce(context.Background(), "", "", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for bad request")
	}
}

func TestLFSClient_ProduceEnvelope(t *testing.T) {
	var received GroupEnvelope

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	client := NewLFSClient(server.URL, "")
	env := &GroupEnvelope{
		Type:          EnvelopeAnnounce,
		CorrelationID: "test-trace",
		SenderID:      "agent-1",
		Timestamp:     time.Now(),
		Payload: AnnouncePayload{
			Action: "join",
			Identity: AgentIdentity{
				AgentID:   "agent-1",
				AgentName: "TestBot",
				Status:    "active",
			},
		},
	}
	err := client.ProduceEnvelope(context.Background(), "group.test.announce", env)
	if err != nil {
		t.Fatalf("ProduceEnvelope failed: %v", err)
	}
	if received.Type != EnvelopeAnnounce {
		t.Errorf("expected type announce, got %s", received.Type)
	}
}

func TestLFSClient_Healthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewLFSClient(server.URL, "")
	if !client.Healthy(context.Background()) {
		t.Error("expected healthy for 400 response")
	}

	client2 := NewLFSClient("http://127.0.0.1:1", "")
	if client2.Healthy(context.Background()) {
		t.Error("expected unhealthy for unreachable server")
	}
}
