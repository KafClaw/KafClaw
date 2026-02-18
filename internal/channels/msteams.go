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

// MSTeamsChannel is a Teams transport scaffold with policy + pairing integration.
type MSTeamsChannel struct {
	BaseChannel
	config   config.MSTeamsConfig
	timeline *timeline.TimelineService
}

func NewMSTeamsChannel(cfg config.MSTeamsConfig, messageBus *bus.MessageBus, tl *timeline.TimelineService) *MSTeamsChannel {
	return &MSTeamsChannel{
		BaseChannel: BaseChannel{Bus: messageBus},
		config:      cfg,
		timeline:    tl,
	}
}

func (c *MSTeamsChannel) Name() string { return "msteams" }

func (c *MSTeamsChannel) Start(ctx context.Context) error {
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

func (c *MSTeamsChannel) Stop() error { return nil }

func (c *MSTeamsChannel) Send(ctx context.Context, msg *bus.OutboundMessage) error {
	accountID, chatID := parseAccountChat(strings.TrimSpace(msg.ChatID))
	ac := c.teamsAccountConfig(accountID)
	if strings.TrimSpace(ac.OutboundURL) == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"channel":             "msteams",
		"account_id":          accountID,
		"chat_id":             strings.TrimSpace(chatID),
		"thread_id":           strings.TrimSpace(msg.ThreadID),
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
	if tok := strings.TrimSpace(ac.AppPassword); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("msteams outbound bridge status: %d", resp.StatusCode)
	}
	return nil
}

func (c *MSTeamsChannel) HandleInbound(senderID, chatID, threadID, messageID, text string, isGroup, wasMentioned bool) error {
	return c.HandleInboundWithContextAndHints("default", senderID, chatID, threadID, messageID, text, isGroup, wasMentioned, "", "", 0, 0)
}

func (c *MSTeamsChannel) HandleInboundWithAccount(accountID, senderID, chatID, threadID, messageID, text string, isGroup, wasMentioned bool) error {
	return c.HandleInboundWithContextAndHints(accountID, senderID, chatID, threadID, messageID, text, isGroup, wasMentioned, "", "", 0, 0)
}

func (c *MSTeamsChannel) HandleInboundWithContext(accountID, senderID, chatID, threadID, messageID, text string, isGroup, wasMentioned bool, groupID, channelID string) error {
	return c.HandleInboundWithContextAndHints(accountID, senderID, chatID, threadID, messageID, text, isGroup, wasMentioned, groupID, channelID, 0, 0)
}

func (c *MSTeamsChannel) HandleInboundWithContextAndHints(accountID, senderID, chatID, threadID, messageID, text string, isGroup, wasMentioned bool, groupID, channelID string, historyLimit, dmHistoryLimit int) error {
	ac := c.teamsAccountConfig(accountID)
	targetAllowlistMode := isGroup && (ac.GroupPolicy == config.GroupPolicyAllowlist || strings.TrimSpace(string(ac.GroupPolicy)) == "") && hasTeamsGroupTargetEntries(ac.GroupAllowFrom)
	groupAllowFrom := ac.GroupAllowFrom
	if targetAllowlistMode {
		// In target-allowlist mode, group access is gated by team/channel mapping,
		// not sender identity matching.
		groupAllowFrom = []string{"*"}
	}
	decision := EvaluateAccess(AccessContext{
		SenderID:     senderID,
		IsGroup:      isGroup,
		WasMentioned: wasMentioned,
	}, AccessConfig{
		Channel:        c.Name(),
		AllowFrom:      ac.AllowFrom,
		GroupAllowFrom: groupAllowFrom,
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
			Content: BuildPairingReply(c.Name(), fmt.Sprintf("MSTeams user: %s", strings.TrimSpace(senderID)), pending.Code),
		})
		return nil
	}
	if !decision.Allowed {
		return nil
	}
	if isGroup && (ac.GroupPolicy == config.GroupPolicyAllowlist || strings.TrimSpace(string(ac.GroupPolicy)) == "") {
		if enforced, allowed := matchTeamsGroupTargetAllowlist(ac.GroupAllowFrom, groupID, channelID); enforced && !allowed {
			return nil
		}
	}
	scopedChatID := withAccountChat(accountID, chatID)
	metadata := map[string]any{
		bus.MetaKeyMessageType: bus.MessageTypeExternal,
		// Isolation boundary is channel + account + conversation/chat room.
		bus.MetaKeySessionScope:   buildSessionScope(c.Name(), accountID, chatID, threadID, senderID, ac.SessionScope),
		bus.MetaKeyChannelAccount: accountIDOrDefault(accountID),
		"group_id":                strings.TrimSpace(groupID),
		"channel_id":              strings.TrimSpace(channelID),
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

func matchTeamsGroupTargetAllowlist(entries []string, groupID, channelID string) (enforced bool, allowed bool) {
	groupID = strings.ToLower(strings.TrimSpace(groupID))
	channelID = strings.ToLower(strings.TrimSpace(channelID))
	targetEntries := 0
	for _, raw := range entries {
		team, channel, isTarget := parseTeamsGroupAllowEntry(raw)
		if !isTarget {
			continue
		}
		targetEntries++
		team = strings.ToLower(strings.TrimSpace(team))
		channel = strings.ToLower(strings.TrimSpace(channel))
		teamMatch := team == "" || team == "*" || (groupID != "" && groupID == team)
		channelMatch := channel == "" || channel == "*" || (channelID != "" && channelID == channel)
		if teamMatch && channelMatch {
			return true, true
		}
	}
	if targetEntries == 0 {
		return false, true
	}
	return true, false
}

func hasTeamsGroupTargetEntries(entries []string) bool {
	for _, raw := range entries {
		if _, _, ok := parseTeamsGroupAllowEntry(raw); ok {
			return true
		}
	}
	return false
}

func parseTeamsGroupAllowEntry(raw string) (teamID, channelID string, isTarget bool) {
	v := strings.TrimSpace(strings.ToLower(raw))
	if v == "" {
		return "", "", false
	}
	v = strings.TrimPrefix(v, "msteams:")
	v = strings.TrimPrefix(v, "group:")
	if strings.HasPrefix(v, "team:") {
		v = strings.TrimPrefix(v, "team:")
		if strings.Contains(v, "/channel:") {
			parts := strings.SplitN(v, "/channel:", 2)
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
		}
		return strings.TrimSpace(v), "", true
	}
	if strings.HasPrefix(v, "channel:") {
		return "", strings.TrimSpace(strings.TrimPrefix(v, "channel:")), true
	}
	if strings.Contains(v, "/") {
		parts := strings.SplitN(v, "/", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
	}
	return "", "", false
}

func (c *MSTeamsChannel) teamsAccountConfig(accountID string) config.MSTeamsAccountConfig {
	base := config.MSTeamsAccountConfig{
		ID:             "default",
		Enabled:        c.config.Enabled,
		AppID:          c.config.AppID,
		AppPassword:    c.config.AppPassword,
		TenantID:       c.config.TenantID,
		InboundToken:   c.config.InboundToken,
		OutboundURL:    c.config.OutboundURL,
		SessionScope:   c.config.SessionScope,
		AllowFrom:      c.config.AllowFrom,
		GroupAllowFrom: c.config.GroupAllowFrom,
		DmPolicy:       c.config.DmPolicy,
		GroupPolicy:    c.config.GroupPolicy,
		RequireMention: c.config.RequireMention,
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
			if strings.TrimSpace(res.AppID) == "" {
				res.AppID = base.AppID
			}
			if strings.TrimSpace(res.AppPassword) == "" {
				res.AppPassword = base.AppPassword
			}
			if strings.TrimSpace(res.TenantID) == "" {
				res.TenantID = base.TenantID
			}
			if strings.TrimSpace(res.InboundToken) == "" {
				res.InboundToken = base.InboundToken
			}
			if strings.TrimSpace(res.OutboundURL) == "" {
				res.OutboundURL = base.OutboundURL
			}
			if strings.TrimSpace(res.SessionScope) == "" {
				res.SessionScope = base.SessionScope
			}
			if len(res.AllowFrom) == 0 {
				res.AllowFrom = base.AllowFrom
			}
			if len(res.GroupAllowFrom) == 0 {
				res.GroupAllowFrom = base.GroupAllowFrom
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
