package timeline

import (
	"testing"
	"time"
)

func TestQueriesReportClosedDatabase(t *testing.T) {
	svc := newTestTimeline(t)
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	queries := map[string]func() error{
		"events":              func() error { _, e := svc.GetEvents(FilterArgs{}); return e },
		"users":               func() error { _, e := svc.ListWebUsers(); return e },
		"user":                func() error { _, e := svc.GetWebUser(1); return e },
		"user-name":           func() error { _, e := svc.GetWebUserByName("fixture"); return e },
		"link":                func() error { _, _, e := svc.GetWebLink(1); return e },
		"task":                func() error { _, e := svc.GetTask("fixture"); return e },
		"task-key":            func() error { _, e := svc.GetTaskByIdempotencyKey("fixture"); return e },
		"task-trace":          func() error { _, e := svc.GetTaskByTraceID("fixture"); return e },
		"tasks":               func() error { _, e := svc.ListTasks("pending", "cli", 0, 0); return e },
		"deliveries":          func() error { _, e := svc.ListPendingDeliveries(0); return e },
		"delivery-count":      func() error { _, e := svc.CountPendingDeliveries(); return e },
		"open-tasks":          func() error { _, e := svc.CountOpenTasks(); return e },
		"daily-tokens":        func() error { _, e := svc.GetDailyTokenUsage(); return e },
		"provider-tokens":     func() error { _, e := svc.GetDailyTokenUsageByProvider(); return e },
		"provider-cost":       func() error { _, e := svc.GetDailyCostByProvider(); return e },
		"token-summary":       func() error { _, e := svc.GetTokenUsageSummary(0); return e },
		"policy":              func() error { _, e := svc.ListPolicyDecisions("fixture"); return e },
		"trace-graph":         func() error { _, e := svc.GetTraceGraph("fixture"); return e },
		"group-traces":        func() error { _, e := svc.GetGroupTraces("fixture"); return e },
		"members":             func() error { _, e := svc.ListGroupMembers(); return e },
		"previous-members":    func() error { _, e := svc.ListPreviousGroupMembers(); return e },
		"membership-history":  func() error { _, e := svc.GetMembershipHistory("fixture", "group", 0, 0); return e },
		"membership-config":   func() error { _, e := svc.GetLatestMembershipConfig("fixture", "group"); return e },
		"group-tasks":         func() error { _, e := svc.ListGroupTasks("outgoing", "pending", 0, 0); return e },
		"open-group-tasks":    func() error { _, e := svc.CountOpenGroupTasks(); return e },
		"all-group-traces":    func() error { _, e := svc.ListAllGroupTraces(0, 0, "fixture"); return e },
		"memory-items":        func() error { _, e := svc.ListGroupMemoryItems("fixture", 0, 0); return e },
		"memory-item":         func() error { _, e := svc.GetGroupMemoryItem("fixture"); return e },
		"skill-channels":      func() error { _, e := svc.ListGroupSkillChannels("fixture"); return e },
		"approvals":           func() error { _, e := svc.GetPendingApprovals(); return e },
		"trace-approvals":     func() error { _, e := svc.GetApprovalsByTraceID("fixture"); return e },
		"scheduled-job":       func() error { _, e := svc.GetScheduledJob("fixture"); return e },
		"scheduled-jobs":      func() error { _, e := svc.ListScheduledJobs(); return e },
		"delegation-events":   func() error { _, e := svc.ListDelegationEvents("fixture"); return e },
		"expired-tasks":       func() error { _, e := svc.ListExpiredGroupTasks(); return e },
		"delegation-chain":    func() error { _, e := svc.GetDelegationChain("fixture"); return e },
		"topic-stats":         func() error { _, e := svc.GetTopicStats(); return e },
		"topic-flow":          func() error { _, e := svc.GetTopicFlowData(); return e },
		"agent-xp":            func() error { _, e := svc.GetAgentXP(); return e },
		"topic-health":        func() error { _, e := svc.GetTopicHealth(); return e },
		"topic-messages":      func() error { _, e := svc.GetTopicMessages("fixture", 0); return e },
		"topic-density":       func() error { _, e := svc.GetTopicMessageDensity("fixture", 0); return e },
		"topic-envelopes":     func() error { _, e := svc.GetTopicEnvelopeTypeCounts("fixture"); return e },
		"knowledge-facts":     func() error { _, e := svc.ListKnowledgeFacts("fixture", 0, 0); return e },
		"knowledge-proposals": func() error { _, e := svc.ListKnowledgeProposals("pending", 0, 0); return e },
		"knowledge-votes":     func() error { _, e := svc.ListKnowledgeVotes("fixture"); return e },
		"knowledge-fact":      func() error { _, e := svc.GetKnowledgeFactLatest("fixture"); return e },
		"knowledge-proposal":  func() error { _, e := svc.GetKnowledgeProposal("fixture"); return e },
		"cascade-task":        func() error { _, e := svc.GetCascadeTask("trace", "fixture"); return e },
		"cascade-tasks":       func() error { _, e := svc.ListCascadeTasks("trace"); return e },
		"cascade-transitions": func() error { _, e := svc.ListCascadeTransitions("trace", "fixture", 0); return e },
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			if err := query(); err == nil {
				t.Fatal("query silently succeeded against closed database")
			}
		})
	}
}

func TestCreationAndTransactionsReportClosedDatabase(t *testing.T) {
	svc := newTestTimeline(t)
	svc.Close()
	for name, call := range map[string]func() error{
		"create-user": func() error { _, e := svc.CreateWebUser("fixture"); return e },
		"create-task": func() error {
			_, e := svc.CreateTask(&AgentTask{TaskID: "fixture", Channel: "cli", ChatID: "test"})
			return e
		},
		"cascade-create": func() error { return svc.CreateCascadeTask(&CascadeTaskRecord{TraceID: "trace", TaskID: "fixture"}) },
		"cascade-advance": func() error {
			_, e := svc.AdvanceCascadeTask("trace", "fixture", "pending", "running", "test", "reason", "", "key")
			return e
		},
		"can-start": func() error { _, _, e := svc.CanStartCascadeTask("trace", "fixture"); return e },
		"record-idempotency": func() error {
			_, e := svc.RecordKnowledgeIdempotency("key", "claw", "instance", "fact", "topic", "trace")
			return e
		},
		"stale-members": func() error { _, e := svc.MarkStaleMembers(time.Now()); return e },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("mutation silently succeeded against closed database")
			}
		})
	}
}
