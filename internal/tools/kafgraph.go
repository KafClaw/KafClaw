package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// QueryGraphTool lets the agent query the er1 knowledge graph (KafGraph) live,
// over its REST API, with no embedder/vector store in the path. This is
// er1-brain leg C(b): the agent pulls graph context on demand to answer.
type QueryGraphTool struct {
	baseURL string
	http    *http.Client
}

// NewQueryGraphTool returns nil if no KafGraph URL is configured.
func NewQueryGraphTool(baseURL string) *QueryGraphTool {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &QueryGraphTool{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (t *QueryGraphTool) Name() string { return "query_graph" }
func (t *QueryGraphTool) Tier() int    { return TierReadOnly }
func (t *QueryGraphTool) Description() string {
	return "Query the er1 knowledge graph: memory/knowledge items unfolded from the live event stream. " +
		"Use 'contains' to find MemoryItems whose text or title mentions a phrase, or 'memory_id' to look up " +
		"one item and its tags, category, and context. Call this to recall what is stored before answering."
}

func (t *QueryGraphTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"contains": map[string]any{
				"type":        "string",
				"description": "Find MemoryItem nodes whose text or title contains this phrase (case-insensitive).",
			},
			"memory_id": map[string]any{
				"type":        "string",
				"description": "Look up one MemoryItem by exact memory_id; returns its text, category, tags, and context.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max matches to return for 'contains' (default 5).",
			},
		},
	}
}

type kgNode struct {
	ID         string         `json:"id"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
}

type kgEdge struct {
	Label  string `json:"label"`
	FromID string `json:"fromId"`
	ToID   string `json:"toId"`
}

func (t *QueryGraphTool) nodes(label string) ([]kgNode, error) {
	resp, err := t.http.Get(t.baseURL + "/api/v1/nodes?label=" + url.QueryEscape(label))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("kafgraph %s: %d %s", label, resp.StatusCode, b)
	}
	var ns []kgNode
	if err := json.NewDecoder(resp.Body).Decode(&ns); err != nil {
		return nil, err
	}
	return ns, nil
}

func propStr(p map[string]any, k string) string {
	if v, ok := p[k].(string); ok {
		return v
	}
	return ""
}

func (t *QueryGraphTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	items, err := t.nodes("MemoryItem")
	if err != nil {
		return "Error querying graph: " + err.Error(), nil
	}

	// Exact lookup by memory_id, with tags/category/context resolved.
	if id := strings.TrimSpace(GetString(params, "memory_id", "")); id != "" {
		for _, n := range items {
			if propStr(n.Properties, "memory_id") != id {
				continue
			}
			return t.describeNode(n), nil
		}
		return fmt.Sprintf("No MemoryItem with memory_id %q in the graph.", id), nil
	}

	// Substring search over text + title.
	q := strings.ToLower(strings.TrimSpace(GetString(params, "contains", "")))
	limit := GetInt(params, "limit", 5)
	if limit <= 0 {
		limit = 5
	}
	var out []string
	for _, n := range items {
		text := propStr(n.Properties, "text")
		title := propStr(n.Properties, "title")
		if q != "" && !strings.Contains(strings.ToLower(text), q) && !strings.Contains(strings.ToLower(title), q) {
			continue
		}
		line := fmt.Sprintf("- memory_id=%s category=%s\n  %s",
			propStr(n.Properties, "memory_id"), propStr(n.Properties, "category"), kgTrunc(text, 220))
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return fmt.Sprintf("No MemoryItem matched %q (graph has %d items).", q, len(items)), nil
	}
	header := fmt.Sprintf("%d match(es) in the er1 graph (of %d items):", len(out), len(items))
	return header + "\n" + strings.Join(out, "\n"), nil
}

// describeNode returns a MemoryItem with its tags, category, and context resolved
// to names via the node's edges.
func (t *QueryGraphTool) describeNode(n kgNode) string {
	var b strings.Builder
	fmt.Fprintf(&b, "memory_id: %s\ncategory: %s\ntext: %s\n",
		propStr(n.Properties, "memory_id"), propStr(n.Properties, "category"),
		kgTrunc(propStr(n.Properties, "text"), 500))

	resp, err := t.http.Get(t.baseURL + "/api/v1/nodes/" + n.ID + "/edges")
	if err == nil {
		defer resp.Body.Close()
		var edges []kgEdge
		if json.NewDecoder(resp.Body).Decode(&edges) == nil && len(edges) > 0 {
			names := t.idNameMap()
			var tags []string
			for _, e := range edges {
				if e.Label == "TAGGED" {
					if nm := names[e.ToID]; nm != "" {
						tags = append(tags, nm)
					}
				}
			}
			if len(tags) > 0 {
				fmt.Fprintf(&b, "tags: %s\n", strings.Join(tags, ", "))
			}
		}
	}
	return b.String()
}

// idNameMap maps Tag/Category/Context node ids to their name/key for edge labels.
func (t *QueryGraphTool) idNameMap() map[string]string {
	m := map[string]string{}
	for _, label := range []string{"Tag", "Category", "Context"} {
		ns, err := t.nodes(label)
		if err != nil {
			continue
		}
		for _, n := range ns {
			name := propStr(n.Properties, "name")
			if name == "" {
				name = propStr(n.Properties, "ctx_id")
			}
			if name != "" {
				m[n.ID] = name
			}
		}
	}
	return m
}

func kgTrunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
