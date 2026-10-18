package channels

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type classifierFixture struct {
	body string
	err  error
}

func (p *classifierFixture) Chat(context.Context, *provider.ChatRequest) (*provider.ChatResponse, error) {
	return &provider.ChatResponse{Content: p.body}, p.err
}
func (p *classifierFixture) DefaultModel() string { return "fixture" }
func (p *classifierFixture) Transcribe(context.Context, *provider.AudioRequest) (*provider.AudioResponse, error) {
	return nil, errors.New("unsupported")
}
func (p *classifierFixture) Speak(context.Context, *provider.TTSRequest) (*provider.TTSResponse, error) {
	return nil, errors.New("unsupported")
}

func TestWhatsAppInboundAuthorizationAndClassification(t *testing.T) {
	tl := newTestTimeline(t)
	defer tl.Close()
	b := bus.NewMessageBus()
	prov := &classifierFixture{body: "```json\n{\"category\":\"appointment\",\"summary\":\"Meet tomorrow\"}\n```"}
	c := NewWhatsAppChannel(config.WhatsAppConfig{DropUnauthorized: true}, b, prov, tl)
	if err := tl.SetSetting("whatsapp_allowlist", "123"); err != nil {
		t.Fatal(err)
	}
	if err := tl.SetSetting("whatsapp_pair_token", "fixture-token"); err != nil {
		t.Fatal(err)
	}
	c.ReloadAuth()
	for i, self := range []bool{false, true} {
		msg := &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: types.NewJID("123", "s.whatsapp.net"), Sender: types.NewJID("123", "s.whatsapp.net"), IsFromMe: self}, ID: fmt.Sprint(i), Timestamp: time.Now()}, Message: &waE2E.Message{Conversation: proto.String("Can we meet tomorrow?")}}
		if self {
			msg.Message = &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("Extended fixture message")}}
		}
		c.eventHandler(msg)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		in, err := b.ConsumeInbound(ctx)
		cancel()
		if err != nil || in.Content == "" || in.Metadata[bus.MetaKeyIsFromMe] != self {
			t.Fatalf("inbound: %+v %v", in, err)
		}
	}
	denied := &events.Message{Info: types.MessageInfo{MessageSource: types.MessageSource{Chat: types.NewJID("999", "s.whatsapp.net"), Sender: types.NewJID("999", "s.whatsapp.net")}, ID: "denied"}, Message: &waE2E.Message{Conversation: proto.String("fixture-token")}}
	c.eventHandler(denied)
	pending, _ := tl.GetSetting("whatsapp_pending")
	if pending != `["999"]` {
		t.Fatalf("pending not recorded: %q", pending)
	}
	c.config.DropUnauthorized = false
	denied.Info.ID = "logged-denied"
	denied.Message.Conversation = proto.String("unauthorized message")
	c.eventHandler(denied)
	logged, err := tl.GetEvents(timeline.FilterArgs{Limit: 10})
	if err != nil || len(logged) != 4 {
		t.Fatalf("logged events: %d %v", len(logged), err)
	}
	if category, summary := c.classifyMessage(context.Background(), "fixture"); category != "APPOINTMENT" || summary != "Meet tomorrow" {
		t.Fatalf("classification: %s %s", category, summary)
	}
	prov.body = "invalid"
	if category, _ := c.classifyMessage(context.Background(), "fixture"); category != "ASSISTANCE" {
		t.Fatal("invalid classification fallback")
	}
	prov.err = errors.New("offline")
	if category, _ := c.classifyMessage(context.Background(), "fixture"); category != "ASSISTANCE" {
		t.Fatal("error fallback")
	}
	c.eventHandler("unrelated")
	if err := c.Start(context.Background()); err != nil {
		t.Fatal("disabled start", err)
	}
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(context.Background(), &bus.OutboundMessage{}); err == nil {
		t.Fatal("uninitialized client accepted")
	}
	c.timeline = nil
	c.logEvent("id", "trace", "sender", "TEXT", "content", "", "category", true)
	for _, n := range []int{0, 1, 8} {
		duration := time.Until(deliveryBackoff(n))
		if duration < 29*time.Second || duration > 301*time.Second {
			t.Fatal("backoff outside bounds")
		}
	}
}

func TestBridgeChannelsDispatchLifecycle(t *testing.T) {
	for _, name := range []string{"slack", "msteams"} {
		for _, status := range []int{200, 403, 503} {
			t.Run(fmt.Sprint(name, status), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
				defer server.Close()
				tl := newTestTimeline(t)
				defer tl.Close()
				b := bus.NewMessageBus()
				var c Channel
				if name == "slack" {
					c = NewSlackChannel(config.SlackConfig{Enabled: true, OutboundURL: server.URL}, b, tl)
				} else {
					c = NewMSTeamsChannel(config.MSTeamsConfig{Enabled: true, OutboundURL: server.URL}, b, tl)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan struct{})
				go func() { defer close(done); b.DispatchOutbound(ctx) }()
				defer func() { cancel(); <-done }()
				if err := c.Start(ctx); err != nil {
					t.Fatal(err)
				}
				defer c.Stop()
				task := &timeline.AgentTask{TaskID: "delivery", Channel: name, ChatID: "chat", Status: "pending"}
				if _, err := tl.CreateTask(task); err != nil {
					t.Fatal(err)
				}
				b.PublishOutbound(&bus.OutboundMessage{Channel: name, ChatID: "chat", TaskID: "delivery", Content: "fixture"})
				want := timeline.DeliverySent
				if status == 403 {
					want = timeline.DeliveryFailed
				} else if status == 503 {
					want = timeline.DeliveryPending
				}
				deadline := time.Now().Add(time.Second)
				for time.Now().Before(deadline) {
					got, err := tl.GetTask("delivery")
					if err == nil && got.DeliveryStatus == want && (status != 503 || got.DeliveryNextAt != nil) {
						return
					}
					time.Sleep(time.Millisecond)
				}
				t.Fatalf("delivery status did not reach %s", want)
			})
		}
	}
}
