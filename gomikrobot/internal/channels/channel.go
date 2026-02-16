package channels

import (
	"context"

	"github.com/kamir/gomikrobot/internal/bus"
)

// Channel defines the interface for chat platforms (Telegram, WhatsApp, etc).
type Channel interface {
	// Name returns the channel name (e.g. "telegram").
	Name() string
	// Start starts the channel listener.
	Start(ctx context.Context) error
	// Stop stops the channel listener.
	Stop() error
	// Send sends a message to a specific chat.
	Send(ctx context.Context, msg *bus.OutboundMessage) error
}

// BaseChannel provides common functionality for channels.
type BaseChannel struct {
	Bus *bus.MessageBus
}
