package memory

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
)

// SkillEvent records a single interaction with a skill domain.
type SkillEvent struct {
	SkillName  string
	TaskID     string
	Action     string  // "task_completed", "tool_used", "error", "user_feedback"
	Quality    float64 // 0.0 - 1.0
	DurationMs int64
	Metadata   string // JSON blob
}

// ExpertiseSummary holds aggregated expertise stats for a skill.
type ExpertiseSummary struct {
	SkillName    string
	SuccessCount int
	FailureCount int
	AvgQuality   float64
	LastUsed     time.Time
	TotalDurMs   int64
	Trend        string // "improving", "stable", "declining"
	Score        float64
}

// ExpertiseTracker records and queries agent skill proficiency.
type ExpertiseTracker struct {
	db *sql.DB
}

// NewExpertiseTracker creates a new ExpertiseTracker using the given database.
func NewExpertiseTracker(db *sql.DB) *ExpertiseTracker {
	return &ExpertiseTracker{db: db}
}

// RecordEvent persists a skill event and updates the aggregate expertise row.
func (e *ExpertiseTracker) RecordEvent(evt SkillEvent) error {
	if e == nil || e.db == nil {
		return nil
	}
	if evt.SkillName == "" {
		return nil
	}

	tx, err := e.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Insert event
	_, err = tx.Exec(`INSERT INTO skill_events (skill_name, task_id, action, quality, duration_ms, metadata)
		VALUES (?, ?, ?, ?, ?, ?)`,
		evt.SkillName, evt.TaskID, evt.Action, evt.Quality, evt.DurationMs, evt.Metadata)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	// Upsert aggregate
	isSuccess := evt.Action == "task_completed" || evt.Action == "tool_used" || evt.Action == "user_feedback"
	successInc, failureInc := 0, 0
	if isSuccess {
		successInc = 1
	} else {
		failureInc = 1
	}

	_, err = tx.Exec(`INSERT INTO agent_expertise (skill_name, success_count, failure_count, avg_quality, last_used, total_duration_ms, trend)
		VALUES (?, ?, ?, ?, ?, ?, 'stable')
		ON CONFLICT(skill_name) DO UPDATE SET
			success_count = success_count + ?,
			failure_count = failure_count + ?,
			total_duration_ms = total_duration_ms + ?,
			last_used = ?`,
		evt.SkillName, successInc, failureInc, evt.Quality, time.Now(), evt.DurationMs,
		successInc, failureInc, evt.DurationMs, time.Now())
	if err != nil {
		return fmt.Errorf("upsert expertise: %w", err)
	}

	// Recompute avg_quality from last 50 events
	var avgQ float64
	err = tx.QueryRow(`SELECT COALESCE(AVG(quality), 0.5) FROM (
		SELECT quality FROM skill_events WHERE skill_name = ? ORDER BY created_at DESC LIMIT 50
	)`, evt.SkillName).Scan(&avgQ)
	if err == nil {
		_, _ = tx.Exec(`UPDATE agent_expertise SET avg_quality = ? WHERE skill_name = ?`, avgQ, evt.SkillName)
	}

	// Recompute trend: last-10 vs previous-10
	trend := computeTrend(tx, evt.SkillName)
	_, _ = tx.Exec(`UPDATE agent_expertise SET trend = ? WHERE skill_name = ?`, trend, evt.SkillName)

	return tx.Commit()
}

// GetExpertise returns the expertise summary for a given skill.
func (e *ExpertiseTracker) GetExpertise(skillName string) (*ExpertiseSummary, error) {
	if e == nil || e.db == nil {
		return nil, nil
	}

	var s ExpertiseSummary
	var lastUsed sql.NullTime
	err := e.db.QueryRow(`SELECT skill_name, success_count, failure_count, avg_quality, last_used, total_duration_ms, trend
		FROM agent_expertise WHERE skill_name = ?`, skillName).Scan(
		&s.SkillName, &s.SuccessCount, &s.FailureCount, &s.AvgQuality, &lastUsed, &s.TotalDurMs, &s.Trend)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		s.LastUsed = lastUsed.Time
	}
	s.Score = computeScore(s)
	return &s, nil
}

// ListExpertise returns all expertise summaries, ordered by score descending.
func (e *ExpertiseTracker) ListExpertise() ([]ExpertiseSummary, error) {
	if e == nil || e.db == nil {
		return nil, nil
	}

	rows, err := e.db.Query(`SELECT skill_name, success_count, failure_count, avg_quality, last_used, total_duration_ms, trend
		FROM agent_expertise ORDER BY avg_quality DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ExpertiseSummary
	for rows.Next() {
		var s ExpertiseSummary
		var lastUsed sql.NullTime
		if err := rows.Scan(&s.SkillName, &s.SuccessCount, &s.FailureCount, &s.AvgQuality, &lastUsed, &s.TotalDurMs, &s.Trend); err != nil {
			continue
		}
		if lastUsed.Valid {
			s.LastUsed = lastUsed.Time
		}
		s.Score = computeScore(s)
		results = append(results, s)
	}
	return results, nil
}

// computeScore calculates the weighted expertise score.
// score = 0.6*successRate + 0.3*avgQuality + 0.1*experienceBonus
func computeScore(s ExpertiseSummary) float64 {
	total := float64(s.SuccessCount + s.FailureCount)
	if total == 0 {
		return 0
	}
	successRate := float64(s.SuccessCount) / total
	experienceBonus := math.Min(1.0, total/100.0)
	return 0.6*successRate + 0.3*s.AvgQuality + 0.1*experienceBonus
}

// computeTrend compares the avg quality of last-10 vs previous-10 events.
func computeTrend(tx *sql.Tx, skillName string) string {
	var recent, previous float64
	var recentCount, prevCount int

	rows, err := tx.Query(`SELECT quality FROM skill_events WHERE skill_name = ? ORDER BY created_at DESC LIMIT 20`, skillName)
	if err != nil {
		return "stable"
	}
	defer rows.Close()

	var qualities []float64
	for rows.Next() {
		var q float64
		if err := rows.Scan(&q); err == nil {
			qualities = append(qualities, q)
		}
	}

	for i, q := range qualities {
		if i < 10 {
			recent += q
			recentCount++
		} else {
			previous += q
			prevCount++
		}
	}

	if recentCount == 0 || prevCount == 0 {
		return "stable"
	}

	recentAvg := recent / float64(recentCount)
	prevAvg := previous / float64(prevCount)
	delta := recentAvg - prevAvg

	if delta > 0.1 {
		return "improving"
	}
	if delta < -0.1 {
		return "declining"
	}
	return "stable"
}

// ClassifySkill maps a tool name to a skill domain.
func ClassifySkill(toolName string) string {
	switch {
	case toolName == "read_file" || toolName == "write_file" || toolName == "edit_file" || toolName == "list_dir":
		return "filesystem"
	case toolName == "exec":
		return "shell"
	case toolName == "remember" || toolName == "recall":
		return "memory"
	case toolName == "web_search" || toolName == "web_fetch":
		return "research"
	case toolName == "message":
		return "communication"
	case strings.HasPrefix(toolName, "day2day") || strings.HasPrefix(toolName, "dt"):
		return "day2day"
	default:
		return "general"
	}
}

// RecordToolUse is a convenience method that records a tool execution as a skill event.
func (e *ExpertiseTracker) RecordToolUse(toolName, taskID string, durationMs int64, success bool) {
	if e == nil {
		return
	}
	quality := 0.7
	action := "tool_used"
	if !success {
		quality = 0.2
		action = "error"
	}
	if err := e.RecordEvent(SkillEvent{
		SkillName:  ClassifySkill(toolName),
		TaskID:     taskID,
		Action:     action,
		Quality:    quality,
		DurationMs: durationMs,
		Metadata:   fmt.Sprintf(`{"tool":"%s"}`, toolName),
	}); err != nil {
		slog.Debug("Failed to record tool use", "tool", toolName, "error", err)
	}
}

// RecordTaskCompletion records a completed task interaction.
func (e *ExpertiseTracker) RecordTaskCompletion(skillName, taskID string, durationMs int64, quality float64) {
	if e == nil {
		return
	}
	if err := e.RecordEvent(SkillEvent{
		SkillName:  skillName,
		TaskID:     taskID,
		Action:     "task_completed",
		Quality:    quality,
		DurationMs: durationMs,
	}); err != nil {
		slog.Debug("Failed to record task completion", "skill", skillName, "error", err)
	}
}
