package cli

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	// version can be overridden at build time via:
	// go build -ldflags "-X github.com/KafClaw/KafClaw/internal/cli.version=1.2.3"
	version = "2.7.0"
	logo    = "\n" +
		"  _  __       __  ____ _\n" +
		" | |/ / __ _ / _|/ ___| | __ ___      __\n" +
		" | ' / / _` | |_| |   | |/ _` \\ \\ /\\ / /\n" +
		" | . \\| (_| |  _| |___| | (_| |\\ V  V /\n" +
		" |_|\\_\\\\__,_|_|  \\____|_|\\__,_| \\_/\\_/\n"
)

var rootCmd = &cobra.Command{
	Use:   "kafclaw",
	Short: "KafClaw - Personal AI Assistant",
	Long:  color.CyanString(logo) + "\nA lightweight, ultra-fast AI assistant framework written in Go.",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Commands will register themselves via init() in their respective files if we export rootCmd,
	// or we can register them here if they are exported vars.
	// For simplicity in this codebase, we'll assume vars are in package scope.
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(gatewayCmd)
	rootCmd.AddCommand(groupCmd)
	rootCmd.AddCommand(ksharkCmd)
}
