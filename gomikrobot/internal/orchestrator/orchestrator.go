package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"github.com/kamir/gomikrobot/internal/config"
	"github.com/kamir/gomikrobot/internal/group"
	"github.com/kamir/gomikrobot/internal/timeline"
)

// Orchestrator coordinates multi-agent hierarchies and zones.
type Orchestrator struct {
	mu        sync.RWMutex
	manager   *group.Manager
	hierarchy *Hierarchy
	zones     *ZoneManager
	discovery *Discovery
	timeline  *timeline.TimelineService
	selfNode  AgentNode
	cfg       config.OrchestratorConfig
	running   bool
}

// New creates a new Orchestrator.
func New(cfg config.OrchestratorConfig, mgr *group.Manager, timeSvc *timeline.TimelineService) *Orchestrator {
	selfNode := AgentNode{
		AgentID:  mgr.AgentID(),
		Role:     cfg.Role,
		ZoneID:   cfg.ZoneID,
		Endpoint: cfg.Endpoint,
		Status:   "active",
	}
	if cfg.ParentID != "" {
		selfNode.ParentID = cfg.ParentID
	}

	h := NewHierarchy()
	h.AddNode(selfNode)

	zm := NewZoneManager()
	// Add self to default public zone
	_ = zm.AddMember("public", selfNode.AgentID)

	// If a specific zone is configured, create and join it
	if cfg.ZoneID != "" && cfg.ZoneID != "public" {
		_ = zm.CreateZone(Zone{
			ZoneID:     cfg.ZoneID,
			Name:       cfg.ZoneID,
			Visibility: "shared",
			OwnerID:    selfNode.AgentID,
		})
		_ = zm.AddMember(cfg.ZoneID, selfNode.AgentID)
	}

	discovery := NewDiscovery(mgr, h, zm, selfNode)

	return &Orchestrator{
		manager:   mgr,
		hierarchy: h,
		zones:     zm,
		discovery: discovery,
		timeline:  timeSvc,
		selfNode:  selfNode,
		cfg:       cfg,
	}
}

// Start begins orchestrator discovery and listening.
func (o *Orchestrator) Start(ctx context.Context) error {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return nil
	}
	o.running = true
	o.mu.Unlock()

	// Send initial discovery
	if err := o.discovery.SendDiscovery(ctx); err != nil {
		fmt.Printf("⚠️ Orchestrator discovery send failed: %v\n", err)
	}

	// Persist self to DB
	o.persistHierarchyNode(o.selfNode)

	fmt.Printf("🎯 Orchestrator started: role=%s zone=%s\n", o.cfg.Role, o.cfg.ZoneID)
	return nil
}

// Stop shuts down the orchestrator.
func (o *Orchestrator) Stop(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.running = false
	return nil
}

// Status returns the orchestrator status.
func (o *Orchestrator) Status() OrchestratorStatus {
	return OrchestratorStatus{
		Enabled:    true,
		Role:       o.selfNode.Role,
		AgentID:    o.selfNode.AgentID,
		ZoneID:     o.selfNode.ZoneID,
		ParentID:   o.selfNode.ParentID,
		AgentCount: o.hierarchy.Count(),
		ZoneCount:  o.zones.Count(),
	}
}

// DispatchTask dispatches a task within a target zone.
func (o *Orchestrator) DispatchTask(ctx context.Context, taskID, desc, targetZone string) error {
	if targetZone == "" {
		targetZone = "public"
	}
	if !o.zones.IsAllowed(targetZone, o.selfNode.AgentID) {
		return fmt.Errorf("agent %s not allowed in zone %s", o.selfNode.AgentID, targetZone)
	}

	// Dispatch to group via existing manager
	if o.manager != nil && o.manager.Active() {
		return o.manager.SubmitTask(ctx, taskID, desc, "")
	}
	return fmt.Errorf("group manager not active")
}

// GetHierarchy returns all nodes.
func (o *Orchestrator) GetHierarchy() []AgentNode {
	return o.hierarchy.AllNodes()
}

// GetZones returns all zones with member counts.
func (o *Orchestrator) GetZones() []map[string]any {
	zones := o.zones.AllZones()
	result := make([]map[string]any, len(zones))
	for i, z := range zones {
		result[i] = map[string]any{
			"zone_id":      z.ZoneID,
			"name":         z.Name,
			"visibility":   z.Visibility,
			"owner_id":     z.OwnerID,
			"parent_zone":  z.ParentZone,
			"member_count": o.zones.MemberCount(z.ZoneID),
			"created_at":   z.CreatedAt,
		}
	}
	return result
}

// GetAgents returns all known agents.
func (o *Orchestrator) GetAgents() []AgentNode {
	return o.hierarchy.AllNodes()
}

// IsAllowed checks if an agent has access to a zone.
func (o *Orchestrator) IsAllowed(zoneID, agentID string) bool {
	return o.zones.IsAllowed(zoneID, agentID)
}

// CreateZone creates a new zone.
func (o *Orchestrator) CreateZone(zone Zone) error {
	if err := o.zones.CreateZone(zone); err != nil {
		return err
	}
	o.persistZone(zone)
	return nil
}

// HandleDiscovery processes a discovery payload from a remote agent.
func (o *Orchestrator) HandleDiscovery(payload DiscoveryPayload) {
	o.discovery.HandleDiscovery(payload)
	// Persist discovered nodes
	o.persistHierarchyNode(payload.Node)
}

func (o *Orchestrator) persistHierarchyNode(node AgentNode) {
	if o.timeline == nil {
		return
	}
	_, _ = o.timeline.DB().Exec(`INSERT INTO orchestrator_hierarchy
		(agent_id, parent_id, role, endpoint, zone_id, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(agent_id) DO UPDATE SET
			parent_id = excluded.parent_id,
			role = excluded.role,
			endpoint = excluded.endpoint,
			zone_id = excluded.zone_id,
			updated_at = datetime('now')`,
		node.AgentID, node.ParentID, node.Role, node.Endpoint, node.ZoneID)
}

func (o *Orchestrator) persistZone(zone Zone) {
	if o.timeline == nil {
		return
	}
	_, _ = o.timeline.DB().Exec(`INSERT INTO orchestrator_zones
		(zone_id, name, visibility, owner_id, parent_zone)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(zone_id) DO UPDATE SET
			name = excluded.name,
			visibility = excluded.visibility,
			owner_id = excluded.owner_id,
			parent_zone = excluded.parent_zone`,
		zone.ZoneID, zone.Name, zone.Visibility, zone.OwnerID, zone.ParentZone)
}
