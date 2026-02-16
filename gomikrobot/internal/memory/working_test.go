package memory

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupWorkingDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE working_memory (
		resource_id TEXT NOT NULL,
		thread_id   TEXT NOT NULL DEFAULT '',
		content     TEXT NOT NULL DEFAULT '',
		updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (resource_id, thread_id)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestWorkingSaveAndLoad(t *testing.T) {
	db := setupWorkingDB(t)
	defer db.Close()
	w := NewWorkingMemoryStore(db)

	// Save resource-level
	if err := w.Save("user-1", "", "Name: Alice\nLang: EN"); err != nil {
		t.Fatal(err)
	}

	got, err := w.Load("user-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Name: Alice\nLang: EN" {
		t.Fatalf("expected resource content, got %q", got)
	}
}

func TestWorkingThreadFallback(t *testing.T) {
	db := setupWorkingDB(t)
	defer db.Close()
	w := NewWorkingMemoryStore(db)

	// Save resource-level only
	if err := w.Save("user-1", "", "Resource content"); err != nil {
		t.Fatal(err)
	}

	// Load with thread should fallback to resource
	got, err := w.Load("user-1", "thread-99")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Resource content" {
		t.Fatalf("expected fallback to resource, got %q", got)
	}
}

func TestWorkingThreadOverridesResource(t *testing.T) {
	db := setupWorkingDB(t)
	defer db.Close()
	w := NewWorkingMemoryStore(db)

	if err := w.Save("user-1", "", "Resource content"); err != nil {
		t.Fatal(err)
	}
	if err := w.Save("user-1", "thread-1", "Thread content"); err != nil {
		t.Fatal(err)
	}

	got, err := w.Load("user-1", "thread-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Thread content" {
		t.Fatalf("expected thread content, got %q", got)
	}
}

func TestWorkingLoadBoth(t *testing.T) {
	db := setupWorkingDB(t)
	defer db.Close()
	w := NewWorkingMemoryStore(db)

	w.Save("user-1", "", "Resource notes")
	w.Save("user-1", "thread-1", "Thread notes")

	res, thr, err := w.LoadBoth("user-1", "thread-1")
	if err != nil {
		t.Fatal(err)
	}
	if res != "Resource notes" {
		t.Fatalf("expected resource content, got %q", res)
	}
	if thr != "Thread notes" {
		t.Fatalf("expected thread content, got %q", thr)
	}
}

func TestWorkingUpsert(t *testing.T) {
	db := setupWorkingDB(t)
	defer db.Close()
	w := NewWorkingMemoryStore(db)

	w.Save("user-1", "", "v1")
	w.Save("user-1", "", "v2")

	got, _ := w.Load("user-1", "")
	if got != "v2" {
		t.Fatalf("expected upsert to v2, got %q", got)
	}
}

func TestWorkingLoadEmpty(t *testing.T) {
	db := setupWorkingDB(t)
	defer db.Close()
	w := NewWorkingMemoryStore(db)

	got, err := w.Load("nonexistent", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestWorkingNilStore(t *testing.T) {
	var w *WorkingMemoryStore

	got, err := w.Load("x", "")
	if err != nil || got != "" {
		t.Fatalf("nil store should return empty: got=%q err=%v", got, err)
	}

	if err := w.Save("x", "", "data"); err != nil {
		t.Fatalf("nil store save should be no-op: %v", err)
	}
}

func TestWorkingDelete(t *testing.T) {
	db := setupWorkingDB(t)
	defer db.Close()
	w := NewWorkingMemoryStore(db)

	w.Save("user-1", "", "data")
	w.Delete("user-1", "")

	got, _ := w.Load("user-1", "")
	if got != "" {
		t.Fatalf("expected deleted, got %q", got)
	}
}
