package orchestrator

import (
	"testing"
)

func TestHierarchyAddAndGet(t *testing.T) {
	h := NewHierarchy()

	root := AgentNode{AgentID: "root", Role: "orchestrator", Status: "active"}
	h.AddNode(root)

	got, ok := h.GetNode("root")
	if !ok {
		t.Fatal("expected to find root node")
	}
	if got.Role != "orchestrator" {
		t.Errorf("expected role orchestrator, got %s", got.Role)
	}
}

func TestHierarchyParentChild(t *testing.T) {
	h := NewHierarchy()

	h.AddNode(AgentNode{AgentID: "root", Role: "orchestrator"})
	h.AddNode(AgentNode{AgentID: "child1", Role: "worker", ParentID: "root"})
	h.AddNode(AgentNode{AgentID: "child2", Role: "worker", ParentID: "root"})
	h.AddNode(AgentNode{AgentID: "grandchild", Role: "worker", ParentID: "child1"})

	children := h.Children("root")
	if len(children) != 2 {
		t.Errorf("expected 2 children of root, got %d", len(children))
	}

	if !h.IsDescendant("grandchild", "root") {
		t.Error("grandchild should be descendant of root")
	}
	if !h.IsDescendant("child1", "root") {
		t.Error("child1 should be descendant of root")
	}
	if h.IsDescendant("root", "child1") {
		t.Error("root should NOT be descendant of child1")
	}

	ancestors := h.Ancestors("grandchild")
	if len(ancestors) != 2 {
		t.Errorf("expected 2 ancestors of grandchild, got %d", len(ancestors))
	}
}

func TestHierarchyRemoveReparents(t *testing.T) {
	h := NewHierarchy()

	h.AddNode(AgentNode{AgentID: "root", Role: "orchestrator"})
	h.AddNode(AgentNode{AgentID: "mid", Role: "worker", ParentID: "root"})
	h.AddNode(AgentNode{AgentID: "leaf", Role: "worker", ParentID: "mid"})

	h.RemoveNode("mid")

	// Leaf should be reparented to root
	node, ok := h.GetNode("leaf")
	if !ok {
		t.Fatal("leaf should still exist")
	}
	if node.ParentID != "root" {
		t.Errorf("leaf should be reparented to root, got parent %s", node.ParentID)
	}

	if h.Count() != 2 {
		t.Errorf("expected 2 nodes after removal, got %d", h.Count())
	}
}

func TestZoneManagerPublic(t *testing.T) {
	zm := NewZoneManager()

	// Public zone should allow anyone
	if !zm.IsAllowed("public", "agent1") {
		t.Error("public zone should allow any agent")
	}
}

func TestZoneManagerPrivate(t *testing.T) {
	zm := NewZoneManager()

	zone := Zone{
		ZoneID:     "team-alpha",
		Name:       "Team Alpha",
		Visibility: "private",
		OwnerID:    "agent1",
		AllowedIDs: []string{"agent2"},
	}
	if err := zm.CreateZone(zone); err != nil {
		t.Fatalf("create zone failed: %v", err)
	}

	// Owner is allowed
	if !zm.IsAllowed("team-alpha", "agent1") {
		t.Error("owner should be allowed")
	}

	// Allowed agent
	if !zm.IsAllowed("team-alpha", "agent2") {
		t.Error("allowed agent should be allowed")
	}

	// Random agent is not allowed
	if zm.IsAllowed("team-alpha", "agent3") {
		t.Error("random agent should not be allowed in private zone")
	}
}

func TestZoneManagerShared(t *testing.T) {
	zm := NewZoneManager()

	zone := Zone{
		ZoneID:     "shared-zone",
		Name:       "Shared Zone",
		Visibility: "shared",
		OwnerID:    "agent1",
	}
	if err := zm.CreateZone(zone); err != nil {
		t.Fatalf("create zone failed: %v", err)
	}

	// Add a member
	if err := zm.AddMember("shared-zone", "agent2"); err != nil {
		t.Fatalf("add member failed: %v", err)
	}

	// Owner allowed
	if !zm.IsAllowed("shared-zone", "agent1") {
		t.Error("owner should be allowed")
	}

	// Member allowed
	if !zm.IsAllowed("shared-zone", "agent2") {
		t.Error("member should be allowed")
	}

	// Non-member not allowed
	if zm.IsAllowed("shared-zone", "agent3") {
		t.Error("non-member should not be allowed")
	}

	// Verify member count
	if count := zm.MemberCount("shared-zone"); count != 1 {
		t.Errorf("expected 1 member, got %d", count)
	}
}

func TestZoneManagerDuplicateCreate(t *testing.T) {
	zm := NewZoneManager()

	zone := Zone{ZoneID: "test", Name: "Test", Visibility: "public"}
	if err := zm.CreateZone(zone); err != nil {
		t.Fatal(err)
	}
	if err := zm.CreateZone(zone); err == nil {
		t.Error("expected error on duplicate create")
	}
}

func TestZoneManagerDeletePublic(t *testing.T) {
	zm := NewZoneManager()
	if err := zm.DeleteZone("public"); err == nil {
		t.Error("should not be able to delete public zone")
	}
}

func TestZoneManagerVisibleAgents(t *testing.T) {
	zm := NewZoneManager()

	// Create a private zone
	zm.CreateZone(Zone{
		ZoneID:     "private-zone",
		Name:       "Private",
		Visibility: "private",
		OwnerID:    "agent1",
		AllowedIDs: []string{"agent2"},
	})
	zm.AddMember("private-zone", "agent2")
	zm.AddMember("public", "agent1")
	zm.AddMember("public", "agent3")

	allAgents := []AgentNode{
		{AgentID: "agent1"},
		{AgentID: "agent2"},
		{AgentID: "agent3"},
	}

	// Agent1 (owner) should see agent2 (member of private zone) + public members
	visible := zm.VisibleAgents("agent1", allAgents)
	if len(visible) != 3 {
		t.Errorf("agent1 should see 3 agents (self + public + private members), got %d", len(visible))
	}

	// Agent3 should NOT see agent2 (only in private zone)
	visible = zm.VisibleAgents("agent3", allAgents)
	visibleIDs := map[string]bool{}
	for _, a := range visible {
		visibleIDs[a.AgentID] = true
	}
	if visibleIDs["agent2"] {
		t.Error("agent3 should not see agent2 who is only in private zone")
	}
}

func TestDiscoveryHandlePayload(t *testing.T) {
	h := NewHierarchy()
	zm := NewZoneManager()
	self := AgentNode{AgentID: "local", Role: "worker"}
	h.AddNode(self)

	d := NewDiscovery(nil, h, zm, self)

	// Simulate receiving a discovery from a remote agent
	payload := DiscoveryPayload{
		Action: "discover",
		Node:   AgentNode{AgentID: "remote-agent", Role: "orchestrator", ZoneID: "team-beta"},
		Zones: []Zone{
			{ZoneID: "team-beta", Name: "Team Beta", Visibility: "shared", OwnerID: "remote-agent"},
		},
		Hierarchy: []AgentNode{
			{AgentID: "remote-worker", Role: "worker", ParentID: "remote-agent"},
		},
	}

	d.HandleDiscovery(payload)

	// Check the remote agent was added
	if _, ok := h.GetNode("remote-agent"); !ok {
		t.Error("remote-agent should be in hierarchy")
	}
	if _, ok := h.GetNode("remote-worker"); !ok {
		t.Error("remote-worker should be in hierarchy")
	}

	// Check the zone was created
	if _, ok := zm.GetZone("team-beta"); !ok {
		t.Error("team-beta zone should be created")
	}

	if h.Count() != 3 {
		t.Errorf("expected 3 nodes, got %d", h.Count())
	}
}
