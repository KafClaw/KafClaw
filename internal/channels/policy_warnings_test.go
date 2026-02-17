package channels

import (
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestCollectUnsafeGroupPolicyWarnings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Slack.GroupPolicy = config.GroupPolicyOpen
	cfg.Channels.Slack.RequireMention = false
	cfg.Channels.MSTeams.GroupPolicy = config.GroupPolicyAllowlist
	cfg.Channels.MSTeams.RequireMention = false
	cfg.Channels.MSTeams.GroupAllowFrom = []string{"*"}

	warnings := CollectUnsafeGroupPolicyWarnings(cfg)
	if len(warnings) < 3 {
		t.Fatalf("expected multiple warnings, got %d (%v)", len(warnings), warnings)
	}
}

func TestCollectUnsafeGroupPolicyWarningsSafeConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Slack.GroupPolicy = config.GroupPolicyAllowlist
	cfg.Channels.Slack.RequireMention = true
	cfg.Channels.MSTeams.GroupPolicy = config.GroupPolicyAllowlist
	cfg.Channels.MSTeams.RequireMention = true
	cfg.Channels.MSTeams.GroupAllowFrom = []string{"team1/channel1"}

	warnings := CollectUnsafeGroupPolicyWarnings(cfg)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}
