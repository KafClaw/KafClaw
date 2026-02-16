// Package group implements multi-agent collaboration via Kafka.
package group

import (
	"fmt"
	"time"
)

// AgentIdentity describes this agent's capabilities for group discovery.
type AgentIdentity struct {
	AgentID      string   `json:"agent_id"`
	AgentName    string   `json:"agent_name"`
	SoulSummary  string   `json:"soul_summary"`
	Capabilities []string `json:"capabilities"`
	Channels     []string `json:"channels"`
	Model        string   `json:"model"`
	JoinedAt     string   `json:"joined_at"`
	Status       string   `json:"status"`
	ParentID     string   `json:"parent_id,omitempty"`
	ZoneID       string   `json:"zone_id,omitempty"`
	Endpoint     string   `json:"endpoint,omitempty"`
	Role         string   `json:"role,omitempty"` // "orchestrator", "worker", "observer"
}

// GroupEnvelope is the wire format for all Kafka group messages.
type GroupEnvelope struct {
	Type          string    `json:"type"`
	CorrelationID string    `json:"correlation_id"`
	SenderID      string    `json:"sender_id"`
	Timestamp     time.Time `json:"timestamp"`
	Payload       any       `json:"payload"`
}

// Envelope type constants.
const (
	EnvelopeAnnounce  = "announce"
	EnvelopeRequest   = "request"
	EnvelopeResponse  = "response"
	EnvelopeTrace     = "trace"
	EnvelopeHeartbeat = "heartbeat"

	// Extended envelope types
	EnvelopeOnboard       = "onboard"
	EnvelopeMemory        = "memory"
	EnvelopeSkillRequest  = "skill_request"
	EnvelopeSkillResponse = "skill_response"
	EnvelopeAudit         = "audit"
	EnvelopeTaskStatus    = "task_status"
	EnvelopeRoster        = "roster"
)

// AnnouncePayload is sent on join/leave/heartbeat.
type AnnouncePayload struct {
	Action   string        `json:"action"` // "join", "leave", "heartbeat"
	Identity AgentIdentity `json:"identity"`
}

// TaskRequestPayload is a task request from one agent to the group.
type TaskRequestPayload struct {
	TaskID              string `json:"task_id"`
	Description         string `json:"description"`
	Content             string `json:"content"`
	RequesterID         string `json:"requester_id"`
	ParentTaskID        string `json:"parent_task_id,omitempty"`
	DelegationDepth     int    `json:"delegation_depth,omitempty"`
	OriginalRequesterID string `json:"original_requester_id,omitempty"`
	DeadlineAt          string `json:"deadline_at,omitempty"` // RFC3339
}

// TaskResponsePayload is a task response from an agent.
type TaskResponsePayload struct {
	TaskID      string `json:"task_id"`
	ResponderID string `json:"responder_id"`
	Content     string `json:"content"`
	Status      string `json:"status"` // "completed", "failed", "rejected"
}

// TaskStatusPayload reports task status changes (accepted, progress, etc.).
type TaskStatusPayload struct {
	TaskID      string `json:"task_id"`
	ResponderID string `json:"responder_id"`
	Status      string `json:"status"` // "accepted", "in_progress", "completed", "failed"
	Summary     string `json:"summary,omitempty"`
}

// DelegatedTaskRequest is the full delegation request including depth/parent info.
type DelegatedTaskRequest struct {
	TaskID              string     `json:"task_id"`
	Description         string     `json:"description"`
	Content             string     `json:"content"`
	RequesterID         string     `json:"requester_id"`
	ParentTaskID        string     `json:"parent_task_id"`
	DelegationDepth     int        `json:"delegation_depth"`
	OriginalRequesterID string     `json:"original_requester_id"`
	DeadlineAt          *time.Time `json:"deadline_at,omitempty"`
}

// TracePayload carries shared trace data between agents.
type TracePayload struct {
	TraceID      string `json:"trace_id"`
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id"`
	SpanType     string `json:"span_type"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	StartedAt    string `json:"started_at"`
	EndedAt      string `json:"ended_at"`
	DurationMs   int    `json:"duration_ms"`
}

// GroupMember represents a known member in the local roster.
type GroupMember struct {
	AgentID      string    `json:"agent_id"`
	AgentName    string    `json:"agent_name"`
	SoulSummary  string    `json:"soul_summary"`
	Capabilities []string  `json:"capabilities"`
	Channels     []string  `json:"channels"`
	Model        string    `json:"model"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	LastSeen     time.Time `json:"last_seen"`
}

// TopicNames returns the Kafka topic names for a group.
type TopicNames struct {
	Announce  string
	Requests  string
	Responses string
	Traces    string
}

// Topics returns the TopicNames for the given group name.
func Topics(groupName string) TopicNames {
	return TopicNames{
		Announce:  fmt.Sprintf("group.%s.announce", groupName),
		Requests:  fmt.Sprintf("group.%s.requests", groupName),
		Responses: fmt.Sprintf("group.%s.responses", groupName),
		Traces:    fmt.Sprintf("group.%s.traces", groupName),
	}
}
