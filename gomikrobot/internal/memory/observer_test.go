package memory

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupObserverDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE observations_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			observed INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			content TEXT NOT NULL,
			priority TEXT NOT NULL DEFAULT 'medium',
			observed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			referenced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestParseObservations(t *testing.T) {
	input := `## 2026-02-15
- [HIGH] User prefers Go over Python
- [MEDIUM] Working on deployment pipeline
## 2026-02-16
- [LOW] User mentioned upcoming vacation`

	results := parseObservations(input, "test-session")

	if len(results) != 3 {
		t.Fatalf("expected 3 observations, got %d", len(results))
	}

	if results[0].Priority != "high" {
		t.Errorf("expected high priority, got %s", results[0].Priority)
	}
	if results[0].Content != "User prefers Go over Python" {
		t.Errorf("unexpected content: %s", results[0].Content)
	}
	if results[0].ReferencedAt.Format("2006-01-02") != "2026-02-15" {
		t.Errorf("unexpected date: %s", results[0].ReferencedAt)
	}

	if results[2].Priority != "low" {
		t.Errorf("expected low priority, got %s", results[2].Priority)
	}
	if results[2].ReferencedAt.Format("2006-01-02") != "2026-02-16" {
		t.Errorf("unexpected date: %s", results[2].ReferencedAt)
	}
}

func TestParseObservationsUnknownPriority(t *testing.T) {
	input := `- [URGENT] Something important`
	results := parseObservations(input, "s1")
	if len(results) != 1 {
		t.Fatalf("expected 1, got %d", len(results))
	}
	if results[0].Priority != "medium" {
		t.Errorf("unknown priority should default to medium, got %s", results[0].Priority)
	}
}

func TestFormatObservations(t *testing.T) {
	obs := []Observation{
		{Content: "Prefers Go", Priority: "high", ReferencedAt: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)},
		{Content: "Likes coffee", Priority: "low", ReferencedAt: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)},
		{Content: "New project started", Priority: "medium", ReferencedAt: time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)},
	}

	output := FormatObservations(obs)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if !contains(output, "## 2026-02-15") {
		t.Error("expected date header 2026-02-15")
	}
	if !contains(output, "## 2026-02-16") {
		t.Error("expected date header 2026-02-16")
	}
	if !contains(output, "[HIGH] Prefers Go") {
		t.Error("expected high priority observation")
	}
}

func TestFormatObservationsEmpty(t *testing.T) {
	if FormatObservations(nil) != "" {
		t.Error("nil observations should return empty string")
	}
}

func TestObserverNil(t *testing.T) {
	var o *Observer
	if o.ShouldObserve("s1") {
		t.Error("nil observer should return false")
	}
	o.EnqueueMessage("s1", "user", "hello world this is a test message that is long enough")
	if err := o.Observe(context.Background(), "s1"); err != nil {
		t.Error("nil observer Observe should be no-op")
	}
	obs, err := o.LoadObservations("s1")
	if err != nil || obs != nil {
		t.Error("nil observer LoadObservations should return nil, nil")
	}
}

func TestEnqueueMessage(t *testing.T) {
	db := setupObserverDB(t)
	defer db.Close()

	o := &Observer{config: ObserverConfig{MessageThreshold: 5}, db: db}
	o.EnqueueMessage("s1", "user", "This is a test message that is long enough to be stored")

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM observations_queue WHERE session_id = 's1'`).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 enqueued message, got %d", count)
	}
}

func TestEnqueueMessageTooShort(t *testing.T) {
	db := setupObserverDB(t)
	defer db.Close()

	o := &Observer{config: ObserverConfig{MessageThreshold: 5}, db: db}
	o.EnqueueMessage("s1", "user", "hi")

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM observations_queue`).Scan(&count)
	if count != 0 {
		t.Fatalf("short messages should be skipped, got %d", count)
	}
}

func TestShouldObserve(t *testing.T) {
	db := setupObserverDB(t)
	defer db.Close()

	o := &Observer{config: ObserverConfig{MessageThreshold: 3}, db: db}

	// Below threshold
	o.EnqueueMessage("s1", "user", "Message one that is long enough to pass")
	o.EnqueueMessage("s1", "assistant", "Response one that is also long enough")
	if o.ShouldObserve("s1") {
		t.Error("should not observe with only 2 messages")
	}

	// At threshold
	o.EnqueueMessage("s1", "user", "Message two that is definitely long enough")
	if !o.ShouldObserve("s1") {
		t.Error("should observe with 3 messages (threshold)")
	}
}

func TestObservationCount(t *testing.T) {
	db := setupObserverDB(t)
	defer db.Close()

	o := &Observer{config: ObserverConfig{}, db: db}

	if o.ObservationCount("s1") != 0 {
		t.Error("expected 0 observations")
	}

	db.Exec(`INSERT INTO observations (session_id, content, priority) VALUES ('s1', 'test', 'high')`)
	if o.ObservationCount("s1") != 1 {
		t.Error("expected 1 observation")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
