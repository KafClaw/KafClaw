package memory

import (
	"context"
	"testing"

	"github.com/kamir/gomikrobot/internal/provider"
)

// fakeEmbedder returns a fixed vector for any input.
type fakeEmbedder struct {
	vector []float32
}

func (f *fakeEmbedder) Embed(ctx context.Context, req *provider.EmbeddingRequest) (*provider.EmbeddingResponse, error) {
	return &provider.EmbeddingResponse{Vector: f.vector}, nil
}

func TestMemoryService_StoreAndSearch(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	store := NewSQLiteVecStore(db, 3)
	emb := &fakeEmbedder{vector: []float32{1, 0, 0}}
	svc := NewMemoryService(store, emb)
	ctx := context.Background()

	id, err := svc.Store(ctx, "I prefer short answers", "user", "prefs")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	chunks, err := svc.Search(ctx, "preferences", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "I prefer short answers" {
		t.Errorf("unexpected content: %q", chunks[0].Content)
	}
	if chunks[0].Source != "user" {
		t.Errorf("unexpected source: %q", chunks[0].Source)
	}
}

func TestMemoryService_NilEmbedder(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	store := NewSQLiteVecStore(db, 3)
	svc := NewMemoryService(store, nil)
	ctx := context.Background()

	// Store should no-op
	id, err := svc.Store(ctx, "something", "user", "")
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Errorf("expected empty ID with nil embedder, got %q", id)
	}

	// Search should return nil
	chunks, err := svc.Search(ctx, "something", 5)
	if err != nil {
		t.Fatal(err)
	}
	if chunks != nil {
		t.Errorf("expected nil result with nil embedder, got %v", chunks)
	}
}

func TestChunkByHeaders(t *testing.T) {
	content := `# Title

Some intro.

## Section A

Content A line 1.
Content A line 2.

## Section B

Content B.
`
	chunks := ChunkByHeaders(content, "test.md")
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks (preamble + 2 sections), got %d", len(chunks))
	}
	// Preamble chunk uses sourceName as heading
	if chunks[0].Heading != "test.md" {
		t.Errorf("expected preamble heading 'test.md', got %q", chunks[0].Heading)
	}
	if chunks[1].Heading != "Section A" {
		t.Errorf("expected heading 'Section A', got %q", chunks[1].Heading)
	}
	if chunks[2].Heading != "Section B" {
		t.Errorf("expected heading 'Section B', got %q", chunks[2].Heading)
	}
}

func TestChunkByHeaders_NoHeaders(t *testing.T) {
	content := "Just a plain paragraph.\nAnother line."
	chunks := ChunkByHeaders(content, "notes.md")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (whole content), got %d", len(chunks))
	}
	if chunks[0].Heading != "notes.md" {
		t.Errorf("expected heading 'notes.md', got %q", chunks[0].Heading)
	}
}

func TestChunkByHeaders_Empty(t *testing.T) {
	chunks := ChunkByHeaders("", "empty.md")
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty content, got %d", len(chunks))
	}
}

func TestChunkID_Deterministic(t *testing.T) {
	id1 := chunkID("user", "hello")
	id2 := chunkID("user", "hello")
	if id1 != id2 {
		t.Errorf("expected same ID, got %q and %q", id1, id2)
	}

	id3 := chunkID("user", "world")
	if id1 == id3 {
		t.Error("expected different IDs for different content")
	}
}
