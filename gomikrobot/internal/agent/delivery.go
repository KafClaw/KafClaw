package agent

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/timeline"
)

// DeliveryWorker polls for completed tasks with pending delivery and retries them.
type DeliveryWorker struct {
	timeline *timeline.TimelineService
	bus      *bus.MessageBus
	interval time.Duration
	maxRetry int
}

// NewDeliveryWorker creates a delivery worker with sensible defaults.
func NewDeliveryWorker(tl *timeline.TimelineService, b *bus.MessageBus) *DeliveryWorker {
	return &DeliveryWorker{
		timeline: tl,
		bus:      b,
		interval: 5 * time.Second,
		maxRetry: 5,
	}
}

// Run starts the polling loop. Blocks until context is cancelled.
func (w *DeliveryWorker) Run(ctx context.Context) error {
	slog.Info("Delivery worker started", "interval", w.interval, "max_retry", w.maxRetry)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Delivery worker stopped")
			return ctx.Err()
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *DeliveryWorker) poll() {
	tasks, err := w.timeline.ListPendingDeliveries(10)
	if err != nil {
		slog.Error("Delivery worker poll failed", "error", err)
		return
	}

	for _, task := range tasks {
		if task.DeliveryAttempts >= w.maxRetry {
			slog.Warn("Delivery max retries exceeded", "task_id", task.TaskID, "attempts", task.DeliveryAttempts)
			_ = w.timeline.UpdateTaskDelivery(task.TaskID, timeline.DeliveryFailed, nil)
			continue
		}

		// Publish to outbound bus for channel delivery
		w.bus.PublishOutbound(&bus.OutboundMessage{
			Channel: task.Channel,
			ChatID:  task.ChatID,
			TraceID: task.TraceID,
			TaskID:  task.TaskID,
			Content: task.ContentOut,
		})

		// Mark as sent (the channel subscriber can override on failure)
		_ = w.timeline.UpdateTaskDelivery(task.TaskID, timeline.DeliverySent, nil)
		slog.Info("Delivery worker dispatched", "task_id", task.TaskID, "channel", task.Channel)
	}
}

// DeliveryBackoff calculates the next retry time using exponential backoff.
// Returns min(30s * 2^attempts, 5min).
func DeliveryBackoff(attempts int) time.Time {
	delay := time.Duration(30*math.Pow(2, float64(attempts))) * time.Second
	maxDelay := 5 * time.Minute
	if delay > maxDelay {
		delay = maxDelay
	}
	return time.Now().Add(delay)
}
