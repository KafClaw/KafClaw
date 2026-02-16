package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/kamir/gomikrobot/internal/agent"
	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/config"
	"github.com/kamir/gomikrobot/internal/provider"
	"github.com/spf13/cobra"
)

var (
	agentMessage   string
	agentSessionID string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Chat with the agent directly in CLI",
	Run:   runAgent,
}

func init() {
	agentCmd.Flags().StringVarP(&agentMessage, "message", "m", "", "Message to send to the agent")
	agentCmd.Flags().StringVarP(&agentSessionID, "session", "s", "cli:default", "Session ID")
}

func runAgent(cmd *cobra.Command, args []string) {
	if agentMessage == "" {
		fmt.Println("Error: --message is required")
		os.Exit(1)
	}

	printHeader("🤖 GoMikroBot Agent")

	// Load Config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config warning: %v (using defaults)\n", err)
	}
	if warn, err := config.EnsureWorkRepo(cfg.Paths.WorkRepoPath); err != nil {
		fmt.Printf("Work repo error: %v\n", err)
	} else if warn != "" {
		fmt.Printf("Work repo warning: %s\n", warn)
	}

	// Setup components
	msgBus := bus.NewMessageBus()
	oaProv := provider.NewOpenAIProvider(cfg.Providers.OpenAI.APIKey, cfg.Providers.OpenAI.APIBase, cfg.Model.Name)
	var prov provider.LLMProvider = oaProv

	if cfg.Providers.LocalWhisper.Enabled {
		prov = provider.NewLocalWhisperProvider(cfg.Providers.LocalWhisper, oaProv)
	}

	// Check API Key
	if cfg.Providers.OpenAI.APIKey == "" {
		fmt.Println("Error: API key not found. Set MIKROBOT_OPENAI_API_KEY, OPENROUTER_API_KEY, or use config.json")
		os.Exit(1)
	}

	loop := agent.NewLoop(agent.LoopOptions{
		Bus:           msgBus,
		Provider:      prov,
		Workspace:     cfg.Paths.Workspace,
		WorkRepo:      cfg.Paths.WorkRepoPath,
		SystemRepo:    cfg.Paths.SystemRepoPath,
		Model:         cfg.Model.Name,
		MaxIterations: cfg.Model.MaxToolIterations,
	})

	fmt.Printf("🤖 GoMikroBot (%s)\n", cfg.Model.Name)
	fmt.Println("Thinking...")

	ctx := context.Background()
	response, err := loop.ProcessDirect(ctx, agentMessage, agentSessionID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n" + response)
}
