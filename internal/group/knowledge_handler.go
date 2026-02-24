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
		payload, _ := json.Marshal(env.Payload)
		_ = h.timeline.AddEvent(&timeline.TimelineEvent{
			EventID:        fmt.Sprintf("KNOWLEDGE_%s_%d", strings.ToUpper(env.Type), time.Now().UnixNano()),
			TraceID:        env.TraceID,
			Timestamp:      time.Now(),
			SenderID:       env.ClawID,
			SenderName:     env.InstanceID,
			EventType:      "SYSTEM",
			ContentText:    string(payload),
			Classification: "KNOWLEDGE_" + strings.ToUpper(env.Type),
			Authorized:     true,
			Metadata:       fmt.Sprintf(`{"topic":"%s","idempotencyKey":"%s"}`, topic, env.IdempotencyKey),
		})
	}
	slog.Debug("Knowledge envelope accepted", "type", env.Type, "claw_id", env.ClawID, "topic", topic)
	return nil
}
