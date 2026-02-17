package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestStatusShowsSlackTeamsCapabilityDetails(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	cfg := config.DefaultConfig()
	cfg.Channels.Slack.Enabled = true
	cfg.Channels.Slack.BotToken = "xoxb-test"
	cfg.Channels.Slack.AppToken = "xapp-test"
	cfg.Channels.Slack.InboundToken = "slack-in"
	cfg.Channels.Slack.OutboundURL = "http://127.0.0.1:18888/slack/outbound"
	cfg.Channels.Slack.AllowFrom = []string{"U1", "U2"}
	cfg.Channels.Slack.DmPolicy = config.DmPolicyAllowlist
	cfg.Channels.Slack.GroupPolicy = config.GroupPolicyOpen
	cfg.Channels.Slack.RequireMention = false

	cfg.Channels.MSTeams.Enabled = true
	cfg.Channels.MSTeams.AppID = "teams-app"
	cfg.Channels.MSTeams.AppPassword = "teams-secret"
	cfg.Channels.MSTeams.TenantID = "botframework.com"
	cfg.Channels.MSTeams.InboundToken = "teams-in"
	cfg.Channels.MSTeams.OutboundURL = "http://127.0.0.1:18888/teams/outbound"
	cfg.Channels.MSTeams.AllowFrom = []string{"A1"}
	cfg.Channels.MSTeams.GroupAllowFrom = []string{"team1/channel1"}
	cfg.Channels.MSTeams.DmPolicy = config.DmPolicyPairing
	cfg.Channels.MSTeams.GroupPolicy = config.GroupPolicyAllowlist
	cfg.Channels.MSTeams.RequireMention = true

	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := runRootCommandWithStdoutCapture(t, "status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	required := []string{
		"Slack Account [default]: enabled",
		"scope: mode=room session=slack:<account>:<chat_id>",
		"policies: dm=allowlist group=open require_mention=false",
		"allowlist: dm=2 group=2",
		"credentials: bot_token=true app_token=true",
		"diagnostics: configured",
		"MSTeams Account [default]: enabled",
		"scope: mode=room session=msteams:<account>:<chat_id>",
		"policies: dm=pairing group=allowlist require_mention=true",
		"allowlist: dm=1 group=1",
		"credentials: app_id=true app_password=true tenant_id=botframework.com",
		"diagnostics: configured",
	}
	for _, needle := range required {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected status output to contain %q, got:\n%s", needle, out)
		}
	}
}

func TestStatusShowsAdditionalAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	cfg := config.DefaultConfig()
	cfg.Channels.Slack.Enabled = true
	cfg.Channels.Slack.Accounts = []config.SlackAccountConfig{
		{ID: "ops", Enabled: true, BotToken: "xoxb-ops", InboundToken: "in", OutboundURL: "http://localhost"},
	}
	cfg.Channels.MSTeams.Enabled = true
	cfg.Channels.MSTeams.Accounts = []config.MSTeamsAccountConfig{
		{ID: "sales", Enabled: false, AppID: "app-x"},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	out, err := runRootCommandWithStdoutCapture(t, "status")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if !strings.Contains(out, "Slack Account [ops]: enabled") {
		t.Fatalf("expected slack account details in status output, got:\n%s", out)
	}
	if !strings.Contains(out, "MSTeams Account [sales]: disabled") {
		t.Fatalf("expected teams account details in status output, got:\n%s", out)
	}
}

func runRootCommandWithStdoutCapture(t *testing.T, args ...string) (string, error) {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	done := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(r)
		done <- data
	}()

	rootCmd.SetArgs(args)
	_, runErr := rootCmd.ExecuteC()
	rootCmd.SetArgs(nil)
	_ = w.Close()
	data := <-done
	return strings.TrimSpace(string(bytes.TrimSpace(data))), runErr
}
