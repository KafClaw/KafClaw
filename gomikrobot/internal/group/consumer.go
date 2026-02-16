package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/timeline"
)

// Consumer reads messages from Kafka topics.
type Consumer interface {
	// Start begins consuming from the configured topics.
	Start(ctx context.Context) error
	// Messages returns a channel of raw messages.
	Messages() <-chan ConsumerMessage
	// Subscribe dynamically adds a topic to consume from.
	Subscribe(topic string) error
	// Close stops the consumer.
	Close() error
}

// ConsumerMessage is a raw message from Kafka.
type ConsumerMessage struct {
	Topic string
	Key   []byte
	Value []byte
}

// OrchestratorHandler is a callback for orchestrator discovery messages.
type OrchestratorHandler func(env *GroupEnvelope)

// GroupRouter routes incoming Kafka messages to the appropriate handler.
type GroupRouter struct {
	manager        *Manager
	msgBus         *bus.MessageBus
	consumer       Consumer
	topics         TopicNames
	extTopics      ExtendedTopicNames
	skillPrefix    string
	orchHandler    OrchestratorHandler
}

// NewGroupRouter creates a router that bridges Kafka messages into the bus.
func NewGroupRouter(manager *Manager, msgBus *bus.MessageBus, consumer Consumer) *GroupRouter {
	return &GroupRouter{
		manager:     manager,
		msgBus:      msgBus,
		consumer:    consumer,
		topics:      manager.Topics(),
		extTopics:   ExtendedTopics(manager.GroupName()),
		skillPrefix: SkillTopicPrefix(manager.GroupName()),
	}
}

// SetOrchestratorHandler registers a callback for orchestrator discovery messages.
func (r *GroupRouter) SetOrchestratorHandler(h OrchestratorHandler) {
	r.orchHandler = h
}

// Run starts consuming and routing messages. Blocks until context is cancelled.
func (r *GroupRouter) Run(ctx context.Context) error {
	if err := r.consumer.Start(ctx); err != nil {
		return fmt.Errorf("group router: start consumer: %w", err)
	}
	defer r.consumer.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-r.consumer.Messages():
			if !ok {
				return nil
			}
			r.handleMessage(msg)
		}
	}
}

func (r *GroupRouter) handleMessage(msg ConsumerMessage) {
	var env GroupEnvelope
	if err := json.Unmarshal(msg.Value, &env); err != nil {
		slog.Warn("GroupRouter: unmarshal envelope", "error", err, "topic", msg.Topic)
		return
	}

	// Log topic message for analytics (before filtering own messages)
	if r.manager.timeline != nil {
		_ = r.manager.timeline.LogTopicMessage(&timeline.TopicMessageLogRecord{
			TopicName:     msg.Topic,
			SenderID:      env.SenderID,
			EnvelopeType:  env.Type,
			CorrelationID: env.CorrelationID,
			PayloadSize:   len(msg.Value),
		})
	}

	// Skip our own messages
	if env.SenderID == r.manager.identity.AgentID {
		return
	}

	switch msg.Topic {
	case r.topics.Announce:
		r.manager.HandleAnnounce(&env)

	case r.topics.Requests:
		r.handleTaskRequest(&env)

	case r.topics.Responses:
		r.handleTaskResponse(&env)

	case r.topics.Traces:
		r.handleTrace(&env)

	case r.extTopics.ControlOnboarding:
		r.manager.HandleOnboard(&env)

	case r.extTopics.ControlRoster:
		r.handleRoster(&env)

	case r.extTopics.TaskStatus:
		r.handleTaskStatus(&env)

	case r.extTopics.MemoryShared, r.extTopics.MemoryContext:
		r.manager.HandleMemoryItem(&env)

	case r.extTopics.ObserveAudit:
		r.handleAudit(&env)

	case r.extTopics.Orchestrator:
		if r.orchHandler != nil {
			r.orchHandler(&env)
		}

	default:
		// Check for skill topic pattern
		if strings.HasPrefix(msg.Topic, r.skillPrefix) {
			r.handleSkillMessage(msg.Topic, &env)
			return
		}
		slog.Debug("GroupRouter: unknown topic", "topic", msg.Topic)
	}
}

func (r *GroupRouter) handleTaskRequest(env *GroupEnvelope) {
	data, err := json.Marshal(env.Payload)
	if err != nil {
		return
	}
	var payload TaskRequestPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}

	// Route into the agent's inbound bus as a "group" channel message
	r.msgBus.PublishInbound(&bus.InboundMessage{
		Channel:        "group",
		SenderID:       payload.RequesterID,
		ChatID:         payload.TaskID,
		TraceID:        env.CorrelationID,
		IdempotencyKey: fmt.Sprintf("group:%s", payload.TaskID),
		Content:        payload.Content,
		Timestamp:      time.Now(),
		Metadata: map[string]any{
			"group_task_id":   payload.TaskID,
			"group_requester": payload.RequesterID,
			"description":     payload.Description,
		},
	})

	slog.Info("GroupRouter: task request routed to bus",
		"task_id", payload.TaskID, "from", payload.RequesterID)
}

func (r *GroupRouter) handleTaskResponse(env *GroupEnvelope) {
	data, err := json.Marshal(env.Payload)
	if err != nil {
		return
	}
	var payload TaskResponsePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}

	slog.Info("GroupRouter: task response received",
		"task_id", payload.TaskID, "from", payload.ResponderID, "status", payload.Status)

	// Route into bus as a group response
	r.msgBus.PublishInbound(&bus.InboundMessage{
		Channel:        "group",
		SenderID:       payload.ResponderID,
		ChatID:         payload.TaskID,
		TraceID:        env.CorrelationID,
		IdempotencyKey: fmt.Sprintf("group-resp:%s:%s", payload.TaskID, payload.ResponderID),
		Content:        fmt.Sprintf("[Task Response from %s] Status: %s\n%s", payload.ResponderID, payload.Status, payload.Content),
		Timestamp:      time.Now(),
	})
}

func (r *GroupRouter) handleTrace(env *GroupEnvelope) {
	data, err := json.Marshal(env.Payload)
	if err != nil {
		return
	}
	var payload TracePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}

	// Store in group_traces table if timeline is available
	if r.manager.timeline != nil {
		var startedAt, endedAt *time.Time
		if payload.StartedAt != "" {
			if t, err := time.Parse(time.RFC3339, payload.StartedAt); err == nil {
				startedAt = &t
			}
		}
		if payload.EndedAt != "" {
			if t, err := time.Parse(time.RFC3339, payload.EndedAt); err == nil {
				endedAt = &t
			}
		}

		_ = r.manager.timeline.AddGroupTrace(&timeline.GroupTrace{
			TraceID:       payload.TraceID,
			SourceAgentID: env.SenderID,
			SpanID:        payload.SpanID,
			ParentSpanID:  payload.ParentSpanID,
			SpanType:      payload.SpanType,
			Title:         payload.Title,
			Content:       payload.Content,
			StartedAt:     startedAt,
			EndedAt:       endedAt,
			DurationMs:    payload.DurationMs,
		})
	}

	slog.Debug("GroupRouter: trace stored", "trace_id", payload.TraceID, "from", env.SenderID)
}

func (r *GroupRouter) handleRoster(env *GroupEnvelope) {
	data, err := json.Marshal(env.Payload)
	if err != nil {
		return
	}
	var manifest TopicManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		slog.Warn("GroupRouter: unmarshal roster manifest", "error", err)
		return
	}
	if r.manager.topicMgr != nil {
		r.manager.topicMgr.UpdateManifest(&manifest)
	}
	slog.Info("GroupRouter: roster manifest updated", "version", manifest.Version, "from", env.SenderID)
}

func (r *GroupRouter) handleTaskStatus(env *GroupEnvelope) {
	// Route task status updates into the bus for the agent to observe
	data, err := json.Marshal(env.Payload)
	if err != nil {
		return
	}
	r.msgBus.PublishInbound(&bus.InboundMessage{
		Channel:   "group",
		SenderID:  env.SenderID,
		ChatID:    env.CorrelationID,
		TraceID:   env.CorrelationID,
		Content:   string(data),
		Timestamp: time.Now(),
		Metadata: map[string]any{
			"type": "task_status",
		},
	})
	slog.Debug("GroupRouter: task status update", "correlation_id", env.CorrelationID, "from", env.SenderID)
}

func (r *GroupRouter) handleAudit(env *GroupEnvelope) {
	data, err := json.Marshal(env.Payload)
	if err != nil {
		return
	}

	if r.manager.timeline != nil {
		_ = r.manager.timeline.AddGroupTrace(&timeline.GroupTrace{
			TraceID:       env.CorrelationID,
			SourceAgentID: env.SenderID,
			SpanType:      "AUDIT",
			Title:         "audit event",
			Content:       string(data),
			StartedAt:     &env.Timestamp,
		})
	}
	slog.Debug("GroupRouter: audit event", "from", env.SenderID)
}

func (r *GroupRouter) handleSkillMessage(topic string, env *GroupEnvelope) {
	// Extract skill name from topic: group.{name}.skill.{skill}.requests/responses
	rest := topic[len(r.skillPrefix):]
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 {
		slog.Warn("GroupRouter: malformed skill topic", "topic", topic)
		return
	}
	skillName := parts[0]
	direction := parts[1] // "requests" or "responses"

	switch direction {
	case "requests":
		data, err := json.Marshal(env.Payload)
		if err != nil {
			return
		}
		var payload TaskRequestPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		r.msgBus.PublishInbound(&bus.InboundMessage{
			Channel:        "group",
			SenderID:       payload.RequesterID,
			ChatID:         payload.TaskID,
			TraceID:        env.CorrelationID,
			IdempotencyKey: fmt.Sprintf("skill:%s:%s", skillName, payload.TaskID),
			Content:        payload.Content,
			Timestamp:      time.Now(),
			Metadata: map[string]any{
				"group_task_id":   payload.TaskID,
				"group_requester": payload.RequesterID,
				"description":     payload.Description,
				"skill":           skillName,
			},
		})
		slog.Info("GroupRouter: skill task request", "skill", skillName, "task_id", payload.TaskID)

	case "responses":
		data, err := json.Marshal(env.Payload)
		if err != nil {
			return
		}
		var payload TaskResponsePayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		r.msgBus.PublishInbound(&bus.InboundMessage{
			Channel:        "group",
			SenderID:       payload.ResponderID,
			ChatID:         payload.TaskID,
			TraceID:        env.CorrelationID,
			IdempotencyKey: fmt.Sprintf("skill-resp:%s:%s:%s", skillName, payload.TaskID, payload.ResponderID),
			Content:        fmt.Sprintf("[Skill %s Response from %s] Status: %s\n%s", skillName, payload.ResponderID, payload.Status, payload.Content),
			Timestamp:      time.Now(),
		})
		slog.Info("GroupRouter: skill task response", "skill", skillName, "task_id", payload.TaskID)

	default:
		slog.Warn("GroupRouter: unknown skill direction", "topic", topic, "direction", direction)
	}
}
