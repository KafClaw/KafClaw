package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

// Manager handles group lifecycle: join, leave, heartbeat, roster management.
type Manager struct {
	cfg       config.GroupConfig
	lfs       *LFSClient
	timeline  *timeline.TimelineService
	identity  AgentIdentity
	topics    TopicNames
	extTopics ExtendedTopicNames
	topicMgr  *TopicManager
	roster    map[string]*GroupMember
	rosterMu  sync.RWMutex
	memoryIdx MemoryIndexer
	active    bool
	activeMu  sync.RWMutex
	cancelHB  context.CancelFunc
}

// NewManager creates a new group manager.
func NewManager(cfg config.GroupConfig, timeSvc *timeline.TimelineService, identity AgentIdentity) *Manager {
	lfs := NewLFSClient(cfg.LFSProxyURL, cfg.LFSProxyAPIKey)
	topics := Topics(cfg.GroupName)
	extTopics := ExtendedTopics(cfg.GroupName)
	topicMgr := NewTopicManager(cfg.GroupName)

	return &Manager{
		cfg:       cfg,
		lfs:       lfs,
		timeline:  timeSvc,
		identity:  identity,
		topics:    topics,
		extTopics: extTopics,
		topicMgr:  topicMgr,
		roster:    make(map[string]*GroupMember),
	}
}

// SetMemoryIndexer sets an optional local memory indexer for group items.
func (m *Manager) SetMemoryIndexer(idx MemoryIndexer) {
	m.memoryIdx = idx
}

// Join announces this agent to the group and starts heartbeat.
func (m *Manager) Join(ctx context.Context) error {
	m.activeMu.Lock()
	defer m.activeMu.Unlock()

	if m.active {
		return fmt.Errorf("already joined group %s", m.cfg.GroupName)
	}

	// Announce join
	env := &GroupEnvelope{
		Type:          EnvelopeAnnounce,
		CorrelationID: fmt.Sprintf("join-%d", time.Now().UnixNano()),
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: AnnouncePayload{
			Action:   "join",
			Identity: m.identity,
		},
	}

	if err := m.lfs.ProduceEnvelope(ctx, m.topics.Announce, env); err != nil {
		return fmt.Errorf("join announce failed: %w", err)
	}

	// Add self to in-memory roster.
	// First agent in the group becomes coordinator.
	m.rosterMu.Lock()
	role := m.identity.Role
	if role == "" && len(m.roster) == 0 {
		role = "coordinator"
		m.identity.Role = role
	}
	m.roster[m.identity.AgentID] = &GroupMember{
		AgentID:      m.identity.AgentID,
		AgentName:    m.identity.AgentName,
		SoulSummary:  m.identity.SoulSummary,
		Capabilities: m.identity.Capabilities,
		Channels:     m.identity.Channels,
		Model:        m.identity.Model,
		Role:         role,
		Status:       "active",
		LastSeen:     time.Now(),
	}
	m.rosterMu.Unlock()

	// Persist self as a member
	if m.timeline != nil {
		caps, _ := json.Marshal(m.identity.Capabilities)
		chs, _ := json.Marshal(m.identity.Channels)
		_ = m.timeline.UpsertGroupMember(&timeline.GroupMemberRecord{
			AgentID:      m.identity.AgentID,
			AgentName:    m.identity.AgentName,
			SoulSummary:  m.identity.SoulSummary,
			Capabilities: string(caps),
			Channels:     string(chs),
			Model:        m.identity.Model,
			Status:       "active",
		})
		// Log membership history
		_ = m.timeline.LogMembershipHistory(&timeline.GroupMembershipHistoryRecord{
			AgentID:       m.identity.AgentID,
			GroupName:     m.cfg.GroupName,
			Role:          role,
			Action:        "joined",
			LFSProxyURL:   m.cfg.LFSProxyURL,
			KafkaBrokers:  m.cfg.KafkaBrokers,
			ConsumerGroup: m.cfg.ConsumerGroup,
			AgentName:     m.identity.AgentName,
			Capabilities:  string(caps),
			Channels:      string(chs),
			Model:         m.identity.Model,
		})
		m.persistHeartbeatMetadata(time.Now(), true)
	}

	m.active = true

	// Start heartbeat
	hbCtx, cancel := context.WithCancel(context.Background())
	m.cancelHB = cancel
	go m.startHeartbeat(hbCtx)

	slog.Info("Joined group", "group", m.cfg.GroupName, "agent_id", m.identity.AgentID)
	return nil
}

// Leave announces departure and stops heartbeat.
func (m *Manager) Leave(ctx context.Context) error {
	m.activeMu.Lock()
	defer m.activeMu.Unlock()

	if !m.active {
		return fmt.Errorf("not in a group")
	}

	// Stop heartbeat
	if m.cancelHB != nil {
		m.cancelHB()
	}

	// Announce leave
	env := &GroupEnvelope{
		Type:          EnvelopeAnnounce,
		CorrelationID: fmt.Sprintf("leave-%d", time.Now().UnixNano()),
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: AnnouncePayload{
			Action:   "leave",
			Identity: m.identity,
		},
	}

	if err := m.lfs.ProduceEnvelope(ctx, m.topics.Announce, env); err != nil {
		slog.Warn("Leave announce failed", "error", err)
	}

	// Soft-delete self from roster db and log history
	if m.timeline != nil {
		_ = m.timeline.SoftDeleteGroupMember(m.identity.AgentID)
		caps, _ := json.Marshal(m.identity.Capabilities)
		chs, _ := json.Marshal(m.identity.Channels)
		_ = m.timeline.LogMembershipHistory(&timeline.GroupMembershipHistoryRecord{
			AgentID:       m.identity.AgentID,
			GroupName:     m.cfg.GroupName,
			Role:          m.identity.Role,
			Action:        "left",
			LFSProxyURL:   m.cfg.LFSProxyURL,
			KafkaBrokers:  m.cfg.KafkaBrokers,
			ConsumerGroup: m.cfg.ConsumerGroup,
			AgentName:     m.identity.AgentName,
			Capabilities:  string(caps),
			Channels:      string(chs),
			Model:         m.identity.Model,
		})
	}

	m.active = false
	m.rosterMu.Lock()
	m.roster = make(map[string]*GroupMember)
	m.rosterMu.Unlock()

	slog.Info("Left group", "group", m.cfg.GroupName)
	return nil
}

// Active returns whether this agent is in a group.
func (m *Manager) Active() bool {
	m.activeMu.RLock()
	defer m.activeMu.RUnlock()
	return m.active
}

// GroupName returns the current group name.
func (m *Manager) GroupName() string {
	return m.cfg.GroupName
}

// Members returns the current roster.
func (m *Manager) Members() []GroupMember {
	m.rosterMu.RLock()
	defer m.rosterMu.RUnlock()

	out := make([]GroupMember, 0, len(m.roster))
	for _, member := range m.roster {
		out = append(out, *member)
	}
	return out
}

// MemberCount returns the number of known members.
func (m *Manager) MemberCount() int {
	m.rosterMu.RLock()
	defer m.rosterMu.RUnlock()
	return len(m.roster)
}

// Status returns a summary of the group state.
func (m *Manager) Status() map[string]any {
	m.activeMu.RLock()
	active := m.active
	m.activeMu.RUnlock()

	healthy := m.lfs.Healthy(context.Background())

	return map[string]any{
		"active":        active,
		"group_name":    m.cfg.GroupName,
		"agent_id":      m.identity.AgentID,
		"member_count":  m.MemberCount(),
		"lfs_proxy_url": m.cfg.LFSProxyURL,
		"lfs_healthy":   healthy,
	}
}

// HandleAnnounce processes an incoming announce message and updates the roster.
func (m *Manager) HandleAnnounce(env *GroupEnvelope) {
	data, err := json.Marshal(env.Payload)
	if err != nil {
		slog.Warn("HandleAnnounce: marshal payload", "error", err)
		return
	}
	var payload AnnouncePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		slog.Warn("HandleAnnounce: unmarshal payload", "error", err)
		return
	}

	id := payload.Identity
	switch payload.Action {
	case "join", "heartbeat":
		member := &GroupMember{
			AgentID:      id.AgentID,
			AgentName:    id.AgentName,
			SoulSummary:  id.SoulSummary,
			Capabilities: id.Capabilities,
			Channels:     id.Channels,
			Model:        id.Model,
			Role:         id.Role,
			Status:       id.Status,
			LastSeen:     time.Now(),
		}
		m.rosterMu.Lock()
		m.roster[id.AgentID] = member
		m.rosterMu.Unlock()

		// Persist to DB
		if m.timeline != nil {
			caps, _ := json.Marshal(id.Capabilities)
			chs, _ := json.Marshal(id.Channels)
			_ = m.timeline.UpsertGroupMember(&timeline.GroupMemberRecord{
				AgentID:      id.AgentID,
				AgentName:    id.AgentName,
				SoulSummary:  id.SoulSummary,
				Capabilities: string(caps),
				Channels:     string(chs),
				Model:        id.Model,
				Status:       "active",
			})
		}

		if payload.Action == "join" {
			slog.Info("Member joined", "agent_id", id.AgentID, "agent_name", id.AgentName)
			// Reply with our own heartbeat so the joiner learns about us
			go m.sendHeartbeat(context.Background())
		}

	case "leave":
		m.rosterMu.Lock()
		delete(m.roster, id.AgentID)
		m.rosterMu.Unlock()

		if m.timeline != nil {
			_ = m.timeline.SoftDeleteGroupMember(id.AgentID)
		}
		slog.Info("Member left", "agent_id", id.AgentID)
	}
}

// PublishTrace publishes a trace span to the group traces topic.
func (m *Manager) PublishTrace(ctx context.Context, tracePayload TracePayload) error {
	if !m.Active() {
		return nil
	}
	env := &GroupEnvelope{
		Type:          EnvelopeTrace,
		CorrelationID: tracePayload.TraceID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload:       tracePayload,
	}
	err := m.lfs.ProduceEnvelope(ctx, m.topics.Traces, env)
	// Also log to topic_message_log so the browse view shows trace data
	if m.timeline != nil {
		_ = m.timeline.LogTopicMessage(&timeline.TopicMessageLogRecord{
			TopicName:     m.extTopics.ObserveTraces,
			SenderID:      m.identity.AgentID,
			EnvelopeType:  EnvelopeTrace,
			CorrelationID: tracePayload.TraceID,
			PayloadSize:   len(tracePayload.Content),
		})
	}
	return err
}

// PublishAudit publishes an audit event to the group audit topic.
func (m *Manager) PublishAudit(ctx context.Context, eventType, traceID, detail string) error {
	if !m.Active() {
		return nil
	}
	env := &GroupEnvelope{
		Type:          EnvelopeAudit,
		CorrelationID: traceID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: map[string]string{
			"event_type": eventType,
			"detail":     detail,
		},
	}
	err := m.lfs.ProduceEnvelope(ctx, m.extTopics.ObserveAudit, env)
	// Also log to topic_message_log so the browse view shows audit data
	if m.timeline != nil {
		_ = m.timeline.LogTopicMessage(&timeline.TopicMessageLogRecord{
			TopicName:     m.extTopics.ObserveAudit,
			SenderID:      m.identity.AgentID,
			EnvelopeType:  EnvelopeAudit,
			CorrelationID: traceID,
			PayloadSize:   len(detail),
		})
	}
	return err
}

// SubmitTask sends a task request to the group.
func (m *Manager) SubmitTask(ctx context.Context, taskID, description, content string) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}
	env := &GroupEnvelope{
		Type:          EnvelopeRequest,
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
	return m.lfs.ProduceEnvelope(ctx, m.topics.Requests, env)
}

// RespondTask sends a task response to the group.
func (m *Manager) RespondTask(ctx context.Context, taskID, content, status string) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}
	env := &GroupEnvelope{
		Type:          EnvelopeResponse,
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
	if err := m.lfs.ProduceEnvelope(ctx, m.topics.Responses, env); err != nil {
		return err
	}

	// Log delegation events for completed/failed responses.
	if m.timeline != nil && (status == "completed" || status == "failed") {
		_ = m.timeline.LogDelegationEvent(
			taskID, status,
			m.identity.AgentID, "",
			content, 0,
		)
	}
	return nil
}

// SubmitDelegatedTask sends a delegated task request, enforcing MaxDelegationDepth.
func (m *Manager) SubmitDelegatedTask(ctx context.Context, req DelegatedTaskRequest) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}

	maxDepth := m.cfg.MaxDelegationDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	if req.DelegationDepth >= maxDepth {
		return fmt.Errorf("delegation depth %d exceeds max %d", req.DelegationDepth, maxDepth)
	}

	originalRequester := req.OriginalRequesterID
	if originalRequester == "" {
		originalRequester = m.identity.AgentID
	}

	var deadlineStr string
	if req.DeadlineAt != nil {
		deadlineStr = req.DeadlineAt.Format("2006-01-02T15:04:05Z07:00")
	}

	env := &GroupEnvelope{
		Type:          EnvelopeRequest,
		CorrelationID: req.TaskID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: TaskRequestPayload{
			TaskID:              req.TaskID,
			Description:         req.Description,
			Content:             req.Content,
			RequesterID:         m.identity.AgentID,
			ParentTaskID:        req.ParentTaskID,
			DelegationDepth:     req.DelegationDepth + 1,
			OriginalRequesterID: originalRequester,
			DeadlineAt:          deadlineStr,
		},
	}

	if err := m.lfs.ProduceEnvelope(ctx, m.topics.Requests, env); err != nil {
		return fmt.Errorf("submit delegated task: %w", err)
	}

	// Persist to DB with delegation info.
	if m.timeline != nil {
		_ = m.timeline.InsertDelegatedGroupTask(&timeline.GroupTaskRecord{
			TaskID:              req.TaskID,
			Description:         req.Description,
			Content:             req.Content,
			Direction:           "outgoing",
			RequesterID:         m.identity.AgentID,
			Status:              "pending",
			ParentTaskID:        req.ParentTaskID,
			DelegationDepth:     req.DelegationDepth + 1,
			OriginalRequesterID: originalRequester,
			DeadlineAt:          req.DeadlineAt,
		})

		// Log delegation event.
		_ = m.timeline.LogDelegationEvent(
			req.TaskID, "submitted",
			m.identity.AgentID, "", // receiver unknown yet
			req.Description, req.DelegationDepth+1,
		)
	}

	slog.Info("Delegated task submitted",
		"task_id", req.TaskID,
		"parent", req.ParentTaskID,
		"depth", req.DelegationDepth+1)
	return nil
}

// ReportTaskStatus publishes an EnvelopeTaskStatus message and logs delegation events.
func (m *Manager) ReportTaskStatus(ctx context.Context, taskID, status, summary string) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}

	env := &GroupEnvelope{
		Type:          EnvelopeTaskStatus,
		CorrelationID: taskID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: TaskStatusPayload{
			TaskID:      taskID,
			ResponderID: m.identity.AgentID,
			Status:      status,
			Summary:     summary,
		},
	}

	if err := m.lfs.ProduceEnvelope(ctx, m.topics.Responses, env); err != nil {
		return fmt.Errorf("report task status: %w", err)
	}

	// Log delegation event for accepted status.
	if m.timeline != nil && status == "accepted" {
		_ = m.timeline.AcceptGroupTask(taskID, m.identity.AgentID)
		_ = m.timeline.LogDelegationEvent(
			taskID, "accepted",
			m.identity.AgentID, "",
			summary, 0,
		)
	}

	slog.Info("Task status reported", "task_id", taskID, "status", status)
	return nil
}

// Config returns the current group configuration.
func (m *Manager) Config() config.GroupConfig {
	return m.cfg
}

// AgentID returns this agent's ID.
func (m *Manager) AgentID() string {
	return m.identity.AgentID
}

// LFSHealthy returns whether the LFS proxy is reachable.
func (m *Manager) LFSHealthy() bool {
	return m.lfs.Healthy(context.Background())
}

// Topics returns the topic names for the current group.
func (m *Manager) Topics() TopicNames {
	return m.topics
}

// PublishEnvelope publishes a pre-built envelope to a specific Kafka topic.
func (m *Manager) PublishEnvelope(ctx context.Context, topic string, env *GroupEnvelope) error {
	return m.lfs.ProduceEnvelope(ctx, topic, env)
}

// EnsureTopic sends a lightweight heartbeat to a topic to auto-create it in Kafka.
func (m *Manager) EnsureTopic(ctx context.Context, topicName string) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}
	// Validate topic belongs to this group (prefix check)
	prefix := fmt.Sprintf("group.%s.", m.cfg.GroupName)
	if !strings.HasPrefix(topicName, prefix) {
		return fmt.Errorf("topic %q does not belong to group %s", topicName, m.cfg.GroupName)
	}

	env := &GroupEnvelope{
		Type:          EnvelopeHeartbeat,
		CorrelationID: fmt.Sprintf("ensure-%d", time.Now().UnixNano()),
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: map[string]string{
			"action": "ensure_topic",
			"topic":  topicName,
		},
	}
	if err := m.lfs.ProduceEnvelope(ctx, topicName, env); err != nil {
		return fmt.Errorf("ensure topic %s: %w", topicName, err)
	}

	slog.Info("Topic ensured", "topic", topicName)
	return nil
}

func (m *Manager) startHeartbeat(ctx context.Context) {
	interval := 30 * time.Second
	if m.cfg.PollIntervalMs > 0 {
		// Heartbeat interval = 15x poll interval (30s default at 2000ms poll)
		interval = time.Duration(m.cfg.PollIntervalMs*15) * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Also periodically mark stale members
	staleTicker := time.NewTicker(interval * 3)
	defer staleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sendHeartbeat(ctx)
		case <-staleTicker.C:
			if m.timeline != nil {
				cutoff := time.Now().Add(-interval * 3)
				if n, err := m.timeline.MarkStaleMembers(cutoff); err == nil && n > 0 {
					slog.Info("Marked stale members", "count", n)
				}
			}
		}
	}
}

func (m *Manager) sendHeartbeat(ctx context.Context) {
	if !m.Active() {
		return
	}
	env := &GroupEnvelope{
		Type:          EnvelopeAnnounce,
		CorrelationID: fmt.Sprintf("hb-%d", time.Now().UnixNano()),
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload: AnnouncePayload{
			Action:   "heartbeat",
			Identity: m.identity,
		},
	}
	if err := m.lfs.ProduceEnvelope(ctx, m.topics.Announce, env); err != nil {
		if m.timeline != nil {
			_ = m.timeline.SetSetting("group_heartbeat_last_attempt_at", time.Now().UTC().Format(time.RFC3339))
		}
		slog.Debug("Heartbeat failed", "error", err)
		return
	}
	m.persistHeartbeatMetadata(time.Now(), true)
}

func (m *Manager) persistHeartbeatMetadata(ts time.Time, incrementSeq bool) {
	if m.timeline == nil {
		return
	}
	now := ts.UTC().Format(time.RFC3339)
	_ = m.timeline.SetSetting("group_heartbeat_last_attempt_at", now)
	_ = m.timeline.SetSetting("group_heartbeat_last_success_at", now)
	if !incrementSeq {
		return
	}
	seq := 1
	if raw, err := m.timeline.GetSetting("group_heartbeat_seq"); err == nil {
		if n, convErr := strconv.Atoi(strings.TrimSpace(raw)); convErr == nil && n > 0 {
			seq = n + 1
		}
	}
	_ = m.timeline.SetSetting("group_heartbeat_seq", strconv.Itoa(seq))
}
