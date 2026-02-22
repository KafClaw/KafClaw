package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/KafClaw/KafClaw/internal/channels"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		printHeader("🏷️ KafClaw Version")
		fmt.Printf("Version: %s\n", version)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status",
	Run: func(cmd *cobra.Command, args []string) {
		printHeader("📊 KafClaw Status")
		fmt.Printf("Version: %s\n", version)

		// Check config
		home, _ := os.UserHomeDir()
		configPath := filepath.Join(home, ".kafclaw", "config.json")
		if _, err := os.Stat(configPath); err == nil {
			fmt.Println("Config:  ✓ Found (" + configPath + ")")
		} else {
			fmt.Println("Config:  ✗ Not found (run 'kafclaw onboard' first)")
		}

		// Check API key presence
		var cfg *config.Config
		if c, err := config.Load(); err == nil {
			cfg = c
			if cfg.Providers.OpenAI.APIKey != "" {
				fmt.Println("API Key: ✓ Found")
			} else {
				fmt.Println("API Key: ✗ Not found")
			}
		} else {
			fmt.Println("API Key: ? Unable to load config")
		}

		// Provider / model info
		if cfg != nil {
			printProviderStatus(cfg)
		}

		// WhatsApp status + QR location
		if cfg != nil && cfg.Channels.WhatsApp.Enabled {
			fmt.Println("WhatsApp: ✓ Enabled")
		} else if cfg != nil {
			fmt.Println("WhatsApp: ✗ Disabled")
		}
		waDB := filepath.Join(home, ".kafclaw", "whatsapp.db")
		qrPath := filepath.Join(home, ".kafclaw", "whatsapp-qr.png")
		if _, err := os.Stat(waDB); err == nil {
			fmt.Println("WhatsApp Link: ✓ Session found (no QR needed)")
		} else {
			fmt.Println("WhatsApp Link: ✗ No session (QR needed)")
			fmt.Println("WhatsApp QR:   " + qrPath)
		}
		if cfg != nil && cfg.Channels.Slack.Enabled {
			fmt.Println("Slack:    ✓ Enabled")
		} else if cfg != nil {
			fmt.Println("Slack:    ✗ Disabled")
		}
		if cfg != nil && cfg.Channels.MSTeams.Enabled {
			fmt.Println("MSTeams:  ✓ Enabled")
		} else if cfg != nil {
			fmt.Println("MSTeams:  ✗ Disabled")
		}
		if cfg != nil {
			printSlackStatusDetails(cfg)
			printMSTeamsStatusDetails(cfg)
		}
		if cfg != nil {
			warnings := channels.CollectUnsafeGroupPolicyWarnings(cfg)
			if len(warnings) == 0 {
				fmt.Println("Policy:   ✓ No unsafe group policy warnings")
			} else {
				fmt.Printf("Policy:   ⚠ %d warning(s)\n", len(warnings))
				for _, w := range warnings {
					fmt.Printf("  - %s\n", w)
				}
			}
		}
		if timeSvc, err := openTimelineService(); err == nil {
			svc := channels.NewPairingService(timeSvc)
			if pending, err := svc.ListPending(); err == nil {
				if len(pending) == 0 {
					fmt.Println("Pairing:  ✓ No pending requests")
				} else {
					counts := map[string]int{}
					for _, p := range pending {
						counts[p.Channel]++
					}
					chs := make([]string, 0, len(counts))
					for k := range counts {
						chs = append(chs, k)
					}
					sort.Strings(chs)
					summary := ""
					for i, ch := range chs {
						if i > 0 {
							summary += ", "
						}
						summary += fmt.Sprintf("%s=%d", ch, counts[ch])
					}
					fmt.Printf("Pairing:  ⚠ %d pending (%s)\n", len(pending), summary)
				}
			} else {
				fmt.Println("Pairing:  ? Unable to read pending requests")
			}
			_ = timeSvc.Close()
		} else {
			fmt.Println("Pairing:  ? Timeline unavailable")
		}

		fmt.Println("Status:  Ready")
	},
}

func printSlackStatusDetails(cfg *config.Config) {
	diags := channels.CollectChannelAccountDiagnostics(cfg)
	printSlackAccount("default", cfg.Channels.Slack.Enabled, cfg.Channels.Slack.BotToken, cfg.Channels.Slack.AppToken, cfg.Channels.Slack.InboundToken, cfg.Channels.Slack.OutboundURL, cfg.Channels.Slack.AllowFrom, cfg.Channels.Slack.DmPolicy, cfg.Channels.Slack.GroupPolicy, cfg.Channels.Slack.RequireMention, cfg.Channels.Slack.SessionScope, issuesForAccount("slack", "default", diags))
	for _, acct := range cfg.Channels.Slack.Accounts {
		id := nonEmpty(strings.ToLower(strings.TrimSpace(acct.ID)), "default")
		printSlackAccount(id, acct.Enabled, acct.BotToken, acct.AppToken, acct.InboundToken, acct.OutboundURL, acct.AllowFrom, acct.DmPolicy, acct.GroupPolicy, acct.RequireMention, nonEmpty(acct.SessionScope, cfg.Channels.Slack.SessionScope), issuesForAccount("slack", id, diags))
	}
}

func printMSTeamsStatusDetails(cfg *config.Config) {
	diags := channels.CollectChannelAccountDiagnostics(cfg)
	printTeamsAccount("default", cfg.Channels.MSTeams.Enabled, cfg.Channels.MSTeams.AppID, cfg.Channels.MSTeams.AppPassword, cfg.Channels.MSTeams.TenantID, cfg.Channels.MSTeams.InboundToken, cfg.Channels.MSTeams.OutboundURL, cfg.Channels.MSTeams.AllowFrom, cfg.Channels.MSTeams.GroupAllowFrom, cfg.Channels.MSTeams.DmPolicy, cfg.Channels.MSTeams.GroupPolicy, cfg.Channels.MSTeams.RequireMention, cfg.Channels.MSTeams.SessionScope, issuesForAccount("msteams", "default", diags))
	for _, acct := range cfg.Channels.MSTeams.Accounts {
		id := nonEmpty(strings.ToLower(strings.TrimSpace(acct.ID)), "default")
		printTeamsAccount(id, acct.Enabled, acct.AppID, acct.AppPassword, acct.TenantID, acct.InboundToken, acct.OutboundURL, acct.AllowFrom, acct.GroupAllowFrom, acct.DmPolicy, acct.GroupPolicy, acct.RequireMention, nonEmpty(acct.SessionScope, cfg.Channels.MSTeams.SessionScope), issuesForAccount("msteams", id, diags))
	}
}

func printSlackAccount(accountID string, enabled bool, botToken, appToken, inboundToken, outboundURL string, allow []string, dm config.DmPolicy, group config.GroupPolicy, requireMention bool, scopeMode string, issues []string) {
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	fmt.Printf("Slack Account [%s]: %s\n", accountID, state)
	fmt.Printf("  - scope: mode=%s session=%s\n", nonEmpty(scopeMode, "room"), sessionScopeHint("slack", scopeMode))
	fmt.Printf("  - policies: dm=%s group=%s require_mention=%t\n", nonEmpty(string(dm), "pairing"), nonEmpty(string(group), "allowlist"), requireMention)
	fmt.Printf("  - allowlist: dm=%d group=%d\n", len(allow), len(allow))
	fmt.Printf("  - bridge: inbound_token=%t outbound_url=%t\n", strings.TrimSpace(inboundToken) != "", strings.TrimSpace(outboundURL) != "")
	fmt.Printf("  - credentials: bot_token=%t app_token=%t\n", strings.TrimSpace(botToken) != "", strings.TrimSpace(appToken) != "")
	if len(issues) == 0 {
		fmt.Printf("  - diagnostics: configured\n")
	} else {
		fmt.Printf("  - diagnostics: %d issue(s)\n", len(issues))
		for _, issue := range issues {
			fmt.Printf("    * %s\n", issue)
		}
	}
}

func printTeamsAccount(accountID string, enabled bool, appID, appPassword, tenantID, inboundToken, outboundURL string, allow, groupAllow []string, dm config.DmPolicy, group config.GroupPolicy, requireMention bool, scopeMode string, issues []string) {
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	fmt.Printf("MSTeams Account [%s]: %s\n", accountID, state)
	fmt.Printf("  - scope: mode=%s session=%s\n", nonEmpty(scopeMode, "room"), sessionScopeHint("msteams", scopeMode))
	fmt.Printf("  - policies: dm=%s group=%s require_mention=%t\n", nonEmpty(string(dm), "pairing"), nonEmpty(string(group), "allowlist"), requireMention)
	fmt.Printf("  - allowlist: dm=%d group=%d\n", len(allow), len(groupAllow))
	fmt.Printf("  - bridge: inbound_token=%t outbound_url=%t\n", strings.TrimSpace(inboundToken) != "", strings.TrimSpace(outboundURL) != "")
	fmt.Printf("  - credentials: app_id=%t app_password=%t tenant_id=%s\n", strings.TrimSpace(appID) != "", strings.TrimSpace(appPassword) != "", nonEmpty(strings.TrimSpace(tenantID), "unset"))
	if len(issues) == 0 {
		fmt.Printf("  - diagnostics: configured\n")
	} else {
		fmt.Printf("  - diagnostics: %d issue(s)\n", len(issues))
		for _, issue := range issues {
			fmt.Printf("    * %s\n", issue)
		}
	}
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func issuesForAccount(channel, account string, diags []channels.AccountDiagnostic) []string {
	ch := strings.TrimSpace(strings.ToLower(channel))
	acct := strings.TrimSpace(strings.ToLower(account))
	for _, d := range diags {
		if strings.TrimSpace(strings.ToLower(d.Channel)) == ch && strings.TrimSpace(strings.ToLower(d.Account)) == acct {
			return d.Issues
		}
	}
	return nil
}

func printProviderStatus(cfg *config.Config) {
	// Active model
	modelName := cfg.Model.Name
	if modelName == "" {
		modelName = "(legacy openai fallback)"
	}
	fmt.Printf("Model:    %s\n", modelName)

	// Configured providers
	var configured []string
	if cfg.Providers.OpenAI.APIKey != "" {
		configured = append(configured, "openai")
	}
	if cfg.Providers.Anthropic.APIKey != "" {
		configured = append(configured, "claude")
	}
	if cfg.Providers.Gemini.APIKey != "" {
		configured = append(configured, "gemini")
	}
	if cfg.Providers.XAI.APIKey != "" {
		configured = append(configured, "xai")
	}
	if cfg.Providers.ScalyticsCopilot.APIKey != "" {
		configured = append(configured, "scalytics-copilot")
	}
	if cfg.Providers.OpenRouter.APIKey != "" {
		configured = append(configured, "openrouter")
	}
	if cfg.Providers.DeepSeek.APIKey != "" {
		configured = append(configured, "deepseek")
	}
	if cfg.Providers.Groq.APIKey != "" {
		configured = append(configured, "groq")
	}
	if cfg.Providers.VLLM.APIBase != "" {
		configured = append(configured, "vllm")
	}
	if len(configured) > 0 {
		sort.Strings(configured)
		fmt.Printf("Providers: %s\n", strings.Join(configured, ", "))
	} else {
		fmt.Println("Providers: none configured")
	}

	// Today's token usage from timeline
	if timeSvc, err := openTimelineService(); err == nil {
		if usage, err := timeSvc.GetDailyTokenUsage(); err == nil {
			fmt.Printf("Tokens:   %d today\n", usage)
		}
		_ = timeSvc.Close()
	}

	// Rate limit snapshots
	snapshots := provider.AllRateLimitSnapshots()
	if len(snapshots) > 0 {
		keys := make([]string, 0, len(snapshots))
		for k := range snapshots {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			snap := snapshots[k]
			parts := []string{}
			if snap.RemainingTokens != nil {
				parts = append(parts, fmt.Sprintf("tokens=%d", *snap.RemainingTokens))
			}
			if snap.RemainingRequests != nil {
				parts = append(parts, fmt.Sprintf("requests=%d", *snap.RemainingRequests))
			}
			if len(parts) > 0 {
				fmt.Printf("Rate [%s]: %s\n", k, strings.Join(parts, " "))
			}
		}
	}

	// Middleware status
	var active []string
	if cfg.ContentClassification.Enabled {
		active = append(active, "classifier")
	}
	if cfg.PromptGuard.Enabled {
		active = append(active, "prompt-guard")
	}
	if cfg.OutputSanitization.Enabled {
		active = append(active, "sanitizer")
	}
	if cfg.FinOps.Enabled {
		active = append(active, "finops")
	}
	if len(active) > 0 {
		fmt.Printf("Middleware: %s\n", strings.Join(active, ", "))
	}
}

func sessionScopeHint(channel, mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "channel":
		return channel
	case "account":
		return channel + ":<account>"
	case "thread":
		return channel + ":<account>:<chat_id>:<thread_id>"
	case "user":
		return channel + ":<account>:<sender_id>"
	default:
		return channel + ":<account>:<chat_id>"
	}
}
