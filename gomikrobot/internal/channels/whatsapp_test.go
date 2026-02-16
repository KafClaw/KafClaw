package channels

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/config"
	"github.com/kamir/gomikrobot/internal/timeline"
)

func newTestTimeline(t *testing.T) *timeline.TimelineService {
	t.Helper()
	baseDir := t.TempDir()
	dbPath := filepath.Join(baseDir, "timeline.db")
	timeSvc, err := timeline.NewTimelineService(dbPath)
	if err != nil {
		t.Fatalf("failed to create timeline service: %v", err)
	}
	t.Cleanup(func() {
		_ = timeSvc.Close()
		_ = os.RemoveAll(baseDir)
	})
	return timeSvc
}

func TestWhatsAppSilentModeSuppressesOutbound(t *testing.T) {
	timeSvc := newTestTimeline(t)
	if err := timeSvc.SetSetting("silent_mode", "true"); err != nil {
		t.Fatalf("failed to set silent mode: %v", err)
	}
	msgBus := bus.NewMessageBus()

	cfg := config.WhatsAppConfig{Enabled: true}
	wa := NewWhatsAppChannel(cfg, msgBus, nil, timeSvc)

	var called int32
	wa.sendFn = func(ctx context.Context, msg *bus.OutboundMessage) error {
		atomic.AddInt32(&called, 1)
		return nil
	}

	wa.handleOutbound(&bus.OutboundMessage{
		Channel: wa.Name(),
		ChatID:  "12345@s.whatsapp.net",
		Content: "test",
	})

	if atomic.LoadInt32(&called) != 0 {
		t.Fatalf("expected send to be suppressed in silent mode")
	}
}

func TestWhatsAppSilentModeDisabledAllowsOutbound(t *testing.T) {
	timeSvc := newTestTimeline(t)
	if err := timeSvc.SetSetting("silent_mode", "false"); err != nil {
		t.Fatalf("failed to disable silent mode: %v", err)
	}

	msgBus := bus.NewMessageBus()
	cfg := config.WhatsAppConfig{Enabled: true}
	wa := NewWhatsAppChannel(cfg, msgBus, nil, timeSvc)

	var called int32
	wa.sendFn = func(ctx context.Context, msg *bus.OutboundMessage) error {
		atomic.AddInt32(&called, 1)
		return nil
	}

	wa.handleOutbound(&bus.OutboundMessage{
		Channel: wa.Name(),
		ChatID:  "12345@s.whatsapp.net",
		Content: "test",
	})

	if atomic.LoadInt32(&called) != 1 {
		t.Fatalf("expected send to occur when silent mode is disabled")
	}
}

func TestWhatsAppOutboundLogsTimeline(t *testing.T) {
	timeSvc := newTestTimeline(t)
	if err := timeSvc.SetSetting("silent_mode", "true"); err != nil {
		t.Fatalf("failed to set silent mode: %v", err)
	}
	msgBus := bus.NewMessageBus()

	cfg := config.WhatsAppConfig{Enabled: true}
	wa := NewWhatsAppChannel(cfg, msgBus, nil, timeSvc)
	wa.sendFn = func(ctx context.Context, msg *bus.OutboundMessage) error {
		return nil
	}

	wa.handleOutbound(&bus.OutboundMessage{
		Channel: wa.Name(),
		ChatID:  "12345@s.whatsapp.net",
		Content: "test outbound",
	})

	events, err := timeSvc.GetEvents(timeline.FilterArgs{Limit: 10})
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	found := false
	for _, e := range events {
		if strings.Contains(e.Classification, "WHATSAPP_OUTBOUND") {
			found = true
			if !strings.Contains(e.Classification, "status=suppressed") {
				t.Fatalf("expected suppressed status, got %s", e.Classification)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected outbound timeline event to be logged")
	}
}
