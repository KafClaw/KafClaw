package group

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/knowledge"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

// KnowledgeEnvelopeHandler processes knowledge protocol envelopes.
type KnowledgeEnvelopeHandler interface {
	Process(topic string, raw []byte) error
}

type defaultKnowledgeHandler struct {
	timeline *timeline.TimelineService
	localID  string
}

func NewKnowledgeHandler(timeSvc *timeline.TimelineService, localClawID string) KnowledgeEnvelopeHandler {
	return &defaultKnowledgeHandler{
		timeline: timeSvc,
		localID:  strings.TrimSpace(localClawID),
	}
}

func (h *defaultKnowledgeHandler) Process(topic string, raw []byte) error {
	var env knowledge.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("unmarshal knowledge envelope: %w", err)
	}
	if err := env.ValidateBase(); err != nil {
		return fmt.Errorf("validate knowledge envelope: %w", err)
	}
	if h.localID != "" && strings.EqualFold(strings.TrimSpace(env.ClawID), h.localID) {
		return nil
	}
	if h.timeline != nil {
		inserted, err := h.timeline.RecordKnowledgeIdempotency(
			env.IdempotencyKey,
			env.ClawID,
			env.InstanceID,
			env.Type,
			topic,
			env.TraceID,
		)
		if err != nil {
			return err
		}
		if !inserted {
			return nil
		}
		applyStatus := "accepted"
		applyReason := ""
		if env.Type == knowledge.TypeFact {
			status, reason, err := h.applyFactPayload(env)
			if err != nil {
				return err
			}
			applyStatus = status
			applyReason = reason
		}
		payload, _ := json.Marshal(env.Payload)
		_ = h.timeline.AddEvent(&timeline.TimelineEvent{
			EventID:        fmt.Sprintf("KNOWLEDGE_%s_%d", strings.ToUpper(env.Type), time.Now().UnixNano()),
			TraceID:        env.TraceID,
			Timestamp:      time.Now(),
			SenderID:       env.ClawID,
			SenderName:     env.InstanceID,
			EventType:      "SYSTEM",
			ContentText:    string(payload),
			Classification: "KNOWLEDGE_" + strings.ToUpper(env.Type) + "_" + strings.ToUpper(applyStatus),
			Authorized:     true,
			Metadata:       fmt.Sprintf(`{"topic":"%s","idempotencyKey":"%s","applyStatus":"%s","applyReason":"%s"}`, topic, env.IdempotencyKey, applyStatus, applyReason),
		})
	}
	slog.Debug("Knowledge envelope accepted", "type", env.Type, "claw_id", env.ClawID, "topic", topic)
	return nil
}

func (h *defaultKnowledgeHandler) applyFactPayload(env knowledge.Envelope) (status string, reason string, err error) {
	data, err := json.Marshal(env.Payload)
	if err != nil {
		return "", "", fmt.Errorf("marshal fact payload: %w", err)
	}
	var p knowledge.FactPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return "", "", fmt.Errorf("unmarshal fact payload: %w", err)
	}
	if err := p.Validate(); err != nil {
		return "", "", fmt.Errorf("validate fact payload: %w", err)
	}

	current, err := h.timeline.GetKnowledgeFactLatest(p.FactID)
	if err != nil {
		return "", "", err
	}
	var existing *knowledge.FactState
	if current != nil {
		existing = &knowledge.FactState{
			FactID:    current.FactID,
			Subject:   current.Subject,
			Predicate: current.Predicate,
			Object:    current.Object,
			Version:   current.Version,
		}
	}
	result := knowledge.EvaluateFactApply(existing, p)
	if result.Status == knowledge.FactApplyAccepted {
		rec := &timeline.KnowledgeFactRecord{
			FactID:     p.FactID,
			GroupName:  p.Group,
			Subject:    p.Subject,
			Predicate:  p.Predicate,
			Object:     p.Object,
			Version:    p.Version,
			Source:     p.Source,
			ProposalID: p.ProposalID,
			DecisionID: p.DecisionID,
			Tags:       mustJSONTags(p.Tags),
		}
		if err := h.timeline.UpsertKnowledgeFactLatest(rec); err != nil {
			return "", "", err
		}
	}
	return result.Status, result.Reason, nil
}

func mustJSONTags(tags []string) string {
	b, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(b)
}
