package scheduler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

func TestSchedulerRunAndJobLifecycle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	defaults := DefaultConfig()
	if defaults.Enabled || defaults.MaxConcLLM != 3 {
		t.Fatal("defaults changed")
	}
	b := bus.NewMessageBus()
	s := New(Config{}, b, nil)
	if s.cfg.TickInterval != time.Minute || s.semaphores[CategoryDefault].Cap() != 5 || NewSemaphore(0).Cap() != 1 {
		t.Fatal("default limits changed")
	}
	s.cfg.TickInterval = time.Millisecond
	s.lock = NewFileLock(filepath.Join(root, "lock"))
	cron, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatal(err)
	}
	s.Register(&Job{Name: "removed", Cron: cron})
	if len(s.Jobs()) != 1 {
		t.Fatal("job registration missing")
	}
	s.Unregister("removed")
	if len(s.Jobs()) != 0 {
		t.Fatal("job not removed")
	}
	s.Register(&Job{Name: "fixture", Cron: cron, Category: "unknown", Content: "scheduled fixture"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	msg, err := b.ConsumeInbound(waitCtx)
	waitCancel()
	cancel()
	if err != nil || msg.Content != "scheduled fixture" || msg.Metadata["scheduler_job"] != "fixture" {
		t.Fatalf("scheduled message: %+v %v", msg, err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run shutdown: %v", err)
	}
	tl, err := timeline.NewTimelineService(filepath.Join(root, "timeline.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	persisted := New(Config{}, b, tl)
	persisted.logJobRun("fixture", "dispatched", time.Now())
	if job, err := tl.GetScheduledJob("fixture"); err != nil || job.LastStatus != "dispatched" {
		t.Fatalf("persisted dispatch: %+v %v", job, err)
	}
	s.lock = NewFileLock(filepath.Join(root, "missing", "lock"))
	s.tick(context.Background(), time.Now())
	lock := NewFileLock(filepath.Join(root, "unlock"))
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatal("lock acquisition", err)
	}
	lock.file.Close()
	if err := lock.Unlock(); err == nil {
		t.Fatal("invalid lock descriptor accepted")
	}
	// Cron jumps across each calendar field and reports impossible schedules.
	expr, err := ParseCron("5 4 3 2 *")
	if err != nil {
		t.Fatal(err)
	}
	next := expr.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if !next.Equal(time.Date(2026, 2, 3, 4, 5, 0, 0, time.UTC)) {
		t.Fatalf("next calendar time: %s", next)
	}
	expr, err = ParseCron("0 0 30 2 *")
	if err != nil {
		t.Fatal(err)
	}
	if !expr.Next(time.Now()).IsZero() {
		t.Fatal("impossible date matched")
	}
	_ = os.Remove(filepath.Join(root, "unlock"))
}
