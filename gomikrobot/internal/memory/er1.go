package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ER1Config configures the ER1 memory service integration.
type ER1Config struct {
	URL          string        // e.g. "http://127.0.0.1:8080"
	APIKey       string        // X-API-KEY header value
	UserID       string        // uid for /user/access
	GivenName    string        // optional, for /user/access
	FamilyName   string        // optional, for /user/access
	Email        string        // optional, for /user/access
	SyncInterval time.Duration // default: 5 minutes
}

// ER1Client syncs personal memories from the ER1 service into the
// GoMikroBot vector store as "er1:" source chunks.
type ER1Client struct {
	config     ER1Config
	httpClient *http.Client
	service    *MemoryService
	ctxID      string    // obtained from /user/access
	lastSync   time.Time // only fetch memories newer than this
	mu         sync.Mutex
}

// er1Memory represents a memory item from the ER1 API.
type er1Memory struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	Audio            bool     `json:"audio"`
	Image            bool     `json:"image"`
	Description      string   `json:"description"`
	Transcript       string   `json:"transcript"`
	TranscriptStatus string   `json:"transcript_status"`
	ReviewStatus     string   `json:"review_status"`
	LocationLat      float64  `json:"location_lat"`
	LocationLon      float64  `json:"location_lon"`
	CreatedAt        string   `json:"created_at"`
	Tags             []string `json:"tags"`
}

// er1AccessResponse is the response from POST /user/access.
type er1AccessResponse struct {
	CtxID string `json:"ctx_id"`
	Tier  string `json:"tier"`
}

// er1MemoryListResponse is the response from GET /memory/{ctx_id}.
type er1MemoryListResponse struct {
	Memories []er1Memory `json:"memories"`
}

// NewER1Client creates a new ER1 client. Returns nil if URL or service is empty/nil.
func NewER1Client(cfg ER1Config, service *MemoryService) *ER1Client {
	if cfg.URL == "" || service == nil {
		return nil
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 5 * time.Minute
	}
	// Ensure no trailing slash
	cfg.URL = strings.TrimRight(cfg.URL, "/")

	return &ER1Client{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		service:    service,
	}
}

// Authenticate calls POST /user/access to obtain a ctx_id.
func (c *ER1Client) Authenticate(ctx context.Context) error {
	if c == nil {
		return nil
	}

	body := map[string]interface{}{
		"uid": c.config.UserID,
	}
	if c.config.GivenName != "" {
		body["given_name"] = c.config.GivenName
	}
	if c.config.FamilyName != "" {
		body["family_name"] = c.config.FamilyName
	}
	if c.config.Email != "" {
		body["email"] = c.config.Email
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.URL+"/user/access", strings.NewReader(string(jsonBody)))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("X-API-KEY", c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ER1 authenticate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ER1 authenticate: status %d", resp.StatusCode)
	}

	var accessResp er1AccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&accessResp); err != nil {
		return fmt.Errorf("decode access response: %w", err)
	}

	c.mu.Lock()
	c.ctxID = accessResp.CtxID
	c.mu.Unlock()

	slog.Info("ER1 authenticated", "ctx_id", accessResp.CtxID, "tier", accessResp.Tier)
	return nil
}

// FetchMemories retrieves memories from ER1 since the last sync.
func (c *ER1Client) FetchMemories(ctx context.Context) ([]er1Memory, error) {
	if c == nil {
		return nil, nil
	}

	c.mu.Lock()
	ctxID := c.ctxID
	lastSync := c.lastSync
	c.mu.Unlock()

	if ctxID == "" {
		return nil, fmt.Errorf("not authenticated (no ctx_id)")
	}

	url := fmt.Sprintf("%s/memory/%s?limit=50", c.config.URL, ctxID)
	if !lastSync.IsZero() {
		url += "&startDate=" + lastSync.Format(time.RFC3339)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.config.APIKey != "" {
		req.Header.Set("X-API-KEY", c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ER1 fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ER1 fetch: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var listResp er1MemoryListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decode memories: %w", err)
	}

	return listResp.Memories, nil
}

// SyncOnce fetches new memories from ER1 and indexes them into the vector store.
// Returns the number of memories indexed.
func (c *ER1Client) SyncOnce(ctx context.Context) (int, error) {
	if c == nil {
		return 0, nil
	}

	memories, err := c.FetchMemories(ctx)
	if err != nil {
		return 0, err
	}

	indexed := 0
	for _, m := range memories {
		if m.Transcript == "" || m.TranscriptStatus != "processed" {
			continue
		}

		content := formatER1Memory(m)
		source := "er1:" + m.ID
		tags := strings.Join(m.Tags, ",")

		if _, err := c.service.Store(ctx, content, source, tags); err != nil {
			slog.Warn("ER1 index failed", "memory_id", m.ID, "error", err)
			continue
		}
		indexed++
	}

	c.mu.Lock()
	c.lastSync = time.Now()
	c.mu.Unlock()

	if indexed > 0 {
		slog.Info("ER1 sync complete", "indexed", indexed, "total", len(memories))
	}
	return indexed, nil
}

// SyncLoop runs periodic sync in the background. Blocks until ctx is cancelled.
func (c *ER1Client) SyncLoop(ctx context.Context) {
	if c == nil {
		return
	}

	// Authenticate first
	if err := c.Authenticate(ctx); err != nil {
		slog.Warn("ER1 initial authentication failed", "error", err)
		return
	}

	// Initial sync
	if _, err := c.SyncOnce(ctx); err != nil {
		slog.Warn("ER1 initial sync failed", "error", err)
	}

	ticker := time.NewTicker(c.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := c.SyncOnce(ctx); err != nil {
				slog.Warn("ER1 sync failed", "error", err)
			}
		}
	}
}

// ER1Status holds the current status of the ER1 client.
type ER1Status struct {
	Connected   bool      `json:"connected"`
	LastSync    time.Time `json:"last_sync"`
	SyncedCount int       `json:"synced_count"`
	URL         string    `json:"url"`
}

// Status returns the current ER1 client status.
func (c *ER1Client) Status() ER1Status {
	if c == nil {
		return ER1Status{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return ER1Status{
		Connected: c.ctxID != "",
		LastSync:  c.lastSync,
		URL:       c.config.URL,
	}
}

// formatER1Memory formats an ER1 memory item for vector store indexing.
func formatER1Memory(m er1Memory) string {
	var parts []string

	if len(m.Tags) > 0 {
		parts = append(parts, fmt.Sprintf("Tags: %s", strings.Join(m.Tags, ", ")))
	}
	if m.LocationLat != 0 || m.LocationLon != 0 {
		parts = append(parts, fmt.Sprintf("Location: %.4f, %.4f", m.LocationLat, m.LocationLon))
	}
	if m.Description != "" {
		parts = append(parts, fmt.Sprintf("Description: %s", m.Description))
	}
	parts = append(parts, m.Transcript)

	return strings.Join(parts, "\n")
}
