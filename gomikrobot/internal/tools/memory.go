package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/kamir/gomikrobot/internal/memory"
)

// RememberTool stores a piece of information in semantic memory.
type RememberTool struct {
	service *memory.MemoryService
}

func NewRememberTool(service *memory.MemoryService) *RememberTool {
	return &RememberTool{service: service}
}

func (t *RememberTool) Name() string        { return "remember" }
func (t *RememberTool) Description() string  { return "Store a piece of information in long-term memory for later recall. Use this when the user asks you to remember something." }
func (t *RememberTool) Tier() int            { return TierWrite }

func (t *RememberTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "The information to remember",
			},
			"tags": map[string]any{
				"type":        "string",
				"description": "Optional comma-separated tags for categorization",
			},
		},
		"required": []string{"content"},
	}
}

func (t *RememberTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	content := GetString(params, "content", "")
	tags := GetString(params, "tags", "")

	if content == "" {
		return "Error: content is required", nil
	}

	id, err := t.service.Store(ctx, content, "user", tags)
	if err != nil {
		return fmt.Sprintf("Error storing memory: %v", err), nil
	}

	if id == "" {
		return "Memory system not available (no embedder configured).", nil
	}

	return fmt.Sprintf("Remembered: %q (id: %s)", truncate(content, 80), id), nil
}

// RecallTool searches semantic memory for relevant information.
type RecallTool struct {
	service *memory.MemoryService
}

func NewRecallTool(service *memory.MemoryService) *RecallTool {
	return &RecallTool{service: service}
}

func (t *RecallTool) Name() string        { return "recall" }
func (t *RecallTool) Description() string  { return "Search long-term memory for information relevant to a query. Returns the most relevant stored memories." }
func (t *RecallTool) Tier() int            { return TierReadOnly }

func (t *RecallTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query to find relevant memories",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results (default: 5)",
			},
		},
		"required": []string{"query"},
	}
}

func (t *RecallTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	query := GetString(params, "query", "")
	limit := GetInt(params, "limit", 5)

	if query == "" {
		return "Error: query is required", nil
	}

	chunks, err := t.service.Search(ctx, query, limit)
	if err != nil {
		return fmt.Sprintf("Error searching memory: %v", err), nil
	}

	if len(chunks) == 0 {
		return "No relevant memories found.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d relevant memories:\n\n", len(chunks)))
	for i, chunk := range chunks {
		sb.WriteString(fmt.Sprintf("%d. [score=%.2f, source=%s] %s\n",
			i+1, chunk.Score, chunk.Source, chunk.Content))
	}

	return sb.String(), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
