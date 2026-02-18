package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/bus"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

// SlackChannel is a Slack transport scaffold with policy + pairing integration.
type SlackChannel struct {
	BaseChannel
	config   config.SlackConfig
	timeline *timeline.TimelineService
}

func NewSlackChannel(cfg config.SlackConfig, messageBus *bus.MessageBus, tl *timeline.TimelineService) *SlackChannel {
	return &SlackChannel{
		BaseChannel: BaseChannel{Bus: messageBus},
		config:      cfg,
		timeline:    tl,
	}
}

func (c *SlackChannel) Name() string { return "slack" }

func (c *SlackChannel) Start(ctx context.Context) error {
	if !c.config.Enabled {
		return nil
	}
	c.Bus.Subscribe(c.Name(), func(msg *bus.OutboundMessage) {
		if err := c.Send(ctx, msg); err != nil {
			if c.timeline != nil && strings.TrimSpace(msg.TaskID) != "" {
				reason, cls := classifyDeliveryError(err)
				if cls == deliveryTransient {
					next := time.Now().Add(30 * time.Second)
					_ = c.timeline.UpdateTaskDeliveryWithReason(msg.TaskID, timeline.DeliveryPending, &next, reason)
				} else {
					_ = c.timeline.UpdateTaskDeliveryWithReason(msg.TaskID, timeline.DeliveryFailed, nil, reason)
				}
			}
			return
		}
		if c.timeline != nil && strings.TrimSpace(msg.TaskID) != "" {
			_ = c.timeline.UpdateTaskDeliveryWithReason(msg.TaskID, timeline.DeliverySent, nil, "")
		}
	})
	return nil
}

func (c *SlackChannel) Stop() error { return nil }

func (c *SlackChannel) Send(ctx context.Context, msg *bus.OutboundMessage) error {
	accountID, chatID := parseAccountChat(strings.TrimSpace(msg.ChatID))
	ac := c.slackAccountConfig(accountID)
	if strings.TrimSpace(ac.OutboundURL) == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"channel":             "slack",
		"account_id":          accountID,
		"chat_id":             strings.TrimSpace(chatID),
		"thread_id":           strings.TrimSpace(msg.ThreadID),
		"native_streaming":    slackNativeStreamingOrDefault(ac.NativeStreaming, c.config.NativeStreaming),
		"stream_mode":         strings.TrimSpace(ac.StreamMode),
		"stream_chunk_chars":  ac.StreamChunkChars,
		"content":             msg.Content,
		"media_urls":          msg.MediaURLs,
		"card":                msg.Card,
		"action":              strings.TrimSpace(msg.Action),
		"action_params":       msg.ActionParams,
		"poll_question":       strings.TrimSpace(msg.PollQuestion),
		"poll_options":        msg.PollOptions,
		"poll_max_selections": msg.PollMaxSelections,
		"trace_id":            msg.TraceID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ac.OutboundURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if tok := strings.TrimSpace(ac.BotToken); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack outbound bridge status: %d", resp.StatusCode)
	}
	return nil
}

func (c *SlackChannel) HandleInbound(senderID, chatID, threadID, messageID, text string, isGroup, wasMentioned bool) error {
	return c.HandleInboundWithAccountAndHints("default", senderID, chatID, threadID, messageID, text, isGroup, wasMentioned, 0, 0)
}

func (c *SlackChannel) HandleInboundWithAccount(accountID, senderID, chatID, threadID, messageID, text string, isGroup, wasMentioned bool) error {
	return c.HandleInboundWithAccountAndHints(accountID, senderID, chatID, threadID, messageID, text, isGroup, wasMentioned, 0, 0)
}

func (c *SlackChannel) HandleInboundWithAccountAndHints(accountID, senderID, chatID, threadID, messageID, text string, isGroup, wasMentioned bool, historyLimit, dmHistoryLimit int) error {
	ac := c.slackAccountConfig(accountID)
	decision := EvaluateAccess(AccessContext{
		SenderID:     senderID,
		IsGroup:      isGroup,
		WasMentioned: wasMentioned,
	}, AccessConfig{
		Channel:        c.Name(),
		AllowFrom:      ac.AllowFrom,
		GroupAllowFrom: ac.AllowFrom,
		DmPolicy:       ac.DmPolicy,
		GroupPolicy:    ac.GroupPolicy,
		RequireMention: ac.RequireMention && isGroup,
	})
	if decision.RequiresPairing {
		if c.timeline == nil {
			return nil
		}
		svc := NewPairingService(c.timeline)
		pending, err := svc.CreateOrGetPending(c.Name(), senderID, 0)
		if err != nil {
			return err
		}
		c.Bus.PublishOutbound(&bus.OutboundMessage{
			Channel: c.Name(),
			ChatID:  withAccountChat(accountID, chatID),
			Content: BuildPairingReply(c.Name(), fmt.Sprintf("Slack user: %s", strings.TrimSpace(senderID)), pending.Code),
		})
		return nil
	}
	if !decision.Allowed {
		return nil
	}
	scopedChatID := withAccountChat(accountID, chatID)
	metadata := map[string]any{
		bus.MetaKeyMessageType: bus.MessageTypeExternal,
		// Isolation boundary is channel + account + conversation/chat room.
		bus.MetaKeySessionScope:   buildSessionScope(c.Name(), accountID, chatID, threadID, senderID, ac.SessionScope),
		bus.MetaKeyChannelAccount: accountIDOrDefault(accountID),
	}
	if historyLimit > 0 {
		metadata["history_limit"] = historyLimit
	}
	if dmHistoryLimit > 0 {
		metadata["dm_history_limit"] = dmHistoryLimit
	}
	c.Bus.PublishInbound(&bus.InboundMessage{
		Channel:   c.Name(),
		SenderID:  strings.TrimSpace(senderID),
		ChatID:    strings.TrimSpace(scopedChatID),
		ThreadID:  strings.TrimSpace(threadID),
		MessageID: strings.TrimSpace(messageID),
		Content:   text,
		Metadata:  metadata,
	})
	return nil
}

func (c *SlackChannel) slackAccountConfig(accountID string) config.SlackAccountConfig {
	base := config.SlackAccountConfig{
		ID:               "default",
		Enabled:          c.config.Enabled,
		BotToken:         c.config.BotToken,
		AppToken:         c.config.AppToken,
		InboundToken:     c.config.InboundToken,
		OutboundURL:      c.config.OutboundURL,
		NativeStreaming:  boolPtr(c.config.NativeStreaming),
		StreamMode:       c.config.StreamMode,
		StreamChunkChars: c.config.StreamChunkChars,
		SessionScope:     c.config.SessionScope,
		AllowFrom:        c.config.AllowFrom,
		DmPolicy:         c.config.DmPolicy,
		GroupPolicy:      c.config.GroupPolicy,
		RequireMention:   c.config.RequireMention,
	}
	id := accountIDOrDefault(accountID)
	if id == "default" {
		return base
	}
	for _, acct := range c.config.Accounts {
		if strings.EqualFold(strings.TrimSpace(acct.ID), id) {
			res := acct
			if strings.TrimSpace(res.ID) == "" {
				res.ID = id
			}
			if strings.TrimSpace(res.BotToken) == "" {
				res.BotToken = base.BotToken
			}
			if strings.TrimSpace(res.AppToken) == "" {
				res.AppToken = base.AppToken
			}
			if strings.TrimSpace(res.InboundToken) == "" {
				res.InboundToken = base.InboundToken
			}
			if strings.TrimSpace(res.OutboundURL) == "" {
				res.OutboundURL = base.OutboundURL
			}
			if res.NativeStreaming == nil {
				res.NativeStreaming = base.NativeStreaming
			}
			if strings.TrimSpace(res.StreamMode) == "" {
				res.StreamMode = base.StreamMode
			}
			if res.StreamChunkChars <= 0 {
				res.StreamChunkChars = base.StreamChunkChars
			}
			if strings.TrimSpace(res.SessionScope) == "" {
				res.SessionScope = base.SessionScope
			}
			if len(res.AllowFrom) == 0 {
				res.AllowFrom = base.AllowFrom
			}
			if strings.TrimSpace(string(res.DmPolicy)) == "" {
				res.DmPolicy = base.DmPolicy
			}
			if strings.TrimSpace(string(res.GroupPolicy)) == "" {
				res.GroupPolicy = base.GroupPolicy
			}
			return res
		}
	}
	return base
}

func boolPtr(v bool) *bool { return &v }

func slackNativeStreamingOrDefault(v *bool, fallback bool) bool {
	if v != nil {
		return *v
	}
	return fallback
}
