package channels

import (
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestCollectChannelAccountDiagnosticsEnabledWithMissingFields(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Slack.Enabled = true
	cfg.Channels.Slack.BotToken = ""
	cfg.Channels.Slack.OutboundURL = ""
	cfg.Channels.Slack.InboundToken = ""

	cfg.Channels.MSTeams.Enabled = true
	cfg.Channels.MSTeams.AppID = ""
	cfg.Channels.MSTeams.AppPassword = ""
	cfg.Channels.MSTeams.OutboundURL = ""
	cfg.Channels.MSTeams.InboundToken = ""

	diags := CollectChannelAccountDiagnostics(cfg)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if len(diags[0].Issues) == 0 || len(diags[1].Issues) == 0 {
		t.Fatalf("expected issues for both channels, got %#v", diags)
	}
}

func TestCollectChannelAccountDiagnosticsDisabledButConfigured(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Slack.Enabled = false
	cfg.Channels.Slack.BotToken = "xoxb-set"
	cfg.Channels.MSTeams.Enabled = false
	cfg.Channels.MSTeams.AppID = "app-set"

	diags := CollectChannelAccountDiagnostics(cfg)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if len(diags[0].Issues) == 0 || len(diags[1].Issues) == 0 {
		t.Fatalf("expected disabled-but-configured issues, got %#v", diags)
	}
}
