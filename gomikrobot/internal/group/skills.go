package group

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kamir/gomikrobot/internal/timeline"
)

// RegisterSkill creates a skill channel pair and updates the topic manifest.
// It also dynamically subscribes the consumer to the new topics if provided.
func (m *Manager) RegisterSkill(ctx context.Context, skillName string, consumer Consumer) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}

	reqTopic, respTopic := SkillTopics(m.cfg.GroupName, skillName)

	// Register in topic manager
	if m.topicMgr != nil {
		m.topicMgr.AddSkillTopic(skillName, m.identity.AgentID)
	}

	// Subscribe consumer dynamically
	if consumer != nil {
		if err := consumer.Subscribe(reqTopic); err != nil {
			slog.Warn("RegisterSkill: subscribe requests failed", "skill", skillName, "error", err)
		}
		if err := consumer.Subscribe(respTopic); err != nil {
			slog.Warn("RegisterSkill: subscribe responses failed", "skill", skillName, "error", err)
		}
	}

	// Store in timeline DB
	if m.timeline != nil {
		_ = m.timeline.InsertGroupSkillChannel(&timeline.GroupSkillChannelRecord{
			SkillName:      skillName,
			GroupName:      m.cfg.GroupName,
			RequestsTopic:  reqTopic,
			ResponsesTopic: respTopic,
			RegisteredBy:   m.identity.AgentID,
		})
	}

	// Publish updated manifest to roster
	if err := m.publishManifest(ctx); err != nil {
		slog.Warn("RegisterSkill: publish manifest failed", "error", err)
	}

	// Publish audit event
	m.publishAudit(ctx, "skill_registered", map[string]any{
		"skill_name":     skillName,
		"requests_topic": reqTopic,
	})

	slog.Info("Skill registered", "skill", skillName, "requests", reqTopic, "responses", respTopic)
	return nil
}

// SubmitSkillTask sends a task request to a specific skill channel.
func (m *Manager) SubmitSkillTask(ctx context.Context, taskID, skillName, description, content string) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}

	reqTopic, _ := SkillTopics(m.cfg.GroupName, skillName)

	env := &GroupEnvelope{
		Type:          EnvelopeSkillRequest,
		CorrelationID: taskID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: TaskRequestPayload{
			TaskID:      taskID,
			Description: description,
			Content:     content,
			RequesterID: m.identity.AgentID,
		},
	}
	return m.lfs.ProduceEnvelope(ctx, reqTopic, env)
}

// RespondSkillTask sends a task response to a specific skill channel.
func (m *Manager) RespondSkillTask(ctx context.Context, taskID, skillName, content, status string) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}

	_, respTopic := SkillTopics(m.cfg.GroupName, skillName)

	env := &GroupEnvelope{
		Type:          EnvelopeSkillResponse,
		CorrelationID: taskID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: TaskResponsePayload{
			TaskID:      taskID,
			ResponderID: m.identity.AgentID,
			Content:     content,
			Status:      status,
		},
	}
	return m.lfs.ProduceEnvelope(ctx, respTopic, env)
}

// publishManifest publishes the current topic manifest to the roster topic.
func (m *Manager) publishManifest(ctx context.Context) error {
	if m.topicMgr == nil {
		return nil
	}

	manifest := m.topicMgr.Manifest()
	env := &GroupEnvelope{
		Type:          EnvelopeRoster,
		CorrelationID: fmt.Sprintf("roster-%d", time.Now().UnixNano()),
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload:       manifest,
	}
	return m.lfs.ProduceEnvelope(ctx, m.extTopics.ControlRoster, env)
}

// publishAudit sends an audit event to the observe.audit topic.
func (m *Manager) publishAudit(ctx context.Context, eventType string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	details["event_type"] = eventType
	details["agent_id"] = m.identity.AgentID

	env := &GroupEnvelope{
		Type:          EnvelopeAudit,
		CorrelationID: fmt.Sprintf("audit-%d", time.Now().UnixNano()),
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload:       details,
	}
	if err := m.lfs.ProduceEnvelope(ctx, m.extTopics.ObserveAudit, env); err != nil {
		slog.Debug("Audit publish failed", "error", err)
	}
}

// TopicManager returns the topic manager for external access (e.g. API endpoints).
func (m *Manager) TopicManager() *TopicManager {
	return m.topicMgr
}

// ExtendedTopicNames returns the extended topic names.
func (m *Manager) ExtendedTopicNames() ExtendedTopicNames {
	return m.extTopics
}
