// Package orchestrator implements multi-agent coordination with hierarchy and zones.
package orchestrator

import "time"

// AgentNode represents an agent in the orchestrator hierarchy.
type AgentNode struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	Role      string `json:"role"`       // "orchestrator", "worker", "observer"
	ParentID  string `json:"parent_id"`  // Empty for root nodes
	ZoneID    string `json:"zone_id"`
	Endpoint  string `json:"endpoint"`   // Remote API URL
	Status    string `json:"status"`     // "active", "stale", "inactive"
}

// Zone represents a security boundary for agent groups.
type Zone struct {
	ZoneID     string   `json:"zone_id"`
	Name       string   `json:"name"`
	Visibility string   `json:"visibility"` // "private", "shared", "public"
	OwnerID    string   `json:"owner_id"`
	ParentZone string   `json:"parent_zone"`
	AllowedIDs []string `json:"allowed_ids,omitempty"` // For private zones
	CreatedAt  time.Time `json:"created_at"`
}

// DiscoveryPayload is sent on the orchestrator Kafka topic for discovery.
type DiscoveryPayload struct {
	Action    string    `json:"action"` // "discover", "hierarchy_update", "zone_update"
	Node      AgentNode `json:"node"`
	Zones     []Zone    `json:"zones,omitempty"`
	Hierarchy []AgentNode `json:"hierarchy,omitempty"`
}

// HierarchyPayload communicates hierarchy changes.
type HierarchyPayload struct {
	Action   string    `json:"action"` // "set_parent", "remove_child"
	AgentID  string    `json:"agent_id"`
	ParentID string    `json:"parent_id"`
}

// ZoneUpdatePayload communicates zone changes.
type ZoneUpdatePayload struct {
	Action  string `json:"action"` // "create", "delete", "join_request", "join_approve", "join_deny"
	Zone    Zone   `json:"zone"`
	AgentID string `json:"agent_id,omitempty"`
}

// OrchestratorStatus is returned by the status API endpoint.
type OrchestratorStatus struct {
	Enabled    bool   `json:"enabled"`
	Role       string `json:"role"`
	AgentID    string `json:"agent_id"`
	ZoneID     string `json:"zone_id"`
	ParentID   string `json:"parent_id"`
	AgentCount int    `json:"agent_count"`
	ZoneCount  int    `json:"zone_count"`
}
