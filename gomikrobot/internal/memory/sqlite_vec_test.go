package memory

import (
	"context"
	"database/sql"
	"math"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS memory_chunks (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			embedding BLOB,
			source TEXT NOT NULL DEFAULT 'user',
			tags TEXT DEFAULT '',
			version INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSQLiteVecStore_UpsertAndSearch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteVecStore(db, 3)
	ctx := context.Background()

	// Upsert two chunks
	err := store.Upsert(ctx, "a", []float32{1, 0, 0}, map[string]interface{}{
		"content": "hello world",
		"source":  "user",
		"tags":    "greeting",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = store.Upsert(ctx, "b", []float32{0, 1, 0}, map[string]interface{}{
		"content": "goodbye world",
		"source":  "user",
		"tags":    "farewell",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Search with vector close to "a"
	results, err := store.Search(ctx, []float32{0.9, 0.1, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "a" {
		t.Errorf("expected first result to be 'a', got %q", results[0].ID)
	}
	if results[1].ID != "b" {
		t.Errorf("expected second result to be 'b', got %q", results[1].ID)
	}
	if results[0].Score <= results[1].Score {
		t.Error("expected first result to have higher score")
	}
}

func TestSQLiteVecStore_UpsertUpdatesExisting(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteVecStore(db, 3)
	ctx := context.Background()

	err := store.Upsert(ctx, "a", []float32{1, 0, 0}, map[string]interface{}{
		"content": "original",
		"source":  "user",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Update same ID
	err = store.Upsert(ctx, "a", []float32{0, 1, 0}, map[string]interface{}{
		"content": "updated",
		"source":  "user",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Search: should find updated content
	results, err := store.Search(ctx, []float32{0, 1, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Payload["content"] != "updated" {
		t.Errorf("expected updated content, got %q", results[0].Payload["content"])
	}
}

func TestSQLiteVecStore_LimitEnforced(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteVecStore(db, 3)
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		_ = store.Upsert(ctx, id, []float32{1, 0, 0}, map[string]interface{}{
			"content": id,
			"source":  "user",
		})
	}

	results, err := store.Search(ctx, []float32{1, 0, 0}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestSQLiteVecStore_DimensionMismatchSkipped(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteVecStore(db, 3)
	ctx := context.Background()

	// Insert a chunk with 3 dimensions
	_ = store.Upsert(ctx, "a", []float32{1, 0, 0}, map[string]interface{}{
		"content": "hello",
		"source":  "user",
	})

	// Manually insert a blob with wrong dimensions
	wrongBlob := encodeFloat32s([]float32{1, 0})
	_, _ = db.Exec(`INSERT INTO memory_chunks (id, content, embedding, source) VALUES (?, ?, ?, ?)`,
		"bad", "bad entry", wrongBlob, "user")

	// Search with 3 dimensions — should only get "a"
	results, err := store.Search(ctx, []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (skipping dimension mismatch), got %d", len(results))
	}
	if results[0].ID != "a" {
		t.Errorf("expected 'a', got %q", results[0].ID)
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Same vector → 1.0
	sim := cosineSimilarity([]float32{1, 0, 0}, []float32{1, 0, 0})
	if math.Abs(float64(sim)-1.0) > 1e-6 {
		t.Errorf("expected ~1.0 for identical vectors, got %f", sim)
	}

	// Orthogonal → 0.0
	sim = cosineSimilarity([]float32{1, 0, 0}, []float32{0, 1, 0})
	if math.Abs(float64(sim)) > 1e-6 {
		t.Errorf("expected ~0.0 for orthogonal vectors, got %f", sim)
	}

	// Opposite → -1.0
	sim = cosineSimilarity([]float32{1, 0, 0}, []float32{-1, 0, 0})
	if math.Abs(float64(sim)+1.0) > 1e-6 {
		t.Errorf("expected ~-1.0 for opposite vectors, got %f", sim)
	}

	// Empty → 0
	sim = cosineSimilarity([]float32{}, []float32{})
	if sim != 0 {
		t.Errorf("expected 0 for empty vectors, got %f", sim)
	}
}

func TestEncodeDecodeFloat32s(t *testing.T) {
	original := []float32{1.5, -2.3, 0, 100.0}
	encoded := encodeFloat32s(original)
	decoded := decodeFloat32s(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("expected %d elements, got %d", len(original), len(decoded))
	}
	for i := range original {
		if original[i] != decoded[i] {
			t.Errorf("mismatch at %d: %f != %f", i, original[i], decoded[i])
		}
	}
}

func TestEnsureCollection_NoOp(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteVecStore(db, 3)

	if err := store.EnsureCollection(context.Background()); err != nil {
		t.Fatal(err)
	}
}
