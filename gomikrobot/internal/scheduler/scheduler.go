package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/timeline"
)

// JobCategory classifies jobs for semaphore-based concurrency limits.
type JobCategory string

const (
	CategoryLLM     JobCategory = "llm"
	CategoryShell   JobCategory = "shell"
	CategoryDefault JobCategory = "default"
)

// Job defines a schedulable unit of work.
type Job struct {
	Name     string      // Unique job identifier.
	Cron     *CronExpr   // Parsed cron expression.
	Category JobCategory // For semaphore selection.
	Content  string      // Message content dispatched to the agent loop.
}

// Config holds scheduler settings.
type Config struct {
	Enabled        bool          `json:"enabled" envconfig:"ENABLED"`
	TickInterval   time.Duration `json:"tickInterval"`
	MaxConcLLM     int           `json:"maxConcLLM"`
	MaxConcShell   int           `json:"maxConcShell"`
	MaxConcDefault int           `json:"maxConcDefault"`
	LockPath       string        `json:"lockPath"`
}

// DefaultConfig returns sensible scheduler defaults.
func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Enabled:        false,
		TickInterval:   60 * time.Second,
		MaxConcLLM:     3,
		MaxConcShell:   1,
		MaxConcDefault: 5,
		LockPath:       filepath.Join(home, ".gomikrobot", "scheduler.lock"),
	}
}

// Scheduler manages job registration, tick dispatch, and concurrency control.
type Scheduler struct {
	cfg        Config
	bus        *bus.MessageBus
	timeline   *timeline.TimelineService
	jobs       map[string]*Job
	mu         sync.RWMutex
	semaphores map[JobCategory]*Semaphore
	lock       *FileLock
}

// New creates a Scheduler.
func New(cfg Config, b *bus.MessageBus, tl *timeline.TimelineService) *Scheduler {
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 60 * time.Second
	}
	if cfg.MaxConcLLM <= 0 {
		cfg.MaxConcLLM = 3
	}
	if cfg.MaxConcShell <= 0 {
		cfg.MaxConcShell = 1
	}
	if cfg.MaxConcDefault <= 0 {
		cfg.MaxConcDefault = 5
	}
	if cfg.LockPath == "" {
		cfg.LockPath = DefaultConfig().LockPath
	}

	return &Scheduler{
		cfg:      cfg,
		bus:      b,
		timeline: tl,
		jobs:     make(map[string]*Job),
		semaphores: map[JobCategory]*Semaphore{
			CategoryLLM:     NewSemaphore(cfg.MaxConcLLM),
			CategoryShell:   NewSemaphore(cfg.MaxConcShell),
			CategoryDefault: NewSemaphore(cfg.MaxConcDefault),
		},
		lock: NewFileLock(cfg.LockPath),
	}
}

// Register adds a job to the scheduler.
func (s *Scheduler) Register(job *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.Name] = job
	slog.Info("Scheduler job registered", "name", job.Name, "category", job.Category)
}

// Unregister removes a job by name.
func (s *Scheduler) Unregister(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, name)
}

// Jobs returns the current registered jobs (snapshot).
func (s *Scheduler) Jobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j)
	}
	return out
}

// Run starts the scheduler tick loop. Blocks until context is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	slog.Info("Scheduler started", "tick", s.cfg.TickInterval, "jobs", len(s.jobs))
	ticker := time.NewTicker(s.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Scheduler stopped")
			return ctx.Err()
		case t := <-ticker.C:
			s.tick(ctx, t)
		}
	}
}

// tick is called every TickInterval. Acquires the global file lock, then
// dispatches any matching jobs.
func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	acquired, err := s.lock.TryLock()
	if err != nil {
		slog.Warn("Scheduler lock error", "error", err)
		return
	}
	if !acquired {
		slog.Debug("Scheduler tick skipped: lock held by another process")
		return
	}
	defer s.lock.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, job := range s.jobs {
		if !job.Cron.Matches(now) {
			continue
		}
		s.dispatch(ctx, job, now)
	}
}

// dispatch sends a job as a bus.InboundMessage if a semaphore slot is available.
func (s *Scheduler) dispatch(ctx context.Context, job *Job, now time.Time) {
	sem := s.semaphores[job.Category]
	if sem == nil {
		sem = s.semaphores[CategoryDefault]
	}

	if !sem.TryAcquire() {
		slog.Warn("Scheduler job skipped: concurrency limit", "job", job.Name, "category", job.Category)
		s.logJobRun(job.Name, "skipped_concurrency", now)
		return
	}

	slog.Info("Scheduler dispatching job", "job", job.Name)

	// Dispatch asynchronously; release semaphore when the bus consume completes.
	go func() {
		defer sem.Release()

		s.bus.PublishInbound(&bus.InboundMessage{
			Channel:  "scheduler",
			SenderID: "scheduler",
			ChatID:   fmt.Sprintf("scheduler:%s", job.Name),
			Content:  job.Content,
			Metadata: map[string]any{
				"message_type":   "internal",
				"scheduler_job":  job.Name,
				"scheduler_tick": now.Format(time.RFC3339),
			},
			Timestamp: now,
		})

		s.logJobRun(job.Name, "dispatched", now)
	}()
}

// logJobRun persists the run status to the scheduled_jobs table (best-effort).
func (s *Scheduler) logJobRun(name, status string, tick time.Time) {
	if s.timeline == nil {
		return
	}
	_ = s.timeline.UpsertScheduledJob(name, status, tick)
}
