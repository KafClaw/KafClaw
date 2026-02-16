package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/kamir/gomikrobot/internal/config"
	"github.com/kamir/gomikrobot/internal/timeline"
	"github.com/spf13/cobra"
)

var whatsappSetupCmd = &cobra.Command{
	Use:   "whatsapp-setup",
	Short: "Configure WhatsApp auth (token + allow/deny lists)",
	Run:   runWhatsAppSetup,
}

func init() {
	rootCmd.AddCommand(whatsappSetupCmd)
}

func runWhatsAppSetup(cmd *cobra.Command, args []string) {
	reader := bufio.NewReader(os.Stdin)

	printHeader("📲 WhatsApp Setup")

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config warning: %v (using defaults)\n", err)
	}

	fmt.Println("WhatsApp Setup")
	fmt.Println("──────────────")
	fmt.Print("Enable WhatsApp channel? (y/N): ")
	enable := readYesNo(reader)
	cfg.Channels.WhatsApp.Enabled = enable

	fmt.Print("Pairing token (share out-of-band): ")
	token := readLine(reader)

	fmt.Print("Initial allowlist (comma or newline separated, optional): ")
	allow := readLine(reader)

	fmt.Print("Initial denylist (comma or newline separated, optional): ")
	deny := readLine(reader)

	if err := config.Save(cfg); err != nil {
		fmt.Printf("Config save error: %v\n", err)
	} else {
		fmt.Println("Config updated.")
	}

	home, _ := os.UserHomeDir()
	timelinePath := fmt.Sprintf("%s/.gomikrobot/timeline.db", home)
	timeSvc, err := timeline.NewTimelineService(timelinePath)
	if err != nil {
		fmt.Printf("Timeline init error: %v\n", err)
		return
	}

	if token != "" {
		_ = timeSvc.SetSetting("whatsapp_pair_token", token)
	}
	if strings.TrimSpace(allow) != "" {
		_ = timeSvc.SetSetting("whatsapp_allowlist", allow)
	}
	if strings.TrimSpace(deny) != "" {
		_ = timeSvc.SetSetting("whatsapp_denylist", deny)
	}

	fmt.Println("WhatsApp auth settings stored.")
	fmt.Println("Next: run `./gomikrobot gateway` and scan the QR code if needed.")
}

func readYesNo(r *bufio.Reader) bool {
	line := strings.ToLower(readLine(r))
	return strings.HasPrefix(line, "y")
}

func readLine(r *bufio.Reader) string {
	text, _ := r.ReadString('\n')
	return strings.TrimSpace(text)
}
