package approval

import (
	"context"
	"testing"
	"time"
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
