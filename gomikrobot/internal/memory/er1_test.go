package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFormatER1Memory(t *testing.T) {
	m := er1Memory{
		ID:         "mem-1",
		Transcript: "Went to the park with Sarah",
		Tags:       []string{"personal", "outdoor"},
		LocationLat: 52.5200,
		LocationLon: 13.4050,
		Description: "Weekend walk",
	}

	result := formatER1Memory(m)

	if !containsStr(result, "Tags: personal, outdoor") {
		t.Errorf("expected tags in output: %s", result)
	}
	if !containsStr(result, "52.5200") {
		t.Errorf("expected location in output: %s", result)
	}
	if !containsStr(result, "Weekend walk") {
		t.Errorf("expected description in output: %s", result)
	}
	if !containsStr(result, "Went to the park with Sarah") {
		t.Errorf("expected transcript in output: %s", result)
	}
}

func TestFormatER1MemoryMinimal(t *testing.T) {
	m := er1Memory{
		Transcript: "Simple note",
	}
	result := formatER1Memory(m)
	if result != "Simple note" {
		t.Errorf("expected just transcript, got: %s", result)
	}
}

func TestNewER1ClientNil(t *testing.T) {
	// Empty URL
	c := NewER1Client(ER1Config{URL: ""}, &MemoryService{})
	if c != nil {
		t.Error("expected nil client with empty URL")
	}

	// Nil service
	c = NewER1Client(ER1Config{URL: "http://localhost:8080"}, nil)
	if c != nil {
		t.Error("expected nil client with nil service")
	}
}

func TestER1Authenticate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/access" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("X-API-KEY") != "test-key" {
			t.Errorf("unexpected API key: %s", r.Header.Get("X-API-KEY"))
		}

		json.NewEncoder(w).Encode(er1AccessResponse{
			CtxID: "ctx-123",
			Tier:  "pro",
		})
	}))
	defer server.Close()

	svc := &MemoryService{} // mock, no actual store needed
	c := NewER1Client(ER1Config{
		URL:    server.URL,
		APIKey: "test-key",
		UserID: "user-1",
	}, svc)

	err := c.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}
	if c.ctxID != "ctx-123" {
		t.Fatalf("expected ctx-123, got %s", c.ctxID)
	}
}

func TestER1FetchMemories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/access" {
			json.NewEncoder(w).Encode(er1AccessResponse{CtxID: "ctx-1"})
			return
		}
		if r.URL.Path == "/memory/ctx-1" {
			json.NewEncoder(w).Encode(er1MemoryListResponse{
				Memories: []er1Memory{
					{ID: "m1", Transcript: "Test memory", TranscriptStatus: "processed", Tags: []string{"test"}},
					{ID: "m2", Transcript: "", TranscriptStatus: "pending"}, // no transcript
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	svc := &MemoryService{} // mock
	c := NewER1Client(ER1Config{URL: server.URL, UserID: "u1"}, svc)
	c.Authenticate(context.Background())

	memories, err := c.FetchMemories(context.Background())
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(memories))
	}
	if memories[0].ID != "m1" {
		t.Errorf("expected m1, got %s", memories[0].ID)
	}
}

func TestER1FetchNotAuthenticated(t *testing.T) {
	svc := &MemoryService{}
	c := NewER1Client(ER1Config{URL: "http://localhost:9999"}, svc)

	_, err := c.FetchMemories(context.Background())
	if err == nil {
		t.Error("expected error when not authenticated")
	}
}

func TestER1NilClient(t *testing.T) {
	var c *ER1Client

	n, err := c.SyncOnce(context.Background())
	if err != nil || n != 0 {
		t.Error("nil client SyncOnce should be no-op")
	}

	if err := c.Authenticate(context.Background()); err != nil {
		t.Error("nil client Authenticate should be no-op")
	}
}
