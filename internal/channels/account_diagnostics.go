package channels

import (
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
)

// AccountDiagnostic describes one channel account configuration health snapshot.
type AccountDiagnostic struct {
	Channel string
	Account string
	Enabled bool
	Issues  []string
}

// CollectChannelAccountDiagnostics returns Slack/Teams account diagnostics.
func CollectChannelAccountDiagnostics(cfg *config.Config) []AccountDiagnostic {
	if cfg == nil {
		return nil
	}
	out := make([]AccountDiagnostic, 0, 2)
	out = append(out, slackAccountDiagnostic(cfg)...)
	out = append(out, teamsAccountDiagnostic(cfg)...)
	return out
}

func slackAccountDiagnostic(cfg *config.Config) []AccountDiagnostic {
	c := cfg.Channels.Slack
	out := make([]AccountDiagnostic, 0)
	out = append(out, AccountDiagnostic{
		Channel: "slack",
		Account: "default",
		Enabled: c.Enabled,
		Issues:  make([]string, 0, 4),
	})
	out[0].Issues = append(out[0].Issues, slackIssues(c.Enabled, c.BotToken, c.AppToken, c.InboundToken, c.OutboundURL)...)
	for _, acct := range c.Accounts {
		d := AccountDiagnostic{
			Channel: "slack",
			Account: accountIDOrDefault(acct.ID),
			Enabled: acct.Enabled,
			Issues:  make([]string, 0, 4),
		}
		d.Issues = append(d.Issues, slackIssues(acct.Enabled, acct.BotToken, acct.AppToken, acct.InboundToken, acct.OutboundURL)...)
		out = append(out, d)
	}
	return out
}

func teamsAccountDiagnostic(cfg *config.Config) []AccountDiagnostic {
	c := cfg.Channels.MSTeams
	out := make([]AccountDiagnostic, 0)
	out = append(out, AccountDiagnostic{
		Channel: "msteams",
		Account: "default",
		Enabled: c.Enabled,
		Issues:  make([]string, 0, 5),
	})
	out[0].Issues = append(out[0].Issues, teamsIssues(c.Enabled, c.AppID, c.AppPassword, c.InboundToken, c.OutboundURL)...)
	for _, acct := range c.Accounts {
		d := AccountDiagnostic{
			Channel: "msteams",
			Account: accountIDOrDefault(acct.ID),
			Enabled: acct.Enabled,
			Issues:  make([]string, 0, 5),
		}
		d.Issues = append(d.Issues, teamsIssues(acct.Enabled, acct.AppID, acct.AppPassword, acct.InboundToken, acct.OutboundURL)...)
		out = append(out, d)
	}
	return out
}

func slackIssues(enabled bool, botToken, appToken, inboundToken, outboundURL string) []string {
	out := make([]string, 0, 4)
	if enabled {
		if strings.TrimSpace(botToken) == "" {
			out = append(out, "enabled but botToken is missing")
		}
		if strings.TrimSpace(outboundURL) == "" {
			out = append(out, "enabled but outboundUrl is missing")
		}
		if strings.TrimSpace(inboundToken) == "" {
			out = append(out, "enabled but inboundToken is missing")
		}
		return out
	}
	if strings.TrimSpace(botToken) != "" || strings.TrimSpace(appToken) != "" || strings.TrimSpace(inboundToken) != "" || strings.TrimSpace(outboundURL) != "" {
		out = append(out, "disabled but credentials/bridge settings are present")
	}
	return out
}

func teamsIssues(enabled bool, appID, appPassword, inboundToken, outboundURL string) []string {
	out := make([]string, 0, 4)
	if enabled {
		if strings.TrimSpace(appID) == "" {
			out = append(out, "enabled but appId is missing")
		}
		if strings.TrimSpace(appPassword) == "" {
			out = append(out, "enabled but appPassword is missing")
		}
		if strings.TrimSpace(outboundURL) == "" {
			out = append(out, "enabled but outboundUrl is missing")
		}
		if strings.TrimSpace(inboundToken) == "" {
			out = append(out, "enabled but inboundToken is missing")
		}
		return out
	}
	if strings.TrimSpace(appID) != "" || strings.TrimSpace(appPassword) != "" || strings.TrimSpace(inboundToken) != "" || strings.TrimSpace(outboundURL) != "" {
		out = append(out, "disabled but credentials/bridge settings are present")
	}
	return out
}
