package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/policy"
	"github.com/KafClaw/KafClaw/internal/provider"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"github.com/KafClaw/KafClaw/internal/tools"
)

type slowProvider struct{}

func (p *slowProvider) Chat(ctx context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(2 * time.Second):
		return &provider.ChatResponse{Content: "late", Usage: provider.Usage{TotalTokens: 1}}, nil
	}
}
func (p *slowProvider) Transcribe(_ context.Context, _ *provider.AudioRequest) (*provider.AudioResponse, error) {
	return &provider.AudioResponse{Text: ""}, nil
}
func (p *slowProvider) Speak(_ context.Context, _ *provider.TTSRequest) (*provider.TTSResponse, error) {
	return &provider.TTSResponse{}, nil
}
func (p *slowProvider) DefaultModel() string { return "slow-model" }

type staticPolicy struct {
	decision policy.Decision
}

func (s *staticPolicy) Evaluate(_ policy.Context) policy.Decision {
	return s.decision
}

func TestLoopCurrentSessionKey(t *testing.T) {
	loop := NewLoop(LoopOptions{Workspace: t.TempDir(), WorkRepo: t.TempDir()})
	if got := loop.currentSessionKey(); got != "cli:default" {
		t.Fatalf("expected default session key, got %s", got)
	}

	loop.activeChannel = "whatsapp"
	loop.activeChatID = "abc123"
	if got := loop.currentSessionKey(); got != "whatsapp:abc123" {
		t.Fatalf("expected whatsapp:abc123, got %s", got)
	}
}

func TestLoopSubagentPolicy(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		MaxSubagentSpawnDepth: 1,
	})
	loop.activeChannel = "subagent"
	loop.activeChatID = "child"
	loop.subagents.sessionDepth["subagent:child"] = 1
	p := loop.subagentPolicy()

	denied := p.Evaluate(policy.Context{Tool: "sessions_spawn", Tier: tools.TierWrite, TraceID: "trace-1"})
	if denied.Allow {
		t.Fatal("expected sessions_spawn to be denied at max depth in child policy")
	}
	if denied.Reason != "subagent_spawn_depth_limit" {
		t.Fatalf("unexpected deny reason: %s", denied.Reason)
	}

	loop.policy = &staticPolicy{
		decision: policy.Decision{Allow: true, Reason: "base_ok", Tier: tools.TierReadOnly},
	}
	allowed := loop.subagentPolicy().Evaluate(policy.Context{Tool: "read_file", Tier: tools.TierReadOnly})
	if !allowed.Allow || allowed.Reason != "base_ok" {
		t.Fatalf("expected base policy decision passthrough, got %+v", allowed)
	}
}

func TestLoopSubagentPolicy_AllowDenyLists(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Workspace:          t.TempDir(),
		WorkRepo:           t.TempDir(),
		SubagentToolsAllow: []string{"read_*", "subagents"},
		SubagentToolsDeny:  []string{"exec"},
	})
	loop.policy = &staticPolicy{decision: policy.Decision{Allow: true, Reason: "base_ok"}}

	read := loop.subagentPolicy().Evaluate(policy.Context{Tool: "read_file", Tier: tools.TierReadOnly})
	if !read.Allow {
		t.Fatalf("expected read_file allowed, got %+v", read)
	}

	exec := loop.subagentPolicy().Evaluate(policy.Context{Tool: "exec", Tier: tools.TierHighRisk})
	if exec.Allow || exec.Reason != "subagent_tool_denied_by_policy" {
		t.Fatalf("expected exec denied by policy, got %+v", exec)
	}

	write := loop.subagentPolicy().Evaluate(policy.Context{Tool: "write_file", Tier: tools.TierWrite})
	if write.Allow || write.Reason != "subagent_tool_denied_by_policy" {
		t.Fatalf("expected allowlist miss denied, got %+v", write)
	}
}

func TestLoopSubagentListAndKill(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		MaxSubagentSpawnDepth: 2,
		MaxSubagentChildren:   3,
	})
	loop.activeChannel = "cli"
	loop.activeChatID = "default"

	run := loop.subagents.register("cli:default", "cli:default", "", "", "", "work", "worker", "", "", "", "keep", 1, func() {})
	loop.subagents.markRunning(run.RunID)

	list := loop.listSubagentsForTool()
	if len(list) != 1 {
		t.Fatalf("expected 1 run, got %d", len(list))
	}
	if list[0].RunID != run.RunID || list[0].Status != "running" {
		t.Fatalf("unexpected list entry: %+v", list[0])
	}

	killed, err := loop.killSubagentForTool("  " + run.RunID + "  ")
	if err != nil {
		t.Fatalf("kill err: %v", err)
	}
	if !killed {
		t.Fatal("expected killed=true")
	}
}

func TestLoopSpawnSubagentFromTool_Success(t *testing.T) {
	msgBus := bus.NewMessageBus()
	outbound := make(chan *bus.OutboundMessage, 8)
	msgBus.Subscribe("whatsapp", func(msg *bus.OutboundMessage) {
		outbound <- msg
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = msgBus.DispatchOutbound(ctx) }()

	mock := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "child done", Usage: provider.Usage{TotalTokens: 10}},
		},
	}

	loop := NewLoop(LoopOptions{
		Bus:                   msgBus,
		Provider:              mock,
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		Model:                 "mock-model",
		MaxIterations:         2,
		MaxSubagentSpawnDepth: 1,
		MaxSubagentChildren:   2,
	})
	loop.activeChannel = "whatsapp"
	loop.activeChatID = "owner@s.whatsapp.net"
	loop.activeTraceID = "trace-parent"

	res, err := loop.spawnSubagentFromTool(context.Background(), tools.SpawnRequest{
		Task:  "say hello",
		Label: "worker-1",
	})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}
	if res.Status != "accepted" || res.RunID == "" || res.ChildSessionKey == "" {
		t.Fatalf("unexpected spawn result: %+v", res)
	}

	var gotCompleted bool
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		list := loop.listSubagentsForTool()
		if len(list) == 1 && list[0].Status == "completed" {
			gotCompleted = true
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !gotCompleted {
		t.Fatal("timed out waiting for subagent completion")
	}

	select {
	case msg := <-outbound:
		if !strings.Contains(msg.Content, "[subagent "+res.RunID+"]") {
			t.Fatalf("unexpected outbound content: %s", msg.Content)
		}
		if !strings.Contains(msg.Content, "Status: completed") {
			t.Fatalf("expected normalized status in outbound content: %s", msg.Content)
		}
		if !strings.Contains(msg.Content, "Result: child done") {
			t.Fatalf("expected child response in outbound content: %s", msg.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected subagent completion outbound message")
	}
}

func TestLoopSpawnSubagentFromTool_DepthDenied(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		MaxSubagentSpawnDepth: 1,
		MaxSubagentChildren:   2,
	})
	loop.activeChannel = "subagent"
	loop.activeChatID = "child-1"
	loop.subagents.sessionDepth["subagent:child-1"] = 1

	_, err := loop.spawnSubagentFromTool(context.Background(), tools.SpawnRequest{Task: "nested"})
	if err == nil {
		t.Fatal("expected depth deny error")
	}
	if !strings.Contains(err.Error(), "not allowed at this depth") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoopSpawnSubagentFromTool_AgentAllowlist(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Provider:              &mockProvider{},
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		Model:                 "mock-model",
		MaxIterations:         1,
		MaxSubagentSpawnDepth: 1,
		MaxSubagentChildren:   2,
		AgentID:               "agent-main",
		SubagentAllowAgents:   []string{"agent-main", "agent-research"},
	})
	loop.activeChannel = "cli"
	loop.activeChatID = "default"

	if _, err := loop.spawnSubagentFromTool(context.Background(), tools.SpawnRequest{
		Task:    "allowed",
		AgentID: "agent-research",
	}); err != nil {
		t.Fatalf("expected allowed agentId, got err: %v", err)
	}

	if _, err := loop.spawnSubagentFromTool(context.Background(), tools.SpawnRequest{
		Task:    "denied",
		AgentID: "agent-other",
	}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected not allowed error, got: %v", err)
	}
}

func TestLoopSpawnSubagentFromTool_DefaultAllowCurrentOnly(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Provider:              &mockProvider{},
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		Model:                 "mock-model",
		MaxIterations:         1,
		MaxSubagentSpawnDepth: 1,
		MaxSubagentChildren:   2,
		AgentID:               "agent-main",
	})
	loop.activeChannel = "cli"
	loop.activeChatID = "default"

	if _, err := loop.spawnSubagentFromTool(context.Background(), tools.SpawnRequest{
		Task:    "same",
		AgentID: "agent-main",
	}); err != nil {
		t.Fatalf("expected current agent allowed, got err: %v", err)
	}
	if _, err := loop.spawnSubagentFromTool(context.Background(), tools.SpawnRequest{
		Task:    "other",
		AgentID: "agent-other",
	}); err == nil || !strings.Contains(err.Error(), "default allows only current agent") {
		t.Fatalf("expected current-only deny, got: %v", err)
	}
}

func TestLoopSteerSubagentForTool(t *testing.T) {
	mock := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "first run", Usage: provider.Usage{TotalTokens: 10}},
			{Content: "steered run", Usage: provider.Usage{TotalTokens: 10}},
		},
	}
	loop := NewLoop(LoopOptions{
		Provider:              mock,
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		Model:                 "mock-model",
		MaxIterations:         2,
		MaxSubagentSpawnDepth: 1,
		MaxSubagentChildren:   3,
	})
	loop.activeChannel = "cli"
	loop.activeChatID = "default"

	first, err := loop.spawnSubagentFromTool(context.Background(), tools.SpawnRequest{Task: "base task", Label: "worker"})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		list := loop.listSubagentsForTool()
		if len(list) > 0 && list[0].RunID == first.RunID && list[0].Status == "completed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	steered, err := loop.steerSubagentForTool(first.RunID, "do this differently")
	if err != nil {
		t.Fatalf("steer err: %v", err)
	}
	if steered.RunID == "" || steered.RunID == first.RunID {
		t.Fatalf("expected new run id, got %+v", steered)
	}
	if !strings.Contains(steered.Message, "steered from") {
		t.Fatalf("unexpected steer message: %s", steered.Message)
	}
}

func TestLoopSubagentAuditEvents(t *testing.T) {
	tl := newTestTimeline(t)
	mock := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "ok", Usage: provider.Usage{TotalTokens: 10}},
			{Content: "ok2", Usage: provider.Usage{TotalTokens: 10}},
		},
	}
	loop := NewLoop(LoopOptions{
		Provider:              mock,
		Timeline:              tl,
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		Model:                 "mock-model",
		MaxIterations:         2,
		MaxSubagentSpawnDepth: 1,
		MaxSubagentChildren:   3,
	})
	loop.activeChannel = "cli"
	loop.activeChatID = "default"
	loop.activeTraceID = "trace-subagent-audit"

	spawned, err := loop.spawnSubagentFromTool(context.Background(), tools.SpawnRequest{Task: "task", Label: "l1"})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}
	if _, err := loop.steerSubagentForTool(spawned.RunID, "retask"); err != nil {
		t.Fatalf("steer err: %v", err)
	}

	events, err := tl.GetEvents(timeline.FilterArgs{TraceID: "trace-subagent-audit", Limit: 50})
	if err != nil {
		t.Fatalf("get events err: %v", err)
	}
	text := ""
	for _, evt := range events {
		text += evt.ContentText + "\n"
	}
	if !strings.Contains(text, "subagent spawn_accepted") {
		t.Fatalf("expected spawn audit event, got:\n%s", text)
	}
	if !strings.Contains(text, "subagent steer") {
		t.Fatalf("expected steer audit event, got:\n%s", text)
	}
	if !strings.Contains(text, "subagent kill") {
		t.Fatalf("expected kill audit event, got:\n%s", text)
	}
}

func TestLoopSteerSubagentForTool_Validation(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		MaxSubagentSpawnDepth: 1,
		MaxSubagentChildren:   2,
	})
	loop.activeChannel = "cli"
	loop.activeChatID = "default"

	if _, err := loop.steerSubagentForTool("", "x"); err == nil {
		t.Fatal("expected empty target validation error")
	}
	if _, err := loop.steerSubagentForTool("run-1", ""); err == nil {
		t.Fatal("expected empty input validation error")
	}
	if _, err := loop.steerSubagentForTool("missing", "x"); err == nil || !strings.Contains(err.Error(), "unknown subagent run") {
		t.Fatalf("unexpected missing run error: %v", err)
	}

}

func TestLoopSpawnSubagentFromTool_Timeout(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Provider:              &slowProvider{},
		Workspace:             t.TempDir(),
		WorkRepo:              t.TempDir(),
		Model:                 "slow-model",
		MaxIterations:         2,
		MaxSubagentSpawnDepth: 1,
		MaxSubagentChildren:   2,
	})
	loop.activeChannel = "cli"
	loop.activeChatID = "default"

	spawned, err := loop.spawnSubagentFromTool(context.Background(), tools.SpawnRequest{
		Task:              "timeout me",
		RunTimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("spawn err: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		list := loop.listSubagentsForTool()
		for _, run := range list {
			if run.RunID == spawned.RunID && run.Status == "timeout" {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for timeout status")
}

func TestNormalizeSubagentAnnounceText(t *testing.T) {
	fallback := subagentAnnounceFields{Status: "completed", Result: "fallback", Notes: "notes"}
	got, skip, complete := normalizeSubagentAnnounceText("Status: done\nResult: all good\nNotes: n/a", fallback)
	if skip {
		t.Fatal("unexpected skip")
	}
	if !complete {
		t.Fatal("expected complete=true")
	}
	if got.Status != "done" || got.Result != "all good" || got.Notes != "n/a" {
		t.Fatalf("unexpected normalized fields: %+v", got)
	}

	_, skip, complete = normalizeSubagentAnnounceText("ANNOUNCE_SKIP", fallback)
	if !skip || !complete {
		t.Fatalf("expected skip complete, got skip=%v complete=%v", skip, complete)
	}
}

func TestLoopBuildSubagentAnnounce_Skip(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Workspace: t.TempDir(),
		WorkRepo:  t.TempDir(),
		Model:     "mock-model",
	})
	run := &subagentRun{RunID: "run-1", Label: "worker"}
	content, skip := loop.buildSubagentAnnounce(context.Background(), run, "completed", "ANNOUNCE_SKIP", nil)
	if !skip {
		t.Fatal("expected skip=true")
	}
	if content != "" {
		t.Fatalf("expected empty content when skipped, got %q", content)
	}
}

func TestLoopListSubagentAgentsForTool(t *testing.T) {
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir err: %v", err)
	}
	cfg := `{"agents":{"list":[{"id":"agent-main","name":"Main Agent"},{"id":"agent-research","name":"Research Agent"}]}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config err: %v", err)
	}
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("set HOME err: %v", err)
	}
	defer os.Setenv("HOME", origHome)

	loop := NewLoop(LoopOptions{
		Workspace:           t.TempDir(),
		WorkRepo:            t.TempDir(),
		AgentID:             "agent-main",
		SubagentAllowAgents: []string{"agent-main", "agent-research", "*"},
	})
	got := loop.listSubagentAgentsForTool()
	if got.CurrentAgentID != "agent-main" {
		t.Fatalf("unexpected current agent: %+v", got)
	}
	if !got.Wildcard {
		t.Fatalf("expected wildcard=true: %+v", got)
	}
	if len(got.AllowAgents) != 3 {
		t.Fatalf("unexpected allow list: %+v", got.AllowAgents)
	}
	if len(got.Agents) == 0 {
		t.Fatalf("expected agents metadata entries, got %+v", got)
	}
	foundResearch := false
	for _, entry := range got.Agents {
		if entry.ID == "agent-research" {
			foundResearch = true
			if entry.Name != "Research Agent" || !entry.Configured {
				t.Fatalf("unexpected research agent metadata: %+v", entry)
			}
		}
	}
	if !foundResearch {
		t.Fatalf("expected agent-research metadata in agents_list response: %+v", got.Agents)
	}
}

func TestLoopPublishSubagentAnnounceWithRetry_DedupAndCleanupDelete(t *testing.T) {
	msgBus := bus.NewMessageBus()
	outbound := make(chan *bus.OutboundMessage, 8)
	msgBus.Subscribe("whatsapp", func(msg *bus.OutboundMessage) {
		outbound <- msg
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = msgBus.DispatchOutbound(ctx) }()

	ws := t.TempDir()
	loop := NewLoop(LoopOptions{
		Bus:       msgBus,
		Workspace: ws,
		WorkRepo:  ws,
		Model:     "mock-model",
	})

	sess := loop.sessions.GetOrCreate("subagent:cleanup-me")
	sess.AddMessage("assistant", "temp")
	if err := loop.sessions.Save(sess); err != nil {
		t.Fatalf("save session err: %v", err)
	}
	infos := loop.sessions.List()
	if len(infos) == 0 {
		t.Fatal("expected session list to include saved child session")
	}

	run := &subagentRun{
		RunID:           "run-dup-1",
		AnnounceID:      "ann-dup-1",
		ChildSessionKey: "subagent:cleanup-me",
		RequesterChan:   "whatsapp",
		RequesterChatID: "owner@s.whatsapp.net",
		RequesterTrace:  "trace-dup",
		Cleanup:         "delete",
	}
	first := loop.publishSubagentAnnounceWithRetry(context.Background(), run, "completed", "done", nil, "", "", "")
	second := loop.publishSubagentAnnounceWithRetry(context.Background(), run, "completed", "done", nil, "", "", "")
	if !first || !second {
		t.Fatalf("expected publish calls to report success")
	}
	select {
	case <-outbound:
	case <-time.After(2 * time.Second):
		t.Fatal("expected first outbound announce")
	}
	select {
	case extra := <-outbound:
		t.Fatalf("expected deduped second announce, got unexpected outbound: %+v", extra)
	case <-time.After(400 * time.Millisecond):
	}
	infos = loop.sessions.List()
	for _, info := range infos {
		if info.Key == "subagent:cleanup-me" {
			t.Fatalf("expected cleanup delete to remove child session, still found in list: %+v", info)
		}
	}
}

func TestLoopStartSubagentRetryWorker_Continuous(t *testing.T) {
	orig := subagentRetryInterval
	subagentRetryInterval = 100 * time.Millisecond
	defer func() { subagentRetryInterval = orig }()

	msgBus := bus.NewMessageBus()
	outbound := make(chan *bus.OutboundMessage, 8)
	msgBus.Subscribe("whatsapp", func(msg *bus.OutboundMessage) {
		outbound <- msg
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = msgBus.DispatchOutbound(ctx) }()

	loop := NewLoop(LoopOptions{
		Bus:       msgBus,
		Workspace: t.TempDir(),
		WorkRepo:  t.TempDir(),
		Model:     "mock-model",
	})
	run := loop.subagents.register(
		"cli:default",
		"cli:default",
		"whatsapp",
		"owner@s.whatsapp.net",
		"trace-retry",
		"task",
		"w1",
		"",
		"",
		"",
		"keep",
		1,
		func() {},
	)
	loop.subagents.markRunning(run.RunID)
	loop.subagents.markCompletionOutput(run.RunID, "ready")
	loop.subagents.markFinished(run.RunID, "completed", nil)

	loop.startSubagentRetryWorker(ctx)

	select {
	case msg := <-outbound:
		if !strings.Contains(msg.Content, "Status: completed") {
			t.Fatalf("unexpected retry announce content: %s", msg.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected retry worker to deliver pending announce")
	}
}

func TestLoopStartSubagentRetryWorker_DeferredCleanupDelete(t *testing.T) {
	orig := subagentRetryInterval
	subagentRetryInterval = 100 * time.Millisecond
	defer func() { subagentRetryInterval = orig }()

	msgBus := bus.NewMessageBus()
	outbound := make(chan *bus.OutboundMessage, 8)
	msgBus.Subscribe("whatsapp", func(msg *bus.OutboundMessage) {
		outbound <- msg
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = msgBus.DispatchOutbound(ctx) }()

	ws := t.TempDir()
	loop := NewLoop(LoopOptions{
		Bus:       msgBus,
		Workspace: ws,
		WorkRepo:  ws,
		Model:     "mock-model",
	})

	run := loop.subagents.register(
		"cli:default",
		"cli:default",
		"whatsapp",
		"owner@s.whatsapp.net",
		"trace-retry-cleanup",
		"task",
		"w1",
		"",
		"",
		"",
		"delete",
		1,
		func() {},
	)
	if run.Cleanup != "delete" {
		t.Fatalf("expected cleanup=delete on registered run, got %q", run.Cleanup)
	}
	sess := loop.sessions.GetOrCreate(run.ChildSessionKey)
	sess.AddMessage("assistant", "tmp")
	if err := loop.sessions.Save(sess); err != nil {
		t.Fatalf("save session err: %v", err)
	}
	loop.subagents.markRunning(run.RunID)
	loop.subagents.markCompletionOutput(run.RunID, "ready")
	loop.subagents.markFinished(run.RunID, "completed", nil)

	loop.startSubagentRetryWorker(ctx)

	select {
	case <-outbound:
	case <-time.After(2 * time.Second):
		t.Fatal("expected retry worker announce")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		found := false
		infos := loop.sessions.List()
		for _, info := range infos {
			if info.Key == run.ChildSessionKey {
				found = true
				break
			}
		}
		if !found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected deferred cleanup delete to remove child session")
		}
		time.Sleep(30 * time.Millisecond)
	}
}

func TestLoopResolveSubagentAnnounceRoute_Fallbacks(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Workspace: t.TempDir(),
		WorkRepo:  t.TempDir(),
	})
	loop.activeChannel = "webui"
	loop.activeChatID = "active-chat"
	loop.activeTraceID = "trace-active"

	run := &subagentRun{
		RequestedBy: "whatsapp:owner@s.whatsapp.net",
		RootSession: "cli:default",
	}
	channel, chatID, traceID, ok := loop.resolveSubagentAnnounceRoute(run, "", "", "")
	if !ok || channel != "whatsapp" || chatID != "owner@s.whatsapp.net" {
		t.Fatalf("expected requestedBy fallback route, got ok=%v channel=%q chat=%q", ok, channel, chatID)
	}
	if traceID != "trace-active" {
		t.Fatalf("expected active trace fallback, got %q", traceID)
	}

	run = &subagentRun{
		RequesterChan:   "telegram",
		RequesterChatID: "12345",
		RequesterTrace:  "trace-run",
	}
	channel, chatID, traceID, ok = loop.resolveSubagentAnnounceRoute(run, "whatsapp", "fallback", "trace-default")
	if !ok || channel != "telegram" || chatID != "12345" || traceID != "trace-run" {
		t.Fatalf("expected explicit requester route, got ok=%v channel=%q chat=%q trace=%q", ok, channel, chatID, traceID)
	}

	run = &subagentRun{}
	channel, chatID, _, ok = loop.resolveSubagentAnnounceRoute(run, "", "", "")
	if !ok || channel != "webui" || chatID != "active-chat" {
		t.Fatalf("expected active session fallback, got ok=%v channel=%q chat=%q", ok, channel, chatID)
	}
}

func TestLoopNestedAnnounceDeferredRetry_RoutesToRootRequester(t *testing.T) {
	orig := subagentRetryInterval
	subagentRetryInterval = 100 * time.Millisecond
	defer func() { subagentRetryInterval = orig }()

	loop := NewLoop(LoopOptions{
		Workspace: t.TempDir(),
		WorkRepo:  t.TempDir(),
		Model:     "mock-model",
	})

	parent := loop.subagents.register(
		"cli:default",
		"whatsapp:owner@s.whatsapp.net",
		"whatsapp",
		"owner@s.whatsapp.net",
		"trace-root",
		"parent",
		"p1",
		"",
		"",
		"",
		"keep",
		1,
		func() {},
	)
	child := loop.subagents.register(
		parent.ChildSessionKey,
		"", // inherited requester metadata from parent run
		"",
		"",
		"",
		"child",
		"c1",
		"",
		"",
		"",
		"keep",
		2,
		func() {},
	)
	loop.subagents.markRunning(child.RunID)
	loop.subagents.markCompletionOutput(child.RunID, "child-result")
	loop.subagents.markFinished(child.RunID, "completed", nil)

	// First publish fails due to missing bus, forcing deferred retry.
	if delivered := loop.publishSubagentAnnounceWithRetry(context.Background(), child, "completed", "child-result", nil, "", "", ""); delivered {
		t.Fatal("expected first publish to fail without bus")
	}
	loop.subagents.mu.Lock()
	if stored, ok := loop.subagents.runs[child.RunID]; ok {
		past := time.Now().Add(-50 * time.Millisecond)
		stored.NextAnnounceAt = &past
	}
	loop.subagents.mu.Unlock()

	msgBus := bus.NewMessageBus()
	outbound := make(chan *bus.OutboundMessage, 8)
	msgBus.Subscribe("whatsapp", func(msg *bus.OutboundMessage) {
		outbound <- msg
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = msgBus.DispatchOutbound(ctx) }()
	loop.bus = msgBus
	loop.startSubagentRetryWorker(ctx)

	select {
	case msg := <-outbound:
		if msg.Channel != "whatsapp" || msg.ChatID != "owner@s.whatsapp.net" {
			t.Fatalf("unexpected announce route channel/chat: %+v", msg)
		}
		if !strings.Contains(msg.Content, "Status: completed") || !strings.Contains(msg.Content, "Result: child-result") {
			t.Fatalf("unexpected announce content: %s", msg.Content)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected deferred nested announce to be delivered")
	}
}
