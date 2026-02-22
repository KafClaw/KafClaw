package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
	"github.com/KafClaw/KafClaw/internal/provider/clinst"
	"github.com/KafClaw/KafClaw/internal/provider/credentials"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"github.com/spf13/cobra"
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Manage LLM providers and models",
}

// --- auth subgroup ---

var modelsAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage provider authentication",
}

var (
	authLoginProvider string
)

var modelsAuthLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with an OAuth-based provider",
	Run:   runModelsAuthLogin,
}

var (
	setKeyProvider string
	setKeyValue    string
	setKeyBase     string
)

var modelsAuthSetKeyCmd = &cobra.Command{
	Use:   "set-key",
	Short: "Store an API key for a provider",
	Run:   runModelsAuthSetKey,
}

// --- list ---

var modelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show configured providers and active model per agent",
	Run:   runModelsList,
}

// --- stats ---

var (
	statsDays int
	statsJSON bool
)

var modelsStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show token usage statistics",
	Run:   runModelsStats,
}

func init() {
	modelsAuthLoginCmd.Flags().StringVar(&authLoginProvider, "provider", "", "Provider to authenticate with (gemini, openai-codex)")
	_ = modelsAuthLoginCmd.MarkFlagRequired("provider")

	modelsAuthSetKeyCmd.Flags().StringVar(&setKeyProvider, "provider", "", "Provider ID (claude, openai, gemini, xai, scalytics-copilot, openrouter, deepseek, groq, vllm)")
	modelsAuthSetKeyCmd.Flags().StringVar(&setKeyValue, "key", "", "API key or bearer token")
	modelsAuthSetKeyCmd.Flags().StringVar(&setKeyBase, "base", "", "Base URL (required for scalytics-copilot, vllm)")
	_ = modelsAuthSetKeyCmd.MarkFlagRequired("provider")
	_ = modelsAuthSetKeyCmd.MarkFlagRequired("key")

	modelsAuthCmd.AddCommand(modelsAuthLoginCmd)
	modelsAuthCmd.AddCommand(modelsAuthSetKeyCmd)

	modelsStatsCmd.Flags().IntVar(&statsDays, "days", 0, "Show per-day per-provider trend for N days")
	modelsStatsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output in JSON format")

	modelsCmd.AddCommand(modelsAuthCmd)
	modelsCmd.AddCommand(modelsListCmd)
	modelsCmd.AddCommand(modelsStatsCmd)

	rootCmd.AddCommand(modelsCmd)
}

// --- auth login ---

func runModelsAuthLogin(_ *cobra.Command, _ []string) {
	provID := strings.ToLower(strings.TrimSpace(authLoginProvider))
	switch provID {
	case "gemini", "gemini-cli":
		if err := clinst.EnsureGeminiCLI(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Running: gemini auth login")
		cmd := exec.Command("gemini", "auth", "login")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error: gemini auth login failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Gemini CLI authentication complete.")

	case "openai-codex", "codex":
		if err := clinst.EnsureCodexCLI(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Running: codex auth")
		cmd := exec.Command("codex", "auth")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error: codex auth failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Codex CLI authentication complete.")

	default:
		fmt.Printf("Error: OAuth login is only supported for gemini and openai-codex. For API key providers, use: kafclaw models auth set-key --provider %s --key <token>\n", provID)
		os.Exit(1)
	}
}

// --- auth set-key ---

var apiKeyProviders = map[string]bool{
	"claude": true, "openai": true, "gemini": true, "xai": true,
	"scalytics-copilot": true, "openrouter": true, "deepseek": true,
	"groq": true, "vllm": true,
}

func runModelsAuthSetKey(_ *cobra.Command, _ []string) {
	provID := strings.ToLower(strings.TrimSpace(setKeyProvider))
	provID = provider.NormalizeProviderID(provID, nil)

	if !apiKeyProviders[provID] {
		fmt.Printf("Error: unknown or non-API-key provider %q\n", provID)
		os.Exit(1)
	}

	if (provID == "scalytics-copilot" || provID == "vllm") && setKeyBase == "" {
		fmt.Printf("Error: --base is required for provider %s\n", provID)
		os.Exit(1)
	}

	if err := credentials.SaveAPIKey(provID, setKeyValue); err != nil {
		fmt.Printf("Error storing API key: %v\n", err)
		os.Exit(1)
	}

	// Also write base URL to config if provided.
	if setKeyBase != "" {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("Warning: could not load config to set base URL: %v\n", err)
		} else {
			switch provID {
			case "scalytics-copilot":
				cfg.Providers.ScalyticsCopilot.APIBase = setKeyBase
			case "vllm":
				cfg.Providers.VLLM.APIBase = setKeyBase
			}
			if saveErr := config.Save(cfg); saveErr != nil {
				fmt.Printf("Warning: could not save config: %v\n", saveErr)
			}
		}
	}

	fmt.Printf("API key for %s stored successfully.\n", provID)
}

// --- list ---

func runModelsList(_ *cobra.Command, _ []string) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config warning: %v\n", err)
	}

	fmt.Println("Configured Providers:")
	fmt.Println()

	providerStatus := []struct {
		ID     string
		HasKey bool
		Base   string
	}{
		{"claude", cfg.Providers.Anthropic.APIKey != "", cfg.Providers.Anthropic.APIBase},
		{"openai", cfg.Providers.OpenAI.APIKey != "", cfg.Providers.OpenAI.APIBase},
		{"gemini", cfg.Providers.Gemini.APIKey != "", ""},
		{"gemini-cli", false, "(OAuth via CLI)"},
		{"openai-codex", false, "(OAuth via CLI)"},
		{"xai", cfg.Providers.XAI.APIKey != "", xaiBase()},
		{"scalytics-copilot", cfg.Providers.ScalyticsCopilot.APIKey != "", cfg.Providers.ScalyticsCopilot.APIBase},
		{"openrouter", cfg.Providers.OpenRouter.APIKey != "", cfg.Providers.OpenRouter.APIBase},
		{"deepseek", cfg.Providers.DeepSeek.APIKey != "", cfg.Providers.DeepSeek.APIBase},
		{"groq", cfg.Providers.Groq.APIKey != "", cfg.Providers.Groq.APIBase},
		{"vllm", cfg.Providers.VLLM.APIKey != "" || cfg.Providers.VLLM.APIBase != "", cfg.Providers.VLLM.APIBase},
	}

	for _, ps := range providerStatus {
		status := "not configured"
		if ps.HasKey {
			status = "configured"
		}
		if ps.Base == "(OAuth via CLI)" {
			status = "OAuth"
		}
		base := ""
		if ps.Base != "" && ps.Base != "(OAuth via CLI)" {
			base = fmt.Sprintf("  base: %s", ps.Base)
		}
		fmt.Printf("  %-20s %s%s\n", ps.ID, status, base)
	}

	fmt.Println()
	fmt.Printf("Global model: %s\n", cfg.Model.Name)

	if cfg.Agents != nil {
		for _, entry := range cfg.Agents.List {
			if entry.Model != nil && entry.Model.Primary != "" {
				fmt.Printf("Agent %-12s model: %s\n", entry.ID, entry.Model.Primary)
				for i, fb := range entry.Model.Fallbacks {
					fmt.Printf("  fallback[%d]: %s\n", i, fb)
				}
			}
			if entry.Subagents != nil && entry.Subagents.Model != "" {
				fmt.Printf("  subagent model: %s\n", entry.Subagents.Model)
			}
		}
	}
}

func xaiBase() string {
	return "https://api.x.ai/v1"
}

// --- stats ---

func runModelsStats(_ *cobra.Command, _ []string) {
	home, _ := os.UserHomeDir()
	timelinePath := filepath.Join(home, ".kafclaw", "timeline.db")
	tl, err := timeline.NewTimelineService(timelinePath)
	if err != nil {
		fmt.Printf("Error opening timeline: %v\n", err)
		os.Exit(1)
	}

	if statsDays > 0 {
		summary, err := tl.GetTokenUsageSummary(statsDays)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		if statsJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(summary)
			return
		}
		fmt.Printf("%-15s %-12s %-10s %s\n", "PROVIDER", "DAY", "TOKENS", "COST")
		for _, s := range summary {
			costStr := "-"
			if s.CostUSD > 0 {
				costStr = fmt.Sprintf("$%.4f", s.CostUSD)
			}
			fmt.Printf("%-15s %-12s %-10d %s\n", s.ProviderID, s.Day, s.Tokens, costStr)
		}
		return
	}

	// Today's usage by provider.
	byProvider, err := tl.GetDailyTokenUsageByProvider()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	totalToday, _ := tl.GetDailyTokenUsage()

	costByProvider, _ := tl.GetDailyCostByProvider()

	if statsJSON {
		out := map[string]any{
			"today_total":       totalToday,
			"today_by_provider": byProvider,
			"today_cost":        costByProvider,
			"rate_limits":       provider.AllRateLimitSnapshots(),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return
	}

	fmt.Printf("Today's token usage: %d\n\n", totalToday)
	if len(byProvider) > 0 {
		fmt.Printf("%-20s %-10s %s\n", "PROVIDER", "TOKENS", "COST")
		for prov, tokens := range byProvider {
			costStr := "-"
			if c, ok := costByProvider[prov]; ok && c > 0 {
				costStr = fmt.Sprintf("$%.4f", c)
			}
			fmt.Printf("%-20s %-10d %s\n", prov, tokens, costStr)
		}
	}

	snapshots := provider.AllRateLimitSnapshots()
	if len(snapshots) > 0 {
		fmt.Println()
		fmt.Printf("%-20s %-15s %-15s %-15s %s\n", "PROVIDER", "REMAINING TOK", "REMAINING REQ", "LIMIT TOK", "RESET")
		for prov, snap := range snapshots {
			rt := "-"
			rr := "-"
			lt := "-"
			reset := "-"
			if snap.RemainingTokens != nil {
				rt = fmt.Sprintf("%d", *snap.RemainingTokens)
			}
			if snap.RemainingRequests != nil {
				rr = fmt.Sprintf("%d", *snap.RemainingRequests)
			}
			if snap.LimitTokens != nil {
				lt = fmt.Sprintf("%d", *snap.LimitTokens)
			}
			if snap.ResetAt != nil {
				reset = snap.ResetAt.Format("15:04:05")
			}
			fmt.Printf("%-20s %-15s %-15s %-15s %s\n", prov, rt, rr, lt, reset)
		}
	}
}
