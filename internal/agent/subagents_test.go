package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSubagentManager_DepthLimit(t *testing.T) {
	m := newSubagentManager(SubagentLimits{
		MaxSpawnDepth:       1,
		MaxChildrenPerAgent: 5,
	}, "", 0)

	if _, err := m.canSpawn("cli:default"); err != nil {
		t.Fatalf("expected root spawn to be allowed, got err: %v", err)
	}

	run := m.register("cli:default", "cli:default", "", "", "", "child task", "", "", "", "", "keep", 1, func() {})
	if run.Depth != 1 {
		t.Fatalf("expected child depth 1, got %d", run.Depth)
	}

	if _, err := m.canSpawn(run.ChildSessionKey); err == nil {
		t.Fatal("expected nested spawn to be denied at maxSpawnDepth=1")
	}
}

func TestSubagentManager_ChildLimit(t *testing.T) {
	m := newSubagentManager(SubagentLimits{
		MaxSpawnDepth:       2,
		MaxChildrenPerAgent: 1,
		MaxConcurrent:       5,
	}, "", 0)

	if _, err := m.canSpawn("cli:default"); err != nil {
		t.Fatalf("expected first spawn to be allowed, got err: %v", err)
	}
	m.register("cli:default", "cli:default", "", "", "", "one", "", "", "", "", "keep", 1, func() {})

	if _, err := m.canSpawn("cli:default"); err == nil {
		t.Fatal("expected second active child to be denied")
	}
}

func TestSubagentManager_GlobalConcurrentLimit(t *testing.T) {
	m := newSubagentManager(SubagentLimits{
		MaxSpawnDepth:       2,
		MaxChildrenPerAgent: 5,
		MaxConcurrent:       1,
	}, "", 0)

	if _, err := m.canSpawn("cli:default"); err != nil {
		t.Fatalf("expected first spawn allowed, got err: %v", err)
	}
	m.register("cli:default", "cli:default", "", "", "", "one", "", "", "", "", "keep", 1, func() {})
	if _, err := m.canSpawn("cli:second"); err == nil {
		t.Fatal("expected second concurrent spawn to be denied globally")
	}
}

func TestSubagentManager_KillAndList(t *testing.T) {
	killedCtx := false
	m := newSubagentManager(SubagentLimits{
		MaxSpawnDepth:       2,
		MaxChildrenPerAgent: 3,
	}, "", 0)

	run := m.register("cli:default", "cli:default", "", "", "", "work", "label", "", "", "", "keep", 1, func() { killedCtx = true })
	m.markRunning(run.RunID)

	list := m.listByParent("cli:default")
	if len(list) != 1 {
		t.Fatalf("expected 1 run, got %d", len(list))
	}
	if list[0].Status != "running" {
		t.Fatalf("expected running status, got %s", list[0].Status)
	}

	killed, err := m.killByRunID("cli:default", run.RunID)
	if err != nil {
		t.Fatalf("kill err: %v", err)
	}
	if !killed {
		t.Fatal("expected kill=true")
	}
	if !killedCtx {
		t.Fatal("expected cancel callback to be invoked")
	}

	list = m.listByParent("cli:default")
	if len(list) != 1 || list[0].Status != "killed" {
		t.Fatalf("expected killed run in list, got %+v", list)
	}
}

func TestSubagentManager_MarkFinished(t *testing.T) {
	m := newSubagentManager(SubagentLimits{}, "", 0)
	run := m.register("cli:default", "cli:default", "", "", "", "work", "", "", "", "", "keep", 1, func() {})
	m.markRunning(run.RunID)
	markErr := context.DeadlineExceeded
	m.markFinished(run.RunID, "failed", markErr)

	list := m.listByParent("cli:default")
	if len(list) != 1 {
		t.Fatalf("expected 1 run, got %d", len(list))
	}
	got := list[0]
	if got.Status != "failed" {
		t.Fatalf("expected failed status, got %s", got.Status)
	}
	if got.Error == "" {
		t.Fatal("expected error text to be recorded")
	}
	if got.EndedAt == nil || got.EndedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("expected recent endedAt timestamp")
	}
}

func TestSubagentManager_GetByRunIDAndCascadeKill(t *testing.T) {
	m := newSubagentManager(SubagentLimits{
		MaxSpawnDepth:       3,
		MaxChildrenPerAgent: 5,
	}, "", 0)

	parent := m.register("cli:default", "cli:default", "", "", "", "parent", "p1", "", "", "", "keep", 1, func() {})
	childKilled := false
	child := m.register(parent.ChildSessionKey, parent.ChildSessionKey, "", "", "", "child", "c1", "", "", "", "keep", 2, func() { childKilled = true })
	m.markRunning(parent.RunID)
	m.markRunning(child.RunID)

	got, err := m.getByRunID("cli:default", parent.RunID)
	if err != nil {
		t.Fatalf("getByRunID err: %v", err)
	}
	if got.RunID != parent.RunID || got.Task != "parent" {
		t.Fatalf("unexpected run returned: %+v", got)
	}

	killed, err := m.killByRunID("cli:default", parent.RunID)
	if err != nil {
		t.Fatalf("kill err: %v", err)
	}
	if !killed {
		t.Fatal("expected parent killed")
	}
	if !childKilled {
		t.Fatal("expected descendant cancel callback to be invoked")
	}

	childList := m.listByParent(parent.ChildSessionKey)
	foundChildKilled := false
	for _, run := range childList {
		if run.RunID == child.RunID && run.Status == "killed" {
			foundChildKilled = true
			break
		}
	}
	if !foundChildKilled {
		t.Fatalf("expected child status killed, got %+v", childList)
	}
}

func TestSubagentManager_ControlScopeByRootSession(t *testing.T) {
	m := newSubagentManager(SubagentLimits{
		MaxSpawnDepth:       3,
		MaxChildrenPerAgent: 5,
	}, "", 0)
	root := m.register("cli:default", "cli:default", "", "", "", "root", "r", "", "", "", "keep", 1, func() {})
	child := m.register(root.ChildSessionKey, root.ChildSessionKey, "", "", "", "child", "c", "", "", "", "keep", 2, func() {})
	m.markRunning(root.RunID)
	m.markRunning(child.RunID)

	// Child session can control sibling/root runs in the same root scope.
	if _, err := m.getByRunID(root.ChildSessionKey, root.RunID); err != nil {
		t.Fatalf("expected root run controllable from child scope, got err: %v", err)
	}

	foreign := m.register("cli:other", "cli:other", "", "", "", "other", "o", "", "", "", "keep", 1, func() {})
	if _, err := m.getByRunID(root.ChildSessionKey, foreign.RunID); err == nil {
		t.Fatal("expected cross-root scope access denied")
	}
}

func TestSubagentManager_PersistAndRestore(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "subagents", "runs.json")
	m := newSubagentManager(SubagentLimits{MaxSpawnDepth: 2, MaxChildrenPerAgent: 3}, storePath, 60)
	run := m.register("cli:default", "cli:default", "", "", "", "persist me", "p", "m", "low", "", "keep", 1, func() {})
	m.markRunning(run.RunID)
	m.markFinished(run.RunID, "completed", nil)

	restored := newSubagentManager(SubagentLimits{MaxSpawnDepth: 2, MaxChildrenPerAgent: 3}, storePath, 60)
	list := restored.listByParent("cli:default")
	if len(list) != 1 {
		t.Fatalf("expected 1 restored run, got %d", len(list))
	}
	if list[0].RunID != run.RunID || list[0].Status != "completed" {
		t.Fatalf("unexpected restored run: %+v", list[0])
	}
}

func TestSubagentManager_RestoreMarksInFlightAsFailed(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "subagents", "runs.json")
	m := newSubagentManager(SubagentLimits{MaxSpawnDepth: 2, MaxChildrenPerAgent: 3}, storePath, 60)
	run := m.register("cli:default", "cli:default", "", "", "", "in-flight", "p", "m", "low", "", "keep", 1, func() {})
	m.markRunning(run.RunID)
	// Persist without completion to simulate restart.
	m.persist()

	restored := newSubagentManager(SubagentLimits{MaxSpawnDepth: 2, MaxChildrenPerAgent: 3}, storePath, 60)
	list := restored.listByParent("cli:default")
	if len(list) != 1 {
		t.Fatalf("expected 1 restored run, got %d", len(list))
	}
	if list[0].Status != "failed" {
		t.Fatalf("expected failed after restore, got %s", list[0].Status)
	}
	if list[0].EndedAt == nil {
		t.Fatal("expected endedAt to be set on restore")
	}
}

func TestSubagentManager_ArchiveSweep(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "subagents", "runs.json")
	m := newSubagentManager(SubagentLimits{MaxSpawnDepth: 2, MaxChildrenPerAgent: 3}, storePath, 1)
	run := m.register("cli:default", "cli:default", "", "", "", "archive", "p", "m", "low", "", "keep", 1, func() {})
	m.markRunning(run.RunID)
	m.markFinished(run.RunID, "completed", nil)

	m.mu.Lock()
	if got, ok := m.runs[run.RunID]; ok {
		past := time.Now().Add(-time.Minute)
		got.ArchiveAt = &past
	}
	m.mu.Unlock()

	m.sweepExpired()
	list := m.listByParent("cli:default")
	if len(list) != 0 {
		t.Fatalf("expected run archived away, got %d", len(list))
	}
}

func TestSubagentManager_AnnounceRetryState(t *testing.T) {
	m := newSubagentManager(SubagentLimits{}, "", 60)
	run := m.register("cli:default", "cli:default", "cli", "abc", "trace-1", "work", "l1", "", "", "", "keep", 1, func() {})
	m.markRunning(run.RunID)
	m.markCompletionOutput(run.RunID, "done")
	m.markFinished(run.RunID, "completed", nil)

	pending := m.pendingAnnounceRuns(10)
	if len(pending) != 1 || pending[0].RunID != run.RunID {
		t.Fatalf("expected one pending announce run, got %+v", pending)
	}

	m.markAnnounceAttempt(run.RunID, false)
	pending = m.pendingAnnounceRuns(10)
	if len(pending) != 0 {
		t.Fatalf("expected retry backoff to hide run temporarily, got %+v", pending)
	}

	m.mu.Lock()
	if stored, ok := m.runs[run.RunID]; ok {
		past := time.Now().Add(-time.Second)
		stored.NextAnnounceAt = &past
	}
	m.mu.Unlock()
	pending = m.pendingAnnounceRuns(10)
	if len(pending) != 1 {
		t.Fatalf("expected run pending after backoff window, got %+v", pending)
	}

	m.markAnnounceAttempt(run.RunID, true)
	pending = m.pendingAnnounceRuns(10)
	if len(pending) != 0 {
		t.Fatalf("expected no pending announces after delivered, got %+v", pending)
	}
}
