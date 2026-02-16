package orchestrator

import (
	"fmt"
	"sync"
	"time"
)

// ZoneManager manages zones and their access control.
type ZoneManager struct {
	mu      sync.RWMutex
	zones   map[string]*Zone    // zone_id -> zone
	members map[string][]string // zone_id -> []agent_id
}

// NewZoneManager creates a new zone manager with a default public zone.
func NewZoneManager() *ZoneManager {
	zm := &ZoneManager{
		zones:   make(map[string]*Zone),
		members: make(map[string][]string),
	}
	// Create default public zone
	zm.zones["public"] = &Zone{
		ZoneID:     "public",
		Name:       "Public",
		Visibility: "public",
		OwnerID:    "",
		CreatedAt:  time.Now(),
	}
	zm.members["public"] = []string{}
	return zm
}

// CreateZone creates a new zone.
func (zm *ZoneManager) CreateZone(zone Zone) error {
	zm.mu.Lock()
	defer zm.mu.Unlock()
	if _, exists := zm.zones[zone.ZoneID]; exists {
		return fmt.Errorf("zone %s already exists", zone.ZoneID)
	}
	if zone.CreatedAt.IsZero() {
		zone.CreatedAt = time.Now()
	}
	zm.zones[zone.ZoneID] = &zone
	zm.members[zone.ZoneID] = []string{}
	return nil
}

// DeleteZone removes a zone. Cannot delete the public zone.
func (zm *ZoneManager) DeleteZone(zoneID string) error {
	zm.mu.Lock()
	defer zm.mu.Unlock()
	if zoneID == "public" {
		return fmt.Errorf("cannot delete public zone")
	}
	delete(zm.zones, zoneID)
	delete(zm.members, zoneID)
	return nil
}

// GetZone returns a zone by ID.
func (zm *ZoneManager) GetZone(zoneID string) (Zone, bool) {
	zm.mu.RLock()
	defer zm.mu.RUnlock()
	zone, ok := zm.zones[zoneID]
	if !ok {
		return Zone{}, false
	}
	return *zone, true
}

// AllZones returns all zones.
func (zm *ZoneManager) AllZones() []Zone {
	zm.mu.RLock()
	defer zm.mu.RUnlock()
	zones := make([]Zone, 0, len(zm.zones))
	for _, z := range zm.zones {
		zones = append(zones, *z)
	}
	return zones
}

// AddMember adds an agent to a zone.
func (zm *ZoneManager) AddMember(zoneID, agentID string) error {
	zm.mu.Lock()
	defer zm.mu.Unlock()
	if _, ok := zm.zones[zoneID]; !ok {
		return fmt.Errorf("zone %s not found", zoneID)
	}
	// Check if already member
	for _, id := range zm.members[zoneID] {
		if id == agentID {
			return nil // Already a member
		}
	}
	zm.members[zoneID] = append(zm.members[zoneID], agentID)
	return nil
}

// RemoveMember removes an agent from a zone.
func (zm *ZoneManager) RemoveMember(zoneID, agentID string) {
	zm.mu.Lock()
	defer zm.mu.Unlock()
	members := zm.members[zoneID]
	for i, id := range members {
		if id == agentID {
			zm.members[zoneID] = append(members[:i], members[i+1:]...)
			return
		}
	}
}

// Members returns the agent IDs in a zone.
func (zm *ZoneManager) Members(zoneID string) []string {
	zm.mu.RLock()
	defer zm.mu.RUnlock()
	return append([]string{}, zm.members[zoneID]...)
}

// IsAllowed checks if an agent is allowed to interact with a zone.
func (zm *ZoneManager) IsAllowed(zoneID, agentID string) bool {
	zm.mu.RLock()
	defer zm.mu.RUnlock()
	zone, ok := zm.zones[zoneID]
	if !ok {
		return false
	}
	switch zone.Visibility {
	case "public":
		return true
	case "shared", "private":
		// Owner always allowed
		if zone.OwnerID == agentID {
			return true
		}
		// Check AllowedIDs for private zones
		if zone.Visibility == "private" {
			for _, id := range zone.AllowedIDs {
				if id == agentID {
					return true
				}
			}
		}
		// Check membership
		for _, id := range zm.members[zoneID] {
			if id == agentID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// VisibleAgents returns the agents visible to the given agent based on zone membership.
func (zm *ZoneManager) VisibleAgents(agentID string, allAgents []AgentNode) []AgentNode {
	zm.mu.RLock()
	defer zm.mu.RUnlock()
	visible := make(map[string]bool)
	for zoneID, zone := range zm.zones {
		if zone.Visibility == "public" {
			// All agents in public zone are visible
			for _, id := range zm.members[zoneID] {
				visible[id] = true
			}
			continue
		}
		// Check if this agent is allowed in this zone
		allowed := false
		if zone.OwnerID == agentID {
			allowed = true
		}
		if !allowed {
			for _, id := range zm.members[zoneID] {
				if id == agentID {
					allowed = true
					break
				}
			}
		}
		if !allowed && zone.Visibility == "private" {
			for _, id := range zone.AllowedIDs {
				if id == agentID {
					allowed = true
					break
				}
			}
		}
		if allowed {
			for _, id := range zm.members[zoneID] {
				visible[id] = true
			}
		}
	}
	// Always visible to self
	visible[agentID] = true

	var result []AgentNode
	for _, a := range allAgents {
		if visible[a.AgentID] {
			result = append(result, a)
		}
	}
	return result
}

// Count returns the number of zones.
func (zm *ZoneManager) Count() int {
	zm.mu.RLock()
	defer zm.mu.RUnlock()
	return len(zm.zones)
}

// MemberCount returns the member count for a zone.
func (zm *ZoneManager) MemberCount(zoneID string) int {
	zm.mu.RLock()
	defer zm.mu.RUnlock()
	return len(zm.members[zoneID])
}
