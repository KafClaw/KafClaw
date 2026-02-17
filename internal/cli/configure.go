package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/spf13/cobra"
)

var configureSubagentsAllowAgents string
var configureClearSubagentsAllowAgents bool
var configureNonInteractive bool

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Guided configuration updates",
	RunE:  runConfigure,
}

func init() {
	configureCmd.Flags().StringVar(&configureSubagentsAllowAgents, "subagents-allow-agents", "", "Comma-separated agent IDs allowed for sessions_spawn agentId (use '*' for any)")
	configureCmd.Flags().BoolVar(&configureClearSubagentsAllowAgents, "clear-subagents-allow-agents", false, "Clear subagent allowlist (default behavior: current agent only)")
	configureCmd.Flags().BoolVar(&configureNonInteractive, "non-interactive", false, "Apply flags only and skip prompts")
	rootCmd.AddCommand(configureCmd)
}

func runConfigure(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	updatedAllowAgents := cfg.Tools.Subagents.AllowAgents
	hasFlagUpdate := configureClearSubagentsAllowAgents || strings.TrimSpace(configureSubagentsAllowAgents) != ""
	if configureClearSubagentsAllowAgents {
		updatedAllowAgents = nil
	} else if strings.TrimSpace(configureSubagentsAllowAgents) != "" {
		updatedAllowAgents = parseCSVList(configureSubagentsAllowAgents)
	}

	if !hasFlagUpdate && !configureNonInteractive {
		reader := bufio.NewReader(cmd.InOrStdin())
		current := strings.Join(cfg.Tools.Subagents.AllowAgents, ",")
		if current == "" {
			current = "(current agent only)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Current subagent allowlist: %s\n", current)
		fmt.Fprint(cmd.OutOrStdout(), "Set subagent allowlist (comma-separated, '*' for any, empty to keep): ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			return fmt.Errorf("read input: %w", readErr)
		}
		line = strings.TrimSpace(line)
		if line != "" {
			updatedAllowAgents = parseCSVList(line)
		}
	}

	cfg.Tools.Subagents.AllowAgents = updatedAllowAgents
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	state := "(current agent only)"
	if len(cfg.Tools.Subagents.AllowAgents) > 0 {
		state = strings.Join(cfg.Tools.Subagents.AllowAgents, ",")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Updated tools.subagents.allowAgents: %s\n", state)
	return nil
}

func parseCSVList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		v := strings.TrimSpace(part)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
