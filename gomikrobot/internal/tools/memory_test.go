package tools

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/kamir/gomikrobot/internal/memory"
	"github.com/kamir/gomikrobot/internal/provider"
	_ "modernc.org/sqlite"
)

type fakeEmbedder struct {
	vector []float32
}

func (f *fakeEmbedder) Embed(ctx context.Context, req *provider.EmbeddingRequest) (*provider.EmbeddingResponse, error) {
	return &provider.EmbeddingResponse{Vector: f.vector}, nil
}

func setupMemoryService(t *testing.T) *memory.MemoryService {
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
	store := memory.NewSQLiteVecStore(db, 3)
	emb := &fakeEmbedder{vector: []float32{1, 0, 0}}
	return memory.NewMemoryService(store, emb)
}

func TestRememberTool(t *testing.T) {
	svc := setupMemoryService(t)
	tool := NewRememberTool(svc)

	if tool.Name() != "remember" {
		t.Errorf("expected name 'remember', got %q", tool.Name())
	}
	if tool.Tier() != TierWrite {
		t.Errorf("expected tier %d, got %d", TierWrite, tool.Tier())
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"content": "I prefer short answers",
		"tags":    "prefs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Remembered") {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestRememberTool_EmptyContent(t *testing.T) {
	svc := setupMemoryService(t)
	tool := NewRememberTool(svc)

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Error") {
		t.Errorf("expected error for empty content, got: %q", result)
	}
}

func TestRecallTool(t *testing.T) {
	svc := setupMemoryService(t)
	ctx := context.Background()

	// First store something
	remTool := NewRememberTool(svc)
	_, _ = remTool.Execute(ctx, map[string]any{"content": "I prefer short answers"})

	// Then recall
	tool := NewRecallTool(svc)
	if tool.Name() != "recall" {
		t.Errorf("expected name 'recall', got %q", tool.Name())
	}
	if tool.Tier() != TierReadOnly {
		t.Errorf("expected tier %d, got %d", TierReadOnly, tool.Tier())
	}

	result, err := tool.Execute(ctx, map[string]any{"query": "preferences"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "short answers") {
		t.Errorf("expected recalled content, got: %q", result)
	}
}

func TestRecallTool_NoResults(t *testing.T) {
	svc := setupMemoryService(t)
	tool := NewRecallTool(svc)

	result, err := tool.Execute(context.Background(), map[string]any{"query": "anything"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "No relevant memories") {
		t.Errorf("expected 'no memories' message, got: %q", result)
	}
}

func TestRecallTool_EmptyQuery(t *testing.T) {
	svc := setupMemoryService(t)
	tool := NewRecallTool(svc)

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Error") {
		t.Errorf("expected error for empty query, got: %q", result)
	}
}
