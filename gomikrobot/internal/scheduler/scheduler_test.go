package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kamir/gomikrobot/internal/bus"
)

func TestSchedulerDispatch(t *testing.T) {
	b := bus.NewMessageBus()
	s := New(Config{
		Enabled:        true,
		TickInterval:   50 * time.Millisecond,
		MaxConcLLM:     3,
		MaxConcShell:   1,
		MaxConcDefault: 5,
		LockPath:       t.TempDir() + "/test.lock",
	}, b, nil)

	cron, _ := ParseCron("* * * * *")
	s.Register(&Job{
		Name:     "test-job",
		Cron:     cron,
		Category: CategoryDefault,
		Content:  "scheduled test message",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Consume the inbound message in a goroutine.
	var received atomic.Int32
	go func() {
		for {
			msg, err := b.ConsumeInbound(ctx)
			if err != nil {
				return
			}
			if msg.Channel == "scheduler" {
				received.Add(1)
			}
		}
	}()

	// Manually tick to trigger dispatch.
	now := time.Now()
	s.tick(ctx, now)

	// Wait for the async dispatch.
	time.Sleep(100 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("expected 1 dispatched message, got %d", received.Load())
	}
}

func TestSchedulerLockPreventsOverlap(t *testing.T) {
	lockPath := t.TempDir() + "/overlap.lock"

	b := bus.NewMessageBus()
	s1 := New(Config{
		Enabled:        true,
		TickInterval:   50 * time.Millisecond,
		MaxConcDefault: 5,
		LockPath:       lockPath,
	}, b, nil)

	s2 := New(Config{
		Enabled:        true,
		TickInterval:   50 * time.Millisecond,
		MaxConcDefault: 5,
		LockPath:       lockPath,
	}, b, nil)

	cron, _ := ParseCron("* * * * *")
	s1.Register(&Job{Name: "overlap-1", Cron: cron, Category: CategoryDefault, Content: "msg1"})
	s2.Register(&Job{Name: "overlap-2", Cron: cron, Category: CategoryDefault, Content: "msg2"})

	ctx := context.Background()

	// Acquire the lock on the first scheduler's behalf.
	acquired, err := s1.lock.TryLock()
	if err != nil || !acquired {
		t.Fatal("s1 should acquire lock")
	}

	// s2 should not be able to acquire.
	acquired2, err := s2.lock.TryLock()
	if err != nil {
		t.Fatal("unexpected error on s2 lock:", err)
	}
	if acquired2 {
		t.Error("s2 should NOT acquire lock while s1 holds it")
		s2.lock.Unlock()
	}

	s1.lock.Unlock()

	// Now s2 should acquire.
	acquired3, err := s2.lock.TryLock()
	if err != nil {
		t.Fatal("unexpected error on s2 retry:", err)
	}
	if !acquired3 {
		t.Error("s2 should acquire lock after s1 released")
	}
	s2.lock.Unlock()

	_ = ctx // used implicitly
}

func TestSemaphoreConcurrencyLimit(t *testing.T) {
	sem := NewSemaphore(2)

	if !sem.TryAcquire() {
		t.Error("first acquire should succeed")
	}
	if !sem.TryAcquire() {
		t.Error("second acquire should succeed")
	}
	if sem.TryAcquire() {
		t.Error("third acquire should fail (cap=2)")
	}
	if sem.Available() != 0 {
		t.Errorf("Available() = %d, want 0", sem.Available())
	}

	sem.Release()
	if sem.Available() != 1 {
		t.Errorf("Available() = %d, want 1", sem.Available())
	}
	if !sem.TryAcquire() {
		t.Error("acquire after release should succeed")
	}
}

func TestSchedulerNonMatchingJobNotDispatched(t *testing.T) {
	b := bus.NewMessageBus()
	s := New(Config{
		Enabled:        true,
		TickInterval:   50 * time.Millisecond,
		MaxConcDefault: 5,
		LockPath:       t.TempDir() + "/test.lock",
	}, b, nil)

	// Job that only runs at midnight.
	cron, _ := ParseCron("0 0 * * *")
	s.Register(&Job{Name: "midnight-only", Cron: cron, Category: CategoryDefault, Content: "midnight"})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var received atomic.Int32
	go func() {
		for {
			msg, err := b.ConsumeInbound(ctx)
			if err != nil {
				return
			}
			if msg.Channel == "scheduler" {
				received.Add(1)
			}
		}
	}()

	// Tick at noon — should NOT dispatch.
	noon := time.Date(2026, 2, 15, 12, 30, 0, 0, time.UTC)
	s.tick(ctx, noon)

	time.Sleep(100 * time.Millisecond)

	if received.Load() != 0 {
		t.Errorf("expected 0 dispatched messages at noon, got %d", received.Load())
	}
}
