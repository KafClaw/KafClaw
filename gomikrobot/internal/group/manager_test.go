package group

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kamir/gomikrobot/internal/config"
)

func newTestManager(serverURL string) *Manager {
	cfg := config.GroupConfig{
		Enabled:        true,
		GroupName:      "test-group",
		LFSProxyURL:    serverURL,
		PollIntervalMs: 100,
	}
	identity := AgentIdentity{
		AgentID:      "test-agent",
		AgentName:    "TestBot",
		SoulSummary:  "A test bot",
		Capabilities: []string{"read_file", "write_file"},
		Channels:     []string{"cli"},
		Model:        "gpt-4o",
		Status:       "active",
	}
	return NewManager(cfg, nil, identity)
}

func TestManager_JoinLeave(t *testing.T) {
	var produced []GroupEnvelope

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env GroupEnvelope
		json.NewDecoder(r.Body).Decode(&env)
		produced = append(produced, env)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	m := newTestManager(server.URL)

	// Join
	if err := m.Join(context.Background()); err != nil {
		t.Fatalf("Join failed: %v", err)
	}
	if !m.Active() {
		t.Fatal("expected active after join")
	}
	if len(produced) != 1 {
		t.Fatalf("expected 1 produced message, got %d", len(produced))
	}
	if produced[0].Type != EnvelopeAnnounce {
		t.Errorf("expected announce type, got %s", produced[0].Type)
	}

	// Double join should fail
	if err := m.Join(context.Background()); err == nil {
		t.Fatal("expected error on double join")
	}

	// Leave
	if err := m.Leave(context.Background()); err != nil {
		t.Fatalf("Leave failed: %v", err)
	}
	if m.Active() {
		t.Fatal("expected inactive after leave")
	}
}

func TestManager_HandleAnnounce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	m := newTestManager(server.URL)

	// Simulate a join announce from another agent
	env := &GroupEnvelope{
		Type:     EnvelopeAnnounce,
		SenderID: "remote-agent",
		Payload: AnnouncePayload{
			Action: "join",
			Identity: AgentIdentity{
				AgentID:      "remote-agent",
				AgentName:    "RemoteBot",
				Capabilities: []string{"exec"},
				Status:       "active",
			},
		},
	}

	// Need to marshal/unmarshal to simulate wire format
	data, _ := json.Marshal(env.Payload)
	var raw any
	json.Unmarshal(data, &raw)
	env.Payload = raw

	m.HandleAnnounce(env)

	members := m.Members()
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].AgentID != "remote-agent" {
		t.Errorf("expected remote-agent, got %s", members[0].AgentID)
	}

	// Simulate leave
	leaveEnv := &GroupEnvelope{
		Type:     EnvelopeAnnounce,
		SenderID: "remote-agent",
		Payload: AnnouncePayload{
			Action: "leave",
			Identity: AgentIdentity{
				AgentID: "remote-agent",
			},
		},
	}
	data, _ = json.Marshal(leaveEnv.Payload)
	json.Unmarshal(data, &raw)
	leaveEnv.Payload = raw

	m.HandleAnnounce(leaveEnv)
	if m.MemberCount() != 0 {
		t.Errorf("expected 0 members after leave, got %d", m.MemberCount())
	}
}

func TestManager_Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	m := newTestManager(server.URL)
	status := m.Status()

	if status["active"].(bool) {
		t.Error("expected inactive")
	}
	if status["group_name"] != "test-group" {
		t.Errorf("expected group name test-group, got %v", status["group_name"])
	}
	if !status["lfs_healthy"].(bool) {
		t.Error("expected lfs healthy (400 is considered healthy)")
	}
}
