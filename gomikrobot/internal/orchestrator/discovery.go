package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kamir/gomikrobot/internal/group"
)

// OrchestratorTopicName returns the Kafka topic for orchestrator messages.
func OrchestratorTopicName(groupName string) string {
	return fmt.Sprintf("group.%s.orchestrator", groupName)
}

// Discovery handles Kafka-based orchestrator discovery.
type Discovery struct {
	manager   *group.Manager
	hierarchy *Hierarchy
	zones     *ZoneManager
	selfNode  AgentNode
}

// NewDiscovery creates a discovery handler.
func NewDiscovery(mgr *group.Manager, hierarchy *Hierarchy, zones *ZoneManager, self AgentNode) *Discovery {
	return &Discovery{
		manager:   mgr,
		hierarchy: hierarchy,
		zones:     zones,
		selfNode:  self,
	}
}

// SendDiscovery broadcasts this agent's identity and state on the orchestrator topic.
func (d *Discovery) SendDiscovery(ctx context.Context) error {
	payload := DiscoveryPayload{
		Action:    "discover",
		Node:      d.selfNode,
		Zones:     d.zones.AllZones(),
		Hierarchy: d.hierarchy.AllNodes(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discovery payload: %w", err)
	}

	topic := OrchestratorTopicName(d.manager.GroupName())
	envelope := &group.GroupEnvelope{
		Type:      "orchestrator",
		SenderID:  d.selfNode.AgentID,
		Timestamp: time.Now(),
		Payload:   json.RawMessage(data),
	}

	return d.manager.PublishEnvelope(ctx, topic, envelope)
}

// HandleDiscovery processes an incoming discovery payload from a remote agent.
func (d *Discovery) HandleDiscovery(payload DiscoveryPayload) {
	// Add the discovered node to our hierarchy
	d.hierarchy.AddNode(payload.Node)

	// Merge hierarchy information
	for _, node := range payload.Hierarchy {
		if _, exists := d.hierarchy.GetNode(node.AgentID); !exists {
			d.hierarchy.AddNode(node)
		}
	}

	// Merge zone information (only create zones we don't have)
	for _, zone := range payload.Zones {
		if _, exists := d.zones.GetZone(zone.ZoneID); !exists {
			_ = d.zones.CreateZone(zone)
		}
	}
}
