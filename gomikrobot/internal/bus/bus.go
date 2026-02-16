// Package bus provides the async message bus for channel-agent communication.
package bus

import (
	"context"
	"sync"
	"time"
)

// Well-known metadata keys and message type constants.
const (
	MetaKeyMessageType  = "message_type"
	MetaKeyIsFromMe     = "is_from_me"
	MessageTypeInternal = "internal"
	MessageTypeExternal = "external"
)

// InboundMessage represents a message from a channel to the agent.
type InboundMessage struct {
	Channel        string         `json:"channel"`
	SenderID       string         `json:"sender_id"`
	ChatID         string         `json:"chat_id"`
	TraceID        string         `json:"trace_id"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Content        string         `json:"content"`
	Media          []string       `json:"media,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Timestamp      time.Time      `json:"timestamp"`
}

// MessageType returns the message type from metadata, defaulting to external.
func (m *InboundMessage) MessageType() string {
	if m.Metadata != nil {
		if v, ok := m.Metadata[MetaKeyMessageType].(string); ok && v != "" {
			return v
		}
	}
	return MessageTypeExternal
}

// OutboundMessage represents a message from the agent to a channel.
type OutboundMessage struct {
	Channel string `json:"channel"`
	ChatID  string `json:"chat_id"`
	TraceID string `json:"trace_id"`
	TaskID  string `json:"task_id,omitempty"`
	Content string `json:"content"`
}

// MessageBus decouples channels from the agent core.
type MessageBus struct {
	inbound  chan *InboundMessage
	outbound chan *OutboundMessage
	subs     map[string][]func(*OutboundMessage)
	running  bool
	mu       sync.RWMutex
}

// NewMessageBus creates a new message bus.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:  make(chan *InboundMessage, 100),
		outbound: make(chan *OutboundMessage, 100),
		subs:     make(map[string][]func(*OutboundMessage)),
	}
}

// PublishInbound sends a message from a channel to the agent.
func (b *MessageBus) PublishInbound(msg *InboundMessage) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	b.inbound <- msg
}

// ConsumeInbound blocks until a message is available or context is cancelled.
func (b *MessageBus) ConsumeInbound(ctx context.Context) (*InboundMessage, error) {
	select {
	case msg := <-b.inbound:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PublishOutbound sends a message from the agent to channels.
func (b *MessageBus) PublishOutbound(msg *OutboundMessage) {
	b.outbound <- msg
}

// Subscribe registers a callback for outbound messages to a specific channel.
func (b *MessageBus) Subscribe(channel string, callback func(*OutboundMessage)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.subs[channel] = append(b.subs[channel], callback)
}

// DispatchOutbound runs the outbound message dispatcher.
// This should be run as a goroutine.
func (b *MessageBus) DispatchOutbound(ctx context.Context) error {
	b.mu.Lock()
	b.running = true
	b.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-b.outbound:
			b.mu.RLock()
			callbacks := b.subs[msg.Channel]
			b.mu.RUnlock()

			for _, cb := range callbacks {
				cb(msg)
			}
		}
	}
}

// Stop signals the bus to stop.
func (b *MessageBus) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = false
}

// InboundSize returns the number of pending inbound messages.
func (b *MessageBus) InboundSize() int {
	return len(b.inbound)
}

// OutboundSize returns the number of pending outbound messages.
func (b *MessageBus) OutboundSize() int {
	return len(b.outbound)
}
