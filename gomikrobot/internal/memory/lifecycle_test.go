package memory

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupLifecycleDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	schema := `
CREATE TABLE memory_chunks (
	id TEXT PRIMARY KEY,
	content TEXT NOT NULL,
	embedding BLOB,
	source TEXT NOT NULL DEFAULT 'user',
	tags TEXT DEFAULT '',
	version INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_memory_chunks_source ON memory_chunks(source);
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertChunk(db *sql.DB, id, source string, createdAt time.Time) {
	db.Exec(`INSERT INTO memory_chunks (id, content, source, created_at) VALUES (?, ?, ?, ?)`,
		id, "content for "+id, source, createdAt)
}

func countChunks(db *sql.DB) int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM memory_chunks").Scan(&n)
	return n
}

func TestPruneExpiredConversations(t *testing.T) {
	db := setupLifecycleDB(t)
	defer db.Close()

	now := time.Now()
	// Insert old conversation chunks (40 days ago)
	insertChunk(db, "c1", "conversation:whatsapp", now.Add(-40*24*time.Hour))
	insertChunk(db, "c2", "conversation:cli", now.Add(-35*24*time.Hour))
	// Insert recent conversation chunk (5 days ago)
	insertChunk(db, "c3", "conversation:whatsapp", now.Add(-5*24*time.Hour))
	// Insert permanent soul chunk (old)
	insertChunk(db, "s1", "soul:IDENTITY.md", now.Add(-90*24*time.Hour))

	lm := NewLifecycleManager(db, LifecycleConfig{
		MaxChunks: 50000,
		Policies:  DefaultPolicies(),
	})

	deleted, err := lm.Prune()
	if err != nil {
		t.Fatalf("Prune error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("Deleted = %d, want 2 (old conversations)", deleted)
	}
	if n := countChunks(db); n != 2 {
		t.Errorf("Remaining = %d, want 2 (c3 + s1)", n)
	}
}

func TestPruneExcessChunks(t *testing.T) {
	db := setupLifecycleDB(t)
	defer db.Close()

	now := time.Now()
	// Insert 15 conversation chunks (recent, within TTL)
	for i := 0; i < 15; i++ {
		insertChunk(db, fmt.Sprintf("c%d", i), "conversation:cli", now.Add(-time.Duration(i)*time.Hour))
	}
	// Insert 2 soul chunks (permanent)
	insertChunk(db, "s1", "soul:SOUL.md", now.Add(-30*24*time.Hour))
	insertChunk(db, "s2", "soul:AGENTS.md", now.Add(-30*24*time.Hour))

	lm := NewLifecycleManager(db, LifecycleConfig{
		MaxChunks: 10, // trigger excess pruning
		Policies:  DefaultPolicies(),
	})

	deleted, err := lm.Prune()
	if err != nil {
		t.Fatalf("Prune error: %v", err)
	}
	// Should delete 7 excess chunks (17 - 10 = 7), but only from non-permanent sources
	if deleted != 7 {
		t.Errorf("Deleted = %d, want 7", deleted)
	}

	remaining := countChunks(db)
	if remaining != 10 {
		t.Errorf("Remaining = %d, want 10", remaining)
	}

	// Soul chunks must survive
	var soulCount int
	db.QueryRow("SELECT COUNT(*) FROM memory_chunks WHERE source LIKE 'soul:%'").Scan(&soulCount)
	if soulCount != 2 {
		t.Errorf("Soul chunks = %d, want 2 (must never be pruned)", soulCount)
	}
}

func TestPruneNoop(t *testing.T) {
	db := setupLifecycleDB(t)
	defer db.Close()

	insertChunk(db, "c1", "conversation:cli", time.Now())

	lm := NewLifecycleManager(db, LifecycleConfig{MaxChunks: 50000})
	deleted, err := lm.Prune()
	if err != nil {
		t.Fatalf("Prune error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Deleted = %d, want 0 (nothing to prune)", deleted)
	}
}

func TestStats(t *testing.T) {
	db := setupLifecycleDB(t)
	defer db.Close()

	now := time.Now()
	insertChunk(db, "c1", "conversation:cli", now)
	insertChunk(db, "s1", "soul:SOUL.md", now.Add(-time.Hour))

	lm := NewLifecycleManager(db, LifecycleConfig{})
	stats, err := lm.Stats()
	if err != nil {
		t.Fatalf("Stats error: %v", err)
	}
	if stats.TotalChunks != 2 {
		t.Errorf("TotalChunks = %d, want 2", stats.TotalChunks)
	}
}

func TestNilLifecycle(t *testing.T) {
	var lm *LifecycleManager
	lm.RunDaily() // should not panic
	deleted, err := lm.Prune()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("Expected 0 deleted")
	}
}
