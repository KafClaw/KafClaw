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

func TestCountPendingDeliveriesAndOpenTasks(t *testing.T) {
	svc := newTestTimeline(t)

	task1, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "a", ContentIn: "1"})
	task2, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "b", ContentIn: "2"})
	task3, _ := svc.CreateTask(&AgentTask{Channel: "cli", ChatID: "c", ContentIn: "3"})

	_ = svc.UpdateTaskStatus(task1.TaskID, TaskStatusCompleted, "done", "")
	_ = svc.UpdateTaskStatus(task2.TaskID, TaskStatusProcessing, "", "")
	_ = svc.UpdateTaskStatus(task3.TaskID, TaskStatusFailed, "", "x")

	pendingDeliveries, err := svc.CountPendingDeliveries()
	if err != nil {
		t.Fatalf("count pending deliveries: %v", err)
	}
	if pendingDeliveries != 1 {
		t.Fatalf("expected 1 pending delivery, got %d", pendingDeliveries)
	}

	openTasks, err := svc.CountOpenTasks()
	if err != nil {
		t.Fatalf("count open tasks: %v", err)
	}
	if openTasks != 1 {
		t.Fatalf("expected 1 open task, got %d", openTasks)
	}
}

func TestCountOpenGroupTasks(t *testing.T) {
	svc := newTestTimeline(t)
	if err := svc.InsertGroupTask(&GroupTaskRecord{
		TaskID:      "g1",
		Description: "pending",
		Direction:   "incoming",
		RequesterID: "a",
		Status:      "pending",
	}); err != nil {
		t.Fatalf("insert group task 1: %v", err)
	}
	if err := svc.InsertGroupTask(&GroupTaskRecord{
		TaskID:      "g2",
		Description: "accepted",
		Direction:   "incoming",
		RequesterID: "a",
		Status:      "pending",
	}); err != nil {
		t.Fatalf("insert group task 2: %v", err)
	}
	if err := svc.AcceptGroupTask("g2", "worker-1"); err != nil {
		t.Fatalf("accept group task: %v", err)
	}
	if err := svc.InsertGroupTask(&GroupTaskRecord{
		TaskID:      "g3",
		Description: "completed",
		Direction:   "incoming",
		RequesterID: "a",
		Status:      "completed",
	}); err != nil {
		t.Fatalf("insert group task 3: %v", err)
	}

	openGroupTasks, err := svc.CountOpenGroupTasks()
	if err != nil {
		t.Fatalf("count open group tasks: %v", err)
	}
	if openGroupTasks != 2 {
		t.Fatalf("expected 2 open group tasks, got %d", openGroupTasks)
	}
}

func TestRecordKnowledgeIdempotency(t *testing.T) {
	svc := newTestTimeline(t)
	inserted, err := svc.RecordKnowledgeIdempotency("idem-1", "claw-a", "inst-a", "proposal", "group.g.knowledge.proposals", "trace-1")
	if err != nil {
		t.Fatalf("record knowledge idempotency (first): %v", err)
	}
	if !inserted {
		t.Fatal("expected first insert to be accepted")
	}

	inserted, err = svc.RecordKnowledgeIdempotency("idem-1", "claw-a", "inst-a", "proposal", "group.g.knowledge.proposals", "trace-1")
	if err != nil {
		t.Fatalf("record knowledge idempotency (duplicate): %v", err)
	}
	if inserted {
		t.Fatal("expected duplicate insert to be ignored")
	}
}

func TestKnowledgeFactLatestCRUD(t *testing.T) {
	svc := newTestTimeline(t)
	got, err := svc.GetKnowledgeFactLatest("fact-1")
	if err != nil {
		t.Fatalf("get missing fact latest: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing fact, got %+v", got)
	}

	rec := &KnowledgeFactRecord{
		FactID:     "fact-1",
		GroupName:  "g",
		Subject:    "service",
		Predicate:  "runbook",
		Object:     "v1",
		Version:    1,
		Source:     "decision:d1",
		ProposalID: "p1",
		DecisionID: "d1",
		Tags:       `["ops"]`,
	}
	if err := svc.UpsertKnowledgeFactLatest(rec); err != nil {
		t.Fatalf("upsert fact v1: %v", err)
	}
	got, err = svc.GetKnowledgeFactLatest("fact-1")
	if err != nil {
		t.Fatalf("get fact v1: %v", err)
	}
	if got == nil || got.Version != 1 || got.Object != "v1" {
		t.Fatalf("unexpected fact v1: %+v", got)
	}

	rec.Object = "v2"
	rec.Version = 2
	if err := svc.UpsertKnowledgeFactLatest(rec); err != nil {
		t.Fatalf("upsert fact v2: %v", err)
	}
	got, err = svc.GetKnowledgeFactLatest("fact-1")
	if err != nil {
		t.Fatalf("get fact v2: %v", err)
	}
	if got == nil || got.Version != 2 || got.Object != "v2" {
		t.Fatalf("unexpected fact v2: %+v", got)
	}
}

func TestKnowledgeProposalVoteCRUD(t *testing.T) {
	svc := newTestTimeline(t)
	prop := &KnowledgeProposalRecord{
		ProposalID:         "p1",
		GroupName:          "g1",
		Title:              "Runbook update",
		Statement:          "Use v2",
		Tags:               `["ops"]`,
		ProposerClawID:     "claw-a",
		ProposerInstanceID: "inst-a",
	}
	if err := svc.CreateKnowledgeProposal(prop); err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	got, err := svc.GetKnowledgeProposal("p1")
	if err != nil {
		t.Fatalf("get proposal: %v", err)
	}
	if got == nil || got.Status != "pending" {
		t.Fatalf("unexpected proposal: %+v", got)
	}
	list, err := svc.ListKnowledgeProposals("pending", 20, 0)
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 proposal, got %d", len(list))
	}

	if err := svc.UpsertKnowledgeVote(&KnowledgeVoteRecord{
		ProposalID: "p1",
		ClawID:     "claw-b",
		InstanceID: "inst-b",
		Vote:       "yes",
		Reason:     "looks good",
		TraceID:    "trace-1",
	}); err != nil {
		t.Fatalf("upsert vote: %v", err)
	}
	if err := svc.UpsertKnowledgeVote(&KnowledgeVoteRecord{
		ProposalID: "p1",
		ClawID:     "claw-b",
		InstanceID: "inst-b",
		Vote:       "no",
		Reason:     "changed",
		TraceID:    "trace-2",
	}); err != nil {
		t.Fatalf("upsert vote overwrite: %v", err)
	}
	votes, err := svc.ListKnowledgeVotes("p1")
	if err != nil {
		t.Fatalf("list votes: %v", err)
	}
	if len(votes) != 1 || votes[0].Vote != "no" {
		t.Fatalf("expected one overwritten vote=no, got %+v", votes)
	}
	if err := svc.UpdateKnowledgeProposalDecision("p1", "rejected", 1, 2, "quorum no"); err != nil {
		t.Fatalf("update proposal decision: %v", err)
	}
	got, err = svc.GetKnowledgeProposal("p1")
	if err != nil {
		t.Fatalf("get proposal after decision: %v", err)
	}
	if got == nil || got.Status != "rejected" || got.NoVotes != 2 {
		t.Fatalf("unexpected proposal after decision: %+v", got)
	}
}

func TestListAndCountKnowledgeFacts(t *testing.T) {
	svc := newTestTimeline(t)
	if err := svc.UpsertKnowledgeFactLatest(&KnowledgeFactRecord{
		FactID:    "f1",
		GroupName: "g1",
		Subject:   "svc",
		Predicate: "runbook",
		Object:    "v1",
		Version:   1,
		Source:    "decision:d1",
		Tags:      "[]",
	}); err != nil {
		t.Fatalf("upsert fact1: %v", err)
	}
	if err := svc.UpsertKnowledgeFactLatest(&KnowledgeFactRecord{
		FactID:    "f2",
		GroupName: "g2",
		Subject:   "svc",
		Predicate: "owner",
		Object:    "team-a",
		Version:   1,
		Source:    "decision:d2",
		Tags:      "[]",
	}); err != nil {
		t.Fatalf("upsert fact2: %v", err)
	}
	all, err := svc.ListKnowledgeFacts("", 20, 0)
	if err != nil {
		t.Fatalf("list all facts: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 facts, got %d", len(all))
	}
	g1, err := svc.ListKnowledgeFacts("g1", 20, 0)
	if err != nil {
		t.Fatalf("list g1 facts: %v", err)
	}
	if len(g1) != 1 || g1[0].FactID != "f1" {
		t.Fatalf("unexpected g1 facts: %+v", g1)
	}
	count, err := svc.CountKnowledgeFacts("g1")
	if err != nil {
		t.Fatalf("count g1 facts: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected g1 count 1, got %d", count)
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
