package memory

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/KafClaw/KafClaw/gomikrobot/internal/provider"
)

// MemoryChunk represents a single piece of stored memory.
type MemoryChunk struct {
	ID      string
	Content string
	Source  string
	Tags    string
	Score   float32
}

// MemoryService provides high-level Store/Search operations for the memory system.
// If embedder is nil, all operations gracefully degrade (no-op Store, empty Search).
type MemoryService struct {
	store    VectorStore
	embedder provider.Embedder
}

// NewMemoryService creates a new MemoryService.
func NewMemoryService(store VectorStore, embedder provider.Embedder) *MemoryService {
	return &MemoryService{store: store, embedder: embedder}
}

// Store embeds content and upserts it into the vector store.
// Returns the chunk ID. Gracefully degrades if embedder is nil.
func (m *MemoryService) Store(ctx context.Context, content, source, tags string) (string, error) {
	if m.embedder == nil {
		return "", nil
	}

	id := chunkID(source, content)

	resp, err := m.embedder.Embed(ctx, &provider.EmbeddingRequest{Input: content})
	if err != nil {
		return "", fmt.Errorf("embed content: %w", err)
	}

	err = m.store.Upsert(ctx, id, resp.Vector, map[string]interface{}{
		"content": content,
		"source":  source,
		"tags":    tags,
	})
	if err != nil {
		return "", fmt.Errorf("upsert chunk: %w", err)
	}

	return id, nil
}

// Search finds the most relevant memory chunks for the given query.
// Gracefully degrades if embedder is nil (returns nil).
func (m *MemoryService) Search(ctx context.Context, query string, limit int) ([]MemoryChunk, error) {
	if m.embedder == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 5
	}

	resp, err := m.embedder.Embed(ctx, &provider.EmbeddingRequest{Input: query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	results, err := m.store.Search(ctx, resp.Vector, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	chunks := make([]MemoryChunk, len(results))
	for i, r := range results {
		content, _ := r.Payload["content"].(string)
		source, _ := r.Payload["source"].(string)
		tags, _ := r.Payload["tags"].(string)
		chunks[i] = MemoryChunk{
			ID:      r.ID,
			Content: content,
			Source:  source,
			Tags:    tags,
			Score:   r.Score,
		}
	}
	return chunks, nil
}

// SearchBySource searches memory filtered by source prefix.
// Results are post-filtered to only include chunks matching sourcePrefix.
func (m *MemoryService) SearchBySource(ctx context.Context, query string, sourcePrefix string, limit int) ([]MemoryChunk, error) {
	// Search broadly, then filter by source
	results, err := m.Search(ctx, query, limit*3) // over-fetch to compensate for filtering
	if err != nil {
		return nil, err
	}

	var filtered []MemoryChunk
	for _, c := range results {
		if strings.HasPrefix(c.Source, sourcePrefix) {
			filtered = append(filtered, c)
			if len(filtered) >= limit {
				break
			}
		}
	}
	return filtered, nil
}

// chunkID generates a deterministic ID from source and content.
func chunkID(source, content string) string {
	h := sha256.Sum256([]byte(source + ":" + content))
	return fmt.Sprintf("%x", h[:8])
}

// ChunkByHeaders splits markdown content by ## headers into (heading, body) pairs.
// Each chunk includes the heading as the first line of the body.
// Used for soul file indexing.
func ChunkByHeaders(content, sourceName string) []struct {
	Heading string
	Body    string
} {
	lines := strings.Split(content, "\n")
	type chunk struct {
		Heading string
		Body    string
	}
	var chunks []chunk
	var currentHeading string
	var currentLines []string

	flush := func() {
		body := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if body != "" {
			heading := currentHeading
			if heading == "" {
				heading = sourceName
			}
			chunks = append(chunks, chunk{
				Heading: heading,
				Body:    body,
			})
		}
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			flush()
			currentHeading = strings.TrimPrefix(line, "## ")
			currentHeading = strings.TrimSpace(currentHeading)
			currentLines = []string{line}
		} else {
			currentLines = append(currentLines, line)
		}
	}
	flush()

	// Convert to return type
	result := make([]struct {
		Heading string
		Body    string
	}, len(chunks))
	for i, c := range chunks {
		result[i].Heading = c.Heading
		result[i].Body = c.Body
	}
	return result
}
