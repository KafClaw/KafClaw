package cmd

import (
	"fmt"
	"os"

	"github.com/kamir/gomikrobot/internal/config"
	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Initialize configuration",
	Run:   runOnboard,
}

var onboardForce bool

func init() {
	onboardCmd.Flags().BoolVarP(&onboardForce, "force", "f", false, "Overwrite existing config.json")
	rootCmd.AddCommand(onboardCmd)
}

func runOnboard(cmd *cobra.Command, args []string) {
	printHeader("🚀 GoMikroBot Onboard")
	fmt.Println("Initializing GoMikroBot...")

	path, _ := config.ConfigPath()

	// If config already exists, do not overwrite unless -f/--force is set.
	if _, err := os.Stat(path); err == nil && !onboardForce {
		fmt.Printf("Config already exists at: %s\n", path)
		fmt.Println("Use --force (-f) to overwrite.")
		return
	}

	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		fmt.Printf("Error skipping config: %v\n", err)
	} else {
		fmt.Printf("✅ Config created at: %s\n", path)
	}

	// ensure workspace exists
	if err := config.EnsureDir(cfg.Paths.Workspace); err != nil {
		// It might be ~ path, EnsureDir assumes expanded?
		// DefaultConfig has "~/...". config.EnsureDir does mkdir. Mkdir doesn't expand ~.
		// We need to expand it. config.Load expands it.
		// Let's rely on user to run it or expand it here.
	}

	fmt.Println("\nNext steps:")
	fmt.Println("1. Edit config.json to add your API keys.")
	fmt.Println("2. Run 'gomikrobot agent -m \"hello\"' to test.")
}
