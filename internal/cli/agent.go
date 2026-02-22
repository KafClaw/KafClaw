package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/KafClaw/KafClaw/internal/agent"
	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
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

	printHeader("🤖 KafClaw Agent")

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
	prov, err := provider.Resolve(cfg, "main")
	if err != nil {
		fmt.Printf("Provider error: %v\n", err)
		os.Exit(1)
	}

	if cfg.Providers.LocalWhisper.Enabled {
		if oaProv, ok := prov.(*provider.OpenAIProvider); ok {
			prov = provider.NewLocalWhisperProvider(cfg.Providers.LocalWhisper, oaProv)
		}
	}

	loop := agent.NewLoop(agent.LoopOptions{
		Bus:                   msgBus,
		Provider:              prov,
		Workspace:             cfg.Paths.Workspace,
		WorkRepo:              cfg.Paths.WorkRepoPath,
		SystemRepo:            cfg.Paths.SystemRepoPath,
		Model:                 cfg.Model.Name,
		MaxIterations:         cfg.Model.MaxToolIterations,
		MaxSubagentSpawnDepth: cfg.Tools.Subagents.MaxSpawnDepth,
		MaxSubagentChildren:   cfg.Tools.Subagents.MaxChildrenPerAgent,
		MaxSubagentConcurrent: cfg.Tools.Subagents.MaxConcurrent,
		SubagentArchiveAfter:  cfg.Tools.Subagents.ArchiveAfterMinutes,
		AgentID:               cfg.Group.AgentID,
		SubagentAllowAgents:   cfg.Tools.Subagents.AllowAgents,
		SubagentModel:         cfg.Tools.Subagents.Model,
		SubagentThinking:      cfg.Tools.Subagents.Thinking,
		SubagentToolsAllow:    cfg.Tools.Subagents.Tools.Allow,
		SubagentToolsDeny:     cfg.Tools.Subagents.Tools.Deny,
		Config:                cfg,
	})

	fmt.Printf("🤖 KafClaw (%s)\n", cfg.Model.Name)
	fmt.Println("Thinking...")

	ctx := context.Background()
	response, err := loop.ProcessDirect(ctx, agentMessage, agentSessionID)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n" + response)
}
