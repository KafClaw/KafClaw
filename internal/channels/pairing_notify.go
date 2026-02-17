package channels

import (
	"context"
	"fmt"
	"strings"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/config"
)

const PairingApprovedMessage = "Pairing approved. You can now send messages to KafClaw."

// NotifyPairingApproved sends a best-effort pairing approval confirmation back to sender.
func NotifyPairingApproved(ctx context.Context, cfg *config.Config, entry *PendingPairing) error {
	if cfg == nil || entry == nil {
		return nil
	}
	switch normalizeChannel(entry.Channel) {
	case "slack":
		ch := NewSlackChannel(cfg.Channels.Slack, bus.NewMessageBus(), nil)
		return ch.Send(ctx, &bus.OutboundMessage{
			Channel: "slack",
			ChatID:  strings.TrimSpace(entry.SenderID),
			Content: PairingApprovedMessage,
		})
	case "msteams":
		ch := NewMSTeamsChannel(cfg.Channels.MSTeams, bus.NewMessageBus(), nil)
		return ch.Send(ctx, &bus.OutboundMessage{
			Channel: "msteams",
			ChatID:  strings.TrimSpace(entry.SenderID),
			Content: PairingApprovedMessage,
		})
	case "whatsapp":
		// WhatsApp pairing notifications remain handled by existing channel flow.
		return nil
	default:
		return fmt.Errorf("unsupported channel: %s", entry.Channel)
	}
}
