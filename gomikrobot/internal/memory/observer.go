package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kamir/gomikrobot/internal/provider"
)

// ObserverConfig configures the observational memory system.
type ObserverConfig struct {
	Enabled          bool
	Model            string // LLM model for compression (empty = use agent's default)
	MessageThreshold int    // compress after N unobserved messages (default: 50)
	MaxObservations  int    // trigger reflector after this many (default: 200)
}

// Observation represents a compressed memory note derived from conversation.
type Observation struct {
	ID           string
	SessionID    string
	Content      string    // compressed observation text
	Priority     string    // "high", "medium", "low"
	ObservedAt   time.Time // when observation was created
	ReferencedAt time.Time // earliest event referenced
}

// Observer compresses old conversation messages into prioritized, dated
// observation notes using an LLM background pass.
type Observer struct {
	config   ObserverConfig
	provider provider.LLMProvider
	db       *sql.DB
}

// NewObserver creates a new Observer. Returns nil if disabled or provider is nil.
func NewObserver(cfg ObserverConfig, prov provider.LLMProvider, db *sql.DB) *Observer {
	if !cfg.Enabled || prov == nil || db == nil {
		return nil
	}
	if cfg.MessageThreshold <= 0 {
		cfg.MessageThreshold = 50
	}
	if cfg.MaxObservations <= 0 {
		cfg.MaxObservations = 200
	}
	return &Observer{
		config:   cfg,
		provider: prov,
		db:       db,
	}
}

// ShouldObserve returns true if the session has enough unobserved messages
// to warrant a compression pass.
func (o *Observer) ShouldObserve(sessionID string) bool {
	if o == nil || o.db == nil {
		return false
	}
	var count int
	err := o.db.QueryRow(
		`SELECT COUNT(*) FROM observations_queue WHERE session_id = ? AND observed = 0`,
		sessionID,
	).Scan(&count)
	if err != nil {
		return false
	}
	return count >= o.config.MessageThreshold
}

// EnqueueMessage records a message for potential future observation.
func (o *Observer) EnqueueMessage(sessionID, role, content string) {
	if o == nil || o.db == nil {
		return
	}
	if len(strings.TrimSpace(content)) < 20 {
		return
	}
	_, err := o.db.Exec(
		`INSERT INTO observations_queue (session_id, role, content) VALUES (?, ?, ?)`,
		sessionID, role, content,
	)
	if err != nil {
		slog.Debug("Observer enqueue failed", "error", err)
	}
}

// Observe compresses unobserved messages for a session into observation notes.
// Runs the LLM to produce compressed, prioritized observations.
func (o *Observer) Observe(ctx context.Context, sessionID string) error {
	if o == nil || o.db == nil {
		return nil
	}

	// Fetch unobserved messages
	rows, err := o.db.Query(
		`SELECT id, role, content, created_at FROM observations_queue
		 WHERE session_id = ? AND observed = 0
		 ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("fetch unobserved: %w", err)
	}
	defer rows.Close()

	var messages []struct {
		ID        int64
		Role      string
		Content   string
		CreatedAt time.Time
	}
	for rows.Next() {
		var m struct {
			ID        int64
			Role      string
			Content   string
			CreatedAt time.Time
		}
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			continue
		}
		messages = append(messages, m)
	}

	if len(messages) < o.config.MessageThreshold {
		return nil
	}

	// Build conversation transcript for the LLM
	var transcript strings.Builder
	for _, m := range messages {
		transcript.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.CreatedAt.Format("2006-01-02 15:04"), m.Role, m.Content))
	}

	// Call LLM to compress
	model := o.config.Model
	if model == "" {
		model = o.provider.DefaultModel()
	}

	resp, err := o.provider.Chat(ctx, &provider.ChatRequest{
		Model: model,
		Messages: []provider.Message{
			{Role: "system", Content: observerPrompt},
			{Role: "user", Content: transcript.String()},
		},
		MaxTokens: 2000,
	})
	if err != nil {
		return fmt.Errorf("observer LLM call: %w", err)
	}

	// Parse and store observations
	observations := parseObservations(resp.Content, sessionID)

	tx, err := o.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, obs := range observations {
		_, err := tx.Exec(
			`INSERT INTO observations (session_id, content, priority, observed_at, referenced_at)
			 VALUES (?, ?, ?, ?, ?)`,
			obs.SessionID, obs.Content, obs.Priority, obs.ObservedAt, obs.ReferencedAt,
		)
		if err != nil {
			slog.Warn("Observer insert failed", "error", err)
		}
	}

	// Mark messages as observed
	var maxID int64
	for _, m := range messages {
		if m.ID > maxID {
			maxID = m.ID
		}
	}
	_, _ = tx.Exec(
		`UPDATE observations_queue SET observed = 1 WHERE session_id = ? AND id <= ?`,
		sessionID, maxID,
	)

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	slog.Info("Observer compressed messages", "session", sessionID,
		"messages", len(messages), "observations", len(observations))
	return nil
}

// LoadObservations returns all observations for a session, ordered by date.
func (o *Observer) LoadObservations(sessionID string) ([]Observation, error) {
	if o == nil || o.db == nil {
		return nil, nil
	}

	rows, err := o.db.Query(
		`SELECT id, session_id, content, priority, observed_at, referenced_at
		 FROM observations WHERE session_id = ?
		 ORDER BY referenced_at ASC, observed_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Observation
	for rows.Next() {
		var obs Observation
		var id int64
		if err := rows.Scan(&id, &obs.SessionID, &obs.Content, &obs.Priority, &obs.ObservedAt, &obs.ReferencedAt); err != nil {
			continue
		}
		obs.ID = fmt.Sprintf("obs-%d", id)
		results = append(results, obs)
	}
	return results, nil
}

// FormatObservations renders observations as a markdown section for the system prompt.
func FormatObservations(observations []Observation) string {
	if len(observations) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Observations (Session Memory)\n\n")

	currentDate := ""
	for _, obs := range observations {
		date := obs.ReferencedAt.Format("2006-01-02")
		if date != currentDate {
			if currentDate != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("## %s\n", date))
			currentDate = date
		}

		priority := strings.ToUpper(obs.Priority)
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", priority, obs.Content))
	}

	return sb.String()
}

// Reflect consolidates observations when they exceed MaxObservations.
// Uses the LLM to merge, deduplicate, and prune old observations.
func (o *Observer) Reflect(ctx context.Context, sessionID string) error {
	if o == nil || o.db == nil {
		return nil
	}

	observations, err := o.LoadObservations(sessionID)
	if err != nil {
		return err
	}
	if len(observations) < o.config.MaxObservations {
		return nil
	}

	// Build observation text for the LLM
	formatted := FormatObservations(observations)

	model := o.config.Model
	if model == "" {
		model = o.provider.DefaultModel()
	}

	resp, err := o.provider.Chat(ctx, &provider.ChatRequest{
		Model: model,
		Messages: []provider.Message{
			{Role: "system", Content: reflectorPrompt},
			{Role: "user", Content: formatted},
		},
		MaxTokens: 3000,
	})
	if err != nil {
		return fmt.Errorf("reflector LLM call: %w", err)
	}

	consolidated := parseObservations(resp.Content, sessionID)

	tx, err := o.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete old observations
	_, _ = tx.Exec(`DELETE FROM observations WHERE session_id = ?`, sessionID)

	// Insert consolidated
	for _, obs := range consolidated {
		_, _ = tx.Exec(
			`INSERT INTO observations (session_id, content, priority, observed_at, referenced_at)
			 VALUES (?, ?, ?, ?, ?)`,
			obs.SessionID, obs.Content, obs.Priority, obs.ObservedAt, obs.ReferencedAt,
		)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	slog.Info("Reflector consolidated observations", "session", sessionID,
		"before", len(observations), "after", len(consolidated))
	return nil
}

// ShouldReflect returns true if observations exceed the configured maximum.
func (o *Observer) ShouldReflect(sessionID string) bool {
	return o.ObservationCount(sessionID) >= o.config.MaxObservations
}

// ObservationCount returns the number of observations for a session.
func (o *Observer) ObservationCount(sessionID string) int {
	if o == nil || o.db == nil {
		return 0
	}
	var count int
	o.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE session_id = ?`, sessionID).Scan(&count)
	return count
}

// ObserverStatus holds the current observer status.
type ObserverStatus struct {
	Enabled          bool      `json:"enabled"`
	ObservationCount int       `json:"observation_count"`
	QueueDepth       int       `json:"queue_depth"`
	LastObservation  time.Time `json:"last_observation"`
}

// Status returns the current observer status.
func (o *Observer) Status() ObserverStatus {
	if o == nil || o.db == nil {
		return ObserverStatus{}
	}
	status := ObserverStatus{Enabled: true}

	o.db.QueryRow(`SELECT COUNT(*) FROM observations`).Scan(&status.ObservationCount)
	o.db.QueryRow(`SELECT COUNT(*) FROM observations_queue WHERE observed = 0`).Scan(&status.QueueDepth)
	o.db.QueryRow(`SELECT MAX(observed_at) FROM observations`).Scan(&status.LastObservation)

	return status
}

// AllObservations returns all observations across all sessions, ordered by date.
func (o *Observer) AllObservations(limit int) ([]Observation, error) {
	if o == nil || o.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := o.db.Query(
		`SELECT id, session_id, content, priority, observed_at, referenced_at
		 FROM observations ORDER BY observed_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Observation
	for rows.Next() {
		var obs Observation
		var id int64
		if err := rows.Scan(&id, &obs.SessionID, &obs.Content, &obs.Priority, &obs.ObservedAt, &obs.ReferencedAt); err != nil {
			continue
		}
		obs.ID = fmt.Sprintf("obs-%d", id)
		results = append(results, obs)
	}
	return results, nil
}

// parseObservations extracts observations from LLM response text.
// Expected format per line: "- [HIGH|MEDIUM|LOW] content text"
// Lines starting with "## YYYY-MM-DD" set the referenced date context.
func parseObservations(text, sessionID string) []Observation {
	lines := strings.Split(text, "\n")
	now := time.Now()
	refDate := now

	var results []Observation
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Date header
		if strings.HasPrefix(line, "## ") {
			dateStr := strings.TrimPrefix(line, "## ")
			dateStr = strings.TrimSpace(dateStr)
			if t, err := time.Parse("2006-01-02", dateStr); err == nil {
				refDate = t
			}
			continue
		}

		// Observation line
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		line = strings.TrimPrefix(line, "- [")
		closeBracket := strings.Index(line, "]")
		if closeBracket < 0 {
			continue
		}
		priority := strings.ToLower(strings.TrimSpace(line[:closeBracket]))
		content := strings.TrimSpace(line[closeBracket+1:])

		if content == "" {
			continue
		}

		// Normalize priority
		switch priority {
		case "high", "medium", "low":
			// ok
		default:
			priority = "medium"
		}

		results = append(results, Observation{
			SessionID:    sessionID,
			Content:      content,
			Priority:     priority,
			ObservedAt:   now,
			ReferencedAt: refDate,
		})
	}
	return results
}

const observerPrompt = `You are a memory compression agent. Your job is to compress conversation messages into concise, factual observation notes.

For each observation, output exactly this format:
- [HIGH] or [MEDIUM] or [LOW] followed by a 1-2 sentence factual summary

Group observations by date using ## YYYY-MM-DD headers.

Priority guidelines:
- HIGH: User preferences, decisions, important facts about the user, action items
- MEDIUM: Topics discussed, context established, project details
- LOW: Casual remarks, greetings, minor details

Rules:
1. Preserve all factual information — names, dates, numbers, preferences
2. Remove conversational filler, repeated content, and pleasantries
3. Keep observations atomic — one fact per observation
4. Use third person ("The user..." or "User prefers...")
5. Include the date the event was referenced, not just when it was discussed`

const reflectorPrompt = `You are a memory consolidation agent. Your job is to consolidate and compress a list of observations.

Input: A list of dated observations with priorities.
Output: A consolidated list in the same format, reduced to ~60% of the original count.

Rules:
1. Combine related observations into single entries
2. Remove observations that have been superseded by newer information
3. Preserve ALL high-priority observations
4. Keep the ## YYYY-MM-DD date grouping
5. When merging, use the most recent date
6. Identify patterns and create summary observations where appropriate
7. Never lose factual information — merge, don't delete facts`
