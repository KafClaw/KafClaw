package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/channels"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/spf13/cobra"
)

func TestPairingCommandLifecycle(t *testing.T) {
	commandHome(t)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := pairingPendingCmd.RunE(cmd, nil); err != nil || !strings.Contains(out.String(), "No pending") {
		t.Fatalf("empty pairing: %s %v", out.String(), err)
	}
	tl, err := openTimelineService()
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	svc := channels.NewPairingService(tl)
	approved, err := svc.CreateOrGetPending("slack", "fixture-user", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	denied, err := svc.CreateOrGetPending("msteams", "denied-user", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := pairingPendingCmd.RunE(cmd, nil); err != nil || !strings.Contains(out.String(), "fixture-user") {
		t.Fatalf("pending list: %s %v", out.String(), err)
	}
	if err := pairingApproveCmd.RunE(cmd, []string{"slack", approved.Code}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Channels.Slack.AllowFrom) != 1 || cfg.Channels.Slack.AllowFrom[0] != "fixture-user" {
		t.Fatal("approved sender not persisted")
	}
	if err := pairingDenyCmd.RunE(cmd, []string{"msteams", denied.Code}); err != nil {
		t.Fatal(err)
	}
	if items, err := svc.ListPending(); err != nil || len(items) != 0 {
		t.Fatalf("pending not cleared: %+v %v", items, err)
	}
	if err := pairingApproveCmd.RunE(cmd, []string{"slack", "missing"}); err == nil {
		t.Fatal("missing code approved")
	}
	if err := pairingDenyCmd.RunE(cmd, []string{"msteams", "missing"}); err == nil {
		t.Fatal("missing code denied")
	}
}
