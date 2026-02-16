package cmd

import (
	"fmt"
	"os"

	"github.com/KafClaw/KafClaw/gomikrobot/internal/config"
	"github.com/KafClaw/KafClaw/gomikrobot/internal/identity"
	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Initialize configuration and scaffold workspace",
	Run:   runOnboard,
}

var onboardForce bool

func init() {
	onboardCmd.Flags().BoolVarP(&onboardForce, "force", "f", false, "Overwrite existing config and soul files")
	rootCmd.AddCommand(onboardCmd)
}

func runOnboard(cmd *cobra.Command, args []string) {
	printHeader("🚀 KafClaw Onboard")
	fmt.Println("Initializing KafClaw...")

	// 1. Config file
	cfgPath, _ := config.ConfigPath()

	if _, err := os.Stat(cfgPath); err == nil && !onboardForce {
		fmt.Printf("Config already exists at: %s\n", cfgPath)
		fmt.Println("Use --force (-f) to overwrite.")
	} else {
		cfg := config.DefaultConfig()
		if err := config.Save(cfg); err != nil {
			fmt.Printf("Error saving config: %v\n", err)
		} else {
			fmt.Printf("Config created at: %s\n", cfgPath)
		}
	}

	// 2. Load config (with expanded paths) for scaffolding
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config warning: %v (using defaults)\n", err)
	}

	// 3. Scaffold workspace soul files
	fmt.Printf("\nWorkspace: %s\n", cfg.Paths.Workspace)
	result, err := identity.ScaffoldWorkspace(cfg.Paths.Workspace, onboardForce)
	if err != nil {
		fmt.Printf("Error scaffolding workspace: %v\n", err)
	} else {
		for _, name := range result.Created {
			fmt.Printf("  + %s\n", name)
		}
		for _, name := range result.Skipped {
			fmt.Printf("  ~ %s (exists, skipped)\n", name)
		}
		for _, e := range result.Errors {
			fmt.Printf("  ! %s\n", e)
		}
	}

	// 4. Ensure work repo directories
	if warn, err := config.EnsureWorkRepo(cfg.Paths.WorkRepoPath); err != nil {
		fmt.Printf("Work repo error: %v\n", err)
	} else {
		fmt.Printf("\nWork repo: %s\n", cfg.Paths.WorkRepoPath)
		if warn != "" {
			fmt.Printf("  Warning: %s\n", warn)
		}
	}

	fmt.Println("\nNext steps:")
	fmt.Println("1. Edit config.json to add your API keys.")
	fmt.Println("2. Customize soul files in your workspace (SOUL.md, USER.md, etc.)")
	fmt.Println("3. Run 'gomikrobot agent -m \"hello\"' to test.")
}
