// Package tools provides the tool framework and implementations for the agent.
package tools

import (
	"context"
	"fmt"
)

// Tool is the interface that all agent tools must implement.
type Tool interface {
	// Name returns the tool identifier used in function calls.
	Name() string
	// Description returns a human-readable description for the LLM.
	Description() string
	// Parameters returns the JSON Schema for tool parameters.
	Parameters() map[string]any
	// Execute runs the tool with the given parameters.
	// Returns result string and error. On error, return user-friendly message.
	Execute(ctx context.Context, params map[string]any) (string, error)
}

// TieredTool is an optional interface for tools that declare a risk tier.
// Tier 0: read-only (always allowed)
// Tier 1: controlled writes (allowed by policy)
// Tier 2: external/high-impact (requires approval)
type TieredTool interface {
	Tool
	Tier() int
}

// Risk tier constants.
const (
	TierReadOnly  = 0 // Read-only internal tools
	TierWrite     = 1 // Controlled write/internal effects
	TierHighRisk  = 2 // External or high-impact actions
)

// ToolTier returns the risk tier for a tool.
// If the tool implements TieredTool, its Tier() is returned.
// Otherwise defaults to TierReadOnly (safe default for unclassified tools).
func ToolTier(t Tool) int {
	if tt, ok := t.(TieredTool); ok {
		return tt.Tier()
	}
	return TierReadOnly
}

// DefaultToolNames returns the names of tools that are registered by default
// in the agent loop. Used for identity announcements when a full registry is
// not available (e.g. group manager startup).
func DefaultToolNames() []string {
	return []string{
		"read_file", "write_file", "edit_file",
		"list_dir", "resolve_path", "exec",
	}
}

// Registry manages tool registration and execution.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}
	return result
}

// Definitions returns tool definitions in OpenAI format.
func (r *Registry) Definitions() []map[string]any {
	result := make([]map[string]any, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name(),
				"description": tool.Description(),
				"parameters":  tool.Parameters(),
			},
		})
	}
	return result
}

// Execute runs a tool by name with the given parameters.
func (r *Registry) Execute(ctx context.Context, name string, params map[string]any) (string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}
	return tool.Execute(ctx, params)
}

// GetString extracts a string parameter with a default value.
func GetString(params map[string]any, key string, defaultVal string) string {
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

// GetInt extracts an int parameter with a default value.
func GetInt(params map[string]any, key string, defaultVal int) int {
	if v, ok := params[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return defaultVal
}

// GetBool extracts a bool parameter with a default value.
func GetBool(params map[string]any, key string, defaultVal bool) bool {
	if v, ok := params[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}
