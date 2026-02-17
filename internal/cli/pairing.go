package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/KafClaw/KafClaw/internal/channels"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"github.com/spf13/cobra"
)

var pairingCmd = &cobra.Command{
	Use:   "pairing",
	Short: "Manage pending channel pairings",
}

var pairingPendingCmd = &cobra.Command{
	Use:   "pending",
	Short: "List pending pairing requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		timeSvc, err := openTimelineService()
		if err != nil {
			return err
		}
		defer timeSvc.Close()

		svc := channels.NewPairingService(timeSvc)
		items, err := svc.ListPending()
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No pending pairing requests.")
			return nil
		}
		for _, it := range items {
			fmt.Fprintf(cmd.OutOrStdout(), "%s code=%s sender=%s expires=%s\n",
				it.Channel,
				it.Code,
				it.SenderID,
				it.ExpiresAt.Format(time.RFC3339),
			)
		}
		return nil
	},
}

var pairingApproveCmd = &cobra.Command{
	Use:   "approve <channel> <code>",
	Short: "Approve a pending sender pairing request",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		timeSvc, err := openTimelineService()
		if err != nil {
			return err
		}
		defer timeSvc.Close()

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		svc := channels.NewPairingService(timeSvc)
		entry, err := svc.Approve(cfg, args[0], args[1])
		if err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		if err := channels.NotifyPairingApproved(cmd.Context(), cfg, entry); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: pairing approval notification failed: %v\n", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Approved %s sender %s\n", entry.Channel, entry.SenderID)
		return nil
	},
}

var pairingDenyCmd = &cobra.Command{
	Use:   "deny <channel> <code>",
	Short: "Deny a pending sender pairing request",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		timeSvc, err := openTimelineService()
		if err != nil {
			return err
		}
		defer timeSvc.Close()

		svc := channels.NewPairingService(timeSvc)
		entry, err := svc.Deny(args[0], args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Denied %s sender %s\n", entry.Channel, entry.SenderID)
		return nil
	},
}

func openTimelineService() (*timeline.TimelineService, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".kafclaw", "timeline.db")
	return timeline.NewTimelineService(path)
}

func init() {
	pairingCmd.AddCommand(pairingPendingCmd)
	pairingCmd.AddCommand(pairingApproveCmd)
	pairingCmd.AddCommand(pairingDenyCmd)
	rootCmd.AddCommand(pairingCmd)
}
