package timeline

import (
	"testing"
	"time"
)

func TestTimelineCoreMethodsCoverage(t *testing.T) {
	svc := newTestTimeline(t)

	evt := &TimelineEvent{
		EventID:        "e1",
		TraceID:        "trace-core",
		SpanID:         "s1",
		ParentSpanID:   "p1",
		Timestamp:      time.Now().UTC(),
		SenderID:       "u1",
		SenderName:     "User",
		EventType:      "TEXT",
		ContentText:    "hello",
		Classification: "INBOUND",
		Authorized:     true,
	}
	if err := svc.AddEvent(evt); err != nil {
		t.Fatalf("add event: %v", err)
	}

	events, err := svc.GetEvents(FilterArgs{TraceID: "trace-core", SenderID: "u1", Limit: 20})
	if err != nil || len(events) == 0 {
		t.Fatalf("get events failed: len=%d err=%v", len(events), err)
	}

	if err := svc.SetSetting("silent_mode", "true"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if !svc.IsSilentMode() {
		t.Fatal("expected silent mode true")
	}
	if got, err := svc.GetSetting("silent_mode"); err != nil || got != "true" {
		t.Fatalf("unexpected setting: %q %v", got, err)
	}

	user, err := svc.CreateWebUser("bob")
	if err != nil {
		t.Fatalf("create web user: %v", err)
	}
	if _, err := svc.GetWebUser(user.ID); err != nil {
		t.Fatalf("get web user: %v", err)
	}
	if _, err := svc.GetWebUserByName("bob"); err != nil {
		t.Fatalf("get web user by name: %v", err)
	}
	if err := svc.SetWebUserForceSend(user.ID, false); err != nil {
		t.Fatalf("set force send: %v", err)
	}
	users, err := svc.ListWebUsers()
	if err != nil || len(users) != 1 {
		t.Fatalf("list web users failed: len=%d err=%v", len(users), err)
	}

	task, err := svc.CreateTask(&AgentTask{
		TraceID:  "trace-core",
		Channel:  "whatsapp",
		ChatID:   "u1@s.whatsapp.net",
		SenderID: "u1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := svc.UpdateTaskTokens(task.TaskID, 10, 20, 30); err != nil {
		t.Fatalf("update task tokens: %v", err)
	}
	usage, err := svc.GetDailyTokenUsage()
	if err != nil || usage < 30 {
		t.Fatalf("unexpected daily token usage: %d err=%v", usage, err)
	}
	if _, err := svc.GetTaskByTraceID("trace-core"); err != nil {
		t.Fatalf("get task by trace: %v", err)
	}

	if err := svc.LogPolicyDecision(&PolicyDecisionRecord{
		TraceID: "trace-core",
		TaskID:  task.TaskID,
		Tool:    "shell",
		Tier:    2,
		Sender:  "u1",
		Channel: "whatsapp",
		Allowed: true,
		Reason:  "ok",
	}); err != nil {
		t.Fatalf("log policy decision: %v", err)
	}
	decisions, err := svc.ListPolicyDecisions("trace-core")
	if err != nil || len(decisions) == 0 {
		t.Fatalf("list policy decisions failed: len=%d err=%v", len(decisions), err)
	}
}

func TestTimelineGroupAndAuditCoverage(t *testing.T) {
	svc := newTestTimeline(t)
	now := time.Now().UTC()

	if err := svc.UpsertGroupMember(&GroupMemberRecord{
		AgentID:      "a1",
		AgentName:    "Alpha",
		SoulSummary:  "helper",
		Capabilities: "[\"code\"]",
		Channels:     "[\"whatsapp\"]",
		Model:        "gpt-4o",
		Status:       "active",
	}); err != nil {
		t.Fatalf("upsert member: %v", err)
	}
	members, err := svc.ListGroupMembers()
	if err != nil || len(members) != 1 {
		t.Fatalf("list members failed: len=%d err=%v", len(members), err)
	}

	if err := svc.SoftDeleteGroupMember("a1"); err != nil {
		t.Fatalf("soft delete member: %v", err)
	}
	prev, err := svc.ListPreviousGroupMembers()
	if err != nil || len(prev) == 0 {
		t.Fatalf("list previous members failed: len=%d err=%v", len(prev), err)
	}
	if err := svc.ReactivateGroupMember("a1"); err != nil {
		t.Fatalf("reactivate member: %v", err)
	}
	if _, err := svc.db.Exec(`UPDATE group_members SET last_seen = datetime('now','-10 days') WHERE agent_id = 'a1'`); err != nil {
		t.Fatalf("set stale last_seen: %v", err)
	}
	if _, err := svc.MarkStaleMembers(time.Now().Add(-48 * time.Hour)); err != nil {
		t.Fatalf("mark stale members: %v", err)
	}
	if err := svc.RemoveGroupMember("a1"); err != nil {
		t.Fatalf("remove member: %v", err)
	}

	if err := svc.LogMembershipHistory(&GroupMembershipHistoryRecord{
		AgentID:       "a2",
		GroupName:     "core",
		Role:          "worker",
		Action:        "joined",
		LFSProxyURL:   "https://lfs",
		KafkaBrokers:  "kafka:9092",
		ConsumerGroup: "cg1",
		AgentName:     "Beta",
		Capabilities:  "[\"x\"]",
		Channels:      "[\"c\"]",
		Model:         "gpt",
	}); err != nil {
		t.Fatalf("log membership history: %v", err)
	}
	hist, err := svc.GetMembershipHistory("a2", "core", 20, 0)
	if err != nil || len(hist) == 0 {
		t.Fatalf("get membership history failed: len=%d err=%v", len(hist), err)
	}
	if _, err := svc.GetLatestMembershipConfig("a2", "core"); err != nil {
		t.Fatalf("get latest membership config: %v", err)
	}

	if err := svc.InsertGroupTask(&GroupTaskRecord{
		TaskID:      "gt1",
		Description: "desc",
		Content:     "content",
		Direction:   "outgoing",
		RequesterID: "a2",
		Status:      "pending",
	}); err != nil {
		t.Fatalf("insert group task: %v", err)
	}
	if err := svc.UpdateGroupTaskResponse("gt1", "a3", "done", "completed"); err != nil {
		t.Fatalf("update group task response: %v", err)
	}
	if _, err := svc.ListGroupTasks("outgoing", "completed", 20, 0); err != nil {
		t.Fatalf("list group tasks: %v", err)
	}

	if err := svc.AddGroupTrace(&GroupTrace{
		TraceID:       "trace-audit",
		SourceAgentID: "a2",
		SpanID:        "sp-1",
		ParentSpanID:  "",
		SpanType:      "TOOL",
		Title:         "tool run",
		Content:       "ok",
		StartedAt:     &now,
		EndedAt:       &now,
		DurationMs:    5,
	}); err != nil {
		t.Fatalf("add group trace: %v", err)
	}
	if _, err := svc.GetGroupTraces("trace-audit"); err != nil {
		t.Fatalf("get group traces: %v", err)
	}
	if _, err := svc.ListAllGroupTraces(20, 0, "a2"); err != nil {
		t.Fatalf("list all group traces: %v", err)
	}

	if err := svc.LogDelegationEvent("gt1", "submitted", "a2", "a3", "summary", 1); err != nil {
		t.Fatalf("log delegation event: %v", err)
	}
	if _, err := svc.ListDelegationEvents("gt1"); err != nil {
		t.Fatalf("list delegation events: %v", err)
	}

	if err := svc.InsertApprovalRequest("ap-1", "trace-audit", "gt1", "shell", 2, "{}", "a2", "whatsapp"); err != nil {
		t.Fatalf("insert approval request: %v", err)
	}
	if err := svc.UpdateApprovalStatus("ap-1", "approved"); err != nil {
		t.Fatalf("update approval status: %v", err)
	}
	if _, err := svc.GetApprovalsByTraceID("trace-audit"); err != nil {
		t.Fatalf("get approvals by trace: %v", err)
	}

	if err := svc.AddEvent(&TimelineEvent{EventID: "mode-1", Timestamp: now, SenderID: "a2", Classification: "MODE_CHANGE", ContentText: "auto"}); err != nil {
		t.Fatalf("add mode event: %v", err)
	}
	if _, err := svc.ListUnifiedAudit(AuditFilter{Limit: 50}); err != nil {
		t.Fatalf("list unified audit: %v", err)
	}

	if _, err := svc.GetGroupStats(); err != nil {
		t.Fatalf("get group stats: %v", err)
	}
}

func TestTimelineDelegationTopicAndAnalyticsCoverage(t *testing.T) {
	svc := newTestTimeline(t)
	past := time.Now().Add(-2 * time.Hour)
	future := time.Now().Add(2 * time.Hour)

	if err := svc.InsertDelegatedGroupTask(&GroupTaskRecord{
		TaskID:              "root-task",
		Description:         "root",
		Content:             "c",
		Direction:           "outgoing",
		RequesterID:         "r1",
		Status:              "pending",
		OriginalRequesterID: "r1",
		DeadlineAt:          &past,
	}); err != nil {
		t.Fatalf("insert root delegated task: %v", err)
	}
	if err := svc.InsertDelegatedGroupTask(&GroupTaskRecord{
		TaskID:              "child-task",
		Description:         "child",
		Content:             "c2",
		Direction:           "outgoing",
		RequesterID:         "r1",
		Status:              "pending",
		ParentTaskID:        "root-task",
		DelegationDepth:     1,
		OriginalRequesterID: "r1",
		DeadlineAt:          &future,
	}); err != nil {
		t.Fatalf("insert child delegated task: %v", err)
	}
	if err := svc.AcceptGroupTask("child-task", "worker-1"); err != nil {
		t.Fatalf("accept group task: %v", err)
	}
	if _, err := svc.ListExpiredGroupTasks(); err != nil {
		t.Fatalf("list expired group tasks: %v", err)
	}
	chain, err := svc.GetDelegationChain("root-task")
	if err != nil || len(chain) == 0 {
		t.Fatalf("get delegation chain failed: len=%d err=%v", len(chain), err)
	}

	if err := svc.UpsertGroupMember(&GroupMemberRecord{AgentID: "worker-1", AgentName: "Worker", Capabilities: "[]", Channels: "[]", Status: "active"}); err != nil {
		t.Fatalf("upsert worker: %v", err)
	}
	if err := svc.UpsertGroupMember(&GroupMemberRecord{AgentID: "worker-2", AgentName: "Worker2", Capabilities: "[]", Channels: "[]", Status: "active"}); err != nil {
		t.Fatalf("upsert worker2: %v", err)
	}

	if err := svc.InsertGroupMemoryItem(&GroupMemoryItemRecord{
		ItemID:      "mem-1",
		AuthorID:    "worker-1",
		Title:       "note",
		ContentType: "text/plain",
		Tags:        "[\"a\"]",
		Metadata:    "{}",
	}); err != nil {
		t.Fatalf("insert group memory item: %v", err)
	}
	if _, err := svc.ListGroupMemoryItems("worker-1", 20, 0); err != nil {
		t.Fatalf("list memory items: %v", err)
	}
	if _, err := svc.GetGroupMemoryItem("mem-1"); err != nil {
		t.Fatalf("get memory item: %v", err)
	}

	if err := svc.InsertGroupSkillChannel(&GroupSkillChannelRecord{
		SkillName:      "sql",
		GroupName:      "core",
		RequestsTopic:  "core.skill.sql.req",
		ResponsesTopic: "core.skill.sql.resp",
		RegisteredBy:   "worker-1",
	}); err != nil {
		t.Fatalf("insert skill channel: %v", err)
	}
	if _, err := svc.ListGroupSkillChannels("core"); err != nil {
		t.Fatalf("list skill channels: %v", err)
	}

	if err := svc.UpsertScheduledJob("daily-sync", "ok", time.Now()); err != nil {
		t.Fatalf("upsert scheduled job 1: %v", err)
	}
	if err := svc.UpsertScheduledJob("daily-sync", "ok", time.Now()); err != nil {
		t.Fatalf("upsert scheduled job 2: %v", err)
	}
	if _, err := svc.GetScheduledJob("daily-sync"); err != nil {
		t.Fatalf("get scheduled job: %v", err)
	}
	if _, err := svc.ListScheduledJobs(); err != nil {
		t.Fatalf("list scheduled jobs: %v", err)
	}

	_ = svc.LogTopicMessage(&TopicMessageLogRecord{TopicName: "team.tasks", SenderID: "worker-1", EnvelopeType: "request", CorrelationID: "c1", PayloadSize: 128})
	_ = svc.LogTopicMessage(&TopicMessageLogRecord{TopicName: "team.tasks", SenderID: "worker-2", EnvelopeType: "response", CorrelationID: "c1", PayloadSize: 256})
	_ = svc.LogTopicMessage(&TopicMessageLogRecord{TopicName: "team.audit", SenderID: "worker-1", EnvelopeType: "audit", CorrelationID: "c2", PayloadSize: 64})

	if _, err := svc.GetTopicStats(); err != nil {
		t.Fatalf("get topic stats: %v", err)
	}
	if _, err := svc.GetTopicFlowData(); err != nil {
		t.Fatalf("get topic flow data: %v", err)
	}
	if _, err := svc.GetAgentXP(); err != nil {
		t.Fatalf("get agent xp: %v", err)
	}
	if _, err := svc.GetTopicHealth(); err != nil {
		t.Fatalf("get topic health: %v", err)
	}
	if _, err := svc.GetTopicMessages("team.tasks", 10); err != nil {
		t.Fatalf("get topic messages: %v", err)
	}
	if _, err := svc.GetTopicMessageDensity("team.tasks", 6); err != nil {
		t.Fatalf("get topic message density: %v", err)
	}
	if _, err := svc.GetTopicEnvelopeTypeCounts("team.tasks"); err != nil {
		t.Fatalf("get topic envelope counts: %v", err)
	}

	if sqrtInt(100) != 10 || sqrtInt(-1) != 0 {
		t.Fatal("sqrtInt unexpected output")
	}
	if !contains("hello world", "world") || contains("abc", "zzz") {
		t.Fatal("contains helper mismatch")
	}
	if !containsStr("hello", "ell") {
		t.Fatal("containsStr helper mismatch")
	}
}

func TestTraceGraphCoverage(t *testing.T) {
	svc := newTestTimeline(t)
	now := time.Now().UTC()
	traceID := "trace-graph"

	if err := svc.AddEvent(&TimelineEvent{
		EventID:        "tg-e1",
		TraceID:        traceID,
		SpanID:         "span-root",
		Timestamp:      now,
		SenderID:       "u1",
		SenderName:     "User",
		Classification: "INBOUND",
	}); err != nil {
		t.Fatalf("add trace graph event: %v", err)
	}
	_ = svc.AddGroupTrace(&GroupTrace{TraceID: traceID, SourceAgentID: "a1", SpanID: "span-child", ParentSpanID: "span-root", SpanType: "TOOL", Title: "tool", DurationMs: 7})
	if err := svc.InsertApprovalRequest("ap-graph", traceID, "", "shell", 2, "{}", "u1", "whatsapp"); err != nil {
		t.Fatalf("insert graph approval: %v", err)
	}
	_ = svc.LogPolicyDecision(&PolicyDecisionRecord{TraceID: traceID, Tool: "shell", Tier: 2, Allowed: true, Reason: "ok"})
	task, _ := svc.CreateTask(&AgentTask{TraceID: traceID, Channel: "whatsapp", ChatID: "u1"})
	_ = svc.LogDelegationEvent(task.TaskID, "submitted", "a1", "a2", "handoff", 1)

	graph, err := svc.GetTraceGraph(traceID)
	if err != nil {
		t.Fatalf("get trace graph: %v", err)
	}
	if graph == nil || len(graph.Nodes) == 0 {
		t.Fatalf("expected non-empty trace graph: %+v", graph)
	}
}
