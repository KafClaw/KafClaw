// Package approval provides interactive approval gates for high-risk tool calls.
package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/kamir/gomikrobot/internal/timeline"
)

// ApprovalRequest represents a pending approval for a tool call.
type ApprovalRequest struct {
	ApprovalID string         `json:"approval_id"`
	Tool       string         `json:"tool"`
	Tier       int            `json:"tier"`
	Arguments  map[string]any `json:"arguments"`
	Sender     string         `json:"sender"`
	Channel    string         `json:"channel"`
	TraceID    string         `json:"trace_id"`
	TaskID     string         `json:"task_id"`
	Status     string         `json:"status"` // pending, approved, denied, timeout
	CreatedAt  time.Time      `json:"created_at"`
}

// Manager handles approval lifecycle: create, wait, respond.
type Manager struct {
	mu       sync.Mutex
	pending  map[string]chan bool
	timeline *timeline.TimelineService
}

// NewManager creates an approval manager. Timeline may be nil.
// On creation, any stale pending approvals in the DB are marked as timeout.
func NewManager(tl *timeline.TimelineService) *Manager {
	m := &Manager{
		pending:  make(map[string]chan bool),
		timeline: tl,
	}
	m.cleanupStale()
	return m
}

// cleanupStale marks any DB-pending approvals as timeout on startup.
// These are leftovers from a previous process that never resolved them.
func (m *Manager) cleanupStale() {
	if m.timeline == nil {
		return
	}
	pending, err := m.timeline.GetPendingApprovals()
	if err != nil {
		return
	}
	for _, r := range pending {
		_ = m.timeline.UpdateApprovalStatus(r.ApprovalID, "timeout")
	}
}

// Create registers a new approval request and returns its ID.
func (m *Manager) Create(req *ApprovalRequest) string {
	id := newApprovalID()
	req.ApprovalID = id
	req.Status = "pending"
	req.CreatedAt = time.Now()

	ch := make(chan bool, 1)
	m.mu.Lock()
	m.pending[id] = ch
	m.mu.Unlock()

	// Persist to timeline (best-effort)
	if m.timeline != nil {
		argsJSON, _ := json.Marshal(req.Arguments)
		_ = m.timeline.InsertApprovalRequest(
			id, req.TraceID, req.TaskID,
			req.Tool, req.Tier, string(argsJSON),
			req.Sender, req.Channel,
		)
	}

	return id
}

// Wait blocks until the approval is responded to or the context expires.
func (m *Manager) Wait(ctx context.Context, id string) (bool, error) {
	m.mu.Lock()
	ch, ok := m.pending[id]
	m.mu.Unlock()
	if !ok {
		return false, fmt.Errorf("no pending approval: %s", id)
	}

	select {
	case approved := <-ch:
		m.cleanup(id)
		status := "denied"
		if approved {
			status = "approved"
		}
		if m.timeline != nil {
			_ = m.timeline.UpdateApprovalStatus(id, status)
		}
		return approved, nil
	case <-ctx.Done():
		m.cleanup(id)
		if m.timeline != nil {
			_ = m.timeline.UpdateApprovalStatus(id, "timeout")
		}
		return false, ctx.Err()
	}
}

// Respond delivers an approval decision for a pending request.
func (m *Manager) Respond(id string, approved bool) error {
	m.mu.Lock()
	ch, ok := m.pending[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending approval: %s", id)
	}

	// Non-blocking send (channel is buffered with size 1)
	select {
	case ch <- approved:
	default:
	}
	return nil
}

func (m *Manager) cleanup(id string) {
	m.mu.Lock()
	delete(m.pending, id)
	m.mu.Unlock()
}

func newApprovalID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("appr-%d", time.Now().UnixNano())
}
