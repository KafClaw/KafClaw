package timeline

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type TimelineService struct {
	db *sql.DB
}

func NewTimelineService(dbPath string) (*TimelineService, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("failed to open timeline db: %w", err)
	}

	// Apply schema
	if _, err := db.Exec(Schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to apply schema: %w", err)
	}
	// Best-effort migration for existing dbs (no-op if column exists).
	_, _ = db.Exec(`ALTER TABLE web_users ADD COLUMN force_send BOOLEAN DEFAULT 1`)
	// Best-effort migrations for tracing columns.
	_, _ = db.Exec(`ALTER TABLE timeline ADD COLUMN trace_id TEXT`)
	_, _ = db.Exec(`ALTER TABLE timeline ADD COLUMN span_id TEXT`)
	_, _ = db.Exec(`ALTER TABLE timeline ADD COLUMN parent_span_id TEXT`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_timeline_trace ON timeline(trace_id)`)
	// Backfill trace_id for existing rows (best-effort).
	_, _ = db.Exec(`
		UPDATE timeline
		SET trace_id = CASE
			WHEN event_id IS NOT NULL AND event_id != '' THEN 'trace:' || event_id
			ELSE 'trace:' || sender_id || ':' || strftime('%s', timestamp)
		END
		WHERE trace_id IS NULL OR trace_id = ''
	`)
	// Backfill for older rows where force_send is NULL.
	_, _ = db.Exec(`UPDATE web_users SET force_send = 1 WHERE force_send IS NULL`)
	// Best-effort migration: add metadata column to timeline table.
	_, _ = db.Exec(`ALTER TABLE timeline ADD COLUMN metadata TEXT DEFAULT ''`)
	// Best-effort migration: ensure tasks table exists on older DBs.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
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
		delivery_status TEXT NOT NULL DEFAULT 'pending',
		delivery_attempts INTEGER NOT NULL DEFAULT 0,
		delivery_next_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		completed_at DATETIME
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_idempotency ON tasks(idempotency_key)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_trace ON tasks(trace_id)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_delivery ON tasks(delivery_status, delivery_next_at)`)
	// Best-effort migration: add message_type column to tasks table.
	_, _ = db.Exec(`ALTER TABLE tasks ADD COLUMN message_type TEXT DEFAULT ''`)
	// Best-effort migration: add token columns to tasks table.
	_, _ = db.Exec(`ALTER TABLE tasks ADD COLUMN prompt_tokens INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE tasks ADD COLUMN completion_tokens INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE tasks ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0`)
	// Best-effort migration: policy_decisions table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS policy_decisions (
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
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_policy_trace ON policy_decisions(trace_id)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_policy_task ON policy_decisions(task_id)`)
	// Best-effort migration: memory_chunks table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS memory_chunks (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		embedding BLOB,
		source TEXT NOT NULL DEFAULT 'user',
		tags TEXT DEFAULT '',
		version INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_memory_chunks_source ON memory_chunks(source)`)
	// Best-effort migration: span timing columns on timeline.
	_, _ = db.Exec(`ALTER TABLE timeline ADD COLUMN span_started_at DATETIME`)
	_, _ = db.Exec(`ALTER TABLE timeline ADD COLUMN span_ended_at DATETIME`)
	_, _ = db.Exec(`ALTER TABLE timeline ADD COLUMN span_duration_ms INTEGER DEFAULT 0`)
	// Best-effort migration: group tables.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS group_traces (
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
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_group_traces_trace ON group_traces(trace_id)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_group_traces_agent ON group_traces(source_agent_id)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS group_members (
		agent_id TEXT UNIQUE NOT NULL,
		agent_name TEXT,
		soul_summary TEXT,
		capabilities TEXT DEFAULT '[]',
		channels TEXT DEFAULT '[]',
		model TEXT,
		status TEXT DEFAULT 'active',
		last_seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	// Best-effort migration: group_tasks table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS group_tasks (
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
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_group_tasks_direction ON group_tasks(direction)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_group_tasks_status ON group_tasks(status)`)
	// Best-effort migration: orchestrator tables.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS orchestrator_zones (
		zone_id TEXT PRIMARY KEY,
		name TEXT,
		visibility TEXT DEFAULT 'public',
		owner_id TEXT,
		parent_zone TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS orchestrator_zone_members (
		zone_id TEXT,
		agent_id TEXT,
		role TEXT DEFAULT 'member',
		joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (zone_id, agent_id)
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS orchestrator_hierarchy (
		agent_id TEXT PRIMARY KEY,
		parent_id TEXT,
		role TEXT DEFAULT 'worker',
		endpoint TEXT,
		zone_id TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	// Best-effort migration: group memory items table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS group_memory_items (
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
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_group_memory_author ON group_memory_items(author_id)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_group_memory_created ON group_memory_items(created_at)`)
	// Best-effort migration: group skill channels table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS group_skill_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		skill_name TEXT NOT NULL,
		group_name TEXT NOT NULL,
		requests_topic TEXT NOT NULL,
		responses_topic TEXT NOT NULL,
		registered_by TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(skill_name, group_name)
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_group_skill_group ON group_skill_channels(group_name)`)
	// Best-effort migration: approval_requests table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS approval_requests (
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
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_approval_status ON approval_requests(status)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_approval_id ON approval_requests(approval_id)`)
	// Best-effort migration: scheduled_jobs table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS scheduled_jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_name TEXT UNIQUE NOT NULL,
		last_status TEXT DEFAULT '',
		last_run_at DATETIME,
		run_count INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	// Best-effort migration: delegation_events table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS delegation_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		sender_id TEXT NOT NULL,
		receiver_id TEXT NOT NULL DEFAULT '',
		summary TEXT DEFAULT '',
		depth INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_delegation_task ON delegation_events(task_id)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_delegation_type ON delegation_events(event_type)`)
	// Best-effort migration: group_membership_history table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS group_membership_history (
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
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_membership_history_agent ON group_membership_history(agent_id)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_membership_history_group ON group_membership_history(group_name)`)
	// Best-effort migration: topic_message_log table.
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS topic_message_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		topic_name TEXT NOT NULL,
		sender_id TEXT NOT NULL,
		envelope_type TEXT NOT NULL,
		correlation_id TEXT DEFAULT '',
		payload_size INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_topic_log_topic ON topic_message_log(topic_name)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_topic_log_sender ON topic_message_log(sender_id)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_topic_log_created ON topic_message_log(created_at)`)
	// Best-effort migration: left_at column on group_members.
	_, _ = db.Exec(`ALTER TABLE group_members ADD COLUMN left_at DATETIME`)
	// Best-effort migration: delegation columns on group_tasks.
	_, _ = db.Exec(`ALTER TABLE group_tasks ADD COLUMN parent_task_id TEXT DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE group_tasks ADD COLUMN delegation_depth INTEGER DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE group_tasks ADD COLUMN original_requester_id TEXT DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE group_tasks ADD COLUMN deadline_at DATETIME`)
	_, _ = db.Exec(`ALTER TABLE group_tasks ADD COLUMN accepted_at DATETIME`)

	return &TimelineService{db: db}, nil
}

// DB returns the underlying *sql.DB for shared access (e.g. memory subsystem).
func (s *TimelineService) DB() *sql.DB { return s.db }

func (s *TimelineService) Close() error {
	return s.db.Close()
}

func (s *TimelineService) AddEvent(evt *TimelineEvent) error {
	query := `
	INSERT INTO timeline (event_id, trace_id, span_id, parent_span_id, timestamp, sender_id, sender_name, event_type, content_text, media_path, vector_id, classification, authorized, metadata)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query,
		evt.EventID,
		evt.TraceID,
		evt.SpanID,
		evt.ParentSpanID,
		evt.Timestamp,
		evt.SenderID,
		evt.SenderName,
		evt.EventType,
		evt.ContentText,
		evt.MediaPath,
		evt.VectorID,
		evt.Classification,
		evt.Authorized,
		evt.Metadata,
	)
	return err
}

type FilterArgs struct {
	SenderID       string
	TraceID        string
	Limit          int
	Offset         int
	StartDate      *time.Time
	EndDate        *time.Time
	AuthorizedOnly *bool // nil = all, true = authorized only, false = unauthorized only
}

func (s *TimelineService) GetEvents(filter FilterArgs) ([]TimelineEvent, error) {
	query := `SELECT id, event_id, COALESCE(trace_id,''), COALESCE(span_id,''), COALESCE(parent_span_id,''), timestamp, sender_id, sender_name, event_type, content_text, media_path, vector_id, classification, authorized, COALESCE(metadata,'') FROM timeline WHERE 1=1`
	args := []interface{}{}

	if filter.SenderID != "" {
		query += " AND sender_id = ?"
		args = append(args, filter.SenderID)
	}
	if filter.StartDate != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.StartDate)
	}
	if filter.EndDate != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.EndDate)
	}
	if filter.AuthorizedOnly != nil {
		query += " AND authorized = ?"
		args = append(args, *filter.AuthorizedOnly)
	}
	if filter.TraceID != "" {
		query += " AND trace_id = ?"
		args = append(args, filter.TraceID)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []TimelineEvent
	for rows.Next() {
		var e TimelineEvent
		err := rows.Scan(
			&e.ID,
			&e.EventID,
			&e.TraceID,
			&e.SpanID,
			&e.ParentSpanID,
			&e.Timestamp,
			&e.SenderID,
			&e.SenderName,
			&e.EventType,
			&e.ContentText,
			&e.MediaPath,
			&e.VectorID,
			&e.Classification,
			&e.Authorized,
			&e.Metadata,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// GetSetting returns a setting value by key.
func (s *TimelineService) GetSetting(key string) (string, error) {
	var val string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

// SetSetting persists a setting value.
func (s *TimelineService) SetSetting(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value)
	return err
}

// IsSilentMode checks if silent mode is enabled. Defaults to true (safe default).
func (s *TimelineService) IsSilentMode() bool {
	val, err := s.GetSetting("silent_mode")
	if err != nil {
		return true // Safe default: silent
	}
	return val == "true"
}

// CreateWebUser creates a web user or returns the existing one with the same name.
func (s *TimelineService) CreateWebUser(name string) (*WebUser, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	_, err := s.db.Exec(`INSERT INTO web_users (name, created_at) VALUES (?, datetime('now'))`, name)
	if err != nil {
		// If duplicate, return existing
		return s.GetWebUserByName(name)
	}
	return s.GetWebUserByName(name)
}

// ListWebUsers returns all web users sorted by name.
func (s *TimelineService) ListWebUsers() ([]WebUser, error) {
	rows, err := s.db.Query(`SELECT id, name, COALESCE(force_send, 1), created_at FROM web_users ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []WebUser
	for rows.Next() {
		var u WebUser
		if err := rows.Scan(&u.ID, &u.Name, &u.ForceSend, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// GetWebUser returns a web user by ID.
func (s *TimelineService) GetWebUser(id int64) (*WebUser, error) {
	var u WebUser
	err := s.db.QueryRow(`SELECT id, name, COALESCE(force_send, 1), created_at FROM web_users WHERE id = ?`, id).
		Scan(&u.ID, &u.Name, &u.ForceSend, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetWebUserByName returns a web user by name.
func (s *TimelineService) GetWebUserByName(name string) (*WebUser, error) {
	var u WebUser
	err := s.db.QueryRow(`SELECT id, name, COALESCE(force_send, 1), created_at FROM web_users WHERE name = ?`, name).
		Scan(&u.ID, &u.Name, &u.ForceSend, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// LinkWebUser links a web user to a WhatsApp JID.
func (s *TimelineService) LinkWebUser(webUserID int64, whatsappJID string) error {
	if whatsappJID == "" {
		return fmt.Errorf("whatsapp_jid is required")
	}
	_, err := s.db.Exec(`
		INSERT INTO web_links (web_user_id, whatsapp_jid, created_at, updated_at)
		VALUES (?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(web_user_id) DO UPDATE SET whatsapp_jid = excluded.whatsapp_jid, updated_at = datetime('now')
	`, webUserID, whatsappJID)
	return err
}

// UnlinkWebUser removes the WhatsApp link for a web user.
func (s *TimelineService) UnlinkWebUser(webUserID int64) error {
	_, err := s.db.Exec(`DELETE FROM web_links WHERE web_user_id = ?`, webUserID)
	return err
}

// GetWebLink returns the WhatsApp JID for a web user, if linked.
func (s *TimelineService) GetWebLink(webUserID int64) (string, bool, error) {
	var jid string
	err := s.db.QueryRow(`SELECT whatsapp_jid FROM web_links WHERE web_user_id = ?`, webUserID).Scan(&jid)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return jid, true, nil
}

// SetWebUserForceSend updates the force_send flag for a web user.
func (s *TimelineService) SetWebUserForceSend(webUserID int64, force bool) error {
	_, err := s.db.Exec(`UPDATE web_users SET force_send = ? WHERE id = ?`, force, webUserID)
	return err
}

// --- Task CRUD ---

func newTaskID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
}

// CreateTask inserts a new agent task. TaskID is generated if empty.
func (s *TimelineService) CreateTask(task *AgentTask) (*AgentTask, error) {
	if task.TaskID == "" {
		task.TaskID = newTaskID()
	}
	if task.Status == "" {
		task.Status = TaskStatusPending
	}
	if task.DeliveryStatus == "" {
		task.DeliveryStatus = DeliveryPending
	}

	query := `
	INSERT INTO tasks (task_id, idempotency_key, trace_id, channel, chat_id, sender_id, message_type, status, content_in, delivery_status)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	// Pass NULL for empty idempotency_key to avoid UNIQUE constraint on empty strings.
	var idempKey interface{}
	if task.IdempotencyKey != "" {
		idempKey = task.IdempotencyKey
	}
	result, err := s.db.Exec(query,
		task.TaskID,
		idempKey,
		task.TraceID,
		task.Channel,
		task.ChatID,
		task.SenderID,
		task.MessageType,
		task.Status,
		task.ContentIn,
		task.DeliveryStatus,
	)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	id, _ := result.LastInsertId()
	task.ID = id
	return s.GetTask(task.TaskID)
}

// GetTask returns a task by task_id.
func (s *TimelineService) GetTask(taskID string) (*AgentTask, error) {
	query := `SELECT id, task_id, COALESCE(idempotency_key,''), COALESCE(trace_id,''),
		channel, chat_id, COALESCE(sender_id,''), COALESCE(message_type,''), status,
		COALESCE(content_in,''), COALESCE(content_out,''), COALESCE(error_text,''),
		prompt_tokens, completion_tokens, total_tokens,
		delivery_status, delivery_attempts, delivery_next_at,
		created_at, updated_at, completed_at
	FROM tasks WHERE task_id = ?`

	var t AgentTask
	var deliveryNextAt, completedAt sql.NullTime
	err := s.db.QueryRow(query, taskID).Scan(
		&t.ID, &t.TaskID, &t.IdempotencyKey, &t.TraceID,
		&t.Channel, &t.ChatID, &t.SenderID, &t.MessageType, &t.Status,
		&t.ContentIn, &t.ContentOut, &t.ErrorText,
		&t.PromptTokens, &t.CompletionTokens, &t.TotalTokens,
		&t.DeliveryStatus, &t.DeliveryAttempts, &deliveryNextAt,
		&t.CreatedAt, &t.UpdatedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	if deliveryNextAt.Valid {
		t.DeliveryNextAt = &deliveryNextAt.Time
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.Time
	}
	return &t, nil
}

// GetTaskByIdempotencyKey returns a task by its idempotency key.
// Returns (nil, nil) if not found — critical for dedup logic.
func (s *TimelineService) GetTaskByIdempotencyKey(key string) (*AgentTask, error) {
	if key == "" {
		return nil, nil
	}
	query := `SELECT id, task_id, COALESCE(idempotency_key,''), COALESCE(trace_id,''),
		channel, chat_id, COALESCE(sender_id,''), COALESCE(message_type,''), status,
		COALESCE(content_in,''), COALESCE(content_out,''), COALESCE(error_text,''),
		prompt_tokens, completion_tokens, total_tokens,
		delivery_status, delivery_attempts, delivery_next_at,
		created_at, updated_at, completed_at
	FROM tasks WHERE idempotency_key = ?`

	var t AgentTask
	var deliveryNextAt, completedAt sql.NullTime
	err := s.db.QueryRow(query, key).Scan(
		&t.ID, &t.TaskID, &t.IdempotencyKey, &t.TraceID,
		&t.Channel, &t.ChatID, &t.SenderID, &t.MessageType, &t.Status,
		&t.ContentIn, &t.ContentOut, &t.ErrorText,
		&t.PromptTokens, &t.CompletionTokens, &t.TotalTokens,
		&t.DeliveryStatus, &t.DeliveryAttempts, &deliveryNextAt,
		&t.CreatedAt, &t.UpdatedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task by idempotency key: %w", err)
	}
	if deliveryNextAt.Valid {
		t.DeliveryNextAt = &deliveryNextAt.Time
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.Time
	}
	return &t, nil
}

// UpdateTaskStatus updates a task's status, content_out, and error_text.
func (s *TimelineService) UpdateTaskStatus(taskID, status, contentOut, errorText string) error {
	query := `UPDATE tasks SET status = ?, content_out = ?, error_text = ?, updated_at = datetime('now')`
	if status == TaskStatusCompleted || status == TaskStatusFailed {
		query += `, completed_at = datetime('now')`
	}
	query += ` WHERE task_id = ?`
	_, err := s.db.Exec(query, status, contentOut, errorText, taskID)
	return err
}

// UpdateTaskDelivery updates delivery_status, increments delivery_attempts, and sets delivery_next_at.
func (s *TimelineService) UpdateTaskDelivery(taskID, deliveryStatus string, nextAt *time.Time) error {
	query := `UPDATE tasks SET delivery_status = ?, delivery_attempts = delivery_attempts + 1, delivery_next_at = ?, updated_at = datetime('now') WHERE task_id = ?`
	var nextAtVal interface{}
	if nextAt != nil {
		nextAtVal = *nextAt
	}
	_, err := s.db.Exec(query, deliveryStatus, nextAtVal, taskID)
	return err
}

// ListPendingDeliveries returns completed tasks that still need delivery.
func (s *TimelineService) ListPendingDeliveries(limit int) ([]AgentTask, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `SELECT id, task_id, COALESCE(idempotency_key,''), COALESCE(trace_id,''),
		channel, chat_id, COALESCE(sender_id,''), COALESCE(message_type,''), status,
		COALESCE(content_in,''), COALESCE(content_out,''), COALESCE(error_text,''),
		prompt_tokens, completion_tokens, total_tokens,
		delivery_status, delivery_attempts, delivery_next_at,
		created_at, updated_at, completed_at
	FROM tasks
	WHERE status = 'completed' AND delivery_status = 'pending'
		AND (delivery_next_at IS NULL OR delivery_next_at <= datetime('now'))
	ORDER BY created_at ASC
	LIMIT ?`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending deliveries: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

// ListTasks returns tasks filtered by optional status and channel.
func (s *TimelineService) ListTasks(status, channel string, limit, offset int) ([]AgentTask, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, task_id, COALESCE(idempotency_key,''), COALESCE(trace_id,''),
		channel, chat_id, COALESCE(sender_id,''), COALESCE(message_type,''), status,
		COALESCE(content_in,''), COALESCE(content_out,''), COALESCE(error_text,''),
		prompt_tokens, completion_tokens, total_tokens,
		delivery_status, delivery_attempts, delivery_next_at,
		created_at, updated_at, completed_at
	FROM tasks WHERE 1=1`
	args := []interface{}{}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	if channel != "" {
		query += " AND channel = ?"
		args = append(args, channel)
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func scanTasks(rows *sql.Rows) ([]AgentTask, error) {
	var tasks []AgentTask
	for rows.Next() {
		var t AgentTask
		var deliveryNextAt, completedAt sql.NullTime
		err := rows.Scan(
			&t.ID, &t.TaskID, &t.IdempotencyKey, &t.TraceID,
			&t.Channel, &t.ChatID, &t.SenderID, &t.MessageType, &t.Status,
			&t.ContentIn, &t.ContentOut, &t.ErrorText,
			&t.PromptTokens, &t.CompletionTokens, &t.TotalTokens,
			&t.DeliveryStatus, &t.DeliveryAttempts, &deliveryNextAt,
			&t.CreatedAt, &t.UpdatedAt, &completedAt,
		)
		if err != nil {
			return nil, err
		}
		if deliveryNextAt.Valid {
			t.DeliveryNextAt = &deliveryNextAt.Time
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// UpdateTaskTokens adds token usage to a task.
func (s *TimelineService) UpdateTaskTokens(taskID string, prompt, completion, total int) error {
	_, err := s.db.Exec(`UPDATE tasks SET
		prompt_tokens = prompt_tokens + ?,
		completion_tokens = completion_tokens + ?,
		total_tokens = total_tokens + ?,
		updated_at = datetime('now')
	WHERE task_id = ?`, prompt, completion, total, taskID)
	return err
}

// GetDailyTokenUsage returns total tokens used today across all tasks.
func (s *TimelineService) GetDailyTokenUsage() (int, error) {
	var total int
	err := s.db.QueryRow(`SELECT COALESCE(SUM(total_tokens), 0) FROM tasks WHERE created_at >= date('now')`).Scan(&total)
	return total, err
}

// LogPolicyDecision records a policy evaluation result.
func (s *TimelineService) LogPolicyDecision(rec *PolicyDecisionRecord) error {
	_, err := s.db.Exec(`INSERT INTO policy_decisions (trace_id, task_id, tool, tier, sender, channel, allowed, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.TraceID, rec.TaskID, rec.Tool, rec.Tier, rec.Sender, rec.Channel, rec.Allowed, rec.Reason)
	return err
}

// ListPolicyDecisions returns policy decisions matching the given trace_id.
func (s *TimelineService) ListPolicyDecisions(traceID string) ([]PolicyDecisionRecord, error) {
	rows, err := s.db.Query(`SELECT id, COALESCE(trace_id,''), COALESCE(task_id,''), tool, tier,
		COALESCE(sender,''), COALESCE(channel,''), allowed, COALESCE(reason,''), created_at
		FROM policy_decisions WHERE trace_id = ? ORDER BY created_at ASC`, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PolicyDecisionRecord
	for rows.Next() {
		var r PolicyDecisionRecord
		if err := rows.Scan(&r.ID, &r.TraceID, &r.TaskID, &r.Tool, &r.Tier,
			&r.Sender, &r.Channel, &r.Allowed, &r.Reason, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetTaskByTraceID returns the first task matching the given trace_id (nil if not found).
func (s *TimelineService) GetTaskByTraceID(traceID string) (*AgentTask, error) {
	row := s.db.QueryRow(`SELECT id, task_id, COALESCE(idempotency_key,''), COALESCE(trace_id,''),
		channel, chat_id, COALESCE(sender_id,''), status, COALESCE(content_in,''), COALESCE(content_out,''),
		COALESCE(error_text,''), COALESCE(delivery_status,'pending'), delivery_attempts,
		delivery_next_at, prompt_tokens, completion_tokens, total_tokens,
		created_at, updated_at, completed_at
		FROM tasks WHERE trace_id = ? LIMIT 1`, traceID)
	var t AgentTask
	var nextAt, completedAt *string
	err := row.Scan(&t.ID, &t.TaskID, &t.IdempotencyKey, &t.TraceID,
		&t.Channel, &t.ChatID, &t.SenderID, &t.Status, &t.ContentIn, &t.ContentOut,
		&t.ErrorText, &t.DeliveryStatus, &t.DeliveryAttempts,
		&nextAt, &t.PromptTokens, &t.CompletionTokens, &t.TotalTokens,
		&t.CreatedAt, &t.UpdatedAt, &completedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	if nextAt != nil {
		parsed, _ := time.Parse("2006-01-02 15:04:05", *nextAt)
		t.DeliveryNextAt = &parsed
	}
	if completedAt != nil {
		parsed, _ := time.Parse("2006-01-02 15:04:05", *completedAt)
		t.CompletedAt = &parsed
	}
	return &t, nil
}

// --- Trace Graph ---

// GetTraceGraph builds a trace graph from timeline events and group traces for a trace ID.
func (s *TimelineService) GetTraceGraph(traceID string) (*TraceGraph, error) {
	// Local events
	events, err := s.GetEvents(FilterArgs{Limit: 500, TraceID: traceID})
	if err != nil {
		return nil, fmt.Errorf("get trace graph events: %w", err)
	}

	var nodes []TraceNode
	edgeMap := make(map[string]string) // child -> parent

	for _, e := range events {
		spanType := "EVENT"
		switch {
		case e.Classification != "" && (contains(e.Classification, "INBOUND") || e.SenderName == "User"):
			spanType = "INBOUND"
		case e.Classification != "" && (contains(e.Classification, "OUTBOUND") || e.SenderName == "Agent"):
			spanType = "OUTBOUND"
		case contains(e.Classification, "LLM"):
			spanType = "LLM"
		case contains(e.Classification, "TOOL"):
			spanType = "TOOL"
		case contains(e.Classification, "POLICY"):
			spanType = "POLICY"
		}
		node := TraceNode{
			ID:           e.EventID,
			SpanID:       e.SpanID,
			ParentSpanID: e.ParentSpanID,
			Type:         spanType,
			Title:        e.Classification,
			StartTime:    e.Timestamp.Format("15:04:05"),
			AgentID:      "local",
		}
		nodes = append(nodes, node)
		if e.ParentSpanID != "" && e.SpanID != "" {
			edgeMap[e.SpanID] = e.ParentSpanID
		}
	}

	// Remote group traces
	remoteTraces, err := s.GetGroupTraces(traceID)
	if err == nil {
		for _, gt := range remoteTraces {
			node := TraceNode{
				ID:           fmt.Sprintf("remote-%d", gt.ID),
				SpanID:       gt.SpanID,
				ParentSpanID: gt.ParentSpanID,
				Type:         gt.SpanType,
				Title:        gt.Title,
				DurationMs:   gt.DurationMs,
				AgentID:      gt.SourceAgentID,
			}
			if gt.StartedAt != nil {
				node.StartTime = gt.StartedAt.Format("15:04:05")
			}
			if gt.EndedAt != nil {
				node.EndTime = gt.EndedAt.Format("15:04:05")
			}
			nodes = append(nodes, node)
			if gt.ParentSpanID != "" && gt.SpanID != "" {
				edgeMap[gt.SpanID] = gt.ParentSpanID
			}
		}
	}

	var edges []TraceEdge
	for child, parent := range edgeMap {
		edges = append(edges, TraceEdge{Source: parent, Target: child})
	}

	// Task info
	var taskInfo map[string]any
	if task, err := s.GetTaskByTraceID(traceID); err == nil && task != nil {
		taskInfo = map[string]any{
			"task_id":           task.TaskID,
			"status":            task.Status,
			"delivery_status":   task.DeliveryStatus,
			"prompt_tokens":     task.PromptTokens,
			"completion_tokens": task.CompletionTokens,
			"total_tokens":      task.TotalTokens,
			"channel":           task.Channel,
			"created_at":        task.CreatedAt,
			"completed_at":      task.CompletedAt,
		}
	}

	// Policy decisions
	var policyDecisions []map[string]any
	if decisions, err := s.ListPolicyDecisions(traceID); err == nil {
		for _, d := range decisions {
			policyDecisions = append(policyDecisions, map[string]any{
				"tool":    d.Tool,
				"tier":    d.Tier,
				"allowed": d.Allowed,
				"reason":  d.Reason,
				"time":    d.CreatedAt.Format("15:04:05"),
			})
		}
	}

	// Approval requests
	var approvalData []map[string]any
	if approvalRecords, err := s.GetApprovalsByTraceID(traceID); err == nil {
		for _, a := range approvalRecords {
			approvalData = append(approvalData, map[string]any{
				"approval_id": a.ApprovalID,
				"tool":        a.Tool,
				"tier":        a.Tier,
				"status":      a.Status,
				"sender":      a.Sender,
				"channel":     a.Channel,
				"time":        a.CreatedAt.Format("15:04:05"),
			})
			// Add as trace node
			node := TraceNode{
				ID:        fmt.Sprintf("approval-%s", a.ApprovalID),
				Type:      "APPROVAL",
				Title:     fmt.Sprintf("Approval: %s (tier %d) → %s", a.Tool, a.Tier, a.Status),
				StartTime: a.CreatedAt.Format("15:04:05"),
				AgentID:   "local",
			}
			if a.RespondedAt != nil {
				node.EndTime = a.RespondedAt.Format("15:04:05")
				dur := a.RespondedAt.Sub(a.CreatedAt)
				node.DurationMs = int(dur.Milliseconds())
			}
			nodes = append(nodes, node)
		}
	}

	// Delegation events (if the task has a trace-correlated task_id)
	if taskInfo != nil {
		if tid, ok := taskInfo["task_id"].(string); ok && tid != "" {
			if delegationEvents, err := s.ListDelegationEvents(tid); err == nil {
				for _, de := range delegationEvents {
					node := TraceNode{
						ID:        fmt.Sprintf("delegation-%d", de.ID),
						Type:      "DELEGATION",
						Title:     fmt.Sprintf("Delegation: %s → %s (%s)", de.SenderID, de.ReceiverID, de.EventType),
						StartTime: de.CreatedAt.Format("15:04:05"),
						AgentID:   de.SenderID,
						Output:    de.Summary,
					}
					nodes = append(nodes, node)
				}
			}
		}
	}

	return &TraceGraph{
		Nodes:           nodes,
		Edges:           edges,
		Task:            taskInfo,
		PolicyDecisions: policyDecisions,
		Approvals:       approvalData,
	}, nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- Group Traces ---

// AddGroupTrace inserts a remote agent trace span.
func (s *TimelineService) AddGroupTrace(gt *GroupTrace) error {
	_, err := s.db.Exec(`INSERT INTO group_traces
		(trace_id, source_agent_id, span_id, parent_span_id, span_type, title, content, started_at, ended_at, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		gt.TraceID, gt.SourceAgentID, gt.SpanID, gt.ParentSpanID, gt.SpanType,
		gt.Title, gt.Content, gt.StartedAt, gt.EndedAt, gt.DurationMs)
	return err
}

// GetGroupTraces returns remote trace spans for a trace ID.
func (s *TimelineService) GetGroupTraces(traceID string) ([]GroupTrace, error) {
	rows, err := s.db.Query(`SELECT id, trace_id, source_agent_id,
		COALESCE(span_id,''), COALESCE(parent_span_id,''), COALESCE(span_type,''),
		COALESCE(title,''), COALESCE(content,''), started_at, ended_at,
		COALESCE(duration_ms,0), created_at
		FROM group_traces WHERE trace_id = ? ORDER BY created_at ASC`, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupTrace
	for rows.Next() {
		var gt GroupTrace
		var startedAt, endedAt sql.NullTime
		if err := rows.Scan(&gt.ID, &gt.TraceID, &gt.SourceAgentID,
			&gt.SpanID, &gt.ParentSpanID, &gt.SpanType,
			&gt.Title, &gt.Content, &startedAt, &endedAt,
			&gt.DurationMs, &gt.CreatedAt); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			gt.StartedAt = &startedAt.Time
		}
		if endedAt.Valid {
			gt.EndedAt = &endedAt.Time
		}
		out = append(out, gt)
	}
	return out, rows.Err()
}

// --- Group Members ---

// UpsertGroupMember inserts or updates a group member in the local roster.
func (s *TimelineService) UpsertGroupMember(m *GroupMemberRecord) error {
	_, err := s.db.Exec(`INSERT INTO group_members
		(agent_id, agent_name, soul_summary, capabilities, channels, model, status, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(agent_id) DO UPDATE SET
			agent_name = excluded.agent_name,
			soul_summary = excluded.soul_summary,
			capabilities = excluded.capabilities,
			channels = excluded.channels,
			model = excluded.model,
			status = excluded.status,
			last_seen = datetime('now')`,
		m.AgentID, m.AgentName, m.SoulSummary, m.Capabilities, m.Channels, m.Model, m.Status)
	return err
}

// ListGroupMembers returns active group members (left_at IS NULL).
func (s *TimelineService) ListGroupMembers() ([]GroupMemberRecord, error) {
	rows, err := s.db.Query(`SELECT agent_id, COALESCE(agent_name,''), COALESCE(soul_summary,''),
		COALESCE(capabilities,'[]'), COALESCE(channels,'[]'), COALESCE(model,''),
		COALESCE(status,'active'), last_seen
		FROM group_members WHERE left_at IS NULL ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupMemberRecord
	for rows.Next() {
		var m GroupMemberRecord
		if err := rows.Scan(&m.AgentID, &m.AgentName, &m.SoulSummary,
			&m.Capabilities, &m.Channels, &m.Model, &m.Status, &m.LastSeen); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// RemoveGroupMember removes a member from the roster.
func (s *TimelineService) RemoveGroupMember(agentID string) error {
	_, err := s.db.Exec(`DELETE FROM group_members WHERE agent_id = ?`, agentID)
	return err
}

// SoftDeleteGroupMember marks a member as left instead of deleting.
func (s *TimelineService) SoftDeleteGroupMember(agentID string) error {
	_, err := s.db.Exec(`UPDATE group_members SET left_at = datetime('now'), status = 'left' WHERE agent_id = ?`, agentID)
	return err
}

// ReactivateGroupMember restores a soft-deleted member to active status.
func (s *TimelineService) ReactivateGroupMember(agentID string) error {
	_, err := s.db.Exec(`UPDATE group_members SET left_at = NULL, status = 'active', last_seen = datetime('now') WHERE agent_id = ?`, agentID)
	return err
}

// ListPreviousGroupMembers returns soft-deleted members (left_at IS NOT NULL).
func (s *TimelineService) ListPreviousGroupMembers() ([]GroupMemberRecord, error) {
	rows, err := s.db.Query(`SELECT agent_id, COALESCE(agent_name,''), COALESCE(soul_summary,''),
		COALESCE(capabilities,'[]'), COALESCE(channels,'[]'), COALESCE(model,''),
		COALESCE(status,'left'), last_seen, left_at
		FROM group_members WHERE left_at IS NOT NULL ORDER BY left_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupMemberRecord
	for rows.Next() {
		var m GroupMemberRecord
		var leftAt sql.NullTime
		if err := rows.Scan(&m.AgentID, &m.AgentName, &m.SoulSummary,
			&m.Capabilities, &m.Channels, &m.Model, &m.Status, &m.LastSeen, &leftAt); err != nil {
			return nil, err
		}
		if leftAt.Valid {
			m.LeftAt = &leftAt.Time
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// LogMembershipHistory inserts a join/leave event with config snapshot.
func (s *TimelineService) LogMembershipHistory(rec *GroupMembershipHistoryRecord) error {
	_, err := s.db.Exec(`INSERT INTO group_membership_history
		(agent_id, group_name, role, action, lfs_proxy_url, kafka_brokers, consumer_group,
		 agent_name, capabilities, channels, model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.AgentID, rec.GroupName, rec.Role, rec.Action,
		rec.LFSProxyURL, rec.KafkaBrokers, rec.ConsumerGroup,
		rec.AgentName, rec.Capabilities, rec.Channels, rec.Model)
	return err
}

// GetMembershipHistory returns membership history with optional filters.
func (s *TimelineService) GetMembershipHistory(agentID, groupName string, limit, offset int) ([]GroupMembershipHistoryRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, agent_id, group_name, COALESCE(role,''), action,
		COALESCE(lfs_proxy_url,''), COALESCE(kafka_brokers,''), COALESCE(consumer_group,''),
		COALESCE(agent_name,''), COALESCE(capabilities,'[]'), COALESCE(channels,'[]'),
		COALESCE(model,''), happened_at
		FROM group_membership_history WHERE 1=1`
	args := []any{}

	if agentID != "" {
		query += " AND agent_id = ?"
		args = append(args, agentID)
	}
	if groupName != "" {
		query += " AND group_name = ?"
		args = append(args, groupName)
	}
	query += " ORDER BY happened_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupMembershipHistoryRecord
	for rows.Next() {
		var r GroupMembershipHistoryRecord
		if err := rows.Scan(&r.ID, &r.AgentID, &r.GroupName, &r.Role, &r.Action,
			&r.LFSProxyURL, &r.KafkaBrokers, &r.ConsumerGroup,
			&r.AgentName, &r.Capabilities, &r.Channels, &r.Model, &r.HappenedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetLatestMembershipConfig returns the most recent "joined" config for an agent in a group.
func (s *TimelineService) GetLatestMembershipConfig(agentID, groupName string) (*GroupMembershipHistoryRecord, error) {
	var r GroupMembershipHistoryRecord
	err := s.db.QueryRow(`SELECT id, agent_id, group_name, COALESCE(role,''), action,
		COALESCE(lfs_proxy_url,''), COALESCE(kafka_brokers,''), COALESCE(consumer_group,''),
		COALESCE(agent_name,''), COALESCE(capabilities,'[]'), COALESCE(channels,'[]'),
		COALESCE(model,''), happened_at
		FROM group_membership_history
		WHERE agent_id = ? AND group_name = ? AND action = 'joined'
		ORDER BY happened_at DESC LIMIT 1`, agentID, groupName).
		Scan(&r.ID, &r.AgentID, &r.GroupName, &r.Role, &r.Action,
			&r.LFSProxyURL, &r.KafkaBrokers, &r.ConsumerGroup,
			&r.AgentName, &r.Capabilities, &r.Channels, &r.Model, &r.HappenedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetGroupStats returns aggregated communication statistics.
func (s *TimelineService) GetGroupStats() (*GroupStats, error) {
	stats := &GroupStats{
		TasksByStatus: make(map[string]int),
	}

	// 1. Tasks by status
	rows, err := s.db.Query(`SELECT COALESCE(status,'unknown'), COUNT(*) FROM group_tasks GROUP BY status`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err == nil {
				stats.TasksByStatus[status] = count
			}
		}
	}

	// 2. Tasks last 24h
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM group_tasks WHERE created_at >= datetime('now', '-1 day')`).Scan(&stats.TasksLast24h)

	// 3. Tasks last 7d
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM group_tasks WHERE created_at >= datetime('now', '-7 days')`).Scan(&stats.TasksLast7d)

	// 4. Avg/max response time (in seconds)
	_ = s.db.QueryRow(`SELECT COALESCE(AVG((julianday(responded_at) - julianday(created_at)) * 86400), 0),
		COALESCE(MAX((julianday(responded_at) - julianday(created_at)) * 86400), 0)
		FROM group_tasks WHERE responded_at IS NOT NULL`).
		Scan(&stats.AvgResponseSecs, &stats.MaxResponseSecs)

	// 5. Delegation depth
	_ = s.db.QueryRow(`SELECT COALESCE(AVG(delegation_depth), 0), COALESCE(MAX(delegation_depth), 0)
		FROM group_tasks WHERE delegation_depth > 0`).
		Scan(&stats.AvgDelegation, &stats.MaxDelegation)

	// 6. Member activity (top 20)
	actRows, err := s.db.Query(`SELECT agent_id,
		SUM(CASE WHEN role = 'requester' THEN cnt ELSE 0 END) as requested,
		SUM(CASE WHEN role = 'responder' THEN cnt ELSE 0 END) as responded,
		SUM(CASE WHEN role = 'completed' THEN cnt ELSE 0 END) as completed
		FROM (
			SELECT requester_id as agent_id, 'requester' as role, COUNT(*) as cnt FROM group_tasks GROUP BY requester_id
			UNION ALL
			SELECT responder_id, 'responder', COUNT(*) FROM group_tasks WHERE responder_id != '' GROUP BY responder_id
			UNION ALL
			SELECT responder_id, 'completed', COUNT(*) FROM group_tasks WHERE status = 'completed' AND responder_id != '' GROUP BY responder_id
		) GROUP BY agent_id ORDER BY (requested + responded) DESC LIMIT 20`)
	if err == nil {
		defer actRows.Close()
		for actRows.Next() {
			var a AgentActivityStat
			if err := actRows.Scan(&a.AgentID, &a.TasksRequested, &a.TasksResponded, &a.TasksCompleted); err == nil {
				stats.MemberActivity = append(stats.MemberActivity, a)
			}
		}
	}

	// 7. Trace performance
	trRows, err := s.db.Query(`SELECT source_agent_id, COUNT(*),
		COALESCE(AVG(duration_ms), 0), COALESCE(MAX(duration_ms), 0)
		FROM group_traces WHERE duration_ms > 0
		GROUP BY source_agent_id ORDER BY COUNT(*) DESC LIMIT 20`)
	if err == nil {
		defer trRows.Close()
		for trRows.Next() {
			var t AgentTraceStat
			if err := trRows.Scan(&t.AgentID, &t.TraceCount, &t.AvgDuration, &t.MaxDuration); err == nil {
				stats.TracePerformance = append(stats.TracePerformance, t)
			}
		}
	}

	return stats, nil
}

// ListUnifiedAudit returns a unified audit log from delegation_events, policy_decisions, and approval_requests.
func (s *TimelineService) ListUnifiedAudit(filter AuditFilter) ([]UnifiedAuditEntry, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	query := `SELECT id, source, event_type, tier, agent_id, target_id, details, created_at FROM (`

	// delegation_events
	query += `SELECT id, 'delegation' as source, event_type, depth as tier,
		sender_id as agent_id, COALESCE(receiver_id,'') as target_id,
		COALESCE(summary,'') as details, created_at
		FROM delegation_events`

	query += ` UNION ALL `

	// policy_decisions
	query += `SELECT id, 'policy' as source,
		CASE WHEN allowed THEN 'allowed' ELSE 'denied' END as event_type,
		tier, COALESCE(sender,'') as agent_id, tool as target_id,
		COALESCE(reason,'') as details, created_at
		FROM policy_decisions`

	query += ` UNION ALL `

	// approval_requests
	query += `SELECT id, 'approval' as source, status as event_type,
		tier, COALESCE(sender,'') as agent_id, tool as target_id,
		COALESCE(arguments,'') as details, created_at
		FROM approval_requests`

	query += ` UNION ALL `

	// mode_change events from timeline
	query += `SELECT id, 'mode_change' as source, classification as event_type,
		0 as tier, sender_id as agent_id, '' as target_id,
		content_text as details, timestamp as created_at
		FROM timeline WHERE classification = 'MODE_CHANGE'`

	query += `) AS unified WHERE 1=1`
	args := []any{}

	if filter.Source != "" {
		query += " AND source = ?"
		args = append(args, filter.Source)
	}
	if filter.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, filter.EventType)
	}
	if filter.AgentID != "" {
		query += " AND agent_id = ?"
		args = append(args, filter.AgentID)
	}
	if filter.StartAt != nil {
		query += " AND created_at >= ?"
		args = append(args, *filter.StartAt)
	}
	if filter.EndAt != nil {
		query += " AND created_at <= ?"
		args = append(args, *filter.EndAt)
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UnifiedAuditEntry
	for rows.Next() {
		var e UnifiedAuditEntry
		if err := rows.Scan(&e.ID, &e.Source, &e.EventType, &e.Tier,
			&e.AgentID, &e.TargetID, &e.Details, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkStaleMembers marks members not seen since the given cutoff as "stale".
func (s *TimelineService) MarkStaleMembers(cutoff time.Time) (int64, error) {
	result, err := s.db.Exec(`UPDATE group_members SET status = 'stale' WHERE last_seen < ? AND status = 'active'`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// --- Group Tasks ---

// InsertGroupTask inserts a new group collaboration task.
func (s *TimelineService) InsertGroupTask(task *GroupTaskRecord) error {
	_, err := s.db.Exec(`INSERT INTO group_tasks
		(task_id, description, content, direction, requester_id, responder_id, response_content, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		task.TaskID, task.Description, task.Content, task.Direction,
		task.RequesterID, task.ResponderID, task.ResponseContent, task.Status)
	return err
}

// UpdateGroupTaskResponse updates a group task with response data.
func (s *TimelineService) UpdateGroupTaskResponse(taskID, responderID, content, status string) error {
	_, err := s.db.Exec(`UPDATE group_tasks SET
		responder_id = ?, response_content = ?, status = ?, responded_at = datetime('now')
		WHERE task_id = ?`,
		responderID, content, status, taskID)
	return err
}

// ListGroupTasks returns group tasks filtered by direction and status.
func (s *TimelineService) ListGroupTasks(direction, status string, limit, offset int) ([]GroupTaskRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, task_id, COALESCE(description,''), COALESCE(content,''),
		direction, requester_id, COALESCE(responder_id,''),
		COALESCE(response_content,''), status, created_at, responded_at
		FROM group_tasks WHERE 1=1`
	args := []interface{}{}

	if direction != "" {
		query += " AND direction = ?"
		args = append(args, direction)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupTaskRecord
	for rows.Next() {
		var t GroupTaskRecord
		var respondedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.TaskID, &t.Description, &t.Content,
			&t.Direction, &t.RequesterID, &t.ResponderID,
			&t.ResponseContent, &t.Status, &t.CreatedAt, &respondedAt); err != nil {
			return nil, err
		}
		if respondedAt.Valid {
			t.RespondedAt = &respondedAt.Time
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListAllGroupTraces returns paginated group traces with optional agent filter.
func (s *TimelineService) ListAllGroupTraces(limit, offset int, agentFilter string) ([]GroupTrace, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, trace_id, source_agent_id,
		COALESCE(span_id,''), COALESCE(parent_span_id,''), COALESCE(span_type,''),
		COALESCE(title,''), COALESCE(content,''), started_at, ended_at,
		COALESCE(duration_ms,0), created_at
		FROM group_traces WHERE 1=1`
	args := []interface{}{}

	if agentFilter != "" {
		query += " AND source_agent_id = ?"
		args = append(args, agentFilter)
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupTrace
	for rows.Next() {
		var gt GroupTrace
		var startedAt, endedAt sql.NullTime
		if err := rows.Scan(&gt.ID, &gt.TraceID, &gt.SourceAgentID,
			&gt.SpanID, &gt.ParentSpanID, &gt.SpanType,
			&gt.Title, &gt.Content, &startedAt, &endedAt,
			&gt.DurationMs, &gt.CreatedAt); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			gt.StartedAt = &startedAt.Time
		}
		if endedAt.Valid {
			gt.EndedAt = &endedAt.Time
		}
		out = append(out, gt)
	}
	return out, rows.Err()
}

// --- Group Memory Items ---

// InsertGroupMemoryItem inserts a shared memory item record.
func (s *TimelineService) InsertGroupMemoryItem(rec *GroupMemoryItemRecord) error {
	_, err := s.db.Exec(`INSERT INTO group_memory_items
		(item_id, author_id, title, content_type, tags, lfs_bucket, lfs_key, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(item_id) DO UPDATE SET
			title = excluded.title,
			tags = excluded.tags,
			metadata = excluded.metadata`,
		rec.ItemID, rec.AuthorID, rec.Title, rec.ContentType,
		rec.Tags, rec.LFSBucket, rec.LFSKey, rec.Metadata)
	return err
}

// ListGroupMemoryItems returns memory items with optional author filter.
func (s *TimelineService) ListGroupMemoryItems(authorID string, limit, offset int) ([]GroupMemoryItemRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT id, item_id, author_id, COALESCE(title,''),
		COALESCE(content_type,'text/plain'), COALESCE(tags,'[]'),
		COALESCE(lfs_bucket,''), COALESCE(lfs_key,''),
		COALESCE(metadata,'{}'), created_at
		FROM group_memory_items WHERE 1=1`
	args := []interface{}{}

	if authorID != "" {
		query += " AND author_id = ?"
		args = append(args, authorID)
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupMemoryItemRecord
	for rows.Next() {
		var r GroupMemoryItemRecord
		if err := rows.Scan(&r.ID, &r.ItemID, &r.AuthorID, &r.Title,
			&r.ContentType, &r.Tags, &r.LFSBucket, &r.LFSKey,
			&r.Metadata, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetGroupMemoryItem returns a single memory item by item_id.
func (s *TimelineService) GetGroupMemoryItem(itemID string) (*GroupMemoryItemRecord, error) {
	var r GroupMemoryItemRecord
	err := s.db.QueryRow(`SELECT id, item_id, author_id, COALESCE(title,''),
		COALESCE(content_type,'text/plain'), COALESCE(tags,'[]'),
		COALESCE(lfs_bucket,''), COALESCE(lfs_key,''),
		COALESCE(metadata,'{}'), created_at
		FROM group_memory_items WHERE item_id = ?`, itemID).
		Scan(&r.ID, &r.ItemID, &r.AuthorID, &r.Title,
			&r.ContentType, &r.Tags, &r.LFSBucket, &r.LFSKey,
			&r.Metadata, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// --- Group Skill Channels ---

// InsertGroupSkillChannel registers a skill channel.
func (s *TimelineService) InsertGroupSkillChannel(rec *GroupSkillChannelRecord) error {
	_, err := s.db.Exec(`INSERT INTO group_skill_channels
		(skill_name, group_name, requests_topic, responses_topic, registered_by)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(skill_name, group_name) DO UPDATE SET
			requests_topic = excluded.requests_topic,
			responses_topic = excluded.responses_topic,
			registered_by = excluded.registered_by`,
		rec.SkillName, rec.GroupName, rec.RequestsTopic, rec.ResponsesTopic, rec.RegisteredBy)
	return err
}

// ListGroupSkillChannels returns skill channels for a group.
func (s *TimelineService) ListGroupSkillChannels(groupName string) ([]GroupSkillChannelRecord, error) {
	query := `SELECT id, skill_name, group_name, requests_topic, responses_topic, registered_by, created_at
		FROM group_skill_channels WHERE 1=1`
	args := []interface{}{}

	if groupName != "" {
		query += " AND group_name = ?"
		args = append(args, groupName)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupSkillChannelRecord
	for rows.Next() {
		var r GroupSkillChannelRecord
		if err := rows.Scan(&r.ID, &r.SkillName, &r.GroupName,
			&r.RequestsTopic, &r.ResponsesTopic, &r.RegisteredBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Approval Requests ---

// InsertApprovalRequest persists a new approval request.
func (s *TimelineService) InsertApprovalRequest(approvalID, traceID, taskID, tool string, tier int, arguments, sender, channel string) error {
	_, err := s.db.Exec(`INSERT INTO approval_requests
		(approval_id, trace_id, task_id, tool, tier, arguments, sender, channel, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
		approvalID, traceID, taskID, tool, tier, arguments, sender, channel)
	return err
}

// UpdateApprovalStatus updates the status and responded_at timestamp.
func (s *TimelineService) UpdateApprovalStatus(approvalID, status string) error {
	_, err := s.db.Exec(`UPDATE approval_requests SET status = ?, responded_at = datetime('now') WHERE approval_id = ?`,
		status, approvalID)
	return err
}

// GetPendingApprovals returns all approval requests with status 'pending'.
func (s *TimelineService) GetPendingApprovals() ([]ApprovalRecord, error) {
	rows, err := s.db.Query(`SELECT id, approval_id, COALESCE(trace_id,''), COALESCE(task_id,''),
		tool, tier, COALESCE(arguments,''), COALESCE(sender,''), COALESCE(channel,''),
		status, created_at, responded_at
		FROM approval_requests WHERE status = 'pending' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ApprovalRecord
	for rows.Next() {
		var r ApprovalRecord
		var respondedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.ApprovalID, &r.TraceID, &r.TaskID,
			&r.Tool, &r.Tier, &r.Arguments, &r.Sender, &r.Channel,
			&r.Status, &r.CreatedAt, &respondedAt); err != nil {
			return nil, err
		}
		if respondedAt.Valid {
			r.RespondedAt = &respondedAt.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetApprovalsByTraceID returns approval records for a given trace ID.
func (s *TimelineService) GetApprovalsByTraceID(traceID string) ([]ApprovalRecord, error) {
	rows, err := s.db.Query(`SELECT id, approval_id, COALESCE(trace_id,''), COALESCE(task_id,''),
		tool, tier, COALESCE(arguments,''), COALESCE(sender,''), COALESCE(channel,''),
		status, created_at, responded_at
		FROM approval_requests WHERE trace_id = ? ORDER BY created_at ASC`, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ApprovalRecord
	for rows.Next() {
		var r ApprovalRecord
		var respondedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.ApprovalID, &r.TraceID, &r.TaskID,
			&r.Tool, &r.Tier, &r.Arguments, &r.Sender, &r.Channel,
			&r.Status, &r.CreatedAt, &respondedAt); err != nil {
			return nil, err
		}
		if respondedAt.Valid {
			r.RespondedAt = &respondedAt.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Scheduled Jobs ---

// UpsertScheduledJob inserts or updates a scheduled job run record.
func (s *TimelineService) UpsertScheduledJob(jobName, status string, runAt time.Time) error {
	_, err := s.db.Exec(`INSERT INTO scheduled_jobs (job_name, last_status, last_run_at, run_count, updated_at)
		VALUES (?, ?, ?, 1, datetime('now'))
		ON CONFLICT(job_name) DO UPDATE SET
			last_status = excluded.last_status,
			last_run_at = excluded.last_run_at,
			run_count = scheduled_jobs.run_count + 1,
			updated_at = datetime('now')`,
		jobName, status, runAt)
	return err
}

// GetScheduledJob returns a scheduled job record by name.
func (s *TimelineService) GetScheduledJob(jobName string) (*ScheduledJobRecord, error) {
	var r ScheduledJobRecord
	var lastRunAt sql.NullTime
	err := s.db.QueryRow(`SELECT id, job_name, COALESCE(last_status,''), last_run_at,
		run_count, created_at, updated_at
		FROM scheduled_jobs WHERE job_name = ?`, jobName).
		Scan(&r.ID, &r.JobName, &r.LastStatus, &lastRunAt,
			&r.RunCount, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if lastRunAt.Valid {
		r.LastRunAt = lastRunAt.Time
	}
	return &r, nil
}

// ListScheduledJobs returns all scheduled job records.
func (s *TimelineService) ListScheduledJobs() ([]ScheduledJobRecord, error) {
	rows, err := s.db.Query(`SELECT id, job_name, COALESCE(last_status,''), last_run_at,
		run_count, created_at, updated_at
		FROM scheduled_jobs ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScheduledJobRecord
	for rows.Next() {
		var r ScheduledJobRecord
		var lastRunAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.JobName, &r.LastStatus, &lastRunAt,
			&r.RunCount, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if lastRunAt.Valid {
			r.LastRunAt = lastRunAt.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Delegation Events ---

// LogDelegationEvent records a delegation audit event.
func (s *TimelineService) LogDelegationEvent(taskID, eventType, senderID, receiverID, summary string, depth int) error {
	_, err := s.db.Exec(`INSERT INTO delegation_events
		(task_id, event_type, sender_id, receiver_id, summary, depth)
		VALUES (?, ?, ?, ?, ?, ?)`,
		taskID, eventType, senderID, receiverID, summary, depth)
	return err
}

// ListDelegationEvents returns delegation events for a task.
func (s *TimelineService) ListDelegationEvents(taskID string) ([]DelegationEventRecord, error) {
	rows, err := s.db.Query(`SELECT id, task_id, event_type, sender_id,
		COALESCE(receiver_id,''), COALESCE(summary,''), depth, created_at
		FROM delegation_events WHERE task_id = ? ORDER BY created_at ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DelegationEventRecord
	for rows.Next() {
		var r DelegationEventRecord
		if err := rows.Scan(&r.ID, &r.TaskID, &r.EventType, &r.SenderID,
			&r.ReceiverID, &r.Summary, &r.Depth, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Delegation-aware Group Task methods ---

// AcceptGroupTask marks a group task as accepted.
func (s *TimelineService) AcceptGroupTask(taskID, responderID string) error {
	_, err := s.db.Exec(`UPDATE group_tasks SET
		responder_id = ?, accepted_at = datetime('now'), status = 'accepted'
		WHERE task_id = ?`, responderID, taskID)
	return err
}

// ListExpiredGroupTasks returns group tasks past their deadline that are still pending.
func (s *TimelineService) ListExpiredGroupTasks() ([]GroupTaskRecord, error) {
	rows, err := s.db.Query(`SELECT id, task_id, COALESCE(description,''), COALESCE(content,''),
		direction, requester_id, COALESCE(responder_id,''),
		COALESCE(response_content,''), status, created_at, responded_at
		FROM group_tasks
		WHERE deadline_at IS NOT NULL AND deadline_at < datetime('now')
			AND status IN ('pending', 'accepted')
		ORDER BY deadline_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupTaskRecord
	for rows.Next() {
		var t GroupTaskRecord
		var respondedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.TaskID, &t.Description, &t.Content,
			&t.Direction, &t.RequesterID, &t.ResponderID,
			&t.ResponseContent, &t.Status, &t.CreatedAt, &respondedAt); err != nil {
			return nil, err
		}
		if respondedAt.Valid {
			t.RespondedAt = &respondedAt.Time
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetDelegationChain returns the chain of delegated tasks from a root task.
func (s *TimelineService) GetDelegationChain(rootTaskID string) ([]GroupTaskRecord, error) {
	var chain []GroupTaskRecord
	queue := []string{rootTaskID}
	visited := make(map[string]bool)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		rows, err := s.db.Query(`SELECT id, task_id, COALESCE(description,''), COALESCE(content,''),
			direction, requester_id, COALESCE(responder_id,''),
			COALESCE(response_content,''), status,
			COALESCE(parent_task_id,''), COALESCE(delegation_depth,0),
			COALESCE(original_requester_id,''),
			deadline_at, accepted_at, created_at, responded_at
			FROM group_tasks
			WHERE task_id = ? OR parent_task_id = ?`, current, current)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var t GroupTaskRecord
			var deadlineAt, acceptedAt, respondedAt sql.NullTime
			if err := rows.Scan(&t.ID, &t.TaskID, &t.Description, &t.Content,
				&t.Direction, &t.RequesterID, &t.ResponderID,
				&t.ResponseContent, &t.Status,
				&t.ParentTaskID, &t.DelegationDepth,
				&t.OriginalRequesterID,
				&deadlineAt, &acceptedAt, &t.CreatedAt, &respondedAt); err != nil {
				rows.Close()
				return nil, err
			}
			if deadlineAt.Valid {
				t.DeadlineAt = &deadlineAt.Time
			}
			if acceptedAt.Valid {
				t.AcceptedAt = &acceptedAt.Time
			}
			if respondedAt.Valid {
				t.RespondedAt = &respondedAt.Time
			}
			if !visited[t.TaskID] {
				chain = append(chain, t)
				queue = append(queue, t.TaskID)
			}
		}
		rows.Close()
	}
	return chain, nil
}

// InsertDelegatedGroupTask inserts a delegated group task with parent/depth info.
func (s *TimelineService) InsertDelegatedGroupTask(task *GroupTaskRecord) error {
	_, err := s.db.Exec(`INSERT INTO group_tasks
		(task_id, description, content, direction, requester_id, responder_id,
		 response_content, status, parent_task_id, delegation_depth,
		 original_requester_id, deadline_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.TaskID, task.Description, task.Content, task.Direction,
		task.RequesterID, task.ResponderID, task.ResponseContent, task.Status,
		task.ParentTaskID, task.DelegationDepth,
		task.OriginalRequesterID, task.DeadlineAt)
	return err
}

// --- Topic Message Log ---

// LogTopicMessage inserts a topic message event.
func (s *TimelineService) LogTopicMessage(rec *TopicMessageLogRecord) error {
	_, err := s.db.Exec(`INSERT INTO topic_message_log
		(topic_name, sender_id, envelope_type, correlation_id, payload_size)
		VALUES (?, ?, ?, ?, ?)`,
		rec.TopicName, rec.SenderID, rec.EnvelopeType, rec.CorrelationID, rec.PayloadSize)
	return err
}

// GetTopicStats returns per-topic aggregated statistics from topic_message_log.
func (s *TimelineService) GetTopicStats() ([]TopicStat, error) {
	rows, err := s.db.Query(`SELECT topic_name,
		COUNT(*) as message_count,
		SUM(CASE WHEN created_at >= datetime('now','-1 day') THEN 1 ELSE 0 END) as last_24h,
		SUM(CASE WHEN created_at >= datetime('now','-7 days') THEN 1 ELSE 0 END) as last_7d,
		COUNT(DISTINCT sender_id) as unique_agents,
		MAX(created_at) as last_message_at,
		AVG(payload_size) as avg_payload_size
		FROM topic_message_log
		GROUP BY topic_name
		ORDER BY message_count DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TopicStat
	for rows.Next() {
		var ts TopicStat
		var lastMsg sql.NullString
		var avgSize sql.NullFloat64
		if err := rows.Scan(&ts.TopicName, &ts.MessageCount,
			&ts.Last24h, &ts.Last7d, &ts.UniqueAgents,
			&lastMsg, &avgSize); err != nil {
			return nil, err
		}
		if lastMsg.Valid {
			ts.LastMessageAt = lastMsg.String
		}
		if avgSize.Valid {
			ts.AvgPayloadSize = avgSize.Float64
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// GetTopicFlowData returns agent-to-agent message flow edges through topics for 3D visualization.
func (s *TimelineService) GetTopicFlowData() ([]TopicFlowEdge, error) {
	rows, err := s.db.Query(`SELECT t1.sender_id as source_agent,
		t1.topic_name,
		t2.sender_id as target_agent,
		COUNT(*) as message_count
		FROM topic_message_log t1
		JOIN topic_message_log t2 ON t1.topic_name = t2.topic_name
			AND t1.sender_id != t2.sender_id
			AND t2.created_at > t1.created_at
			AND t2.created_at <= datetime(t1.created_at, '+5 minutes')
		GROUP BY t1.sender_id, t1.topic_name, t2.sender_id
		ORDER BY message_count DESC
		LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TopicFlowEdge
	for rows.Next() {
		var e TopicFlowEdge
		if err := rows.Scan(&e.SourceAgentID, &e.TopicName, &e.TargetAgentID, &e.MessageCount); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetAgentXP calculates gamification XP from group activity.
func (s *TimelineService) GetAgentXP() ([]AgentXP, error) {
	rows, err := s.db.Query(`SELECT gm.agent_id, COALESCE(gm.agent_name,''),
		COALESCE(tc.cnt, 0) as tasks_completed,
		COALESCE(tr.cnt, 0) as traces_shared,
		COALESCE(mem.cnt, 0) as memories_shared,
		COALESCE(sk.cnt, 0) as skills_registered
		FROM group_members gm
		LEFT JOIN (SELECT responder_id, COUNT(*) cnt FROM group_tasks WHERE status='completed' GROUP BY responder_id) tc ON gm.agent_id = tc.responder_id
		LEFT JOIN (SELECT source_agent_id, COUNT(*) cnt FROM group_traces GROUP BY source_agent_id) tr ON gm.agent_id = tr.source_agent_id
		LEFT JOIN (SELECT author_id, COUNT(*) cnt FROM group_memory_items GROUP BY author_id) mem ON gm.agent_id = mem.author_id
		LEFT JOIN (SELECT registered_by, COUNT(*) cnt FROM group_skill_channels GROUP BY registered_by) sk ON gm.agent_id = sk.registered_by
		WHERE gm.left_at IS NULL
		GROUP BY gm.agent_id
		ORDER BY (COALESCE(tc.cnt,0)*10 + COALESCE(tr.cnt,0)*5 + COALESCE(mem.cnt,0)*15 + COALESCE(sk.cnt,0)*25) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentXP
	for rows.Next() {
		var a AgentXP
		if err := rows.Scan(&a.AgentID, &a.AgentName,
			&a.TasksCompleted, &a.TracesShared,
			&a.MemoriesShared, &a.SkillsRegistered); err != nil {
			return nil, err
		}
		a.TotalXP = a.TasksCompleted*10 + a.TracesShared*5 + a.MemoriesShared*15 + a.SkillsRegistered*25
		// Level: floor(sqrt(totalXP / 10))
		if a.TotalXP > 0 {
			a.Level = int(sqrtInt(a.TotalXP / 10))
		}
		// Rank tiers
		switch {
		case a.TotalXP >= 350:
			a.Rank = "Diamond"
		case a.TotalXP >= 150:
			a.Rank = "Gold"
		case a.TotalXP >= 50:
			a.Rank = "Silver"
		default:
			a.Rank = "Bronze"
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// sqrtInt returns integer square root (floor).
func sqrtInt(n int) int {
	if n <= 0 {
		return 0
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x = y
		y = (x + n/x) / 2
	}
	return x
}

// GetTopicHealth returns per-topic health/pulse data.
func (s *TimelineService) GetTopicHealth() ([]TopicHealth, error) {
	rows, err := s.db.Query(`SELECT topic_name,
		SUM(CASE WHEN created_at >= datetime('now','-4 hours') THEN 1 ELSE 0 END) as msgs_4h,
		COUNT(DISTINCT CASE WHEN created_at >= datetime('now','-1 hour') THEN sender_id END) as active_agents,
		MAX(created_at) as last_msg
		FROM topic_message_log
		GROUP BY topic_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TopicHealth
	for rows.Next() {
		var h TopicHealth
		var msgs4h int
		var lastMsg sql.NullString
		if err := rows.Scan(&h.TopicName, &msgs4h, &h.ActiveAgents, &lastMsg); err != nil {
			return nil, err
		}
		h.MessagesPerHour = float64(msgs4h) / 4.0
		// Stale if no messages in last hour
		h.IsStale = h.ActiveAgents == 0
		// Score: 0-100 based on activity
		h.Score = 100
		if h.IsStale {
			h.Score = 20
		} else if h.MessagesPerHour < 1 {
			h.Score = 50
		} else if h.MessagesPerHour < 5 {
			h.Score = 75
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// GetTopicMessages returns recent messages for a specific topic (for browsing).
func (s *TimelineService) GetTopicMessages(topicName string, limit int) ([]TopicMessageLogRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, topic_name, sender_id, envelope_type,
		COALESCE(correlation_id,''), payload_size, created_at
		FROM topic_message_log WHERE topic_name = ?
		ORDER BY created_at DESC LIMIT ?`, topicName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TopicMessageLogRecord
	for rows.Next() {
		var r TopicMessageLogRecord
		if err := rows.Scan(&r.ID, &r.TopicName, &r.SenderID, &r.EnvelopeType,
			&r.CorrelationID, &r.PayloadSize, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetTopicMessageDensity returns hourly message count buckets for a topic over the last N hours.
func (s *TimelineService) GetTopicMessageDensity(topicName string, hours int) ([]TopicDensityBucket, error) {
	if hours <= 0 {
		hours = 48
	}
	rows, err := s.db.Query(`
		WITH RECURSIVE buckets(bucket_start) AS (
			SELECT datetime('now', '-' || ? || ' hours')
			UNION ALL
			SELECT datetime(bucket_start, '+1 hour')
			FROM buckets
			WHERE bucket_start < datetime('now')
		)
		SELECT b.bucket_start,
			datetime(b.bucket_start, '+1 hour') as bucket_end,
			COUNT(t.id) as cnt
		FROM buckets b
		LEFT JOIN topic_message_log t
			ON t.topic_name = ?
			AND t.created_at >= b.bucket_start
			AND t.created_at < datetime(b.bucket_start, '+1 hour')
		WHERE b.bucket_start < datetime('now')
		GROUP BY b.bucket_start
		ORDER BY b.bucket_start`, hours, topicName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TopicDensityBucket
	for rows.Next() {
		var b TopicDensityBucket
		if err := rows.Scan(&b.BucketStart, &b.BucketEnd, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetTopicEnvelopeTypeCounts returns per-envelope-type message counts for a topic.
func (s *TimelineService) GetTopicEnvelopeTypeCounts(topicName string) (map[string]int, error) {
	rows, err := s.db.Query(`SELECT envelope_type, COUNT(*) as cnt
		FROM topic_message_log
		WHERE topic_name = ?
		GROUP BY envelope_type
		ORDER BY cnt DESC`, topicName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]int)
	for rows.Next() {
		var envType string
		var cnt int
		if err := rows.Scan(&envType, &cnt); err != nil {
			return nil, err
		}
		out[envType] = cnt
	}
	return out, rows.Err()
}
