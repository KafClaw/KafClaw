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
		Channel:   "cli",
		ChatID:    "default",
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
		Channel:   "webui",
		ChatID:    "1",
		ContentIn: "test",
	})
	_ = svc.UpdateTaskStatus(task.TaskID, TaskStatusCompleted, "done", "")

	// Create a processing task (should not appear)
	task2, _ := svc.CreateTask(&AgentTask{
		Channel:   "cli",
		ChatID:    "default",
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

func TestUpdateTaskTokensWithProvider(t *testing.T) {
	svc := newTestTimeline(t)
	task, err := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "test", ContentIn: "hi"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := svc.UpdateTaskTokensWithProvider(task.TaskID, 100, 50, 150, "claude", "claude-sonnet-4-6"); err != nil {
		t.Fatalf("update tokens: %v", err)
	}

	got, err := svc.GetTask(task.TaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.PromptTokens != 100 || got.CompletionTokens != 50 || got.TotalTokens != 150 {
		t.Errorf("tokens mismatch: prompt=%d completion=%d total=%d", got.PromptTokens, got.CompletionTokens, got.TotalTokens)
	}

	// Second update should accumulate tokens but keep provider/model from first.
	if err := svc.UpdateTaskTokensWithProvider(task.TaskID, 200, 100, 300, "openai", "gpt-4o"); err != nil {
		t.Fatalf("update tokens 2: %v", err)
	}
	got, _ = svc.GetTask(task.TaskID)
	if got.TotalTokens != 450 {
		t.Errorf("expected accumulated tokens 450, got %d", got.TotalTokens)
	}
}

func TestUpdateTaskCost(t *testing.T) {
	svc := newTestTimeline(t)
	task, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "test", ContentIn: "cost test"})

	if err := svc.UpdateTaskCost(task.TaskID, 0.0123); err != nil {
		t.Fatalf("update cost: %v", err)
	}
	if err := svc.UpdateTaskCost(task.TaskID, 0.0050); err != nil {
		t.Fatalf("update cost 2: %v", err)
	}
	// Cost should accumulate (0.0123 + 0.005 = 0.0173)
}

func TestGetDailyTokenUsageByProvider(t *testing.T) {
	svc := newTestTimeline(t)

	t1, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "a", ContentIn: "1"})
	t2, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "b", ContentIn: "2"})
	t3, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "c", ContentIn: "3"})

	_ = svc.UpdateTaskTokensWithProvider(t1.TaskID, 100, 50, 150, "claude", "sonnet")
	_ = svc.UpdateTaskTokensWithProvider(t2.TaskID, 200, 100, 300, "claude", "sonnet")
	_ = svc.UpdateTaskTokensWithProvider(t3.TaskID, 50, 25, 75, "openai", "gpt-4o")

	byProv, err := svc.GetDailyTokenUsageByProvider()
	if err != nil {
		t.Fatalf("get by provider: %v", err)
	}
	if byProv["claude"] != 450 {
		t.Errorf("expected claude=450, got %d", byProv["claude"])
	}
	if byProv["openai"] != 75 {
		t.Errorf("expected openai=75, got %d", byProv["openai"])
	}
}

func TestGetDailyCostByProvider(t *testing.T) {
	svc := newTestTimeline(t)

	t1, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "a", ContentIn: "1"})
	t2, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "b", ContentIn: "2"})

	_ = svc.UpdateTaskTokensWithProvider(t1.TaskID, 100, 50, 150, "claude", "sonnet")
	_ = svc.UpdateTaskCost(t1.TaskID, 0.05)
	_ = svc.UpdateTaskTokensWithProvider(t2.TaskID, 200, 100, 300, "openai", "gpt-4o")
	_ = svc.UpdateTaskCost(t2.TaskID, 0.02)

	costs, err := svc.GetDailyCostByProvider()
	if err != nil {
		t.Fatalf("get cost by provider: %v", err)
	}
	if costs["claude"] < 0.049 || costs["claude"] > 0.051 {
		t.Errorf("expected claude cost ~0.05, got %f", costs["claude"])
	}
	if costs["openai"] < 0.019 || costs["openai"] > 0.021 {
		t.Errorf("expected openai cost ~0.02, got %f", costs["openai"])
	}
}

func TestGetDailyTokenUsage(t *testing.T) {
	svc := newTestTimeline(t)

	task, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "a", ContentIn: "1"})
	_ = svc.UpdateTaskTokens(task.TaskID, 100, 50, 150)

	total, err := svc.GetDailyTokenUsage()
	if err != nil {
		t.Fatalf("get daily usage: %v", err)
	}
	if total != 150 {
		t.Errorf("expected 150, got %d", total)
	}
}

func TestGetTokenUsageSummary(t *testing.T) {
	svc := newTestTimeline(t)

	t1, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "a", ContentIn: "1"})
	t2, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "b", ContentIn: "2"})

	_ = svc.UpdateTaskTokensWithProvider(t1.TaskID, 100, 50, 150, "claude", "sonnet")
	_ = svc.UpdateTaskCost(t1.TaskID, 0.01)
	_ = svc.UpdateTaskTokensWithProvider(t2.TaskID, 200, 100, 300, "openai", "gpt-4o")
	_ = svc.UpdateTaskCost(t2.TaskID, 0.03)

	summary, err := svc.GetTokenUsageSummary(1)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if len(summary) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(summary))
	}
	// Verify cost is included
	totalCost := 0.0
	for _, s := range summary {
		totalCost += s.CostUSD
	}
	if totalCost < 0.03 {
		t.Errorf("expected total cost >= 0.03, got %f", totalCost)
	}
}
