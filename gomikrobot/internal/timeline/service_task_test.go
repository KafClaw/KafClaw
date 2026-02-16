package timeline

import (
	"testing"
)

func TestCreateAndGetTask(t *testing.T) {
	svc := newTestTimeline(t)

	task, err := svc.CreateTask(&AgentTask{
		Channel:        "whatsapp",
		ChatID:         "123@s.whatsapp.net",
		SenderID:       "user1",
		ContentIn:      "hello",
		IdempotencyKey: "wa:msg001",
		TraceID:        "trace-001",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.TaskID == "" {
		t.Fatal("expected generated task_id")
	}
	if task.Status != TaskStatusPending {
		t.Fatalf("expected pending status, got %s", task.Status)
	}
	if task.DeliveryStatus != DeliveryPending {
		t.Fatalf("expected pending delivery, got %s", task.DeliveryStatus)
	}

	got, err := svc.GetTask(task.TaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Channel != "whatsapp" || got.ContentIn != "hello" {
		t.Fatalf("unexpected task data: %+v", got)
	}
}

func TestGetTaskByIdempotencyKey(t *testing.T) {
	svc := newTestTimeline(t)

	// Not found returns nil, nil
	got, err := svc.GetTaskByIdempotencyKey("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent key")
	}

	// Empty key returns nil, nil
	got, err = svc.GetTaskByIdempotencyKey("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for empty key")
	}

	// Create and find
	_, err = svc.CreateTask(&AgentTask{
		Channel:        "webui",
		ChatID:         "1",
		IdempotencyKey: "web:trace123",
		ContentIn:      "test",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err = svc.GetTaskByIdempotencyKey("web:trace123")
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	if got == nil {
		t.Fatal("expected task for existing key")
	}
	if got.Channel != "webui" {
		t.Fatalf("unexpected channel: %s", got.Channel)
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	svc := newTestTimeline(t)

	task, err := svc.CreateTask(&AgentTask{
		Channel:  "cli",
		ChatID:   "default",
		ContentIn: "hi",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Processing
	if err := svc.UpdateTaskStatus(task.TaskID, TaskStatusProcessing, "", ""); err != nil {
		t.Fatalf("update to processing: %v", err)
	}
	got, _ := svc.GetTask(task.TaskID)
	if got.Status != TaskStatusProcessing {
		t.Fatalf("expected processing, got %s", got.Status)
	}
	if got.CompletedAt != nil {
		t.Fatal("completed_at should be nil for processing")
	}

	// Completed
	if err := svc.UpdateTaskStatus(task.TaskID, TaskStatusCompleted, "response text", ""); err != nil {
		t.Fatalf("update to completed: %v", err)
	}
	got, _ = svc.GetTask(task.TaskID)
	if got.Status != TaskStatusCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.ContentOut != "response text" {
		t.Fatalf("expected content_out, got %s", got.ContentOut)
	}
	if got.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

func TestUpdateTaskDelivery(t *testing.T) {
	svc := newTestTimeline(t)

	task, _ := svc.CreateTask(&AgentTask{
		Channel: "whatsapp",
		ChatID:  "123@s.whatsapp.net",
	})

	if err := svc.UpdateTaskDelivery(task.TaskID, DeliverySent, nil); err != nil {
		t.Fatalf("update delivery: %v", err)
	}
	got, _ := svc.GetTask(task.TaskID)
	if got.DeliveryStatus != DeliverySent {
		t.Fatalf("expected sent, got %s", got.DeliveryStatus)
	}
	if got.DeliveryAttempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", got.DeliveryAttempts)
	}
}

func TestListPendingDeliveries(t *testing.T) {
	svc := newTestTimeline(t)

	// Create a completed task with pending delivery
	task, _ := svc.CreateTask(&AgentTask{
		Channel:  "webui",
		ChatID:   "1",
		ContentIn: "test",
	})
	_ = svc.UpdateTaskStatus(task.TaskID, TaskStatusCompleted, "done", "")

	// Create a processing task (should not appear)
	task2, _ := svc.CreateTask(&AgentTask{
		Channel:  "cli",
		ChatID:   "default",
		ContentIn: "test2",
	})
	_ = svc.UpdateTaskStatus(task2.TaskID, TaskStatusProcessing, "", "")

	pending, err := svc.ListPendingDeliveries(10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending delivery, got %d", len(pending))
	}
	if pending[0].TaskID != task.TaskID {
		t.Fatalf("unexpected task in pending list")
	}
}

func TestListTasks(t *testing.T) {
	svc := newTestTimeline(t)

	_, _ = svc.CreateTask(&AgentTask{Channel: "whatsapp", ChatID: "a", ContentIn: "1"})
	_, _ = svc.CreateTask(&AgentTask{Channel: "webui", ChatID: "b", ContentIn: "2"})
	_, _ = svc.CreateTask(&AgentTask{Channel: "whatsapp", ChatID: "c", ContentIn: "3"})

	// All
	all, err := svc.ListTasks("", "", 50, 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(all))
	}

	// Filter by channel
	wa, err := svc.ListTasks("", "whatsapp", 50, 0)
	if err != nil {
		t.Fatalf("list whatsapp: %v", err)
	}
	if len(wa) != 2 {
		t.Fatalf("expected 2 whatsapp tasks, got %d", len(wa))
	}
}
