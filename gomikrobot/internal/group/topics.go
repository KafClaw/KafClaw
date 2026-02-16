package group

import (
	"fmt"
	"sync"
	"time"
)

// ExtendedTopicNames holds all topic names for the hierarchical topic structure.
type ExtendedTopicNames struct {
	// Control topics (was: announce)
	ControlAnnounce   string // join/leave/heartbeat (backward-compatible)
	ControlRoster     string // topic registry manifest, member capabilities
	ControlOnboarding string // onboard request/challenge/response/complete

	// Task topics (was: requests/responses)
	TaskRequests  string // general task requests (backward-compatible)
	TaskResponses string // general task responses (backward-compatible)
	TaskStatus    string // task status updates, progress

	// Observe topics (was: traces)
	ObserveTraces string // trace spans (backward-compatible)
	ObserveAudit  string // admin events: member added, topic created, etc.

	// Memory topics
	MemoryShared  string // memory items visible to all group members
	MemoryContext string // ephemeral context sharing (short-lived)

	// Orchestrator topic
	Orchestrator string // orchestrator discovery and coordination
}

// ExtendedTopics returns all topic names for the given group, including the new hierarchical structure.
func ExtendedTopics(groupName string) ExtendedTopicNames {
	return ExtendedTopicNames{
		// Control
		ControlAnnounce:   fmt.Sprintf("group.%s.announce", groupName),
		ControlRoster:     fmt.Sprintf("group.%s.control.roster", groupName),
		ControlOnboarding: fmt.Sprintf("group.%s.control.onboarding", groupName),

		// Tasks
		TaskRequests:  fmt.Sprintf("group.%s.requests", groupName),
		TaskResponses: fmt.Sprintf("group.%s.responses", groupName),
		TaskStatus:    fmt.Sprintf("group.%s.tasks.status", groupName),

		// Observe
		ObserveTraces: fmt.Sprintf("group.%s.traces", groupName),
		ObserveAudit:  fmt.Sprintf("group.%s.observe.audit", groupName),

		// Memory
		MemoryShared:  fmt.Sprintf("group.%s.memory.shared", groupName),
		MemoryContext: fmt.Sprintf("group.%s.memory.context", groupName),

		// Orchestrator
		Orchestrator: fmt.Sprintf("group.%s.orchestrator", groupName),
	}
}

// AllTopics returns all extended topic names as a slice for consumer subscription.
func (t ExtendedTopicNames) AllTopics() []string {
	return []string{
		t.ControlAnnounce,
		t.ControlRoster,
		t.ControlOnboarding,
		t.TaskRequests,
		t.TaskResponses,
		t.TaskStatus,
		t.ObserveTraces,
		t.ObserveAudit,
		t.MemoryShared,
		t.MemoryContext,
		t.Orchestrator,
	}
}

// CoreTopics returns only the original 4 backward-compatible topic names.
func (t ExtendedTopicNames) CoreTopics() []string {
	return []string{
		t.ControlAnnounce,
		t.TaskRequests,
		t.TaskResponses,
		t.ObserveTraces,
	}
}

// SkillTopicPrefix returns the prefix for dynamic skill topics in this group.
func SkillTopicPrefix(groupName string) string {
	return fmt.Sprintf("group.%s.skill.", groupName)
}

// SkillTopics returns the request and response topic names for a skill.
func SkillTopics(groupName, skillName string) (requests, responses string) {
	prefix := SkillTopicPrefix(groupName)
	return prefix + skillName + ".requests", prefix + skillName + ".responses"
}

// TopicDescriptor describes a single topic in the registry.
type TopicDescriptor struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"` // "control", "tasks", "observe", "memory", "skill"
	Description string   `json:"description"`
	Consumers   []string `json:"consumers"` // Agent IDs subscribed
}

// TopicManifest is the registry of all topics in a group, published to control.roster.
type TopicManifest struct {
	GroupName   string            `json:"group_name"`
	Version     int               `json:"version"`
	CoreTopics  []TopicDescriptor `json:"core_topics"`
	SkillTopics []TopicDescriptor `json:"skill_topics"`
	UpdatedAt   time.Time         `json:"updated_at"`
	UpdatedBy   string            `json:"updated_by"`
}

// TopicManager maintains the local topic registry and manifest for a group.
type TopicManager struct {
	groupName string
	manifest  *TopicManifest
	mu        sync.RWMutex
}

// NewTopicManager creates a TopicManager for the given group.
func NewTopicManager(groupName string) *TopicManager {
	ext := ExtendedTopics(groupName)
	manifest := &TopicManifest{
		GroupName: groupName,
		Version:   1,
		CoreTopics: []TopicDescriptor{
			{Name: ext.ControlAnnounce, Category: "control", Description: "Join/leave/heartbeat announcements"},
			{Name: ext.ControlRoster, Category: "control", Description: "Topic registry manifest and member capabilities"},
			{Name: ext.ControlOnboarding, Category: "control", Description: "Agent onboarding protocol messages"},
			{Name: ext.TaskRequests, Category: "tasks", Description: "General task requests"},
			{Name: ext.TaskResponses, Category: "tasks", Description: "General task responses"},
			{Name: ext.TaskStatus, Category: "tasks", Description: "Task status updates and progress"},
			{Name: ext.ObserveTraces, Category: "observe", Description: "Trace spans for distributed tracing"},
			{Name: ext.ObserveAudit, Category: "observe", Description: "Administrative audit events"},
			{Name: ext.MemoryShared, Category: "memory", Description: "Shared memory items via LFS"},
			{Name: ext.MemoryContext, Category: "memory", Description: "Ephemeral context sharing"},
			{Name: ext.Orchestrator, Category: "control", Description: "Orchestrator discovery and coordination"},
		},
		SkillTopics: []TopicDescriptor{},
		UpdatedAt:   time.Now(),
	}

	return &TopicManager{
		groupName: groupName,
		manifest:  manifest,
	}
}

// Manifest returns a copy of the current topic manifest.
func (tm *TopicManager) Manifest() TopicManifest {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return *tm.manifest
}

// AddSkillTopic registers a new skill topic pair in the manifest.
func (tm *TopicManager) AddSkillTopic(skillName, agentID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	reqTopic, respTopic := SkillTopics(tm.groupName, skillName)

	// Check if already registered
	for _, st := range tm.manifest.SkillTopics {
		if st.Name == reqTopic {
			return
		}
	}

	tm.manifest.SkillTopics = append(tm.manifest.SkillTopics,
		TopicDescriptor{
			Name:        reqTopic,
			Category:    "skill",
			Description: fmt.Sprintf("Skill requests for %s", skillName),
			Consumers:   []string{agentID},
		},
		TopicDescriptor{
			Name:        respTopic,
			Category:    "skill",
			Description: fmt.Sprintf("Skill responses for %s", skillName),
			Consumers:   []string{agentID},
		},
	)
	tm.manifest.Version++
	tm.manifest.UpdatedAt = time.Now()
	tm.manifest.UpdatedBy = agentID
}

// AddConsumer adds an agent ID to the consumer list of a topic.
func (tm *TopicManager) AddConsumer(topicName, agentID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	addTo := func(topics []TopicDescriptor) {
		for i, td := range topics {
			if td.Name == topicName {
				for _, c := range td.Consumers {
					if c == agentID {
						return
					}
				}
				topics[i].Consumers = append(topics[i].Consumers, agentID)
				return
			}
		}
	}

	addTo(tm.manifest.CoreTopics)
	addTo(tm.manifest.SkillTopics)
}

// UpdateManifest replaces the current manifest with a newer version (e.g., received from roster topic).
func (tm *TopicManager) UpdateManifest(m *TopicManifest) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if m.Version > tm.manifest.Version {
		tm.manifest = m
	}
}

// SkillNames returns the names of all registered skills.
func (tm *TopicManager) SkillNames() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	seen := make(map[string]bool)
	var names []string
	prefix := SkillTopicPrefix(tm.groupName)

	for _, st := range tm.manifest.SkillTopics {
		// Extract skill name from topic: group.{name}.skill.{skill}.requests
		if len(st.Name) > len(prefix) {
			rest := st.Name[len(prefix):]
			// rest = "{skill}.requests" or "{skill}.responses"
			for i, c := range rest {
				if c == '.' {
					skill := rest[:i]
					if !seen[skill] {
						seen[skill] = true
						names = append(names, skill)
					}
					break
				}
			}
		}
	}
	return names
}
