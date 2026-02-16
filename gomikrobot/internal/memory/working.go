package memory

import (
	"database/sql"
	"time"
)

// WorkingMemoryStore provides scoped persistent working memory keyed by
// (resourceID, threadID). Resource-scoped entries use threadID = "".
type WorkingMemoryStore struct {
	db *sql.DB
}

// WorkingMemoryEntry represents a single working-memory record.
type WorkingMemoryEntry struct {
	ResourceID string
	ThreadID   string
	Content    string
	UpdatedAt  time.Time
}

// NewWorkingMemoryStore creates a new store backed by the given database.
// Returns nil if db is nil (callers must handle nil gracefully).
func NewWorkingMemoryStore(db *sql.DB) *WorkingMemoryStore {
	if db == nil {
		return nil
	}
	return &WorkingMemoryStore{db: db}
}

// Load returns the working memory content for a resource and optional thread.
// Lookup order: thread-specific first, then resource-level fallback.
// Returns empty string if nothing is stored.
func (w *WorkingMemoryStore) Load(resourceID, threadID string) (string, error) {
	if w == nil || w.db == nil {
		return "", nil
	}

	// Try thread-specific first
	if threadID != "" {
		var content string
		err := w.db.QueryRow(
			`SELECT content FROM working_memory WHERE resource_id = ? AND thread_id = ?`,
			resourceID, threadID,
		).Scan(&content)
		if err == nil {
			return content, nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
		// Fall through to resource-level
	}

	// Resource-level (thread_id = '')
	var content string
	err := w.db.QueryRow(
		`SELECT content FROM working_memory WHERE resource_id = ? AND thread_id = ''`,
		resourceID,
	).Scan(&content)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return content, err
}

// Save persists working memory content. Uses upsert (INSERT OR REPLACE).
func (w *WorkingMemoryStore) Save(resourceID, threadID, content string) error {
	if w == nil || w.db == nil {
		return nil
	}
	if threadID == "" {
		threadID = "" // explicit for clarity
	}
	_, err := w.db.Exec(
		`INSERT INTO working_memory (resource_id, thread_id, content, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(resource_id, thread_id) DO UPDATE SET content = excluded.content, updated_at = excluded.updated_at`,
		resourceID, threadID, content, time.Now(),
	)
	return err
}

// LoadBoth returns both thread-specific and resource-level working memory.
// Either may be empty. Useful for context building where both scopes are shown.
func (w *WorkingMemoryStore) LoadBoth(resourceID, threadID string) (resourceContent, threadContent string, err error) {
	if w == nil || w.db == nil {
		return "", "", nil
	}

	// Resource-level
	_ = w.db.QueryRow(
		`SELECT content FROM working_memory WHERE resource_id = ? AND thread_id = ''`,
		resourceID,
	).Scan(&resourceContent)

	// Thread-specific
	if threadID != "" {
		_ = w.db.QueryRow(
			`SELECT content FROM working_memory WHERE resource_id = ? AND thread_id = ?`,
			resourceID, threadID,
		).Scan(&threadContent)
	}

	return resourceContent, threadContent, nil
}

// ListAll returns all working memory entries.
func (w *WorkingMemoryStore) ListAll() ([]WorkingMemoryEntry, error) {
	if w == nil || w.db == nil {
		return nil, nil
	}

	rows, err := w.db.Query(`SELECT resource_id, thread_id, content, updated_at FROM working_memory ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []WorkingMemoryEntry
	for rows.Next() {
		var e WorkingMemoryEntry
		if err := rows.Scan(&e.ResourceID, &e.ThreadID, &e.Content, &e.UpdatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// DeleteAll removes all working memory entries.
func (w *WorkingMemoryStore) DeleteAll() error {
	if w == nil || w.db == nil {
		return nil
	}
	_, err := w.db.Exec(`DELETE FROM working_memory`)
	return err
}

// Delete removes working memory for a given resource/thread combination.
func (w *WorkingMemoryStore) Delete(resourceID, threadID string) error {
	if w == nil || w.db == nil {
		return nil
	}
	_, err := w.db.Exec(
		`DELETE FROM working_memory WHERE resource_id = ? AND thread_id = ?`,
		resourceID, threadID,
	)
	return err
}
