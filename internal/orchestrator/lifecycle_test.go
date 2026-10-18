package orchestrator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/group"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

func TestOrchestratorLifecycleAndPersistence(t *testing.T) {
	var envelopes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelopes++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"offset":1}`))
	}))
	defer server.Close()
	tl, err := timeline.NewTimelineService(filepath.Join(t.TempDir(), "timeline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	mgr := group.NewManager(config.GroupConfig{GroupName: "test", LFSProxyURL: server.URL}, tl, group.AgentIdentity{AgentID: "self", AgentName: "Self"})
	o := New(config.OrchestratorConfig{Role: "coordinator", ZoneID: "team", ParentID: "parent", Endpoint: "http://localhost"}, mgr, tl)
	ctx := context.Background()
	if err := o.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := o.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if envelopes != 1 {
		t.Fatalf("duplicate discovery: %d", envelopes)
	}
	status := o.Status()
	if !status.Enabled || status.AgentID != "self" || status.ParentID != "parent" || status.AgentCount != 1 || status.ZoneCount != 2 {
		t.Fatalf("status: %+v", status)
	}
	if !o.IsAllowed("team", "self") || o.IsAllowed("team", "stranger") {
		t.Fatal("zone membership mismatch")
	}
	if len(o.GetZones()) != 2 || len(o.GetAgents()) != 1 || len(o.GetHierarchy()) != 1 {
		t.Fatal("inventory mismatch")
	}
	zone := Zone{ZoneID: "private", Name: "Private", Visibility: "private", OwnerID: "self"}
	if err := o.CreateZone(zone); err != nil {
		t.Fatal(err)
	}
	if err := o.CreateZone(zone); err == nil {
		t.Fatal("duplicate accepted")
	}
	remote := AgentNode{AgentID: "remote", ParentID: "self", Role: "worker", ZoneID: "team"}
	o.HandleDiscovery(DiscoveryPayload{Node: remote, Hierarchy: []AgentNode{remote, {AgentID: "third"}}, Zones: []Zone{zone, {ZoneID: "other", Visibility: "public"}}})
	var parent, role string
	if err := tl.DB().QueryRow("SELECT parent_id,role FROM orchestrator_hierarchy WHERE agent_id='remote'").Scan(&parent, &role); err != nil {
		t.Fatal(err)
	}
	if parent != "self" || role != "worker" {
		t.Fatalf("persisted remote: %s %s", parent, role)
	}
	var name string
	if err := tl.DB().QueryRow("SELECT name FROM orchestrator_zones WHERE zone_id='private'").Scan(&name); err != nil || name != "Private" {
		t.Fatalf("zone persistence: %q %v", name, err)
	}
	if err := o.DispatchTask(ctx, "task", "description", "missing"); err == nil {
		t.Fatal("unknown zone accepted")
	}
	if err := o.DispatchTask(ctx, "task", "description", ""); err == nil {
		t.Fatal("inactive group accepted")
	}
	if err := mgr.Join(ctx); err != nil {
		t.Fatal(err)
	}
	defer mgr.Leave(ctx)
	if err := o.DispatchTask(ctx, "task", "description", ""); err != nil {
		t.Fatal(err)
	}
	if err := o.Stop(ctx); err != nil || o.running {
		t.Fatal("stop did not clear running state")
	}
	// A missing timeline remains a supported mode.
	bare := New(config.OrchestratorConfig{}, mgr, nil)
	if err := bare.CreateZone(Zone{ZoneID: "local"}); err != nil {
		t.Fatal(err)
	}
	bare.HandleDiscovery(DiscoveryPayload{Node: remote})
}

func TestDiscoveryFailureAndZoneMutation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	mgr := group.NewManager(config.GroupConfig{GroupName: "test", LFSProxyURL: server.URL}, nil, group.AgentIdentity{AgentID: "self"})
	o := New(config.OrchestratorConfig{}, mgr, nil)
	if err := o.Start(context.Background()); err != nil {
		t.Fatal("discovery failure should not prevent local startup", err)
	}
	zm := o.zones
	if err := zm.AddMember("missing", "self"); err == nil {
		t.Fatal("unknown zone accepted")
	}
	if zm.IsAllowed("missing", "self") {
		t.Fatal("unknown zone allowed")
	}
	if err := zm.CreateZone(Zone{ZoneID: "test", Visibility: "unknown"}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := zm.AddMember("test", "self"); err != nil {
			t.Fatal(err)
		}
	}
	members := zm.Members("test")
	if len(members) != 1 {
		t.Fatalf("duplicate members: %v", members)
	}
	members[0] = "mutated"
	if zm.Members("test")[0] != "self" {
		t.Fatal("members aliases internal state")
	}
	if zm.IsAllowed("test", "self") {
		t.Fatal("unknown visibility allowed")
	}
	zm.RemoveMember("test", "missing")
	zm.RemoveMember("test", "self")
	if zm.MemberCount("test") != 0 {
		t.Fatal("member not removed")
	}
	if err := zm.DeleteZone("test"); err != nil {
		t.Fatal(err)
	}
	if _, ok := zm.GetZone("test"); ok {
		t.Fatal("zone not removed")
	}
	if OrchestratorTopicName("test") != "group.test.orchestrator" {
		t.Fatal("topic name mismatch")
	}
	b, err := json.Marshal(o.Status())
	if err != nil || len(b) == 0 {
		t.Fatal("status not serializable")
	}
}
