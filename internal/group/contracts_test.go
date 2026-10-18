package group

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"github.com/segmentio/kafka-go"
)

type recordedMemory struct {
	source string
	err    error
}

func (r *recordedMemory) Store(_ context.Context, content, source, tags string) (string, error) {
	r.source = source
	return "id", r.err
}

func TestManagerTaskAndMemoryContracts(t *testing.T) {
	var mu sync.Mutex
	var sent []GroupEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env GroupEnvelope
		_ = json.NewDecoder(r.Body).Decode(&env)
		mu.Lock()
		sent = append(sent, env)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"kfs_lfs":1,"bucket":"fixture","key":"memory"}`))
	}))
	defer server.Close()
	tl, err := timeline.NewTimelineService(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	m := newTestManagerWithTimeline(server.URL, tl)
	ctx := context.Background()
	calls := []func() error{func() error { return m.SubmitTask(ctx, "task", "test", "payload") }, func() error { return m.RespondTask(ctx, "task", "done", "completed") }, func() error {
		return m.SubmitDelegatedTask(ctx, DelegatedTaskRequest{TaskID: "delegated", ParentTaskID: "task"})
	}, func() error { return m.ReportTaskStatus(ctx, "task", "accepted", "working") }, func() error { return m.RegisterSkill(ctx, "lookup", nil) }, func() error { return m.SubmitSkillTask(ctx, "skill-task", "lookup", "test", "payload") }, func() error { return m.RespondSkillTask(ctx, "skill-task", "lookup", "done", "completed") }, func() error { return m.EnsureTopic(ctx, "group.test-group.extra") }, func() error { return m.ShareContext(ctx, "context", []byte("fixture"), time.Minute) }}
	for _, call := range calls {
		if err := call(); err == nil {
			t.Fatal("inactive group operation accepted")
		}
	}
	if err := m.PublishTrace(ctx, TracePayload{}); err != nil {
		t.Fatal(err)
	}
	if err := m.PublishAudit(ctx, "test", "trace", "detail"); err != nil {
		t.Fatal(err)
	}
	if err := m.Join(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Leave(ctx)
	for _, call := range calls {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.PublishTrace(ctx, TracePayload{TraceID: "trace", Content: "content"}); err != nil {
		t.Fatal(err)
	}
	if err := m.PublishAudit(ctx, "test", "trace", "detail"); err != nil {
		t.Fatal(err)
	}
	if err := m.EnsureTopic(ctx, "unrelated.topic"); err == nil {
		t.Fatal("foreign topic accepted")
	}
	if err := m.SubmitDelegatedTask(ctx, DelegatedTaskRequest{DelegationDepth: 3}); err == nil {
		t.Fatal("depth limit not enforced")
	}
	deadline := time.Now().Add(time.Minute)
	if err := m.SubmitDelegatedTask(ctx, DelegatedTaskRequest{TaskID: "delegated-explicit", OriginalRequesterID: "origin", DeadlineAt: &deadline}); err != nil {
		t.Fatal(err)
	}
	if err := m.ShareMemory(ctx, "shared", "text/plain", []byte("content"), []string{"ops"}); err != nil {
		t.Fatal(err)
	}
	tasks, err := tl.ListGroupTasks("outgoing", "pending", 10, 0)
	if err != nil || len(tasks) < 2 {
		t.Fatalf("delegated persistence: %+v %v", tasks, err)
	}
	items, err := tl.ListGroupMemoryItems("", 10, 0)
	if err != nil || len(items) != 1 || items[0].LFSBucket != "fixture" {
		t.Fatalf("memory persistence: %+v %v", items, err)
	}
	idx := &recordedMemory{}
	m.SetMemoryIndexer(idx)
	m.HandleMemoryItem(&GroupEnvelope{Payload: MemoryItem{ItemID: "received", AuthorID: "peer", Title: "received note", Tags: []string{"ops"}, LFSEnvelope: &LFSEnvelope{Bucket: "fixture", Key: "memory"}}})
	if idx.source != "group:peer:received" {
		t.Fatalf("index source: %s", idx.source)
	}
	idx.err = errors.New("store failed")
	m.HandleMemoryItem(&GroupEnvelope{Payload: MemoryItem{ItemID: "failed", Title: "failed note"}})
	for _, bad := range []any{make(chan int), "invalid"} {
		m.HandleMemoryItem(&GroupEnvelope{Payload: bad})
	}
	m.sendHeartbeat(ctx)
	if m.Config().GroupName != "test-group" || m.AgentID() != "test-agent" || !m.LFSHealthy() || m.TopicManager() == nil || m.ExtendedTopicNames().ControlRoster == "" {
		t.Fatal("manager accessors")
	}
	mu.Lock()
	n := len(sent)
	mu.Unlock()
	if n < 15 {
		t.Fatalf("missing produced envelopes: %d", n)
	}
	server.Close()

	if err := m.RespondTask(ctx, "task", "error", "failed"); err == nil {
		t.Fatal("publish failure hidden")
	}
	if err := m.ShareContext(ctx, "context", nil, time.Minute); err == nil {
		t.Fatal("context publish failure hidden")
	}
}

func TestRouterEnvelopeContracts(t *testing.T) {
	tl, err := timeline.NewTimelineService(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	m := newTestManagerWithTimeline("http://127.0.0.1:1", tl)
	b := bus.NewMessageBus()
	r := NewGroupRouter(m, b, NewChannelConsumer())
	calls := 0
	r.SetOrchestratorHandler(func(*GroupEnvelope) { calls++ })
	route := func(topic string, payload any) {
		t.Helper()
		data, err := json.Marshal(GroupEnvelope{SenderID: "peer", CorrelationID: "trace", Timestamp: time.Now(), Payload: payload})
		if err != nil {
			t.Fatal(err)
		}
		r.handleMessage(ConsumerMessage{Topic: topic, Value: data})
	}
	route(r.extTopics.Orchestrator, map[string]string{"action": "discover"})
	if calls != 1 {
		t.Fatal("discovery callback missing")
	}
	route(r.topics.Responses, TaskResponsePayload{TaskID: "task", ResponderID: "peer", Content: "done", Status: "completed"})
	msg, err := b.ConsumeInbound(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if msg.ChatID != "task" || !strings.Contains(msg.Content, "done") {
		t.Fatalf("task response: %+v", msg)
	}
	for _, suffix := range []string{"requests", "responses"} {
		topic := r.skillPrefix + "lookup." + suffix
		var payload any = TaskRequestPayload{TaskID: "skill", RequesterID: "peer", Content: "request"}
		if suffix == "responses" {
			payload = TaskResponsePayload{TaskID: "skill", ResponderID: "peer", Content: "response"}
		}
		route(topic, payload)
		msg, err := b.ConsumeInbound(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if msg.ChatID != "skill" || !strings.Contains(msg.IdempotencyKey, "lookup") {
			t.Fatalf("skill message: %+v", msg)
		}
	}
	route(r.extTopics.TaskStatus, TaskStatusPayload{TaskID: "task", Status: "accepted"})
	statusMsg, err := b.ConsumeInbound(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if statusMsg.Metadata["type"] != "task_status" {
		t.Fatal("task status metadata lost")
	}
	route(r.topics.Traces, TracePayload{TraceID: "trace", Title: "span", StartedAt: time.Now().Format(time.RFC3339), EndedAt: time.Now().Format(time.RFC3339)})
	route(r.extTopics.ObserveAudit, map[string]string{"event": "test"})
	traces, err := tl.ListAllGroupTraces(10, 0, "")
	if err != nil || len(traces) != 2 {
		t.Fatalf("traces: %+v %v", traces, err)
	}
	route(r.extTopics.ControlRoster, TopicManifest{Version: 3})
	if m.topicMgr.Manifest().Version != 3 {
		t.Fatal("manifest not updated")
	}
	route(r.extTopics.MemoryShared, MemoryItem{ItemID: "shared", Title: "title"})
	route(r.extTopics.MemoryContext, MemoryItem{ItemID: "context", Title: "context"})
	route(r.skillPrefix+"malformed", nil)
	route(r.skillPrefix+"lookup.unknown", nil)
	route("unknown", nil)
	r.handleMessage(ConsumerMessage{Value: []byte("invalid")})
	for _, bad := range []any{make(chan int), "invalid"} {
		env := &GroupEnvelope{Payload: bad}
		r.handleTaskRequest(env)
		r.handleTaskResponse(env)
		r.handleTrace(env)
		r.handleRoster(env)
		r.handleAudit(env)
		r.handleSkillMessage(r.skillPrefix+"lookup.requests", env)
		r.handleSkillMessage(r.skillPrefix+"lookup.responses", env)
	}
}

func TestKafkaConsumerSubscribeAfterStart(t *testing.T) {
	c := NewKafkaConsumerWithDialer("127.0.0.1:1", "test", nil, &kafka.Dialer{Timeout: time.Millisecond})
	if err := c.Subscribe("before-start"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- c.Subscribe("after-start") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Subscribe deadlocked after Start")
	}
	if err := c.Subscribe("after-start"); err != nil {
		t.Fatal(err)
	}
	if len(c.readers) != 2 || len(c.topics) != 2 || c.Messages() == nil {
		t.Fatal("duplicate subscription or missing reader")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOnboardingChallengeResponses(t *testing.T) {
	var sent []GroupEnvelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env GroupEnvelope
		json.NewDecoder(r.Body).Decode(&env)
		sent = append(sent, env)
		w.Write([]byte(`{"kfs_lfs":1}`))
	}))
	defer server.Close()
	m := newTestManager(server.URL)
	env := &GroupEnvelope{CorrelationID: "onboard"}
	m.handleOnboardChallenge(env, &OnboardPayload{RequesterID: "other"})
	m.handleOnboardResponse(env, &OnboardPayload{SponsorID: "other"})
	if len(sent) != 0 {
		t.Fatal("responded to unrelated onboarding")
	}
	m.handleOnboardChallenge(env, &OnboardPayload{RequesterID: m.AgentID(), SponsorID: "sponsor", Challenge: "introduce yourself"})
	if len(sent) != 1 {
		t.Fatal("challenge response not produced")
	}
	m.handleOnboardResponse(env, &OnboardPayload{SponsorID: m.AgentID(), RequesterID: "peer"})
	m.handleOnboardResponse(env, &OnboardPayload{SponsorID: m.AgentID(), RequesterID: "peer", Response: "ready"})
	if len(sent) != 3 {
		t.Fatal("accept/reject not produced")
	}
}
