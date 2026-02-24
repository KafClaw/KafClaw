package cli

import (
	"path/filepath"
	"testing"

	"github.com/KafClaw/KafClaw/internal/timeline"
)

func TestReconcileDurableRuntimeState(t *testing.T) {
	tl, err := timeline.NewTimelineService(filepath.Join(t.TempDir(), "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	defer tl.Close()

	t1, err := tl.CreateTask(&timeline.AgentTask{Channel: "cli", ChatID: "a", ContentIn: "x"})
	if err != nil {
		t.Fatalf("create task 1: %v", err)
	}
	t2, err := tl.CreateTask(&timeline.AgentTask{Channel: "cli", ChatID: "b", ContentIn: "y"})
	if err != nil {
		t.Fatalf("create task 2: %v", err)
	}
	_ = tl.UpdateTaskStatus(t1.TaskID, timeline.TaskStatusCompleted, "done", "")
	_ = tl.UpdateTaskStatus(t2.TaskID, timeline.TaskStatusProcessing, "", "")
	if err := tl.InsertGroupTask(&timeline.GroupTaskRecord{
		TaskID:      "g-open",
		Description: "open",
		Direction:   "incoming",
		RequesterID: "a",
		Status:      "pending",
	}); err != nil {
		t.Fatalf("insert group task: %v", err)
	}

	if err := reconcileDurableRuntimeState(tl); err != nil {
		t.Fatalf("reconcile durable runtime state: %v", err)
	}

	if v, err := tl.GetSetting("runtime_reconcile_pending_deliveries"); err != nil || v != "1" {
		t.Fatalf("expected pending deliveries=1, got %q err=%v", v, err)
	}
	if v, err := tl.GetSetting("runtime_reconcile_open_tasks"); err != nil || v != "1" {
		t.Fatalf("expected open tasks=1, got %q err=%v", v, err)
	}
	if v, err := tl.GetSetting("runtime_reconcile_open_group_tasks"); err != nil || v != "1" {
		t.Fatalf("expected open group tasks=1, got %q err=%v", v, err)
	}
	if v, err := tl.GetSetting("runtime_reconcile_last_at"); err != nil || v == "" {
		t.Fatalf("expected reconcile timestamp, got %q err=%v", v, err)
	}
}
