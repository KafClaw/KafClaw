package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSessionsSpawnTool_Execute(t *testing.T) {
	tool := NewSessionsSpawnTool(func(_ context.Context, req SpawnRequest) (SpawnResult, error) {
		if req.Task != "do work" {
			t.Fatalf("unexpected task: %s", req.Task)
		}
		if req.Label != "worker-a" {
			t.Fatalf("unexpected label: %s", req.Label)
		}
		if req.AgentID != "agent-b" {
			t.Fatalf("unexpected agentID: %s", req.AgentID)
		}
		if req.Model != "gpt-test" {
			t.Fatalf("unexpected model: %s", req.Model)
		}
		if req.Thinking != "high" {
			t.Fatalf("unexpected thinking: %s", req.Thinking)
		}
		if req.RunTimeoutSeconds != 45 {
			t.Fatalf("unexpected timeout seconds: %d", req.RunTimeoutSeconds)
		}
		if req.Cleanup != "delete" {
			t.Fatalf("unexpected cleanup: %s", req.Cleanup)
		}
		return SpawnResult{
			Status:          "accepted",
			RunID:           "run-1",
			ChildSessionKey: "subagent:abc",
		}, nil
	})

	out, err := tool.Execute(context.Background(), map[string]any{
		"task":              "do work",
		"label":             "worker-a",
		"agentId":           "agent-b",
		"model":             "gpt-test",
		"thinking":          "high",
		"runTimeoutSeconds": 45,
		"cleanup":           "delete",
	})
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}

	var body SpawnResult
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("json parse err: %v", err)
	}
	if body.Status != "accepted" || body.RunID != "run-1" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestSessionsSpawnTool_Execute_TimeoutAlias(t *testing.T) {
	tool := NewSessionsSpawnTool(func(_ context.Context, req SpawnRequest) (SpawnResult, error) {
		if req.RunTimeoutSeconds != 12 {
			t.Fatalf("expected timeout alias to map into runTimeoutSeconds, got %d", req.RunTimeoutSeconds)
		}
		return SpawnResult{Status: "accepted", RunID: "run-alias"}, nil
	})
	out, err := tool.Execute(context.Background(), map[string]any{
		"task":           "do work",
		"timeoutSeconds": 12,
	})
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	var body SpawnResult
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("json parse err: %v", err)
	}
	if body.RunID != "run-alias" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestSessionsSpawnTool_MetadataAndUnavailable(t *testing.T) {
	tool := NewSessionsSpawnTool(nil)
	if tool.Name() != "sessions_spawn" {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Tier() != TierWrite {
		t.Fatalf("unexpected tier: %d", tool.Tier())
	}
	if tool.Description() == "" {
		t.Fatal("expected non-empty description")
	}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Fatalf("unexpected schema: %+v", params)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"task": "x"}); err == nil {
		t.Fatal("expected unavailable error when spawn callback is nil")
	}
}

func TestSessionsSpawnTool_Execute_Validation(t *testing.T) {
	tool := NewSessionsSpawnTool(func(_ context.Context, _ SpawnRequest) (SpawnResult, error) {
		return SpawnResult{}, nil
	})
	if _, err := tool.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected validation error when task is missing")
	}
}

func TestSubagentsTool_ListAndKill(t *testing.T) {
	killedID := ""
	tool := NewSubagentsTool(
		func() []SubagentRunView {
			return []SubagentRunView{
				{
					RunID:           "run-1",
					ParentSession:   "cli:default",
					ChildSessionKey: "subagent:1",
					Task:            "work",
					Status:          "running",
					Depth:           1,
				},
			}
		},
		func(runID string) (bool, error) {
			killedID = runID
			return true, nil
		},
		nil,
	)

	listOut, err := tool.Execute(context.Background(), map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("list err: %v", err)
	}
	var listBody map[string]any
	if err := json.Unmarshal([]byte(listOut), &listBody); err != nil {
		t.Fatalf("list json err: %v", err)
	}
	if listBody["status"] != "ok" {
		t.Fatalf("unexpected list status: %+v", listBody)
	}

	killOut, err := tool.Execute(context.Background(), map[string]any{
		"action": "kill",
		"target": "run-1",
	})
	if err != nil {
		t.Fatalf("kill err: %v", err)
	}
	if killedID != "run-1" {
		t.Fatalf("expected kill callback for run-1, got %s", killedID)
	}
	var killBody map[string]any
	if err := json.Unmarshal([]byte(killOut), &killBody); err != nil {
		t.Fatalf("kill json err: %v", err)
	}
	if killBody["killed"] != true {
		t.Fatalf("unexpected kill body: %+v", killBody)
	}
}

func TestSubagentsTool_UnsupportedAction(t *testing.T) {
	tool := NewSubagentsTool(func() []SubagentRunView { return nil }, nil, nil)
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "unknown"}); err == nil {
		t.Fatal("expected unsupported action error")
	}
}

func TestSubagentsTool_MetadataAndValidation(t *testing.T) {
	tool := NewSubagentsTool(nil, nil, nil)
	if tool.Name() != "subagents" {
		t.Fatalf("unexpected name: %s", tool.Name())
	}
	if tool.Tier() != TierWrite {
		t.Fatalf("unexpected tier: %d", tool.Tier())
	}
	if tool.Description() == "" {
		t.Fatal("expected non-empty description")
	}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Fatalf("unexpected schema: %+v", params)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "list"}); err == nil {
		t.Fatal("expected list unavailable error")
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "kill"}); err == nil {
		t.Fatal("expected kill unavailable error")
	}

	tool = NewSubagentsTool(func() []SubagentRunView { return nil }, func(_ string) (bool, error) {
		return true, nil
	}, nil)
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "kill"}); err == nil {
		t.Fatal("expected target required validation error")
	}
}

func TestSubagentsTool_Steer(t *testing.T) {
	tool := NewSubagentsTool(
		func() []SubagentRunView {
			return []SubagentRunView{
				{
					RunID:     "run-1",
					Label:     "worker",
					CreatedAt: time.Now(),
				},
			}
		},
		func(_ string) (bool, error) { return true, nil },
		func(runID, input string) (SpawnResult, error) {
			if runID != "run-1" {
				t.Fatalf("unexpected runID: %s", runID)
			}
			if input != "adjust" {
				t.Fatalf("unexpected input: %s", input)
			}
			return SpawnResult{
				RunID:           "run-2",
				ChildSessionKey: "subagent:2",
				Message:         "steered",
			}, nil
		},
	)

	out, err := tool.Execute(context.Background(), map[string]any{
		"action": "steer",
		"target": "run-1",
		"input":  "adjust",
	})
	if err != nil {
		t.Fatalf("steer err: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("json parse err: %v", err)
	}
	if body["action"] != "steer" || body["targetRunId"] != "run-1" || body["newRunId"] != "run-2" {
		t.Fatalf("unexpected steer response: %+v", body)
	}
}

func TestSubagentsTool_SteerValidationAndUnavailable(t *testing.T) {
	tool := NewSubagentsTool(
		func() []SubagentRunView { return nil },
		func(_ string) (bool, error) { return true, nil },
		nil,
	)
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "steer", "target": "run-1", "input": "x"}); err == nil {
		t.Fatal("expected unavailable error for steer")
	}

	tool = NewSubagentsTool(
		func() []SubagentRunView { return nil },
		func(_ string) (bool, error) { return true, nil },
		func(_, _ string) (SpawnResult, error) { return SpawnResult{}, nil },
	)
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "steer", "input": "x"}); err == nil {
		t.Fatal("expected target required validation error")
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"action": "steer", "target": "run-1"}); err == nil {
		t.Fatal("expected input required validation error")
	}
}

func TestSubagentsTool_KillAll(t *testing.T) {
	killed := make([]string, 0)
	now := time.Now()
	ended := now
	tool := NewSubagentsTool(
		func() []SubagentRunView {
			return []SubagentRunView{
				{RunID: "run-1", CreatedAt: now.Add(-time.Minute)},
				{RunID: "run-2", CreatedAt: now, EndedAt: &ended},
			}
		},
		func(runID string) (bool, error) {
			killed = append(killed, runID)
			return true, nil
		},
		nil,
	)

	out, err := tool.Execute(context.Background(), map[string]any{"action": "kill_all"})
	if err != nil {
		t.Fatalf("kill_all err: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("json parse err: %v", err)
	}
	if len(killed) != 1 || killed[0] != "run-1" {
		t.Fatalf("unexpected killed set: %+v", killed)
	}
	if body["killed"] != float64(1) {
		t.Fatalf("unexpected kill_all response: %+v", body)
	}
}

func TestResolveSubagentTarget(t *testing.T) {
	now := time.Now()
	recentEnded := now.Add(-10 * time.Minute)
	oldEnded := now.Add(-2 * time.Hour)
	runs := []SubagentRunView{
		{RunID: "run-active", Label: "worker-a", ChildSessionKey: "subagent:aaa", CreatedAt: now.Add(-2 * time.Minute)},
		{RunID: "run-recent", Label: "worker-b", ChildSessionKey: "subagent:bbb", CreatedAt: now.Add(-3 * time.Minute), EndedAt: &recentEnded},
		{RunID: "run-old", Label: "archived", ChildSessionKey: "subagent:ccc", CreatedAt: now.Add(-4 * time.Minute), EndedAt: &oldEnded},
	}

	got, err := resolveSubagentTarget(runs, "last", 30)
	if err != nil || got.RunID != "run-active" {
		t.Fatalf("expected last->run-active, got %+v err=%v", got, err)
	}
	got, err = resolveSubagentTarget(runs, "1", 30)
	if err != nil || got.RunID != "run-active" {
		t.Fatalf("expected numeric 1->run-active, got %+v err=%v", got, err)
	}
	got, err = resolveSubagentTarget(runs, "2", 30)
	if err != nil || got.RunID != "run-recent" {
		t.Fatalf("expected numeric 2->run-recent, got %+v err=%v", got, err)
	}
	if _, err := resolveSubagentTarget(runs, "3", 30); err == nil {
		t.Fatal("expected invalid index error")
	}
	got, err = resolveSubagentTarget(runs, "subagent:bbb", 30)
	if err != nil || got.RunID != "run-recent" {
		t.Fatalf("expected child key lookup, got %+v err=%v", got, err)
	}
	got, err = resolveSubagentTarget(runs, "worker-a", 30)
	if err != nil || got.RunID != "run-active" {
		t.Fatalf("expected label lookup, got %+v err=%v", got, err)
	}
	got, err = resolveSubagentTarget(runs, "run-act", 30)
	if err != nil || got.RunID != "run-active" {
		t.Fatalf("expected run prefix lookup, got %+v err=%v", got, err)
	}
}

func TestAgentsListTool(t *testing.T) {
	tool := NewAgentsListTool(func() AgentDiscovery {
		return AgentDiscovery{
			CurrentAgentID:   "agent-main",
			AllowAgents:      []string{"agent-main", "agent-research"},
			EffectiveTargets: []string{"agent-main", "agent-research"},
			Wildcard:         false,
		}
	})
	if tool.Name() != "agents_list" {
		t.Fatalf("unexpected tool name: %s", tool.Name())
	}
	if tool.Tier() != TierReadOnly {
		t.Fatalf("unexpected tier: %d", tool.Tier())
	}
	out, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("json parse err: %v", err)
	}
	if body["action"] != "agents_list" {
		t.Fatalf("unexpected action: %+v", body)
	}
	agents, ok := body["agents"].(map[string]any)
	if !ok {
		t.Fatalf("expected agents object, got %+v", body["agents"])
	}
	if agents["currentAgentId"] != "agent-main" {
		t.Fatalf("unexpected discovery payload: %+v", agents)
	}

	unavailable := NewAgentsListTool(nil)
	if _, err := unavailable.Execute(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected unavailable error")
	}
}
