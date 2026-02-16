package memory

import "context"

type VectorStore interface {
	// Upsert stores a text with its embedding and metadata.
	Upsert(ctx context.Context, id string, vector []float32, payload map[string]interface{}) error

	// Search finds the most similar items.
	Search(ctx context.Context, vector []float32, limit int) ([]Result, error)

	// EnsureCollection makes sure the storage exists.
	EnsureCollection(ctx context.Context) error
}

type Result struct {
	ID      string                 `json:"id"`
	Score   float32                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
}
