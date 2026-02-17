package approval

import (
	"context"
	"crypto/rand"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/timeline"
)

func TestApproved(t *testing.T) {
	m := NewManager(nil)
	req := &ApprovalRequest{Tool: "exec", Tier: 2}
	id := m.Create(req)

	go func() {
		time.Sleep(10 * time.Millisecond)
		if err := m.Respond(id, true); err != nil {
			t.Errorf("respond failed: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	approved, err := m.Wait(ctx, id)
	if err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	if !approved {
		t.Fatal("expected approved=true")
	}
}

func TestDenied(t *testing.T) {
	m := NewManager(nil)
	req := &ApprovalRequest{Tool: "exec", Tier: 2}
	id := m.Create(req)

	go func() {
		time.Sleep(10 * time.Millisecond)
		if err := m.Respond(id, false); err != nil {
			t.Errorf("respond failed: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	approved, err := m.Wait(ctx, id)
	if err != nil {
		t.Fatalf("wait failed: %v", err)
	}
	if approved {
		t.Fatal("expected approved=false")
	}
}

func TestTimeout(t *testing.T) {
	m := NewManager(nil)
	req := &ApprovalRequest{Tool: "exec", Tier: 2}
	id := m.Create(req)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	approved, err := m.Wait(ctx, id)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if approved {
		t.Fatal("expected approved=false on timeout")
	}
}

func TestRespondNonexistent(t *testing.T) {
	m := NewManager(nil)
	err := m.Respond("nonexistent", true)
	if err == nil {
		t.Fatal("expected error for nonexistent approval")
	}
}

func TestWaitNonexistent(t *testing.T) {
	m := NewManager(nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := m.Wait(ctx, "missing"); err == nil {
		t.Fatal("expected error for missing approval id")
	}
}

func TestCleanupStaleFromTimeline(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "timeline.db")
	tl, err := timeline.NewTimelineService(dbPath)
	if err != nil {
		t.Fatalf("new timeline: %v", err)
	}
	t.Cleanup(func() { _ = tl.Close() })

	if err := tl.InsertApprovalRequest("ap-1", "tr-1", "task-1", "exec", 2, "{}", "u1", "whatsapp"); err != nil {
		t.Fatalf("insert approval request: %v", err)
	}

	// Constructor performs stale cleanup.
	_ = NewManager(tl)

	records, err := tl.GetApprovalsByTraceID("tr-1")
	if err != nil {
		t.Fatalf("get approvals by trace: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one approval record, got %d", len(records))
	}
	if records[0].Status != "timeout" {
		t.Fatalf("expected stale pending to become timeout, got %q", records[0].Status)
	}
}

func TestCleanupStaleIgnoresTimelineErrors(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "timeline.db")
	tl, err := timeline.NewTimelineService(dbPath)
	if err != nil {
		t.Fatalf("new timeline: %v", err)
	}
	_ = tl.Close() // force DB errors for GetPendingApprovals

	m := &Manager{
		pending:  map[string]chan bool{},
		timeline: tl,
	}
	// Should not panic even if timeline operations fail.
	m.cleanupStale()
}

func TestNewApprovalIDFallbackWhenCryptoRandFails(t *testing.T) {
	orig := rand.Reader
	t.Cleanup(func() { rand.Reader = orig })
	rand.Reader = failingReader{}

	id := newApprovalID()
	if id == "" {
		t.Fatal("expected fallback approval id")
	}
}

func TestApprovalLifecyclePersistsTimelineStatuses(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "timeline.db")
	tl, err := timeline.NewTimelineService(dbPath)
	if err != nil {
		t.Fatalf("new timeline: %v", err)
	}
	t.Cleanup(func() { _ = tl.Close() })

	m := NewManager(tl)

	approvedReq := &ApprovalRequest{
		Tool:      "exec",
		Tier:      2,
		Arguments: map[string]any{"command": "pwd"},
		Sender:    "u1",
		Channel:   "whatsapp",
		TraceID:   "trace-approval",
		TaskID:    "task-approved",
	}
	approvedID := m.Create(approvedReq)
	if err := m.Respond(approvedID, true); err != nil {
		t.Fatalf("respond approved: %v", err)
	}
	ctxOK, cancelOK := context.WithTimeout(context.Background(), time.Second)
	defer cancelOK()
	ok, err := m.Wait(ctxOK, approvedID)
	if err != nil || !ok {
		t.Fatalf("wait approved: ok=%v err=%v", ok, err)
	}

	deniedReq := &ApprovalRequest{
		Tool:    "exec",
		Tier:    2,
		TraceID: "trace-approval",
		TaskID:  "task-denied",
	}
	deniedID := m.Create(deniedReq)
	if err := m.Respond(deniedID, false); err != nil {
		t.Fatalf("respond denied: %v", err)
	}
	ctxDenied, cancelDenied := context.WithTimeout(context.Background(), time.Second)
	defer cancelDenied()
	ok, err = m.Wait(ctxDenied, deniedID)
	if err != nil || ok {
		t.Fatalf("wait denied: ok=%v err=%v", ok, err)
	}

	timeoutReq := &ApprovalRequest{
		Tool:    "exec",
		Tier:    2,
		TraceID: "trace-approval",
		TaskID:  "task-timeout",
	}
	timeoutID := m.Create(timeoutReq)
	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelTimeout()
	ok, err = m.Wait(ctxTimeout, timeoutID)
	if err == nil || ok {
		t.Fatalf("wait timeout expected error: ok=%v err=%v", ok, err)
	}

	records, err := tl.GetApprovalsByTraceID("trace-approval")
	if err != nil {
		t.Fatalf("get approvals by trace: %v", err)
	}
	byTask := map[string]string{}
	for _, r := range records {
		byTask[r.TaskID] = r.Status
	}
	if byTask["task-approved"] != "approved" {
		t.Fatalf("expected approved status, got %q", byTask["task-approved"])
	}
	if byTask["task-denied"] != "denied" {
		t.Fatalf("expected denied status, got %q", byTask["task-denied"])
	}
	if byTask["task-timeout"] != "timeout" {
		t.Fatalf("expected timeout status, got %q", byTask["task-timeout"])
	}
}

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
