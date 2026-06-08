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

// KafGraphConfig configures the KafGraph knowledge-graph sync.
type KafGraphConfig struct {
	URL          string        // e.g. "http://192.168.0.135:17474"
	Label        string        // node label to sync (default "MemoryItem")
	SyncInterval time.Duration // default: 5 minutes
}

// KafGraphClient syncs nodes from a KafGraph knowledge graph into the KafClaw
// vector store as "kafgraph:" source chunks, so the agent reasons over the
// graph via the same RAG path as other memory sources. Mirrors ER1Client; this
// is leg C(a) of er1-brain (graph -> vector). The complementary live-query path
// (a query_graph tool, leg C(b)) traverses relationships instead of flattening.
type KafGraphClient struct {
	config     KafGraphConfig
	httpClient *http.Client
	service    *MemoryService
	lastSync   time.Time
	mu         sync.Mutex
}

// kgNode is a KafGraph node as returned by GET /api/v1/nodes.
type kgNode struct {
	ID         string                 `json:"id"`
	Label      string                 `json:"label"`
	Properties map[string]interface{} `json:"properties"`
	CreatedAt  time.Time              `json:"createdAt"`
}

// NewKafGraphClient creates a new KafGraph client. Returns nil if URL or service
// is empty/nil.
func NewKafGraphClient(cfg KafGraphConfig, service *MemoryService) *KafGraphClient {
	if cfg.URL == "" || service == nil {
		return nil
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 5 * time.Minute
	}
	if cfg.Label == "" {
		cfg.Label = "MemoryItem"
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")

	return &KafGraphClient{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		service:    service,
	}
}

// FetchNodes retrieves all nodes of the configured label from KafGraph.
func (c *KafGraphClient) FetchNodes(ctx context.Context) ([]kgNode, error) {
	if c == nil {
		return nil, nil
	}
	url := fmt.Sprintf("%s/api/v1/nodes?label=%s", c.config.URL, c.config.Label)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kafgraph fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("kafgraph fetch: status %d: %s", resp.StatusCode, string(b))
	}
	var nodes []kgNode
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		return nil, fmt.Errorf("decode nodes: %w", err)
	}
	return nodes, nil
}

// SyncOnce fetches nodes from KafGraph and indexes new ones into the vector
// store. Returns the number indexed. Only nodes created after the last sync are
// indexed (the graph's createdAt drives the incremental).
func (c *KafGraphClient) SyncOnce(ctx context.Context) (int, error) {
	if c == nil {
		return 0, nil
	}
	nodes, err := c.FetchNodes(ctx)
	if err != nil {
		return 0, err
	}
	c.mu.Lock()
	lastSync := c.lastSync
	c.mu.Unlock()

	indexed := 0
	for _, n := range nodes {
		if !lastSync.IsZero() && !n.CreatedAt.After(lastSync) {
			continue
		}
		content := formatKafGraphNode(n)
		if content == "" {
			continue
		}
		source := "kafgraph:" + propStr(n.Properties, "memory_id")
		if source == "kafgraph:" {
			source = "kafgraph:" + n.ID
		}
		tags := propStr(n.Properties, "category")
		if _, err := c.service.Store(ctx, content, source, tags); err != nil {
			slog.Warn("kafgraph index failed", "node", n.ID, "error", err)
			continue
		}
		indexed++
	}

	c.mu.Lock()
	c.lastSync = time.Now()
	c.mu.Unlock()

	if indexed > 0 {
		slog.Info("kafgraph sync complete", "indexed", indexed, "total", len(nodes), "label", c.config.Label)
	}
	return indexed, nil
}

// SyncLoop runs periodic sync in the background. Blocks until ctx is cancelled.
func (c *KafGraphClient) SyncLoop(ctx context.Context) {
	if c == nil {
		return
	}
	if _, err := c.SyncOnce(ctx); err != nil {
		slog.Warn("kafgraph initial sync failed", "error", err)
	}
	ticker := time.NewTicker(c.config.SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := c.SyncOnce(ctx); err != nil {
				slog.Warn("kafgraph sync failed", "error", err)
			}
		}
	}
}

func propStr(p map[string]interface{}, k string) string {
	if v, ok := p[k].(string); ok {
		return v
	}
	return ""
}

// formatKafGraphNode formats a KafGraph node for vector store indexing.
func formatKafGraphNode(n kgNode) string {
	var parts []string
	if t := propStr(n.Properties, "title"); t != "" {
		parts = append(parts, "Title: "+t)
	}
	if cat := propStr(n.Properties, "category"); cat != "" {
		parts = append(parts, "Category: "+cat)
	}
	if id := propStr(n.Properties, "memory_id"); id != "" {
		parts = append(parts, "MemoryID: "+id)
	}
	if txt := propStr(n.Properties, "text"); txt != "" {
		parts = append(parts, txt)
	}
	return strings.Join(parts, "\n")
}
