package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/kamir/gomikrobot/internal/timeline"
	"github.com/spf13/cobra"
)

var (
	waApproveJID string
	waDenyJID    string
	waListAuth   bool
)

var whatsappAuthCmd = &cobra.Command{
	Use:   "whatsapp-auth",
	Short: "Approve or deny WhatsApp JIDs from pending list",
	Run:   runWhatsAppAuth,
}

func init() {
	whatsappAuthCmd.Flags().StringVar(&waApproveJID, "approve", "", "Approve a pending JID")
	whatsappAuthCmd.Flags().StringVar(&waDenyJID, "deny", "", "Deny a pending JID")
	whatsappAuthCmd.Flags().BoolVar(&waListAuth, "list", false, "List allow/deny/pending")
	rootCmd.AddCommand(whatsappAuthCmd)
}

func runWhatsAppAuth(cmd *cobra.Command, args []string) {
	printHeader("📲 WhatsApp Auth")

	home, _ := os.UserHomeDir()
	timelinePath := fmt.Sprintf("%s/.gomikrobot/timeline.db", home)
	timeSvc, err := timeline.NewTimelineService(timelinePath)
	if err != nil {
		fmt.Printf("Timeline init error: %v\n", err)
		return
	}

	if waListAuth {
		allow := getSetting(timeSvc, "whatsapp_allowlist")
		deny := getSetting(timeSvc, "whatsapp_denylist")
		pending := getSetting(timeSvc, "whatsapp_pending")
		fmt.Println("Allowlist:\n" + allow)
		fmt.Println("Denylist:\n" + deny)
		fmt.Println("Pending:\n" + pending)
		return
	}

	if waApproveJID == "" && waDenyJID == "" {
		fmt.Println("Provide --approve or --deny (or --list).")
		return
	}

	if waApproveJID != "" {
		if err := moveJID(timeSvc, waApproveJID, "whatsapp_pending", "whatsapp_allowlist"); err != nil {
			fmt.Printf("Approve error: %v\n", err)
			return
		}
		fmt.Printf("Approved: %s\n", waApproveJID)
	}

	if waDenyJID != "" {
		if err := moveJID(timeSvc, waDenyJID, "whatsapp_pending", "whatsapp_denylist"); err != nil {
			fmt.Printf("Deny error: %v\n", err)
			return
		}
		fmt.Printf("Denied: %s\n", waDenyJID)
	}
}

func getSetting(svc *timeline.TimelineService, key string) string {
	val, err := svc.GetSetting(key)
	if err != nil {
		return ""
	}
	return val
}

func moveJID(svc *timeline.TimelineService, jid, fromKey, toKey string) error {
	jid = strings.TrimSpace(jid)
	if jid == "" {
		return fmt.Errorf("jid is empty")
	}
	from := parseList(getSetting(svc, fromKey))
	to := parseList(getSetting(svc, toKey))
	if !contains(from, jid) {
		return fmt.Errorf("jid not found in %s", fromKey)
	}
	from = remove(from, jid)
	if !contains(to, jid) {
		to = append(to, jid)
	}
	if err := svc.SetSetting(fromKey, formatList(from)); err != nil {
		return err
	}
	return svc.SetSetting(toKey, formatList(to))
}

func parseList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = strings.ReplaceAll(raw, ",", "\n")
	parts := strings.Split(raw, "\n")
	return normalizeList(parts)
}

func normalizeList(parts []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func formatList(list []string) string {
	return strings.Join(normalizeList(list), "\n")
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

func remove(list []string, v string) []string {
	var out []string
	for _, item := range list {
		if item != v {
			out = append(out, item)
		}
	}
	return out
}
