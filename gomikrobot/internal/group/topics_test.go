package group

import (
	"testing"
)

func TestTopics_BackwardCompat(t *testing.T) {
	tn := Topics("mygroup")
	if tn.Announce != "group.mygroup.announce" {
		t.Errorf("expected group.mygroup.announce, got %s", tn.Announce)
	}
	if tn.Requests != "group.mygroup.requests" {
		t.Errorf("expected group.mygroup.requests, got %s", tn.Requests)
	}
	if tn.Responses != "group.mygroup.responses" {
		t.Errorf("expected group.mygroup.responses, got %s", tn.Responses)
	}
	if tn.Traces != "group.mygroup.traces" {
		t.Errorf("expected group.mygroup.traces, got %s", tn.Traces)
	}
}

func TestExtendedTopics(t *testing.T) {
	ext := ExtendedTopics("mygroup")

	// Control topics
	if ext.ControlAnnounce != "group.mygroup.announce" {
		t.Errorf("ControlAnnounce: expected group.mygroup.announce, got %s", ext.ControlAnnounce)
	}
	if ext.ControlRoster != "group.mygroup.control.roster" {
		t.Errorf("ControlRoster: expected group.mygroup.control.roster, got %s", ext.ControlRoster)
	}
	if ext.ControlOnboarding != "group.mygroup.control.onboarding" {
		t.Errorf("ControlOnboarding: expected group.mygroup.control.onboarding, got %s", ext.ControlOnboarding)
	}

	// Task topics (backward-compat)
	if ext.TaskRequests != "group.mygroup.requests" {
		t.Errorf("TaskRequests: expected group.mygroup.requests, got %s", ext.TaskRequests)
	}
	if ext.TaskResponses != "group.mygroup.responses" {
		t.Errorf("TaskResponses: expected group.mygroup.responses, got %s", ext.TaskResponses)
	}
	if ext.TaskStatus != "group.mygroup.tasks.status" {
		t.Errorf("TaskStatus: expected group.mygroup.tasks.status, got %s", ext.TaskStatus)
	}

	// Observe topics
	if ext.ObserveTraces != "group.mygroup.traces" {
		t.Errorf("ObserveTraces: expected group.mygroup.traces, got %s", ext.ObserveTraces)
	}
	if ext.ObserveAudit != "group.mygroup.observe.audit" {
		t.Errorf("ObserveAudit: expected group.mygroup.observe.audit, got %s", ext.ObserveAudit)
	}

	// Memory topics
	if ext.MemoryShared != "group.mygroup.memory.shared" {
		t.Errorf("MemoryShared: expected group.mygroup.memory.shared, got %s", ext.MemoryShared)
	}
	if ext.MemoryContext != "group.mygroup.memory.context" {
		t.Errorf("MemoryContext: expected group.mygroup.memory.context, got %s", ext.MemoryContext)
	}
}

func TestExtendedTopics_BackwardCompat(t *testing.T) {
	ext := ExtendedTopics("mygroup")
	orig := Topics("mygroup")

	// Ensure the 4 original topics match
	if ext.ControlAnnounce != orig.Announce {
		t.Errorf("ControlAnnounce != Announce: %s != %s", ext.ControlAnnounce, orig.Announce)
	}
	if ext.TaskRequests != orig.Requests {
		t.Errorf("TaskRequests != Requests: %s != %s", ext.TaskRequests, orig.Requests)
	}
	if ext.TaskResponses != orig.Responses {
		t.Errorf("TaskResponses != Responses: %s != %s", ext.TaskResponses, orig.Responses)
	}
	if ext.ObserveTraces != orig.Traces {
		t.Errorf("ObserveTraces != Traces: %s != %s", ext.ObserveTraces, orig.Traces)
	}
}

func TestExtendedTopics_AllTopics(t *testing.T) {
	ext := ExtendedTopics("mygroup")
	all := ext.AllTopics()

	if len(all) != 11 {
		t.Fatalf("expected 11 topics, got %d", len(all))
	}

	// Verify all topics are unique
	seen := make(map[string]bool)
	for _, topic := range all {
		if seen[topic] {
			t.Errorf("duplicate topic: %s", topic)
		}
		seen[topic] = true
	}
}

func TestExtendedTopics_CoreTopics(t *testing.T) {
	ext := ExtendedTopics("mygroup")
	core := ext.CoreTopics()

	if len(core) != 4 {
		t.Fatalf("expected 4 core topics, got %d", len(core))
	}

	// Should be the original 4
	expected := []string{
		"group.mygroup.announce",
		"group.mygroup.requests",
		"group.mygroup.responses",
		"group.mygroup.traces",
	}
	for i, exp := range expected {
		if core[i] != exp {
			t.Errorf("core[%d]: expected %s, got %s", i, exp, core[i])
		}
	}
}

func TestSkillTopics(t *testing.T) {
	req, resp := SkillTopics("mygroup", "web_search")
	if req != "group.mygroup.skill.web_search.requests" {
		t.Errorf("expected group.mygroup.skill.web_search.requests, got %s", req)
	}
	if resp != "group.mygroup.skill.web_search.responses" {
		t.Errorf("expected group.mygroup.skill.web_search.responses, got %s", resp)
	}
}

func TestTopicManifest_NewTopicManager(t *testing.T) {
	tm := NewTopicManager("mygroup")
	manifest := tm.Manifest()

	if manifest.GroupName != "mygroup" {
		t.Errorf("expected group name mygroup, got %s", manifest.GroupName)
	}
	if manifest.Version != 1 {
		t.Errorf("expected version 1, got %d", manifest.Version)
	}
	if len(manifest.CoreTopics) != 11 {
		t.Errorf("expected 11 core topics, got %d", len(manifest.CoreTopics))
	}
	if len(manifest.SkillTopics) != 0 {
		t.Errorf("expected 0 skill topics, got %d", len(manifest.SkillTopics))
	}
}

func TestTopicManager_AddSkillTopic(t *testing.T) {
	tm := NewTopicManager("mygroup")

	tm.AddSkillTopic("web_search", "agent-1")
	manifest := tm.Manifest()

	if manifest.Version != 2 {
		t.Errorf("expected version 2 after adding skill, got %d", manifest.Version)
	}
	if len(manifest.SkillTopics) != 2 {
		t.Fatalf("expected 2 skill topic descriptors, got %d", len(manifest.SkillTopics))
	}
	if manifest.SkillTopics[0].Name != "group.mygroup.skill.web_search.requests" {
		t.Errorf("unexpected skill topic name: %s", manifest.SkillTopics[0].Name)
	}
	if manifest.SkillTopics[0].Category != "skill" {
		t.Errorf("expected category skill, got %s", manifest.SkillTopics[0].Category)
	}

	// Adding same skill again should be a no-op
	tm.AddSkillTopic("web_search", "agent-1")
	manifest = tm.Manifest()
	if manifest.Version != 2 {
		t.Errorf("expected version still 2 after duplicate add, got %d", manifest.Version)
	}
	if len(manifest.SkillTopics) != 2 {
		t.Errorf("expected still 2 skill topics after duplicate, got %d", len(manifest.SkillTopics))
	}
}

func TestTopicManager_SkillNames(t *testing.T) {
	tm := NewTopicManager("mygroup")

	tm.AddSkillTopic("web_search", "agent-1")
	tm.AddSkillTopic("code_review", "agent-2")

	names := tm.SkillNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 skill names, got %d", len(names))
	}

	has := make(map[string]bool)
	for _, n := range names {
		has[n] = true
	}
	if !has["web_search"] {
		t.Error("missing web_search")
	}
	if !has["code_review"] {
		t.Error("missing code_review")
	}
}

func TestTopicManager_AddConsumer(t *testing.T) {
	tm := NewTopicManager("mygroup")
	tm.AddSkillTopic("web_search", "agent-1")

	reqTopic, _ := SkillTopics("mygroup", "web_search")
	tm.AddConsumer(reqTopic, "agent-2")

	manifest := tm.Manifest()
	found := false
	for _, st := range manifest.SkillTopics {
		if st.Name == reqTopic {
			for _, c := range st.Consumers {
				if c == "agent-2" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected agent-2 in consumers list")
	}
}

func TestTopicManager_UpdateManifest(t *testing.T) {
	tm := NewTopicManager("mygroup")

	// Newer version replaces
	newManifest := &TopicManifest{
		GroupName: "mygroup",
		Version:   5,
		CoreTopics: []TopicDescriptor{
			{Name: "custom", Category: "control"},
		},
	}
	tm.UpdateManifest(newManifest)
	if tm.Manifest().Version != 5 {
		t.Errorf("expected version 5, got %d", tm.Manifest().Version)
	}

	// Older version doesn't replace
	oldManifest := &TopicManifest{
		GroupName: "mygroup",
		Version:   3,
	}
	tm.UpdateManifest(oldManifest)
	if tm.Manifest().Version != 5 {
		t.Errorf("expected version still 5, got %d", tm.Manifest().Version)
	}
}
