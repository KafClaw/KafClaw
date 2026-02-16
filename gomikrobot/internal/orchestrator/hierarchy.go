package orchestrator

import "sync"

// Hierarchy is a thread-safe tree structure for agent parent/child relationships.
type Hierarchy struct {
	mu    sync.RWMutex
	nodes map[string]*AgentNode // agent_id -> node
}

// NewHierarchy creates an empty hierarchy.
func NewHierarchy() *Hierarchy {
	return &Hierarchy{
		nodes: make(map[string]*AgentNode),
	}
}

// AddNode adds or updates an agent in the hierarchy.
func (h *Hierarchy) AddNode(node AgentNode) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nodes[node.AgentID] = &node
}

// RemoveNode removes an agent from the hierarchy.
// Children of the removed node are reparented to the removed node's parent.
func (h *Hierarchy) RemoveNode(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	node, ok := h.nodes[agentID]
	if !ok {
		return
	}
	parentID := node.ParentID
	// Reparent children
	for _, n := range h.nodes {
		if n.ParentID == agentID {
			n.ParentID = parentID
		}
	}
	delete(h.nodes, agentID)
}

// SetParent sets the parent of an agent.
func (h *Hierarchy) SetParent(agentID, parentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if node, ok := h.nodes[agentID]; ok {
		node.ParentID = parentID
	}
}

// RemoveChild removes a specific child relationship.
func (h *Hierarchy) RemoveChild(parentID, childID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if node, ok := h.nodes[childID]; ok && node.ParentID == parentID {
		node.ParentID = ""
	}
}

// Children returns the direct children of an agent.
func (h *Hierarchy) Children(agentID string) []AgentNode {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var children []AgentNode
	for _, n := range h.nodes {
		if n.ParentID == agentID {
			children = append(children, *n)
		}
	}
	return children
}

// Ancestors returns the ancestor chain from the given agent to root.
func (h *Hierarchy) Ancestors(agentID string) []AgentNode {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var ancestors []AgentNode
	seen := make(map[string]bool)
	current := agentID
	for {
		node, ok := h.nodes[current]
		if !ok || node.ParentID == "" || seen[node.ParentID] {
			break
		}
		seen[node.ParentID] = true
		parent, ok := h.nodes[node.ParentID]
		if !ok {
			break
		}
		ancestors = append(ancestors, *parent)
		current = parent.AgentID
	}
	return ancestors
}

// IsDescendant checks if agentID is a descendant of ancestorID.
func (h *Hierarchy) IsDescendant(agentID, ancestorID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := make(map[string]bool)
	current := agentID
	for {
		node, ok := h.nodes[current]
		if !ok || node.ParentID == "" || seen[current] {
			return false
		}
		seen[current] = true
		if node.ParentID == ancestorID {
			return true
		}
		current = node.ParentID
	}
}

// GetNode returns a node by ID.
func (h *Hierarchy) GetNode(agentID string) (AgentNode, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	node, ok := h.nodes[agentID]
	if !ok {
		return AgentNode{}, false
	}
	return *node, true
}

// AllNodes returns all nodes in the hierarchy.
func (h *Hierarchy) AllNodes() []AgentNode {
	h.mu.RLock()
	defer h.mu.RUnlock()
	nodes := make([]AgentNode, 0, len(h.nodes))
	for _, n := range h.nodes {
		nodes = append(nodes, *n)
	}
	return nodes
}

// Count returns the number of nodes.
func (h *Hierarchy) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.nodes)
}
