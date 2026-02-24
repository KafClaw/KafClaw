package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/KafClaw/KafClaw/internal/timeline"
)

// reconcileDurableRuntimeState restores/surfaces durable runtime counters on startup.
// This gives restart continuity for pending deliveries, open tasks, and heartbeat metadata.
func reconcileDurableRuntimeState(timeSvc *timeline.TimelineService) error {
	if timeSvc == nil {
		return fmt.Errorf("timeline service is nil")
	}

	pendingDeliveries, err := timeSvc.CountPendingDeliveries()
	if err != nil {
		return err
	}
	openTasks, err := timeSvc.CountOpenTasks()
	if err != nil {
		return err
	}
	openGroupTasks, err := timeSvc.CountOpenGroupTasks()
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_ = timeSvc.SetSetting("runtime_reconcile_last_at", now)
	_ = timeSvc.SetSetting("runtime_reconcile_pending_deliveries", strconv.Itoa(pendingDeliveries))
	_ = timeSvc.SetSetting("runtime_reconcile_open_tasks", strconv.Itoa(openTasks))
	_ = timeSvc.SetSetting("runtime_reconcile_open_group_tasks", strconv.Itoa(openGroupTasks))

	_ = timeSvc.AddEvent(&timeline.TimelineEvent{
		EventID:        fmt.Sprintf("RUNTIME_RECONCILE_%d", time.Now().UnixNano()),
		Timestamp:      time.Now(),
		SenderID:       "system",
		SenderName:     "KafClaw",
		EventType:      "SYSTEM",
		ContentText:    fmt.Sprintf("runtime reconcile: pending_deliveries=%d open_tasks=%d open_group_tasks=%d", pendingDeliveries, openTasks, openGroupTasks),
		Classification: "RUNTIME_RECONCILE",
		Authorized:     true,
	})

	return nil
}
