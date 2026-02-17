package bus

import (
	"context"
	"testing"
	"time"
)

func TestInboundMessageMessageType(t *testing.T) {
	msg := &InboundMessage{}
	if got := msg.MessageType(); got != MessageTypeExternal {
		t.Fatalf("expected default %q, got %q", MessageTypeExternal, got)
	}

	msg.Metadata = map[string]any{MetaKeyMessageType: MessageTypeInternal}
	if got := msg.MessageType(); got != MessageTypeInternal {
		t.Fatalf("expected %q, got %q", MessageTypeInternal, got)
	}
}

func TestMessageBusInboundOutboundAndDispatch(t *testing.T) {
	b := NewMessageBus()

	in := &InboundMessage{Channel: "wa", Content: "hello"}
	if !in.Timestamp.IsZero() {
		t.Fatal("expected zero timestamp in fixture")
	}
	b.PublishInbound(in)
	if in.Timestamp.IsZero() {
		t.Fatal("expected PublishInbound to set timestamp")
	}
	if b.InboundSize() != 1 {
		t.Fatalf("expected inbound size 1, got %d", b.InboundSize())
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gotIn, err := b.ConsumeInbound(ctx)
	if err != nil {
		t.Fatalf("consume inbound: %v", err)
	}
	if gotIn.Content != "hello" {
		t.Fatalf("unexpected inbound content: %q", gotIn.Content)
	}

	recv := make(chan *OutboundMessage, 1)
	b.Subscribe("wa", func(msg *OutboundMessage) { recv <- msg })

	dispatchCtx, dispatchCancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.DispatchOutbound(dispatchCtx) }()

	out := &OutboundMessage{Channel: "wa", Content: "reply"}
	b.PublishOutbound(out)
	if b.OutboundSize() == 0 {
		// Dispatcher may already have consumed it; this branch keeps timing-safe behavior.
	}

	select {
	case msg := <-recv:
		if msg.Content != "reply" {
			t.Fatalf("unexpected outbound content: %q", msg.Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outbound callback")
	}

	dispatchCancel()
	if err := <-done; err == nil {
		t.Fatal("expected dispatch to return context cancellation error")
	}

	b.Stop()
}

func TestConsumeInboundCanceled(t *testing.T) {
	b := NewMessageBus()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.ConsumeInbound(ctx); err == nil {
		t.Fatal("expected cancellation error")
	}
}
