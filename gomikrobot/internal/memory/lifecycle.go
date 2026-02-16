package memory

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// RetentionPolicy defines how long chunks from a given source should be kept.
type RetentionPolicy struct {
	SourcePrefix string        // e.g. "conversation:", "soul:", "user"
	TTL          time.Duration // 0 = permanent
}

// LifecycleConfig holds configuration for the lifecycle manager.
type LifecycleConfig struct {
	MaxChunks int               // prune when exceeding this count (default: 50000)
	Policies  []RetentionPolicy // retention rules per source prefix
}

// LifecycleManager handles memory pruning and retention.
type LifecycleManager struct {
	db     *sql.DB
	config LifecycleConfig
}

// DefaultPolicies returns the standard retention policies.
func DefaultPolicies() []RetentionPolicy {
	return []RetentionPolicy{
		{SourcePrefix: "soul:", TTL: 0},             // permanent
		{SourcePrefix: "user", TTL: 0},              // permanent (explicit memories)
		{SourcePrefix: "consolidated:", TTL: 0},      // permanent (summarized)
		{SourcePrefix: "observation:", TTL: 0},       // permanent (compressed observations)
		{SourcePrefix: "er1:", TTL: 0},               // permanent (ER1 personal memories)
		{SourcePrefix: "conversation:", TTL: 30 * 24 * time.Hour},  // 30 days
		{SourcePrefix: "tool:", TTL: 14 * 24 * time.Hour},          // 14 days
		{SourcePrefix: "group:", TTL: 60 * 24 * time.Hour},         // 60 days
	}
}

// NewLifecycleManager creates a new lifecycle manager.
func NewLifecycleManager(db *sql.DB, cfg LifecycleConfig) *LifecycleManager {
	if cfg.MaxChunks <= 0 {
		cfg.MaxChunks = 50000
	}
	if len(cfg.Policies) == 0 {
		cfg.Policies = DefaultPolicies()
	}
	return &LifecycleManager{db: db, config: cfg}
}

// Prune removes expired chunks and enforces the max chunk limit.
// Returns the number of chunks deleted.
func (lm *LifecycleManager) Prune() (int, error) {
	if lm == nil || lm.db == nil {
		return 0, nil
	}

	totalDeleted := 0

	// Phase 1: Delete chunks past their TTL
	for _, p := range lm.config.Policies {
		if p.TTL == 0 {
			continue // permanent, skip
		}
		cutoff := time.Now().Add(-p.TTL)
		pattern := p.SourcePrefix + "%"
		result, err := lm.db.Exec(`DELETE FROM memory_chunks WHERE source LIKE ? AND created_at < ?`, pattern, cutoff)
		if err != nil {
			slog.Warn("Lifecycle prune TTL failed", "source", p.SourcePrefix, "error", err)
			continue
		}
		if n, _ := result.RowsAffected(); n > 0 {
			totalDeleted += int(n)
			slog.Info("Lifecycle pruned expired chunks", "source", p.SourcePrefix, "deleted", n)
		}
	}

	// Phase 2: If still over maxChunks, delete oldest non-permanent chunks
	var count int
	if err := lm.db.QueryRow(`SELECT COUNT(*) FROM memory_chunks`).Scan(&count); err != nil {
		return totalDeleted, fmt.Errorf("count chunks: %w", err)
	}

	excess := count - lm.config.MaxChunks
	if excess > 0 {
		// Build exclusion pattern for permanent sources
		permanentPatterns := lm.permanentPatterns()
		whereClause := ""
		if len(permanentPatterns) > 0 {
			conditions := make([]string, len(permanentPatterns))
			for i, p := range permanentPatterns {
				conditions[i] = fmt.Sprintf("source NOT LIKE '%s%%'", p)
			}
			whereClause = "WHERE " + strings.Join(conditions, " AND ")
		}

		query := fmt.Sprintf(`DELETE FROM memory_chunks WHERE id IN (
			SELECT id FROM memory_chunks %s ORDER BY created_at ASC LIMIT ?
		)`, whereClause)

		result, err := lm.db.Exec(query, excess)
		if err != nil {
			return totalDeleted, fmt.Errorf("prune excess: %w", err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			totalDeleted += int(n)
			slog.Info("Lifecycle pruned excess chunks", "deleted", n, "remaining", count-int(n))
		}
	}

	return totalDeleted, nil
}

// Stats returns current memory statistics.
func (lm *LifecycleManager) Stats() (MemoryStats, error) {
	if lm == nil || lm.db == nil {
		return MemoryStats{}, nil
	}

	var stats MemoryStats
	lm.db.QueryRow(`SELECT COUNT(*) FROM memory_chunks`).Scan(&stats.TotalChunks)

	rows, err := lm.db.Query(`SELECT source, COUNT(*) FROM memory_chunks GROUP BY
		CASE
			WHEN source LIKE 'soul:%' THEN 'soul'
			WHEN source LIKE 'conversation:%' THEN 'conversation'
			WHEN source LIKE 'tool:%' THEN 'tool'
			WHEN source LIKE 'group:%' THEN 'group'
			WHEN source LIKE 'observation:%' THEN 'observation'
			WHEN source LIKE 'er1:%' THEN 'er1'
			WHEN source = 'user' THEN 'user'
			WHEN source LIKE 'consolidated:%' THEN 'consolidated'
			ELSE 'other'
		END`)
	if err == nil {
		defer rows.Close()
		stats.BySource = make(map[string]int)
		for rows.Next() {
			var source string
			var count int
			if rows.Scan(&source, &count) == nil {
				stats.BySource[source] = count
			}
		}
	}

	lm.db.QueryRow(`SELECT MIN(created_at) FROM memory_chunks`).Scan(&stats.OldestChunk)
	lm.db.QueryRow(`SELECT MAX(created_at) FROM memory_chunks`).Scan(&stats.NewestChunk)

	return stats, nil
}

// MemoryStats holds aggregate memory statistics.
type MemoryStats struct {
	TotalChunks int
	BySource    map[string]int
	OldestChunk *time.Time
	NewestChunk *time.Time
}

// DeleteBySource deletes all chunks matching a source prefix.
func (lm *LifecycleManager) DeleteBySource(sourcePrefix string) (int, error) {
	if lm == nil || lm.db == nil {
		return 0, nil
	}
	pattern := sourcePrefix + "%"
	result, err := lm.db.Exec(`DELETE FROM memory_chunks WHERE source LIKE ?`, pattern)
	if err != nil {
		return 0, fmt.Errorf("delete by source: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// DeleteAll deletes all chunks from memory.
func (lm *LifecycleManager) DeleteAll() (int, error) {
	if lm == nil || lm.db == nil {
		return 0, nil
	}
	result, err := lm.db.Exec(`DELETE FROM memory_chunks`)
	if err != nil {
		return 0, fmt.Errorf("delete all: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// permanentPatterns returns source prefixes with TTL=0.
func (lm *LifecycleManager) permanentPatterns() []string {
	var patterns []string
	for _, p := range lm.config.Policies {
		if p.TTL == 0 {
			patterns = append(patterns, p.SourcePrefix)
		}
	}
	return patterns
}

// RunDaily is a convenience wrapper that should be called periodically (e.g. once per day).
func (lm *LifecycleManager) RunDaily() {
	if lm == nil {
		return
	}
	deleted, err := lm.Prune()
	if err != nil {
		slog.Warn("Lifecycle daily prune error", "error", err)
	} else if deleted > 0 {
		slog.Info("Lifecycle daily prune complete", "deleted", deleted)
	}
}
