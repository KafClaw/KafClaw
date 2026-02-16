package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// OnboardAction is the action type for onboarding messages.
type OnboardAction string

const (
	OnboardActionRequest   OnboardAction = "onboard_request"
	OnboardActionChallenge OnboardAction = "onboard_challenge"
	OnboardActionResponse  OnboardAction = "onboard_response"
	OnboardActionComplete  OnboardAction = "onboard_complete"
	OnboardActionReject    OnboardAction = "onboard_reject"
)

// OnboardMode controls whether onboarding requires a challenge.
type OnboardMode string

const (
	OnboardModeOpen  OnboardMode = "open"  // Auto-accept, skip challenge
	OnboardModeGated OnboardMode = "gated" // Full 4-step handshake with challenge
)

// OnboardPayload is the wire format for onboarding protocol messages.
type OnboardPayload struct {
	Action      OnboardAction  `json:"action"`
	RequesterID string         `json:"requester_id"`
	SponsorID   string         `json:"sponsor_id,omitempty"`
	Identity    *AgentIdentity `json:"identity,omitempty"`
	Skills      []string       `json:"skills,omitempty"`
	Challenge   string         `json:"challenge,omitempty"`
	Response    string         `json:"response,omitempty"`
	Manifest    *TopicManifest `json:"manifest,omitempty"`
	Reason      string         `json:"reason,omitempty"`
}

// Onboard initiates the onboarding protocol by sending an OnboardRequest to the group.
func (m *Manager) Onboard(ctx context.Context) error {
	if m.Active() {
		return fmt.Errorf("already active in group %s", m.cfg.GroupName)
	}

	ext := ExtendedTopics(m.cfg.GroupName)

	env := &GroupEnvelope{
		Type:          EnvelopeOnboard,
		CorrelationID: fmt.Sprintf("onboard-%d", time.Now().UnixNano()),
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: OnboardPayload{
			Action:      OnboardActionRequest,
			RequesterID: m.identity.AgentID,
			Identity:    &m.identity,
			Skills:      m.identity.Capabilities,
		},
	}

	if err := m.lfs.ProduceEnvelope(ctx, ext.ControlOnboarding, env); err != nil {
		return fmt.Errorf("onboard request failed: %w", err)
	}

	slog.Info("Onboard request sent", "group", m.cfg.GroupName, "agent_id", m.identity.AgentID)
	return nil
}

// HandleOnboard processes incoming onboarding messages.
func (m *Manager) HandleOnboard(env *GroupEnvelope) {
	data, err := json.Marshal(env.Payload)
	if err != nil {
		slog.Warn("HandleOnboard: marshal payload", "error", err)
		return
	}
	var payload OnboardPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		slog.Warn("HandleOnboard: unmarshal payload", "error", err)
		return
	}

	switch payload.Action {
	case OnboardActionRequest:
		m.handleOnboardRequest(env, &payload)
	case OnboardActionChallenge:
		m.handleOnboardChallenge(env, &payload)
	case OnboardActionResponse:
		m.handleOnboardResponse(env, &payload)
	case OnboardActionComplete:
		m.handleOnboardComplete(env, &payload)
	case OnboardActionReject:
		m.handleOnboardReject(env, &payload)
	default:
		slog.Warn("HandleOnboard: unknown action", "action", payload.Action)
	}
}

// handleOnboardRequest processes an incoming onboard request from a new agent.
// In open mode, responds immediately with OnboardComplete + manifest.
// In gated mode, sends a challenge.
func (m *Manager) handleOnboardRequest(env *GroupEnvelope, payload *OnboardPayload) {
	if !m.Active() {
		return
	}

	slog.Info("Onboard request received", "from", payload.RequesterID,
		"skills", payload.Skills)

	mode := m.onboardMode()

	ext := ExtendedTopics(m.cfg.GroupName)
	ctx := context.Background()

	if mode == OnboardModeGated {
		// Send challenge
		challenge := m.generateChallenge(payload)
		resp := &GroupEnvelope{
			Type:          EnvelopeOnboard,
			CorrelationID: env.CorrelationID,
			SenderID:      m.identity.AgentID,
			Timestamp:     time.Now(),
			Payload: OnboardPayload{
				Action:      OnboardActionChallenge,
				RequesterID: payload.RequesterID,
				SponsorID:   m.identity.AgentID,
				Challenge:   challenge,
			},
		}
		if err := m.lfs.ProduceEnvelope(ctx, ext.ControlOnboarding, resp); err != nil {
			slog.Warn("Onboard challenge send failed", "error", err)
		}
		return
	}

	// Open mode: auto-accept
	m.sendOnboardComplete(ctx, env.CorrelationID, payload.RequesterID)
}

// handleOnboardChallenge processes a challenge sent to us during gated onboarding.
func (m *Manager) handleOnboardChallenge(env *GroupEnvelope, payload *OnboardPayload) {
	// Only respond if the challenge is addressed to us
	if payload.RequesterID != m.identity.AgentID {
		return
	}

	slog.Info("Onboard challenge received", "from", payload.SponsorID,
		"challenge", payload.Challenge)

	response := m.answerChallenge(payload.Challenge)

	ext := ExtendedTopics(m.cfg.GroupName)
	ctx := context.Background()

	resp := &GroupEnvelope{
		Type:          EnvelopeOnboard,
		CorrelationID: env.CorrelationID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: OnboardPayload{
			Action:      OnboardActionResponse,
			RequesterID: m.identity.AgentID,
			SponsorID:   payload.SponsorID,
			Identity:    &m.identity,
			Skills:      m.identity.Capabilities,
			Response:    response,
		},
	}
	if err := m.lfs.ProduceEnvelope(ctx, ext.ControlOnboarding, resp); err != nil {
		slog.Warn("Onboard response send failed", "error", err)
	}
}

// handleOnboardResponse processes a challenge response from a prospective member.
func (m *Manager) handleOnboardResponse(env *GroupEnvelope, payload *OnboardPayload) {
	// Only the sponsor processes the response
	if payload.SponsorID != m.identity.AgentID {
		return
	}

	slog.Info("Onboard response received", "from", payload.RequesterID,
		"response", payload.Response)

	ctx := context.Background()

	// Validate response (basic: non-empty response = accepted)
	if payload.Response == "" {
		m.sendOnboardReject(ctx, env.CorrelationID, payload.RequesterID, "empty challenge response")
		return
	}

	m.sendOnboardComplete(ctx, env.CorrelationID, payload.RequesterID)
}

// handleOnboardComplete processes the welcome message after successful onboarding.
func (m *Manager) handleOnboardComplete(env *GroupEnvelope, payload *OnboardPayload) {
	// Only the requester processes their own complete message
	if payload.RequesterID != m.identity.AgentID {
		return
	}

	slog.Info("Onboard complete! Welcome to group",
		"sponsor", payload.SponsorID,
		"manifest_version", 0)

	// Store the manifest if provided
	if payload.Manifest != nil && m.topicMgr != nil {
		m.topicMgr.UpdateManifest(payload.Manifest)
		slog.Info("Received topic manifest", "version", payload.Manifest.Version,
			"core_topics", len(payload.Manifest.CoreTopics),
			"skill_topics", len(payload.Manifest.SkillTopics))
	}

	// Now proceed with standard join
	ctx := context.Background()
	if err := m.Join(ctx); err != nil {
		slog.Warn("Post-onboard join failed", "error", err)
	}
}

// handleOnboardReject processes a rejection of our onboarding request.
func (m *Manager) handleOnboardReject(env *GroupEnvelope, payload *OnboardPayload) {
	if payload.RequesterID != m.identity.AgentID {
		return
	}
	slog.Warn("Onboard rejected", "reason", payload.Reason, "by", payload.SponsorID)
}

func (m *Manager) sendOnboardComplete(ctx context.Context, correlationID, requesterID string) {
	ext := ExtendedTopics(m.cfg.GroupName)

	var manifest *TopicManifest
	if m.topicMgr != nil {
		man := m.topicMgr.Manifest()
		manifest = &man
	}

	resp := &GroupEnvelope{
		Type:          EnvelopeOnboard,
		CorrelationID: correlationID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: OnboardPayload{
			Action:      OnboardActionComplete,
			RequesterID: requesterID,
			SponsorID:   m.identity.AgentID,
			Manifest:    manifest,
		},
	}
	if err := m.lfs.ProduceEnvelope(ctx, ext.ControlOnboarding, resp); err != nil {
		slog.Warn("Onboard complete send failed", "error", err)
	}
	slog.Info("Onboard complete sent", "to", requesterID)
}

func (m *Manager) sendOnboardReject(ctx context.Context, correlationID, requesterID, reason string) {
	ext := ExtendedTopics(m.cfg.GroupName)

	resp := &GroupEnvelope{
		Type:          EnvelopeOnboard,
		CorrelationID: correlationID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: OnboardPayload{
			Action:      OnboardActionReject,
			RequesterID: requesterID,
			SponsorID:   m.identity.AgentID,
			Reason:      reason,
		},
	}
	if err := m.lfs.ProduceEnvelope(ctx, ext.ControlOnboarding, resp); err != nil {
		slog.Warn("Onboard reject send failed", "error", err)
	}
}

func (m *Manager) onboardMode() OnboardMode {
	if m.cfg.OnboardMode == string(OnboardModeGated) {
		return OnboardModeGated
	}
	return OnboardModeOpen
}

func (m *Manager) generateChallenge(payload *OnboardPayload) string {
	if len(payload.Skills) > 0 {
		return fmt.Sprintf("Describe your capabilities for: %s", payload.Skills[0])
	}
	return "Describe your agent capabilities"
}

func (m *Manager) answerChallenge(challenge string) string {
	// The agent provides a description of its capabilities as the response
	caps := ""
	for _, c := range m.identity.Capabilities {
		caps += c + ", "
	}
	return fmt.Sprintf("I am %s (%s). My capabilities: %s. Soul: %s",
		m.identity.AgentName, m.identity.AgentID, caps, m.identity.SoulSummary)
}
