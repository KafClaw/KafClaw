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

func newTestManagerForOnboard(serverURL string, agentID string, onboardMode string) *Manager {
	cfg := config.GroupConfig{
		Enabled:        true,
		GroupName:      "test-group",
		LFSProxyURL:    serverURL,
		PollIntervalMs: 100,
		OnboardMode:    onboardMode,
	}
	identity := AgentIdentity{
		AgentID:      agentID,
		AgentName:    "TestBot-" + agentID,
		SoulSummary:  "A test bot",
		Capabilities: []string{"read_file", "write_file"},
		Channels:     []string{"cli"},
		Model:        "gpt-4o",
		Status:       "active",
	}
	return NewManager(cfg, nil, identity)
}

func TestOnboard_OpenMode(t *testing.T) {
	var produced []GroupEnvelope

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env GroupEnvelope
		json.NewDecoder(r.Body).Decode(&env)
		produced = append(produced, env)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	// Orchestrator (existing member) - open mode
	orchestrator := newTestManagerForOnboard(server.URL, "orch-agent", "open")
	if err := orchestrator.Join(context.Background()); err != nil {
		t.Fatalf("orchestrator join failed: %v", err)
	}

	// New agent sends onboard request
	newAgent := newTestManagerForOnboard(server.URL, "new-agent", "open")
	if err := newAgent.Onboard(context.Background()); err != nil {
		t.Fatalf("onboard request failed: %v", err)
	}

	// Verify the onboard request was produced
	foundRequest := false
	for _, env := range produced {
		if env.Type == EnvelopeOnboard {
			data, _ := json.Marshal(env.Payload)
			var payload OnboardPayload
			json.Unmarshal(data, &payload)
			if payload.Action == OnboardActionRequest {
				foundRequest = true
				if payload.RequesterID != "new-agent" {
					t.Errorf("expected requester new-agent, got %s", payload.RequesterID)
				}
			}
		}
	}
	if !foundRequest {
		t.Error("expected onboard request to be produced")
	}
}

func TestOnboard_HandleRequest_OpenMode(t *testing.T) {
	var produced []GroupEnvelope

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env GroupEnvelope
		json.NewDecoder(r.Body).Decode(&env)
		produced = append(produced, env)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	// Orchestrator receives the request in open mode
	orch := newTestManagerForOnboard(server.URL, "orch-agent", "open")
	if err := orch.Join(context.Background()); err != nil {
		t.Fatalf("join failed: %v", err)
	}

	// Simulate receiving an onboard request
	requestPayload := OnboardPayload{
		Action:      OnboardActionRequest,
		RequesterID: "new-agent",
		Identity: &AgentIdentity{
			AgentID:      "new-agent",
			AgentName:    "NewBot",
			Capabilities: []string{"web_search"},
		},
		Skills: []string{"web_search"},
	}
	reqData, _ := json.Marshal(requestPayload)
	var raw any
	json.Unmarshal(reqData, &raw)

	env := &GroupEnvelope{
		Type:          EnvelopeOnboard,
		CorrelationID: "onboard-123",
		SenderID:      "new-agent",
		Timestamp:     time.Now(),
		Payload:       raw,
	}

	produced = nil // reset
	orch.HandleOnboard(env)

	// In open mode, should produce OnboardComplete directly
	foundComplete := false
	for _, env := range produced {
		if env.Type == EnvelopeOnboard {
			data, _ := json.Marshal(env.Payload)
			var payload OnboardPayload
			json.Unmarshal(data, &payload)
			if payload.Action == OnboardActionComplete {
				foundComplete = true
				if payload.RequesterID != "new-agent" {
					t.Errorf("expected requester new-agent, got %s", payload.RequesterID)
				}
				if payload.SponsorID != "orch-agent" {
					t.Errorf("expected sponsor orch-agent, got %s", payload.SponsorID)
				}
				if payload.Manifest == nil {
					t.Error("expected manifest in complete response")
				}
			}
		}
	}
	if !foundComplete {
		t.Error("expected OnboardComplete in open mode")
	}
}

func TestOnboard_HandleRequest_GatedMode(t *testing.T) {
	var produced []GroupEnvelope

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env GroupEnvelope
		json.NewDecoder(r.Body).Decode(&env)
		produced = append(produced, env)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	// Orchestrator in gated mode
	orch := newTestManagerForOnboard(server.URL, "orch-agent", "gated")
	if err := orch.Join(context.Background()); err != nil {
		t.Fatalf("join failed: %v", err)
	}

	// Simulate receiving an onboard request
	requestPayload := OnboardPayload{
		Action:      OnboardActionRequest,
		RequesterID: "new-agent",
		Skills:      []string{"web_search"},
	}
	reqData, _ := json.Marshal(requestPayload)
	var raw any
	json.Unmarshal(reqData, &raw)

	env := &GroupEnvelope{
		Type:          EnvelopeOnboard,
		CorrelationID: "onboard-456",
		SenderID:      "new-agent",
		Timestamp:     time.Now(),
		Payload:       raw,
	}

	produced = nil
	orch.HandleOnboard(env)

	// In gated mode, should produce OnboardChallenge (not Complete)
	foundChallenge := false
	for _, env := range produced {
		if env.Type == EnvelopeOnboard {
			data, _ := json.Marshal(env.Payload)
			var payload OnboardPayload
			json.Unmarshal(data, &payload)
			if payload.Action == OnboardActionChallenge {
				foundChallenge = true
				if payload.Challenge == "" {
					t.Error("expected non-empty challenge")
				}
			}
		}
	}
	if !foundChallenge {
		t.Error("expected OnboardChallenge in gated mode")
	}
}

func TestOnboard_HandleComplete(t *testing.T) {
	var produced []GroupEnvelope

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env GroupEnvelope
		json.NewDecoder(r.Body).Decode(&env)
		produced = append(produced, env)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	newAgent := newTestManagerForOnboard(server.URL, "new-agent", "open")

	// Simulate receiving OnboardComplete addressed to us
	manifest := &TopicManifest{
		GroupName: "test-group",
		Version:   3,
		CoreTopics: []TopicDescriptor{
			{Name: "group.test-group.announce", Category: "control"},
		},
	}
	completePayload := OnboardPayload{
		Action:      OnboardActionComplete,
		RequesterID: "new-agent",
		SponsorID:   "orch-agent",
		Manifest:    manifest,
	}
	data, _ := json.Marshal(completePayload)
	var raw any
	json.Unmarshal(data, &raw)

	env := &GroupEnvelope{
		Type:          EnvelopeOnboard,
		CorrelationID: "onboard-789",
		SenderID:      "orch-agent",
		Timestamp:     time.Now(),
		Payload:       raw,
	}

	newAgent.HandleOnboard(env)

	// Should have received the manifest
	updatedManifest := newAgent.topicMgr.Manifest()
	if updatedManifest.Version != 3 {
		t.Errorf("expected manifest version 3, got %d", updatedManifest.Version)
	}

	// Should have auto-joined
	if !newAgent.Active() {
		t.Error("expected agent to be active after onboard complete")
	}
}

func TestOnboard_HandleReject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	newAgent := newTestManagerForOnboard(server.URL, "new-agent", "open")

	// Simulate receiving rejection
	rejectPayload := OnboardPayload{
		Action:      OnboardActionReject,
		RequesterID: "new-agent",
		SponsorID:   "orch-agent",
		Reason:      "insufficient capabilities",
	}
	data, _ := json.Marshal(rejectPayload)
	var raw any
	json.Unmarshal(data, &raw)

	env := &GroupEnvelope{
		Type:          EnvelopeOnboard,
		CorrelationID: "onboard-rej",
		SenderID:      "orch-agent",
		Timestamp:     time.Now(),
		Payload:       raw,
	}

	// Should not panic; just logs warning
	newAgent.HandleOnboard(env)

	if newAgent.Active() {
		t.Error("expected agent NOT to be active after rejection")
	}
}

func TestOnboard_RouterIntegration(t *testing.T) {
	var produced []GroupEnvelope

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env GroupEnvelope
		json.NewDecoder(r.Body).Decode(&env)
		produced = append(produced, env)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LFSEnvelope{KfsLFS: 1})
	}))
	defer server.Close()

	// Set up orchestrator with router
	orch := newTestManagerForOnboard(server.URL, "orch-agent", "open")
	if err := orch.Join(context.Background()); err != nil {
		t.Fatalf("join failed: %v", err)
	}

	msgBus := bus.NewMessageBus()
	consumer := NewChannelConsumer()
	router := NewGroupRouter(orch, msgBus, consumer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go router.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Send onboard request via the router
	requestPayload := OnboardPayload{
		Action:      OnboardActionRequest,
		RequesterID: "newcomer",
		Skills:      []string{"shell"},
	}
	reqData, _ := json.Marshal(requestPayload)
	var raw any
	json.Unmarshal(reqData, &raw)

	env := GroupEnvelope{
		Type:          EnvelopeOnboard,
		CorrelationID: "onboard-router",
		SenderID:      "newcomer",
		Timestamp:     time.Now(),
		Payload:       raw,
	}
	data, _ := json.Marshal(env)

	ext := ExtendedTopics("test-group")
	produced = nil
	consumer.Send(ConsumerMessage{
		Topic: ext.ControlOnboarding,
		Value: data,
	})

	time.Sleep(100 * time.Millisecond)

	// Verify an OnboardComplete was produced
	foundComplete := false
	for _, env := range produced {
		if env.Type == EnvelopeOnboard {
			pData, _ := json.Marshal(env.Payload)
			var payload OnboardPayload
			json.Unmarshal(pData, &payload)
			if payload.Action == OnboardActionComplete {
				foundComplete = true
			}
		}
	}
	if !foundComplete {
		t.Error("expected OnboardComplete via router in open mode")
	}
}
