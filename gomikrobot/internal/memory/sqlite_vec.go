package memory

import (
	"context"
	"database/sql"
	"encoding/binary"
	"math"
	"sort"
)

// SQLiteVecStore implements VectorStore using the shared timeline SQLite DB.
// Embeddings are stored as BLOBs (little-endian float32 arrays) in the
// memory_chunks table. Cosine similarity is computed in Go — at <10K chunks
// this is sub-millisecond.
type SQLiteVecStore struct {
	db        *sql.DB
	dimension int
}

// NewSQLiteVecStore creates a new SQLiteVecStore with the given database
// connection and expected embedding dimension.
func NewSQLiteVecStore(db *sql.DB, dimension int) *SQLiteVecStore {
	return &SQLiteVecStore{db: db, dimension: dimension}
}

// EnsureCollection is a no-op — the table is created by the schema migration.
func (s *SQLiteVecStore) EnsureCollection(ctx context.Context) error {
	return nil
}

// Upsert stores or updates a memory chunk with its embedding.
func (s *SQLiteVecStore) Upsert(ctx context.Context, id string, vector []float32, payload map[string]interface{}) error {
	content, _ := payload["content"].(string)
	source, _ := payload["source"].(string)
	tags, _ := payload["tags"].(string)
	if source == "" {
		source = "user"
	}

	blob := encodeFloat32s(vector)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO memory_chunks (id, content, embedding, source, tags)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			embedding = excluded.embedding,
			source = excluded.source,
			tags = excluded.tags,
			version = memory_chunks.version + 1,
			updated_at = CURRENT_TIMESTAMP
	`, id, content, blob, source, tags)
	return err
}

// Search finds the top-k most similar chunks by cosine similarity.
func (s *SQLiteVecStore) Search(ctx context.Context, vector []float32, limit int) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, content, embedding, source, tags
		FROM memory_chunks
		WHERE embedding IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scored struct {
		result Result
		score  float32
	}

	var candidates []scored

	for rows.Next() {
		var id, content, source, tags string
		var blob []byte

		if err := rows.Scan(&id, &content, &blob, &source, &tags); err != nil {
			continue
		}

		stored := decodeFloat32s(blob)
		if len(stored) != len(vector) {
			continue // dimension mismatch, skip
		}

		sim := cosineSimilarity(vector, stored)

		candidates = append(candidates, scored{
			result: Result{
				ID:    id,
				Score: sim,
				Payload: map[string]interface{}{
					"content": content,
					"source":  source,
					"tags":    tags,
				},
			},
			score: sim,
		})
	}

	// Sort by similarity descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Apply limit
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	results := make([]Result, len(candidates))
	for i, c := range candidates {
		results[i] = c.result
	}
	return results, nil
}

// encodeFloat32s converts a float32 slice to little-endian bytes.
func encodeFloat32s(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeFloat32s converts little-endian bytes back to a float32 slice.
func decodeFloat32s(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// cosineSimilarity computes the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return float32(dot / denom)
}
