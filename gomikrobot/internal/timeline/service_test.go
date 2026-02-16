package timeline

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestTimeline(t *testing.T) *TimelineService {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "timeline.db")
	svc, err := NewTimelineService(dbPath)
	if err != nil {
		t.Fatalf("failed to create timeline service: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.Close()
		_ = os.RemoveAll(dir)
	})
	return svc
}

func TestWebUserLinkLifecycle(t *testing.T) {
	svc := newTestTimeline(t)

	user, err := svc.CreateWebUser("alice")
	if err != nil {
		t.Fatalf("create web user: %v", err)
	}
	if user.ID == 0 {
		t.Fatalf("expected web user ID")
	}

	if err := svc.LinkWebUser(user.ID, "123@s.whatsapp.net"); err != nil {
		t.Fatalf("link web user: %v", err)
	}

	jid, ok, err := svc.GetWebLink(user.ID)
	if err != nil {
		t.Fatalf("get web link: %v", err)
	}
	if !ok || jid == "" {
		t.Fatalf("expected linked jid")
	}

	if err := svc.UnlinkWebUser(user.ID); err != nil {
		t.Fatalf("unlink web user: %v", err)
	}

	jid, ok, err = svc.GetWebLink(user.ID)
	if err != nil {
		t.Fatalf("get web link after unlink: %v", err)
	}
	if ok || jid != "" {
		t.Fatalf("expected no link after unlink")
	}
}
