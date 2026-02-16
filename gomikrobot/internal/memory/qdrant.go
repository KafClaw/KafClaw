package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type QdrantStore struct {
	baseURL    string
	collection string
	dimension  int
	client     *http.Client
}

func NewQdrantStore(url, collection string, dim int) *QdrantStore {
	return &QdrantStore{
		baseURL:    url,
		collection: collection,
		dimension:  dim,
		client:     &http.Client{},
	}
}

func (s *QdrantStore) EnsureCollection(ctx context.Context) error {
	// Check if exists
	resp, err := s.client.Get(s.baseURL + "/collections/" + s.collection)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return nil
	}

	// Create
	body := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     s.dimension,
			"distance": "Cosine",
		},
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "PUT", s.baseURL+"/collections/"+s.collection, bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err = s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create collection: %s", string(b))
	}
	return nil
}

func (s *QdrantStore) Upsert(ctx context.Context, id string, vector []float32, payload map[string]interface{}) error {
	// Qdrant points API expects 'points' structure
	// We use uuid for IDs usually, but string is fine if standard format, else need conversion?
	// Qdrant IDs can be integers or UUIDs. If 'id' is a string messageID, we might need to hash it to UUID or use it if uuint64.
	// For simplicity, let's assume client sends a UUID or we let Qdrant autogen? No, we need to link it.
	// We will attempt to use it as UUID string.

	body := map[string]interface{}{
		"points": []map[string]interface{}{
			{
				"id":      id, // MUST be UUID or Int. If random string, this will fail.
				"vector":  vector,
				"payload": payload,
			},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, _ := http.NewRequestWithContext(ctx, "PUT", fmt.Sprintf("%s/collections/%s/points?wait=true", s.baseURL, s.collection), bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant upsert failed: %s", string(b))
	}
	return nil
}

func (s *QdrantStore) Search(ctx context.Context, vector []float32, limit int) ([]Result, error) {
	body := map[string]interface{}{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/collections/%s/points/search", s.baseURL, s.collection), bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("qdrant search failed: %d", resp.StatusCode)
	}

	var response struct {
		Result []struct {
			ID      string                 `json:"id"`
			Score   float32                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	results := make([]Result, len(response.Result))
	for i, r := range response.Result {
		results[i] = Result{
			ID:      r.ID,
			Score:   r.Score,
			Payload: r.Payload,
		}
	}
	return results, nil
}
