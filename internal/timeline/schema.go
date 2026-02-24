package timeline

import (
	"time"
)

// TimelineEvent represents a single interaction in the history.
type TimelineEvent struct {
	ID             int64     `json:"id"`
	EventID        string    `json:"event_id"`           // Unique ID (e.g. WhatsApp MessageID)
	TraceID        string    `json:"trace_id"`           // End-to-end trace identifier
	SpanID         string    `json:"span_id"`            // Span identifier (optional)
	ParentSpanID   string    `json:"parent_span_id"`     // Parent span (optional)
	Timestamp      time.Time `json:"timestamp"`          // When it happened
	SenderID       string    `json:"sender_id"`          // Phone number
	SenderName     string    `json:"sender_name"`        // Display name
	EventType      string    `json:"event_type"`         // TEXT, AUDIO, IMAGE, SYSTEM
	ContentText    string    `json:"content_text"`       // The text or transcript
	MediaPath      string    `json:"media_path"`         // Path to local file if any
	VectorID       string    `json:"vector_id"`          // Qdrant ID
	Classification string    `json:"classification"`     // ABM1 Category
	Authorized     bool      `json:"authorized"`         // Whether sender is in AllowFrom list
	Metadata       string    `json:"metadata,omitempty"` // JSON blob for rich span detail
}

// WebUser represents a user identity in the Web UI.
type WebUser struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	ForceSend bool      `json:"force_send"`
	CreatedAt time.Time `json:"created_at"`
}

// WebLink maps a WebUser to a WhatsApp JID.
type WebLink struct {
	WebUserID   int64     `json:"web_user_id"`
	WhatsAppJID string    `json:"whatsapp_jid"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AgentTask represents a tracked agent processing task.
type AgentTask struct {
	ID               int64      `json:"id"`
	TaskID           string     `json:"task_id"`
	IdempotencyKey   string     `json:"idempotency_key,omitempty"`
	TraceID          string     `json:"trace_id,omitempty"`
	Channel          string     `json:"channel"`
	ChatID           string     `json:"chat_id"`
	SenderID         string     `json:"sender_id,omitempty"`
	MessageType      string     `json:"message_type,omitempty"`
	Status           string     `json:"status"`
	ContentIn        string     `json:"content_in,omitempty"`
	ContentOut       string     `json:"content_out,omitempty"`
	ErrorText        string     `json:"error_text,omitempty"`
	DeliveryStatus   string     `json:"delivery_status"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	TotalTokens      int        `json:"total_tokens"`
	ProviderID       string     `json:"provider_id,omitempty"`
	ModelName        string     `json:"model_name,omitempty"`
	DeliveryAttempts int        `json:"delivery_attempts"`
	DeliveryNextAt   *time.Time `json:"delivery_next_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

const (
	TaskStatusPending    = "pending"
	TaskStatusProcessing = "processing"
	TaskStatusCompleted  = "completed"
	TaskStatusFailed     = "failed"

	DeliveryPending = "pending"
	DeliverySent    = "sent"
	DeliveryFailed  = "failed"
	DeliverySkipped = "skipped"
)

// TraceNode represents a node in the trace graph.
type TraceNode struct {
	ID           string `json:"id"`
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id"`
	Type         string `json:"type"` // INBOUND, LLM, TOOL, OUTBOUND, POLICY
	Title        string `json:"title"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	DurationMs   int    `json:"duration_ms"`
	AgentID      string `json:"agent_id"`
	Output       string `json:"output"`
}

// TraceEdge represents an edge in the trace graph.
type TraceEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// TraceGraph is the full graph response for a trace.
type TraceGraph struct {
	Nodes           []TraceNode      `json:"nodes"`
	Edges           []TraceEdge      `json:"edges"`
	Task            map[string]any   `json:"task"`
	PolicyDecisions []map[string]any `json:"policy_decisions"`
	Approvals       []map[string]any `json:"approvals,omitempty"`
}

// GroupTrace represents a trace span from a remote agent.
type GroupTrace struct {
	ID            int64      `json:"id"`
	TraceID       string     `json:"trace_id"`
	SourceAgentID string     `json:"source_agent_id"`
	SpanID        string     `json:"span_id"`
	ParentSpanID  string     `json:"parent_span_id"`
	SpanType      string     `json:"span_type"`
	Title         string     `json:"title"`
	Content       string     `json:"content"`
	StartedAt     *time.Time `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at"`
	DurationMs    int        `json:"duration_ms"`
	CreatedAt     time.Time  `json:"created_at"`
}

// GroupMemberRecord represents a persisted group member in the database.
type GroupMemberRecord struct {
	AgentID      string     `json:"agent_id"`
	AgentName    string     `json:"agent_name"`
	SoulSummary  string     `json:"soul_summary"`
	Capabilities string     `json:"capabilities"` // JSON array
	Channels     string     `json:"channels"`     // JSON array
	Model        string     `json:"model"`
	Status       string     `json:"status"`
	LastSeen     time.Time  `json:"last_seen"`
	LeftAt       *time.Time `json:"left_at,omitempty"`
}

// GroupMembershipHistoryRecord represents a single join/leave event with config snapshot.
type GroupMembershipHistoryRecord struct {
	ID            int64     `json:"id"`
	AgentID       string    `json:"agent_id"`
	GroupName     string    `json:"group_name"`
	Role          string    `json:"role"`
	Action        string    `json:"action"` // "joined" or "left"
	LFSProxyURL   string    `json:"lfs_proxy_url"`
	KafkaBrokers  string    `json:"kafka_brokers"`
	ConsumerGroup string    `json:"consumer_group"`
	AgentName     string    `json:"agent_name"`
	Capabilities  string    `json:"capabilities"` // JSON array
	Channels      string    `json:"channels"`     // JSON array
	Model         string    `json:"model"`
	HappenedAt    time.Time `json:"happened_at"`
}

// GroupStats holds aggregated communication statistics.
type GroupStats struct {
	TasksByStatus    map[string]int      `json:"tasks_by_status"`
	TasksLast24h     int                 `json:"tasks_last_24h"`
	TasksLast7d      int                 `json:"tasks_last_7d"`
	AvgResponseSecs  float64             `json:"avg_response_secs"`
	MaxResponseSecs  float64             `json:"max_response_secs"`
	AvgDelegation    float64             `json:"avg_delegation_depth"`
	MaxDelegation    int                 `json:"max_delegation_depth"`
	MemberActivity   []AgentActivityStat `json:"member_activity"`
	TracePerformance []AgentTraceStat    `json:"trace_performance"`
}

// AgentActivityStat holds per-agent task counts.
type AgentActivityStat struct {
	AgentID        string `json:"agent_id"`
	TasksRequested int    `json:"tasks_requested"`
	TasksResponded int    `json:"tasks_responded"`
	TasksCompleted int    `json:"tasks_completed"`
}

// AgentTraceStat holds per-agent trace performance.
type AgentTraceStat struct {
	AgentID     string  `json:"agent_id"`
	TraceCount  int     `json:"trace_count"`
	AvgDuration float64 `json:"avg_duration_ms"`
	MaxDuration int     `json:"max_duration_ms"`
}

// UnifiedAuditEntry is a merged row from delegation_events, policy_decisions, and approval_requests.
type UnifiedAuditEntry struct {
	ID        int64     `json:"id"`
	Source    string    `json:"source"`     // "delegation", "policy", "approval"
	EventType string    `json:"event_type"` // submitted, accepted, allowed, denied, pending, approved, etc.
	Tier      int       `json:"tier"`
	AgentID   string    `json:"agent_id"`
	TargetID  string    `json:"target_id"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditFilter holds query parameters for the unified audit log.
type AuditFilter struct {
	EventType string
	Source    string
	AgentID   string
	StartAt   *time.Time
	EndAt     *time.Time
	Limit     int
	Offset    int
}

// GroupTaskRecord represents a group collaboration task.
type GroupTaskRecord struct {
	ID                  int64      `json:"id"`
	TaskID              string     `json:"task_id"`
	Description         string     `json:"description"`
	Content             string     `json:"content"`
	Direction           string     `json:"direction"` // "outgoing" | "incoming"
	RequesterID         string     `json:"requester_id"`
	ResponderID         string     `json:"responder_id"`
	ResponseContent     string     `json:"response_content"`
	Status              string     `json:"status"` // pending/completed/failed/rejected
	ParentTaskID        string     `json:"parent_task_id,omitempty"`
	DelegationDepth     int        `json:"delegation_depth,omitempty"`
	OriginalRequesterID string     `json:"original_requester_id,omitempty"`
	DeadlineAt          *time.Time `json:"deadline_at,omitempty"`
	AcceptedAt          *time.Time `json:"accepted_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	RespondedAt         *time.Time `json:"responded_at,omitempty"`
}

// DelegationEventRecord represents a delegation audit event.
type DelegationEventRecord struct {
	ID         int64     `json:"id"`
	TaskID     string    `json:"task_id"`
	EventType  string    `json:"event_type"` // submitted, accepted, completed, failed
	SenderID   string    `json:"sender_id"`
	ReceiverID string    `json:"receiver_id"`
	Summary    string    `json:"summary"`
	Depth      int       `json:"depth"`
	CreatedAt  time.Time `json:"created_at"`
}

// ScheduledJobRecord represents persisted scheduler job state.
type ScheduledJobRecord struct {
	ID         int64     `json:"id"`
	JobName    string    `json:"job_name"`
	LastStatus string    `json:"last_status"`
	LastRunAt  time.Time `json:"last_run_at"`
	RunCount   int       `json:"run_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// GroupMemoryItemRecord represents a shared memory item from group collaboration.
type GroupMemoryItemRecord struct {
	ID          int64     `json:"id"`
	ItemID      string    `json:"item_id"`
	AuthorID    string    `json:"author_id"`
	Title       string    `json:"title"`
	ContentType string    `json:"content_type"`
	Tags        string    `json:"tags"` // JSON array
	LFSBucket   string    `json:"lfs_bucket"`
	LFSKey      string    `json:"lfs_key"`
	Metadata    string    `json:"metadata"` // JSON object
	CreatedAt   time.Time `json:"created_at"`
}

// GroupSkillChannelRecord represents a registered skill channel.
type GroupSkillChannelRecord struct {
	ID             int64     `json:"id"`
	SkillName      string    `json:"skill_name"`
	GroupName      string    `json:"group_name"`
	RequestsTopic  string    `json:"requests_topic"`
	ResponsesTopic string    `json:"responses_topic"`
	RegisteredBy   string    `json:"registered_by"`
	CreatedAt      time.Time `json:"created_at"`
}

// TopicMessageLogRecord represents a single message event on a topic.
type TopicMessageLogRecord struct {
	ID            int64     `json:"id"`
	TopicName     string    `json:"topic_name"`
	SenderID      string    `json:"sender_id"`
	EnvelopeType  string    `json:"envelope_type"`
	CorrelationID string    `json:"correlation_id"`
	PayloadSize   int       `json:"payload_size"`
	CreatedAt     time.Time `json:"created_at"`
}

// KnowledgeFactRecord is the latest accepted state of a shared knowledge fact.
type KnowledgeFactRecord struct {
	FactID     string    `json:"fact_id"`
	GroupName  string    `json:"group_name"`
	Subject    string    `json:"subject"`
	Predicate  string    `json:"predicate"`
	Object     string    `json:"object"`
	Version    int       `json:"version"`
	Source     string    `json:"source"`
	ProposalID string    `json:"proposal_id,omitempty"`
	DecisionID string    `json:"decision_id,omitempty"`
	Tags       string    `json:"tags"` // JSON array
	UpdatedAt  time.Time `json:"updated_at"`
}

// KnowledgeProposalRecord is a persisted shared-knowledge proposal.
type KnowledgeProposalRecord struct {
	ProposalID         string    `json:"proposal_id"`
	GroupName          string    `json:"group_name"`
	Title              string    `json:"title"`
	Statement          string    `json:"statement"`
	Tags               string    `json:"tags"` // JSON array
	ProposerClawID     string    `json:"proposer_claw_id"`
	ProposerInstanceID string    `json:"proposer_instance_id"`
	Status             string    `json:"status"` // pending|approved|rejected|expired
	YesVotes           int       `json:"yes_votes"`
	NoVotes            int       `json:"no_votes"`
	Reason             string    `json:"reason"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// KnowledgeVoteRecord is a single claw vote for one proposal.
type KnowledgeVoteRecord struct {
	ProposalID string    `json:"proposal_id"`
	ClawID     string    `json:"claw_id"`
	InstanceID string    `json:"instance_id"`
	Vote       string    `json:"vote"` // yes|no
	Reason     string    `json:"reason"`
	TraceID    string    `json:"trace_id"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TopicStat holds per-topic aggregated statistics.
type TopicStat struct {
	TopicName      string  `json:"topic_name"`
	Category       string  `json:"category"`
	Description    string  `json:"description"`
	MessageCount   int     `json:"message_count"`
	Last24h        int     `json:"last_24h"`
	Last7d         int     `json:"last_7d"`
	UniqueAgents   int     `json:"unique_agents"`
	LastMessageAt  string  `json:"last_message_at"`
	AvgPayloadSize float64 `json:"avg_payload_size"`
}

// TopicFlowEdge represents agent-to-agent message flow through a topic.
type TopicFlowEdge struct {
	SourceAgentID string `json:"source_agent_id"`
	TopicName     string `json:"topic_name"`
	TargetAgentID string `json:"target_agent_id"`
	MessageCount  int    `json:"message_count"`
}

// AgentXP holds gamification score data for an agent.
type AgentXP struct {
	AgentID          string `json:"agent_id"`
	AgentName        string `json:"agent_name"`
	TotalXP          int    `json:"total_xp"`
	Level            int    `json:"level"`
	Rank             string `json:"rank"`
	TasksCompleted   int    `json:"tasks_completed"`
	TracesShared     int    `json:"traces_shared"`
	MemoriesShared   int    `json:"memories_shared"`
	SkillsRegistered int    `json:"skills_registered"`
}

// TopicHealth holds health/pulse data for a topic.
type TopicHealth struct {
	TopicName       string  `json:"topic_name"`
	Score           int     `json:"score"`
	MessagesPerHour float64 `json:"messages_per_hour"`
	ActiveAgents    int     `json:"active_agents"`
	IsStale         bool    `json:"is_stale"`
}

// TopicDensityBucket holds a single hourly bucket for topic message density.
type TopicDensityBucket struct {
	BucketStart string `json:"bucket_start"`
	BucketEnd   string `json:"bucket_end"`
	Count       int    `json:"count"`
}

// PolicyDecisionRecord represents a logged policy evaluation.
type PolicyDecisionRecord struct {
	ID        int64     `json:"id"`
	TraceID   string    `json:"trace_id,omitempty"`
	TaskID    string    `json:"task_id,omitempty"`
	Tool      string    `json:"tool"`
	Tier      int       `json:"tier"`
	Sender    string    `json:"sender,omitempty"`
	Channel   string    `json:"channel,omitempty"`
	Allowed   bool      `json:"allowed"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ApprovalRecord represents a tool approval request stored in the database.
type ApprovalRecord struct {
	ID          int64      `json:"id"`
	ApprovalID  string     `json:"approval_id"`
	TraceID     string     `json:"trace_id,omitempty"`
	TaskID      string     `json:"task_id,omitempty"`
	Tool        string     `json:"tool"`
	Tier        int        `json:"tier"`
	Arguments   string     `json:"arguments,omitempty"`
	Sender      string     `json:"sender,omitempty"`
	Channel     string     `json:"channel,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
}

const Schema = `
CREATE TABLE IF NOT EXISTS timeline (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	event_id TEXT UNIQUE,
	trace_id TEXT,
	span_id TEXT,
	parent_span_id TEXT,
	timestamp DATETIME,
	sender_id TEXT,
	sender_name TEXT,
	event_type TEXT,
	content_text TEXT,
	media_path TEXT,
	vector_id TEXT,
	classification TEXT,
	authorized BOOLEAN DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_timeline_timestamp ON timeline(timestamp);
CREATE INDEX IF NOT EXISTS idx_timeline_sender ON timeline(sender_id);
CREATE INDEX IF NOT EXISTS idx_timeline_authorized ON timeline(authorized);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT,
	updated_at DATETIME
);

CREATE TABLE IF NOT EXISTS web_users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT UNIQUE,
	force_send BOOLEAN DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS web_links (
	web_user_id INTEGER PRIMARY KEY,
	whatsapp_jid TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_web_links_whatsapp ON web_links(whatsapp_jid);

CREATE TABLE IF NOT EXISTS tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT UNIQUE NOT NULL,
	idempotency_key TEXT UNIQUE,
	trace_id TEXT,
	channel TEXT NOT NULL,
	chat_id TEXT NOT NULL,
	sender_id TEXT,
	status TEXT NOT NULL DEFAULT 'pending',
	content_in TEXT,
	content_out TEXT,
	error_text TEXT,
	prompt_tokens INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	total_tokens INTEGER NOT NULL DEFAULT 0,
	delivery_status TEXT NOT NULL DEFAULT 'pending',
	delivery_attempts INTEGER NOT NULL DEFAULT 0,
	delivery_next_at DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	completed_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_idempotency ON tasks(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_tasks_trace ON tasks(trace_id);
CREATE INDEX IF NOT EXISTS idx_tasks_delivery ON tasks(delivery_status, delivery_next_at);

CREATE TABLE IF NOT EXISTS policy_decisions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	trace_id TEXT,
	task_id TEXT,
	tool TEXT NOT NULL,
	tier INTEGER NOT NULL,
	sender TEXT,
	channel TEXT,
	allowed BOOLEAN NOT NULL,
	reason TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_policy_trace ON policy_decisions(trace_id);
CREATE INDEX IF NOT EXISTS idx_policy_task ON policy_decisions(task_id);

CREATE TABLE IF NOT EXISTS memory_chunks (
	id TEXT PRIMARY KEY,
	content TEXT NOT NULL,
	embedding BLOB,
	source TEXT NOT NULL DEFAULT 'user',
	tags TEXT DEFAULT '',
	version INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_memory_chunks_source ON memory_chunks(source);

CREATE TABLE IF NOT EXISTS group_traces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	trace_id TEXT NOT NULL,
	source_agent_id TEXT NOT NULL,
	span_id TEXT,
	parent_span_id TEXT,
	span_type TEXT,
	title TEXT,
	content TEXT,
	started_at DATETIME,
	ended_at DATETIME,
	duration_ms INTEGER DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_group_traces_trace ON group_traces(trace_id);
CREATE INDEX IF NOT EXISTS idx_group_traces_agent ON group_traces(source_agent_id);

CREATE TABLE IF NOT EXISTS group_members (
	agent_id TEXT UNIQUE NOT NULL,
	agent_name TEXT,
	soul_summary TEXT,
	capabilities TEXT DEFAULT '[]',
	channels TEXT DEFAULT '[]',
	model TEXT,
	status TEXT DEFAULT 'active',
	last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS group_tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT UNIQUE NOT NULL,
	description TEXT,
	content TEXT,
	direction TEXT NOT NULL DEFAULT 'outgoing',
	requester_id TEXT NOT NULL,
	responder_id TEXT,
	response_content TEXT,
	status TEXT NOT NULL DEFAULT 'pending',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	responded_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_group_tasks_direction ON group_tasks(direction);
CREATE INDEX IF NOT EXISTS idx_group_tasks_status ON group_tasks(status);

CREATE TABLE IF NOT EXISTS orchestrator_zones (
	zone_id TEXT PRIMARY KEY,
	name TEXT,
	visibility TEXT DEFAULT 'public',
	owner_id TEXT,
	parent_zone TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS orchestrator_zone_members (
	zone_id TEXT,
	agent_id TEXT,
	role TEXT DEFAULT 'member',
	joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (zone_id, agent_id)
);

CREATE TABLE IF NOT EXISTS orchestrator_hierarchy (
	agent_id TEXT PRIMARY KEY,
	parent_id TEXT,
	role TEXT DEFAULT 'worker',
	endpoint TEXT,
	zone_id TEXT,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS group_memory_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	item_id TEXT UNIQUE NOT NULL,
	author_id TEXT NOT NULL,
	title TEXT,
	content_type TEXT DEFAULT 'text/plain',
	tags TEXT DEFAULT '[]',
	lfs_bucket TEXT,
	lfs_key TEXT,
	metadata TEXT DEFAULT '{}',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_group_memory_author ON group_memory_items(author_id);
CREATE INDEX IF NOT EXISTS idx_group_memory_created ON group_memory_items(created_at);

CREATE TABLE IF NOT EXISTS group_skill_channels (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	skill_name TEXT NOT NULL,
	group_name TEXT NOT NULL,
	requests_topic TEXT NOT NULL,
	responses_topic TEXT NOT NULL,
	registered_by TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE(skill_name, group_name)
);
CREATE INDEX IF NOT EXISTS idx_group_skill_group ON group_skill_channels(group_name);

CREATE TABLE IF NOT EXISTS approval_requests (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	approval_id TEXT UNIQUE NOT NULL,
	trace_id TEXT,
	task_id TEXT,
	tool TEXT NOT NULL,
	tier INTEGER NOT NULL,
	arguments TEXT,
	sender TEXT,
	channel TEXT,
	status TEXT NOT NULL DEFAULT 'pending',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	responded_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_approval_status ON approval_requests(status);
CREATE INDEX IF NOT EXISTS idx_approval_id ON approval_requests(approval_id);

CREATE TABLE IF NOT EXISTS scheduled_jobs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	job_name TEXT UNIQUE NOT NULL,
	last_status TEXT DEFAULT '',
	last_run_at DATETIME,
	run_count INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS delegation_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT NOT NULL,
	event_type TEXT NOT NULL,
	sender_id TEXT NOT NULL,
	receiver_id TEXT NOT NULL DEFAULT '',
	summary TEXT DEFAULT '',
	depth INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_delegation_task ON delegation_events(task_id);
CREATE INDEX IF NOT EXISTS idx_delegation_type ON delegation_events(event_type);

CREATE TABLE IF NOT EXISTS group_membership_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	agent_id TEXT NOT NULL,
	group_name TEXT NOT NULL,
	role TEXT DEFAULT '',
	action TEXT NOT NULL,
	lfs_proxy_url TEXT DEFAULT '',
	kafka_brokers TEXT DEFAULT '',
	consumer_group TEXT DEFAULT '',
	agent_name TEXT DEFAULT '',
	capabilities TEXT DEFAULT '[]',
	channels TEXT DEFAULT '[]',
	model TEXT DEFAULT '',
	happened_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_membership_history_agent ON group_membership_history(agent_id);
CREATE INDEX IF NOT EXISTS idx_membership_history_group ON group_membership_history(group_name);

CREATE TABLE IF NOT EXISTS topic_message_log (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	topic_name TEXT NOT NULL,
	sender_id TEXT NOT NULL,
	envelope_type TEXT NOT NULL,
	correlation_id TEXT DEFAULT '',
	payload_size INTEGER DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_topic_log_topic ON topic_message_log(topic_name);
CREATE INDEX IF NOT EXISTS idx_topic_log_sender ON topic_message_log(sender_id);
CREATE INDEX IF NOT EXISTS idx_topic_log_created ON topic_message_log(created_at);

CREATE TABLE IF NOT EXISTS agent_expertise (
	skill_name TEXT PRIMARY KEY,
	success_count INTEGER NOT NULL DEFAULT 0,
	failure_count INTEGER NOT NULL DEFAULT 0,
	avg_quality REAL NOT NULL DEFAULT 0.0,
	last_used DATETIME,
	total_duration_ms INTEGER NOT NULL DEFAULT 0,
	trend TEXT NOT NULL DEFAULT 'stable'
);

CREATE TABLE IF NOT EXISTS skill_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	skill_name TEXT NOT NULL,
	task_id TEXT,
	action TEXT NOT NULL,
	quality REAL NOT NULL DEFAULT 0.5,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	metadata TEXT DEFAULT '{}',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_skill_events_skill ON skill_events(skill_name);
CREATE INDEX IF NOT EXISTS idx_skill_events_created ON skill_events(created_at);

CREATE TABLE IF NOT EXISTS working_memory (
	resource_id TEXT NOT NULL,
	thread_id   TEXT NOT NULL DEFAULT '',
	content     TEXT NOT NULL DEFAULT '',
	updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (resource_id, thread_id)
);

CREATE TABLE IF NOT EXISTS observations_queue (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	observed INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_obs_queue_session ON observations_queue(session_id, observed);

CREATE TABLE IF NOT EXISTS observations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL,
	content TEXT NOT NULL,
	priority TEXT NOT NULL DEFAULT 'medium',
	observed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	referenced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_observations_session ON observations(session_id);
`
