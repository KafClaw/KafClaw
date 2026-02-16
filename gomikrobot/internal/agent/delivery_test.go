package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/timeline"
)

func newTestTimeline(t *testing.T) *timeline.TimelineService {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "timeline.db")
	svc, err := timeline.NewTimelineService(dbPath)
	if err != nil {
		t.Fatalf("failed to create timeline service: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.Close()
		_ = os.RemoveAll(dir)
	})
	return svc
}

func TestDeliveryWorkerPollPicksPendingTasks(t *testing.T) {
	tl := newTestTimeline(t)
	msgBus := bus.NewMessageBus()
	worker := NewDeliveryWorker(tl, msgBus)

	// Create a completed task with pending delivery
	task, err := tl.CreateTask(&timeline.AgentTask{
		Channel:  "whatsapp",
		ChatID:   "123@s.whatsapp.net",
		ContentIn: "hello",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := tl.UpdateTaskStatus(task.TaskID, timeline.TaskStatusCompleted, "response", ""); err != nil {
		t.Fatalf("update status: %v", err)
	}

	// Poll
	worker.poll()

	// Check the task was marked as sent
	got, err := tl.GetTask(task.TaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.DeliveryStatus != timeline.DeliverySent {
		t.Fatalf("expected sent delivery, got %s", got.DeliveryStatus)
	}
}

func TestDeliveryWorkerMaxRetryMarksFailed(t *testing.T) {
	tl := newTestTimeline(t)
	msgBus := bus.NewMessageBus()
	worker := NewDeliveryWorker(tl, msgBus)
	worker.maxRetry = 2

	// Create a completed task
	task, err := tl.CreateTask(&timeline.AgentTask{
		Channel:  "webui",
		ChatID:   "1",
		ContentIn: "test",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_ = tl.UpdateTaskStatus(task.TaskID, timeline.TaskStatusCompleted, "done", "")

	// Simulate delivery_attempts >= maxRetry by manually incrementing
	_ = tl.UpdateTaskDelivery(task.TaskID, timeline.DeliveryPending, nil) // attempt 1
	_ = tl.UpdateTaskDelivery(task.TaskID, timeline.DeliveryPending, nil) // attempt 2

	// Poll — should mark as failed since attempts (2) >= maxRetry (2)
	worker.poll()

	got, _ := tl.GetTask(task.TaskID)
	if got.DeliveryStatus != timeline.DeliveryFailed {
		t.Fatalf("expected failed delivery, got %s", got.DeliveryStatus)
	}
}

func TestDeliveryBackoff(t *testing.T) {
	before := time.Now()
	next := DeliveryBackoff(0)
	// 30s * 2^0 = 30s
	if next.Before(before.Add(29 * time.Second)) {
		t.Fatal("backoff(0) should be ~30s")
	}

	next = DeliveryBackoff(3)
	// 30s * 2^3 = 240s = 4min
	if next.Before(before.Add(239 * time.Second)) {
		t.Fatal("backoff(3) should be ~240s")
	}

	// Large attempt should cap at 5min
	next = DeliveryBackoff(10)
	maxDelay := 5*time.Minute + 1*time.Second
	if next.After(before.Add(maxDelay)) {
		t.Fatal("backoff should cap at 5min")
	}
}
