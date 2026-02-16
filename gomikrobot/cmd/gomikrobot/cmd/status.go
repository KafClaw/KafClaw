package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamir/gomikrobot/internal/config"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		printHeader("🏷️ GoMikroBot Version")
		fmt.Printf("Version: %s\n", version)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status",
	Run: func(cmd *cobra.Command, args []string) {
		printHeader("📊 GoMikroBot Status")
		fmt.Printf("Version: %s\n", version)

		// Check config
		home, _ := os.UserHomeDir()
		configPath := filepath.Join(home, ".gomikrobot", "config.json")
		if _, err := os.Stat(configPath); err == nil {
			fmt.Println("Config:  ✓ Found (" + configPath + ")")
		} else {
			fmt.Println("Config:  ✗ Not found (run 'gomikrobot onboard' first)")
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

		// WhatsApp status + QR location
		if cfg != nil && cfg.Channels.WhatsApp.Enabled {
			fmt.Println("WhatsApp: ✓ Enabled")
		} else if cfg != nil {
			fmt.Println("WhatsApp: ✗ Disabled")
		}
		waDB := filepath.Join(home, ".gomikrobot", "whatsapp.db")
		qrPath := filepath.Join(home, ".gomikrobot", "whatsapp-qr.png")
		if _, err := os.Stat(waDB); err == nil {
			fmt.Println("WhatsApp Link: ✓ Session found (no QR needed)")
		} else {
			fmt.Println("WhatsApp Link: ✗ No session (QR needed)")
			fmt.Println("WhatsApp QR:   " + qrPath)
		}

		fmt.Println("Status:  Ready")
	},
}
