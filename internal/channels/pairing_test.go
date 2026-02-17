package channels

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

type mockPairingStore struct {
	raw        string
	getErr     error
	setErr     error
	lastSetKey string
	lastSetVal string
}

func (m *mockPairingStore) GetSetting(string) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	return m.raw, nil
}

func (m *mockPairingStore) SetSetting(key, value string) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.lastSetKey = key
	m.lastSetVal = value
	m.raw = value
	return nil
}

func TestPairingServiceCreateApproveDeny(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "timeline.db")
	timeSvc, err := timeline.NewTimelineService(db)
	if err != nil {
		t.Fatalf("new timeline: %v", err)
	}
	defer timeSvc.Close()

	svc := NewPairingService(timeSvc)
	entry, err := svc.CreateOrGetPending("slack", "U123", time.Hour)
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	if entry.Code == "" {
		t.Fatal("expected pairing code")
	}

	cfg := config.DefaultConfig()
	approved, err := svc.Approve(cfg, "slack", entry.Code)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.SenderID != "U123" {
		t.Fatalf("unexpected sender: %s", approved.SenderID)
	}
	if len(cfg.Channels.Slack.AllowFrom) != 1 || cfg.Channels.Slack.AllowFrom[0] != "u123" {
		t.Fatalf("expected sender in allowlist, got: %#v", cfg.Channels.Slack.AllowFrom)
	}

	entry2, err := svc.CreateOrGetPending("teams", "A456", time.Hour)
	if err != nil {
		t.Fatalf("create teams pending: %v", err)
	}
	denied, err := svc.Deny("msteams", entry2.Code)
	if err != nil {
		t.Fatalf("deny: %v", err)
	}
	if denied.SenderID != "A456" {
		t.Fatalf("unexpected denied sender: %s", denied.SenderID)
	}
}

func TestNotifyPairingApprovedSlack(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.Channels.Slack.OutboundURL = srv.URL
	entry := &PendingPairing{
		Channel:  "slack",
		SenderID: "U-1",
	}
	if err := NotifyPairingApproved(context.Background(), cfg, entry); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if got["chat_id"] != "U-1" {
		t.Fatalf("unexpected payload: %#v", got)
	}
}

func TestPairingServiceExpiresEntries(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "timeline.db")
	timeSvc, err := timeline.NewTimelineService(db)
	if err != nil {
		t.Fatalf("new timeline: %v", err)
	}
	defer timeSvc.Close()

	svc := NewPairingService(timeSvc)
	_, err = svc.CreateOrGetPending("slack", "U123", time.Millisecond)
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	items, err := svc.ListPending()
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no pending entries, got %d", len(items))
	}

	raw, err := timeSvc.GetSetting(pairingPendingKey)
	if err != nil {
		t.Fatalf("get setting: %v", err)
	}
	if raw == "" {
		t.Fatal("expected persisted empty JSON array")
	}
}

func TestPairingCodeFormatAndMaxPending(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "timeline.db")
	timeSvc, err := timeline.NewTimelineService(db)
	if err != nil {
		t.Fatalf("new timeline: %v", err)
	}
	defer timeSvc.Close()

	svc := NewPairingService(timeSvc)
	for i := 0; i < 5; i++ {
		_, err := svc.CreateOrGetPending("slack", "U"+string(rune('A'+i)), 0)
		if err != nil {
			t.Fatalf("create pending: %v", err)
		}
	}
	items, err := svc.ListPending()
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(items) != maxPendingPairings {
		t.Fatalf("expected max %d pending, got %d", maxPendingPairings, len(items))
	}
	for _, it := range items {
		if len(it.Code) != pairingCodeLength {
			t.Fatalf("unexpected code length: %q", it.Code)
		}
		for _, ch := range it.Code {
			if !strings.ContainsRune(pairingCodeAlphabet, ch) {
				t.Fatalf("invalid code char %q in %q", ch, it.Code)
			}
		}
	}
}

func TestPairingCriticalHelpers(t *testing.T) {
	cfg := config.DefaultConfig()

	if err := addChannelAllowFrom(cfg, "slack", "slack:user:U123"); err != nil {
		t.Fatalf("add slack allowfrom: %v", err)
	}
	if len(cfg.Channels.Slack.AllowFrom) != 1 || cfg.Channels.Slack.AllowFrom[0] != "u123" {
		t.Fatalf("unexpected slack allowfrom: %#v", cfg.Channels.Slack.AllowFrom)
	}

	if err := addChannelAllowFrom(cfg, "msteams", "teams:user:ABC"); err != nil {
		t.Fatalf("add teams allowfrom: %v", err)
	}
	if len(cfg.Channels.MSTeams.AllowFrom) != 1 || cfg.Channels.MSTeams.AllowFrom[0] != "abc" {
		t.Fatalf("unexpected teams allowfrom: %#v", cfg.Channels.MSTeams.AllowFrom)
	}

	if err := addChannelAllowFrom(cfg, "whatsapp", " +123@s.whatsapp.net "); err != nil {
		t.Fatalf("add whatsapp allowfrom: %v", err)
	}
	if len(cfg.Channels.WhatsApp.AllowFrom) != 1 || cfg.Channels.WhatsApp.AllowFrom[0] != "+123@s.whatsapp.net" {
		t.Fatalf("unexpected whatsapp allowfrom: %#v", cfg.Channels.WhatsApp.AllowFrom)
	}

	if err := addChannelAllowFrom(cfg, "slack", ""); err == nil {
		t.Fatal("expected error for empty sender")
	}
	if err := addChannelAllowFrom(cfg, "telegram", "u1"); err == nil {
		t.Fatal("expected error for unsupported channel")
	}

	items := []string{"a", "B"}
	items = appendUnique(items, " b ")
	if len(items) != 2 {
		t.Fatalf("appendUnique duplicated value: %#v", items)
	}
	items = appendUnique(items, " c ")
	if len(items) != 3 || items[2] != "c" {
		t.Fatalf("appendUnique missing new value: %#v", items)
	}
	items = appendUnique(items, " ")
	if len(items) != 3 {
		t.Fatalf("appendUnique should ignore empty values: %#v", items)
	}
}

func TestNormalizeAllowEntryForChannel(t *testing.T) {
	tests := []struct {
		channel string
		raw     string
		want    string
	}{
		{channel: "slack", raw: "slack:user:U1", want: "u1"},
		{channel: "msteams", raw: "teams:user:AbC", want: "abc"},
		{channel: "msteams", raw: "msteams:user:AbC", want: "abc"},
		{channel: "whatsapp", raw: " +1@s.whatsapp.net ", want: "+1@s.whatsapp.net"},
	}
	for _, tt := range tests {
		if got := normalizeAllowEntryForChannel(tt.channel, tt.raw); got != tt.want {
			t.Fatalf("channel=%s raw=%q got=%q want=%q", tt.channel, tt.raw, got, tt.want)
		}
	}
}

func TestPairingApproveDenyErrorPaths(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "timeline.db")
	timeSvc, err := timeline.NewTimelineService(db)
	if err != nil {
		t.Fatalf("new timeline: %v", err)
	}
	defer timeSvc.Close()

	svc := NewPairingService(timeSvc)
	entry, err := svc.CreateOrGetPending("slack", "U100", time.Hour)
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	if _, err := svc.Approve(nil, "slack", entry.Code); err == nil {
		t.Fatal("expected error for nil config")
	}
	if _, err := svc.Approve(config.DefaultConfig(), "slack", "missing-code"); err == nil {
		t.Fatal("expected error for missing pairing code")
	}

	entryUnknown, err := svc.CreateOrGetPending("unknown", "U200", time.Hour)
	if err != nil {
		t.Fatalf("create unknown pending: %v", err)
	}
	cfg := config.DefaultConfig()
	if _, err := svc.Approve(cfg, "unknown", entryUnknown.Code); err == nil {
		t.Fatal("expected error for unsupported channel")
	}

	if _, err := svc.Deny("slack", "does-not-exist"); err == nil {
		t.Fatal("expected deny error for unknown code")
	}
	if _, err := svc.Deny("", "abc"); err == nil {
		t.Fatal("expected deny error for missing channel")
	}
	if _, err := svc.Deny("slack", ""); err == nil {
		t.Fatal("expected deny error for missing code")
	}
}

func TestPairingApproveDenySaveErrors(t *testing.T) {
	now := time.Now().UTC()
	entry := []PendingPairing{{
		Channel:   "slack",
		SenderID:  "U1",
		Code:      "ABC12345",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}}
	raw, _ := json.Marshal(entry)

	store := &mockPairingStore{raw: string(raw), setErr: errors.New("set failed")}
	svc := &PairingService{store: store}
	cfg := config.DefaultConfig()

	if _, err := svc.Approve(cfg, "slack", "ABC12345"); err == nil {
		t.Fatal("expected approve save error")
	}

	store.raw = string(raw)
	if _, err := svc.Deny("slack", "ABC12345"); err == nil {
		t.Fatal("expected deny save error")
	}
}
