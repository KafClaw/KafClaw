package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type config struct {
	ListenAddr string

	KafclawBase string

	KafclawSlackInboundToken   string
	KafclawMSTeamsInboundToken string

	SlackBotToken      string
	SlackAppToken      string
	SlackAccountID     string
	SlackReplyMode     string
	SlackBotUserID     string
	SlackSigningSecret string
	SlackAPIBase       string

	MSTeamsAppID         string
	MSTeamsAppPassword   string
	MSTeamsAccountID     string
	MSTeamsReplyMode     string
	MSTeamsTenantID      string
	MSTeamsInboundBearer string
	MSTeamsOpenIDConfig  string
	MSTeamsAPIBase       string
	MSTeamsGraphBase     string

	StatePath string
}

type bridge struct {
	cfg    config
	client *http.Client
	jwt    *teamsJWTVerifier

	teamsMu           sync.RWMutex
	teamsConvByID     map[string]teamsConversationRef
	teamsConvByUserID map[string]teamsConversationRef
	teamsToken        tokenCache
	teamsGraphToken   tokenCache

	inboundMu   sync.Mutex
	inboundSeen map[string]time.Time
	inboundTTL  time.Duration

	pollMu     sync.Mutex
	teamsPolls map[string]map[string]any
	replyMu    sync.Mutex
	replySeen  map[string]bool

	metricsMu sync.RWMutex
	metrics   bridgeMetrics
}

type bridgeMetrics struct {
	StartedAt time.Time `json:"started_at"`

	SlackInboundForwarded int `json:"slack_inbound_forwarded"`
	SlackOutboundSent     int `json:"slack_outbound_sent"`

	TeamsInboundForwarded int `json:"teams_inbound_forwarded"`
	TeamsOutboundSent     int `json:"teams_outbound_sent"`

	InboundForwardErrors int `json:"inbound_forward_errors"`
	OutboundErrors       int `json:"outbound_errors"`
	SlackInboundDeduped  int `json:"slack_inbound_deduped"`
	TeamsInboundDeduped  int `json:"teams_inbound_deduped"`
	InboundAuthRejected  int `json:"inbound_auth_rejected"`

	LastError   string `json:"last_error,omitempty"`
	LastErrorAt string `json:"last_error_at,omitempty"`
}

type teamsJWTVerifier struct {
	client *http.Client
	cfgURL string
	appID  string

	mu         sync.Mutex
	issuer     string
	jwksURI    string
	keysByKid  map[string]*rsa.PublicKey
	cacheUntil time.Time
}

type tokenCache struct {
	accessToken string
	expiresAt   time.Time
}

type teamsConversationRef struct {
	ServiceURL     string `json:"service_url"`
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
}

type bridgeState struct {
	TeamsConvByID     map[string]teamsConversationRef `json:"teams_conv_by_id"`
	TeamsConvByUserID map[string]teamsConversationRef `json:"teams_conv_by_user_id"`
	InboundSeen       map[string]time.Time            `json:"inbound_seen,omitempty"`
	TeamsPolls        map[string]map[string]any       `json:"teams_polls,omitempty"`
}

func main() {
	cfg := loadConfig()
	httpClient := &http.Client{Timeout: 20 * time.Second}
	b := &bridge{
		cfg:               cfg,
		client:            httpClient,
		jwt:               newTeamsJWTVerifier(httpClient, cfg.MSTeamsOpenIDConfig, cfg.MSTeamsAppID),
		teamsConvByID:     map[string]teamsConversationRef{},
		teamsConvByUserID: map[string]teamsConversationRef{},
		inboundSeen:       map[string]time.Time{},
		inboundTTL:        10 * time.Minute,
		teamsPolls:        map[string]map[string]any{},
		replySeen:         map[string]bool{},
		metrics: bridgeMetrics{
			StartedAt: time.Now().UTC(),
		},
	}
	if err := b.loadState(); err != nil {
		log.Printf("channelbridge state load warning: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	mux.HandleFunc("/status", b.handleStatus)
	mux.HandleFunc("/slack/events", b.handleSlackEvents)
	mux.HandleFunc("/slack/commands", b.handleSlackCommands)
	mux.HandleFunc("/slack/interactions", b.handleSlackInteractions)
	mux.HandleFunc("/slack/outbound", b.handleSlackOutbound)
	mux.HandleFunc("/slack/resolve/users", b.handleSlackResolveUsers)
	mux.HandleFunc("/slack/resolve/channels", b.handleSlackResolveChannels)
	mux.HandleFunc("/slack/probe", b.handleSlackProbe)
	mux.HandleFunc("/teams/messages", b.handleTeamsMessages)
	mux.HandleFunc("/teams/outbound", b.handleTeamsOutbound)
	mux.HandleFunc("/teams/resolve/users", b.handleTeamsResolveUsers)
	mux.HandleFunc("/teams/resolve/channels", b.handleTeamsResolveChannels)
	mux.HandleFunc("/teams/probe", b.handleTeamsProbe)
	b.startSlackSocketMode()

	log.Printf("channelbridge listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		log.Fatalf("channelbridge failed: %v", err)
	}
}

func loadConfig() config {
	defaultState := ".kafclaw/channelbridge/state.json"
	if home, err := os.UserHomeDir(); err == nil {
		defaultState = filepath.Join(home, defaultState)
	}
	return config{
		ListenAddr: strings.TrimSpace(getEnvDefault("CHANNEL_BRIDGE_ADDR", ":18888")),

		KafclawBase: strings.TrimSpace(getEnvDefault("KAFCLAW_BASE_URL", "http://127.0.0.1:18791")),

		KafclawSlackInboundToken:   strings.TrimSpace(os.Getenv("KAFCLAW_SLACK_INBOUND_TOKEN")),
		KafclawMSTeamsInboundToken: strings.TrimSpace(os.Getenv("KAFCLAW_MSTEAMS_INBOUND_TOKEN")),

		SlackBotToken:      strings.TrimSpace(os.Getenv("SLACK_BOT_TOKEN")),
		SlackAppToken:      strings.TrimSpace(os.Getenv("SLACK_APP_TOKEN")),
		SlackAccountID:     strings.TrimSpace(getEnvDefault("SLACK_ACCOUNT_ID", "default")),
		SlackReplyMode:     strings.TrimSpace(getEnvDefault("SLACK_REPLY_MODE", "all")),
		SlackBotUserID:     strings.TrimSpace(os.Getenv("SLACK_BOT_USER_ID")),
		SlackSigningSecret: strings.TrimSpace(os.Getenv("SLACK_SIGNING_SECRET")),
		SlackAPIBase:       strings.TrimSpace(getEnvDefault("SLACK_API_BASE", "https://slack.com/api")),

		MSTeamsAppID:         strings.TrimSpace(os.Getenv("MSTEAMS_APP_ID")),
		MSTeamsAppPassword:   strings.TrimSpace(os.Getenv("MSTEAMS_APP_PASSWORD")),
		MSTeamsAccountID:     strings.TrimSpace(getEnvDefault("MSTEAMS_ACCOUNT_ID", "default")),
		MSTeamsReplyMode:     strings.TrimSpace(getEnvDefault("MSTEAMS_REPLY_MODE", "all")),
		MSTeamsTenantID:      strings.TrimSpace(getEnvDefault("MSTEAMS_TENANT_ID", "botframework.com")),
		MSTeamsInboundBearer: strings.TrimSpace(os.Getenv("MSTEAMS_INBOUND_BEARER")),
		MSTeamsOpenIDConfig:  strings.TrimSpace(getEnvDefault("MSTEAMS_OPENID_CONFIG", "https://login.botframework.com/v1/.well-known/openidconfiguration")),
		MSTeamsAPIBase:       strings.TrimSpace(getEnvDefault("MSTEAMS_API_BASE", "")),
		MSTeamsGraphBase:     strings.TrimSpace(getEnvDefault("MSTEAMS_GRAPH_BASE", "https://graph.microsoft.com/v1.0")),

		StatePath: strings.TrimSpace(getEnvDefault("CHANNEL_BRIDGE_STATE", defaultState)),
	}
}

func getEnvDefault(k, d string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	return v
}

func (b *bridge) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	b.metricsMu.RLock()
	metrics := b.metrics
	b.metricsMu.RUnlock()

	b.teamsMu.RLock()
	convCount := len(b.teamsConvByID)
	userCount := len(b.teamsConvByUserID)
	hasToken := b.teamsToken.accessToken != "" && time.Until(b.teamsToken.expiresAt) > 0
	b.teamsMu.RUnlock()

	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"metrics": metrics,
		"teams": map[string]any{
			"conversation_refs":       convCount,
			"user_refs":               userCount,
			"token_cached":            hasToken,
			"inbound_bearer_required": strings.TrimSpace(b.cfg.MSTeamsInboundBearer) != "",
		},
		"inbound_dedupe_cache": b.inboundCacheSize(),
	})
}

func (b *bridge) inboundCacheSize() int {
	b.inboundMu.Lock()
	defer b.inboundMu.Unlock()
	b.pruneInboundSeenLocked(time.Now())
	return len(b.inboundSeen)
}

func (b *bridge) noteInboundForward(success bool, err error) {
	b.metricsMu.Lock()
	defer b.metricsMu.Unlock()
	if success {
		return
	}
	b.metrics.InboundForwardErrors++
	if err != nil {
		b.metrics.LastError = err.Error()
		b.metrics.LastErrorAt = time.Now().UTC().Format(time.RFC3339)
	}
}

func (b *bridge) noteOutbound(success bool, isSlack bool, err error) {
	b.metricsMu.Lock()
	defer b.metricsMu.Unlock()
	if success {
		if isSlack {
			b.metrics.SlackOutboundSent++
		} else {
			b.metrics.TeamsOutboundSent++
		}
		return
	}
	b.metrics.OutboundErrors++
	if err != nil {
		b.metrics.LastError = err.Error()
		b.metrics.LastErrorAt = time.Now().UTC().Format(time.RFC3339)
	}
}

func (b *bridge) noteInboundDeduped(isSlack bool) {
	b.metricsMu.Lock()
	defer b.metricsMu.Unlock()
	if isSlack {
		b.metrics.SlackInboundDeduped++
		return
	}
	b.metrics.TeamsInboundDeduped++
}

func (b *bridge) noteInboundAuthRejected() {
	b.metricsMu.Lock()
	defer b.metricsMu.Unlock()
	b.metrics.InboundAuthRejected++
}

func (b *bridge) handleSlackEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if err := verifySlackSignature(body, r, b.cfg.SlackSigningSecret); err != nil {
		http.Error(w, "invalid slack signature", http.StatusUnauthorized)
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	resp, err := b.processSlackEventsPayload(payload)
	if err != nil {
		http.Error(w, "forward failed", http.StatusBadGateway)
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (b *bridge) handleSlackCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if err := verifySlackSignature(body, r, b.cfg.SlackSigningSecret); err != nil {
		http.Error(w, "invalid slack signature", http.StatusUnauthorized)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	cmd, err := slack.SlashCommandParse(r)
	if err != nil {
		http.Error(w, "invalid slash command", http.StatusBadRequest)
		return
	}
	if err := b.forwardSlackSlashCommand(cmd); err != nil {
		http.Error(w, "forward failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"response_type": "ephemeral", "text": "accepted"})
}

func (b *bridge) handleSlackInteractions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if err := verifySlackSignature(body, r, b.cfg.SlackSigningSecret); err != nil {
		http.Error(w, "invalid slack signature", http.StatusUnauthorized)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	cb, err := slack.InteractionCallbackParse(r)
	if err != nil {
		http.Error(w, "invalid interaction payload", http.StatusBadRequest)
		return
	}
	if err := b.forwardSlackInteraction(cb); err != nil {
		http.Error(w, "forward failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func verifySlackSignature(body []byte, r *http.Request, secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	ts := strings.TrimSpace(r.Header.Get("X-Slack-Request-Timestamp"))
	sig := strings.TrimSpace(r.Header.Get("X-Slack-Signature"))
	if ts == "" || sig == "" {
		return errors.New("missing slack signature headers")
	}
	tsNum, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return err
	}
	if delta := time.Since(time.Unix(tsNum, 0)); delta > 5*time.Minute || delta < -5*time.Minute {
		return errors.New("slack signature timestamp out of range")
	}
	base := "v0:" + ts + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return errors.New("slack signature mismatch")
	}
	return nil
}

func (b *bridge) processSlackEventsPayload(payload map[string]any) (map[string]any, error) {
	switch strings.TrimSpace(asString(payload["type"])) {
	case "url_verification":
		return map[string]any{"challenge": asString(payload["challenge"])}, nil
	case "event_callback":
		if eventID := strings.TrimSpace(asString(payload["event_id"])); eventID != "" {
			if b.seenInboundEvent("slack:event:"+eventID, time.Now()) {
				b.noteInboundDeduped(true)
				return map[string]any{"ok": true, "deduped": true}, nil
			}
		}
		event, _ := payload["event"].(map[string]any)
		if event == nil {
			return map[string]any{"ok": true}, nil
		}
		in, ok := normalizeSlackInboundEvent(event, strings.TrimSpace(b.cfg.SlackBotUserID))
		if !ok {
			return map[string]any{"ok": true}, nil
		}
		if err := b.forwardSlackInbound(in.senderID, in.channelID, in.threadID, in.messageID, in.text, in.isGroup, in.wasMentioned); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	default:
		return map[string]any{"ok": true}, nil
	}
}

type slackInbound struct {
	senderID     string
	channelID    string
	threadID     string
	messageID    string
	text         string
	isGroup      bool
	wasMentioned bool
}

func normalizeSlackInboundEvent(event map[string]any, botUserID string) (slackInbound, bool) {
	eventType := strings.TrimSpace(asString(event["type"]))
	if eventType == "app_mention" {
		text := strings.TrimSpace(asString(event["text"]))
		channelID := strings.TrimSpace(asString(event["channel"]))
		senderID := strings.TrimSpace(asString(event["user"]))
		if channelID == "" || senderID == "" {
			return slackInbound{}, false
		}
		return slackInbound{
			senderID:     senderID,
			channelID:    channelID,
			threadID:     strings.TrimSpace(asString(event["thread_ts"])),
			messageID:    firstNonEmpty(asString(event["ts"]), asString(event["event_ts"])),
			text:         text,
			isGroup:      true,
			wasMentioned: true,
		}, true
	}
	if eventType != "message" {
		return slackInbound{}, false
	}

	subtype := strings.TrimSpace(asString(event["subtype"]))
	if strings.TrimSpace(asString(event["bot_id"])) != "" || subtype == "bot_message" {
		return slackInbound{}, false
	}

	msg := event
	prevMsg, _ := event["previous_message"].(map[string]any)
	if subtype == "message_changed" {
		changed, _ := event["message"].(map[string]any)
		if changed != nil {
			msg = changed
		}
	}

	channelID := strings.TrimSpace(firstNonEmpty(asString(event["channel"]), asString(msg["channel"])))
	senderID := strings.TrimSpace(firstNonEmpty(asString(msg["user"]), asString(event["user"]), asString(prevMsg["user"])))
	if channelID == "" || senderID == "" {
		return slackInbound{}, false
	}

	text := strings.TrimSpace(asString(msg["text"]))
	switch subtype {
	case "message_deleted":
		if text == "" {
			text = strings.TrimSpace(asString(prevMsg["text"]))
		}
		if text == "" {
			text = "[message deleted]"
		}
	case "file_share":
		if text == "" {
			text = "[file shared]"
		}
	}
	if text == "" {
		return slackInbound{}, false
	}

	threadID := strings.TrimSpace(firstNonEmpty(asString(msg["thread_ts"]), asString(event["thread_ts"])))
	messageID := strings.TrimSpace(firstNonEmpty(
		asString(msg["ts"]),
		asString(event["deleted_ts"]),
		asString(event["ts"]),
		asString(event["event_ts"]),
	))
	channelType := strings.ToLower(strings.TrimSpace(asString(event["channel_type"])))
	if channelType == "" && strings.HasPrefix(strings.ToUpper(channelID), "D") {
		channelType = "im"
	}
	isGroup := channelType != "im"
	botUserID = strings.TrimSpace(botUserID)
	wasMentioned := botUserID != "" && strings.Contains(text, "<@"+botUserID+">")

	return slackInbound{
		senderID:     senderID,
		channelID:    channelID,
		threadID:     threadID,
		messageID:    messageID,
		text:         text,
		isGroup:      isGroup,
		wasMentioned: wasMentioned,
	}, true
}

func (b *bridge) forwardSlackInbound(senderID, channelID, threadID, messageID, text string, isGroup, wasMentioned bool) error {
	channelID = strings.TrimSpace(channelID)
	senderID = strings.TrimSpace(senderID)
	if channelID == "" || senderID == "" {
		return nil
	}
	if messageID != "" && b.seenInboundEvent("slack:msg:"+channelID+":"+messageID, time.Now()) {
		b.noteInboundDeduped(true)
		return nil
	}
	err := b.postInbound("/api/v1/channels/slack/inbound", b.cfg.KafclawSlackInboundToken, map[string]any{
		"account_id":    strings.TrimSpace(b.cfg.SlackAccountID),
		"sender_id":     senderID,
		"chat_id":       channelID,
		"thread_id":     strings.TrimSpace(threadID),
		"message_id":    strings.TrimSpace(messageID),
		"text":          text,
		"is_group":      isGroup,
		"was_mentioned": wasMentioned,
	})
	if err != nil {
		b.noteInboundForward(false, err)
		log.Printf("slack inbound forward failed: %v", err)
		return err
	}
	b.metricsMu.Lock()
	b.metrics.SlackInboundForwarded++
	b.metricsMu.Unlock()
	return nil
}

func (b *bridge) forwardSlackSlashCommand(cmd slack.SlashCommand) error {
	content := strings.TrimSpace(strings.TrimSpace(cmd.Command) + " " + strings.TrimSpace(cmd.Text))
	isGroup := !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(cmd.ChannelID)), "D")
	return b.forwardSlackInbound(cmd.UserID, cmd.ChannelID, "", cmd.TriggerID, content, isGroup, true)
}

func (b *bridge) forwardSlackInteraction(cb slack.InteractionCallback) error {
	channelID := strings.TrimSpace(cb.Channel.ID)
	if channelID == "" {
		channelID = strings.TrimSpace(cb.Container.ChannelID)
	}
	threadID := strings.TrimSpace(cb.Container.ThreadTs)
	actionID := strings.TrimSpace(cb.ActionID)
	actionVal := strings.TrimSpace(cb.Value)
	if len(cb.ActionCallback.BlockActions) > 0 {
		if actionID == "" {
			actionID = strings.TrimSpace(cb.ActionCallback.BlockActions[0].ActionID)
		}
		if actionVal == "" {
			actionVal = strings.TrimSpace(cb.ActionCallback.BlockActions[0].Value)
		}
	}
	content := strings.TrimSpace("interactive " + actionID + " " + actionVal)
	if content == "interactive" {
		content = "interactive " + strings.TrimSpace(string(cb.Type))
	}
	isGroup := !strings.HasPrefix(strings.ToUpper(channelID), "D")
	messageID := strings.TrimSpace(cb.ActionTs)
	if messageID == "" {
		messageID = strings.TrimSpace(cb.TriggerID)
	}
	return b.forwardSlackInbound(cb.User.ID, channelID, threadID, messageID, content, isGroup, true)
}

func (b *bridge) startSlackSocketMode() {
	appToken := strings.TrimSpace(b.cfg.SlackAppToken)
	if appToken == "" {
		return
	}
	api, err := b.slackClientWithAppToken(appToken)
	if err != nil {
		log.Printf("slack socket mode disabled: %v", err)
		return
	}
	client := socketmode.New(api)
	go b.runSlackSocketMode(client)
}

func (b *bridge) runSlackSocketMode(client *socketmode.Client) {
	go func() {
		for evt := range client.Events {
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				if evt.Request != nil {
					client.Ack(*evt.Request)
				}
				ev, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok || ev.Type != slackevents.CallbackEvent {
					continue
				}
				switch in := ev.InnerEvent.Data.(type) {
				case *slackevents.MessageEvent:
					if in == nil {
						continue
					}
					wasMentioned := false
					if botID := strings.TrimSpace(b.cfg.SlackBotUserID); botID != "" {
						wasMentioned = strings.Contains(in.Text, "<@"+botID+">")
					}
					_ = b.forwardSlackInbound(in.User, in.Channel, in.ThreadTimeStamp, in.TimeStamp, in.Text, in.ChannelType != "im", wasMentioned)
				case *slackevents.AppMentionEvent:
					if in == nil {
						continue
					}
					_ = b.forwardSlackInbound(in.User, in.Channel, in.ThreadTimeStamp, in.TimeStamp, in.Text, true, true)
				}
			case socketmode.EventTypeSlashCommand:
				if evt.Request != nil {
					client.Ack(*evt.Request, map[string]any{"response_type": "ephemeral", "text": "accepted"})
				}
				cmd, ok := evt.Data.(slack.SlashCommand)
				if ok {
					_ = b.forwardSlackSlashCommand(cmd)
				}
			case socketmode.EventTypeInteractive:
				if evt.Request != nil {
					client.Ack(*evt.Request)
				}
				cb, ok := evt.Data.(slack.InteractionCallback)
				if ok {
					_ = b.forwardSlackInteraction(cb)
				}
			}
		}
	}()
	client.Run()
}

func (b *bridge) handleSlackOutbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AccountID         string         `json:"account_id"`
		ChatID            string         `json:"chat_id"`
		ThreadID          string         `json:"thread_id"`
		ReplyMode         string         `json:"reply_mode"`
		Content           string         `json:"content"`
		MediaURLs         []string       `json:"media_urls"`
		Card              map[string]any `json:"card"`
		Action            string         `json:"action"`
		ActionParams      map[string]any `json:"action_params"`
		PollQuestion      string         `json:"poll_question"`
		PollOptions       []string       `json:"poll_options"`
		PollMaxSelections int            `json:"poll_max_selections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ChatID) == "" {
		http.Error(w, "chat_id required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Content) == "" && len(req.MediaURLs) == 0 && len(req.Card) == 0 && strings.TrimSpace(req.Action) == "" {
		http.Error(w, "content, media_urls, card or action required", http.StatusBadRequest)
		return
	}
	accountID := strings.TrimSpace(req.AccountID)
	if accountID == "" {
		accountID = "default"
	}
	threadID := b.resolveReplyThread("slack", accountID, req.ChatID, req.ThreadID, req.ReplyMode, b.cfg.SlackReplyMode)
	channelID, err := b.resolveSlackChannelID(req.ChatID)
	if err != nil {
		b.noteOutbound(false, true, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if act := strings.TrimSpace(strings.ToLower(req.Action)); act != "" {
		result, err := b.slackHandleAction(act, channelID, strings.TrimSpace(threadID), req.Content, req.ActionParams)
		if err != nil {
			b.noteOutbound(false, true, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		b.noteOutbound(true, true, nil)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
		return
	}
	if len(req.MediaURLs) > 0 {
		if err := b.slackUploadMedia(channelID, threadID, req.MediaURLs[0], req.Content); err != nil {
			b.noteOutbound(false, true, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	if len(req.Card) > 0 {
		if err := b.slackPostCard(channelID, threadID, req.Content, req.Card); err != nil {
			b.noteOutbound(false, true, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	} else if strings.TrimSpace(req.Content) != "" {
		if err := b.slackPostMessage(channelID, threadID, req.Content); err != nil {
			b.noteOutbound(false, true, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	b.noteOutbound(true, true, nil)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (b *bridge) handleSlackResolveUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Entries []string `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	out, err := b.slackResolveUsers(req.Entries)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
}

func (b *bridge) handleSlackResolveChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Entries []string `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	out, err := b.slackResolveChannels(req.Entries)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
}

func (b *bridge) handleSlackProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	api, err := b.slackClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auth, err := api.AuthTestContext(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"team": auth.Team,
		"user": auth.User,
	})
}

func (b *bridge) resolveSlackChannelID(chatID string) (string, error) {
	chatID = normalizeSlackTarget(chatID)
	if chatID == "" {
		return "", errors.New("empty chat id")
	}
	if strings.HasPrefix(chatID, "C") || strings.HasPrefix(chatID, "G") || strings.HasPrefix(chatID, "D") {
		return chatID, nil
	}
	if !strings.HasPrefix(chatID, "U") {
		return chatID, nil
	}
	api, err := b.slackClient()
	if err != nil {
		return "", err
	}
	var channelID string
	err = withRetry(3, 200*time.Millisecond, func() (bool, error) {
		ch, _, _, err := api.OpenConversationContext(context.Background(), &slack.OpenConversationParameters{
			Users: []string{chatID},
		})
		if err == nil && ch != nil && strings.TrimSpace(ch.ID) != "" {
			channelID = strings.TrimSpace(ch.ID)
			return false, nil
		}
		return b.slackRetryDecision(err)
	})
	if err != nil {
		return "", err
	}
	return channelID, nil
}

func normalizeSlackTarget(v string) string {
	s := strings.TrimSpace(v)
	l := strings.ToLower(s)
	switch {
	case strings.HasPrefix(l, "user:"):
		return strings.TrimSpace(s[len("user:"):])
	case strings.HasPrefix(l, "slack:user:"):
		return strings.TrimSpace(s[len("slack:user:"):])
	case strings.HasPrefix(l, "channel:"):
		return strings.TrimSpace(s[len("channel:"):])
	case strings.HasPrefix(l, "slack:channel:"):
		return strings.TrimSpace(s[len("slack:channel:"):])
	default:
		return s
	}
}

func (b *bridge) slackResolveUsers(entries []string) ([]map[string]any, error) {
	users, err := b.slackListUsers()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(entries))
	for _, raw := range entries {
		q := strings.TrimSpace(raw)
		if q == "" {
			out = append(out, map[string]any{"input": raw, "resolved": false, "note": "empty input"})
			continue
		}
		qNorm := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(q), "user:"), "@")
		if strings.HasPrefix(strings.ToUpper(q), "U") {
			out = append(out, map[string]any{"input": raw, "resolved": true, "id": strings.ToUpper(q)})
			continue
		}
		resolved := false
		id := ""
		name := ""
		for _, u := range users {
			uid := strings.TrimSpace(asString(u["id"]))
			uname := strings.ToLower(strings.TrimSpace(asString(u["name"])))
			real := strings.ToLower(strings.TrimSpace(asString(u["real_name"])))
			display := ""
			if prof, ok := u["profile"].(map[string]any); ok {
				display = strings.ToLower(strings.TrimSpace(asString(prof["display_name"])))
			}
			if qNorm == uname || qNorm == real || qNorm == display {
				resolved = true
				id = uid
				name = asString(u["name"])
				break
			}
		}
		entry := map[string]any{"input": raw, "resolved": resolved}
		if resolved {
			entry["id"] = id
			if name != "" {
				entry["name"] = name
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

func (b *bridge) slackResolveChannels(entries []string) ([]map[string]any, error) {
	chs, err := b.slackListChannels()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(entries))
	for _, raw := range entries {
		q := strings.TrimSpace(raw)
		if q == "" {
			out = append(out, map[string]any{"input": raw, "resolved": false, "note": "empty input"})
			continue
		}
		qNorm := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(q), "channel:"), "#")
		if strings.HasPrefix(strings.ToUpper(q), "C") || strings.HasPrefix(strings.ToUpper(q), "G") {
			out = append(out, map[string]any{"input": raw, "resolved": true, "id": strings.ToUpper(q)})
			continue
		}
		resolved := false
		id := ""
		name := ""
		for _, c := range chs {
			cid := strings.TrimSpace(asString(c["id"]))
			cname := strings.ToLower(strings.TrimSpace(asString(c["name"])))
			if qNorm == cname {
				resolved = true
				id = cid
				name = asString(c["name"])
				break
			}
		}
		entry := map[string]any{"input": raw, "resolved": resolved}
		if resolved {
			entry["id"] = id
			if name != "" {
				entry["name"] = name
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

func (b *bridge) slackListUsers() ([]map[string]any, error) {
	api, err := b.slackClient()
	if err != nil {
		return nil, err
	}
	users, err := api.GetUsersContext(context.Background(), slack.GetUsersOptionLimit(200))
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, map[string]any{
			"id":        u.ID,
			"name":      u.Name,
			"real_name": u.RealName,
			"profile": map[string]any{
				"display_name": u.Profile.DisplayName,
			},
		})
	}
	return out, nil
}

func (b *bridge) slackListChannels() ([]map[string]any, error) {
	api, err := b.slackClient()
	if err != nil {
		return nil, err
	}
	all := make([]map[string]any, 0)
	cursor := ""
	for {
		chs, next, err := api.GetConversationsContext(context.Background(), &slack.GetConversationsParameters{
			Cursor: cursor,
			Limit:  200,
			Types:  []string{"public_channel", "private_channel"},
		})
		if err != nil {
			return nil, err
		}
		for _, ch := range chs {
			all = append(all, map[string]any{
				"id":   ch.ID,
				"name": ch.Name,
			})
		}
		cursor = strings.TrimSpace(next)
		if cursor == "" {
			break
		}
	}
	return all, nil
}

func (b *bridge) slackPostMessage(channelID, threadID, text string) error {
	api, err := b.slackClient()
	if err != nil {
		return err
	}
	return withRetry(3, 200*time.Millisecond, func() (bool, error) {
		opts := []slack.MsgOption{slack.MsgOptionText(text, false)}
		if ts := strings.TrimSpace(threadID); ts != "" {
			opts = append(opts, slack.MsgOptionTS(ts))
		}
		_, _, err := api.PostMessageContext(context.Background(), channelID, opts...)
		return b.slackRetryDecision(err)
	})
}

func (b *bridge) slackPostCard(channelID, threadID, text string, card map[string]any) error {
	api, err := b.slackClient()
	if err != nil {
		return err
	}
	var blocks slack.Blocks
	if rawBlocks, ok := card["blocks"]; ok && rawBlocks != nil {
		blob, _ := json.Marshal(rawBlocks)
		_ = json.Unmarshal(blob, &blocks)
	}
	return withRetry(3, 200*time.Millisecond, func() (bool, error) {
		opts := []slack.MsgOption{slack.MsgOptionText(strings.TrimSpace(text), false)}
		if len(blocks.BlockSet) > 0 {
			opts = append(opts, slack.MsgOptionBlocks(blocks.BlockSet...))
		}
		if ts := strings.TrimSpace(threadID); ts != "" {
			opts = append(opts, slack.MsgOptionTS(ts))
		}
		_, _, err := api.PostMessageContext(context.Background(), channelID, opts...)
		return b.slackRetryDecision(err)
	})
}

func (b *bridge) slackHandleAction(action, channelID, threadID, content string, params map[string]any) (map[string]any, error) {
	api, err := b.slackClient()
	if err != nil {
		return nil, err
	}

	switch action {
	case "react":
		emoji := strings.TrimSpace(asString(params["emoji"]))
		msgTS := strings.TrimSpace(asString(params["message_id"]))
		if emoji == "" || msgTS == "" {
			return nil, errors.New("react requires action_params.emoji and action_params.message_id")
		}
		if err := api.AddReactionContext(context.Background(), emoji, slack.ItemRef{Channel: channelID, Timestamp: msgTS}); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "edit":
		msgTS := strings.TrimSpace(asString(params["message_id"]))
		text := strings.TrimSpace(content)
		if text == "" {
			text = strings.TrimSpace(asString(params["text"]))
		}
		if msgTS == "" || text == "" {
			return nil, errors.New("edit requires action_params.message_id and content/text")
		}
		ch, ts, txt, err := api.UpdateMessageContext(context.Background(), channelID, msgTS, slack.MsgOptionText(text, false))
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "channel": ch, "ts": ts, "text": txt}, nil
	case "delete":
		msgTS := strings.TrimSpace(asString(params["message_id"]))
		if msgTS == "" {
			return nil, errors.New("delete requires action_params.message_id")
		}
		ch, ts, err := api.DeleteMessageContext(context.Background(), channelID, msgTS)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "channel": ch, "ts": ts}, nil
	case "pin":
		msgTS := strings.TrimSpace(asString(params["message_id"]))
		if msgTS == "" {
			msgTS = strings.TrimSpace(threadID)
		}
		if msgTS == "" {
			return nil, errors.New("pin requires action_params.message_id")
		}
		if err := api.AddPinContext(context.Background(), channelID, slack.ItemRef{Channel: channelID, Timestamp: msgTS}); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "unpin":
		msgTS := strings.TrimSpace(asString(params["message_id"]))
		if msgTS == "" {
			return nil, errors.New("unpin requires action_params.message_id")
		}
		if err := api.RemovePinContext(context.Background(), channelID, slack.ItemRef{Channel: channelID, Timestamp: msgTS}); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "read":
		limit := 20
		if n, ok := params["limit"].(float64); ok && int(n) > 0 {
			limit = int(n)
		}
		resp, err := api.GetConversationHistoryContext(context.Background(), &slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Limit:     limit,
		})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"ok":         true,
			"messages":   resp.Messages,
			"has_more":   resp.HasMore,
			"nextCursor": strings.TrimSpace(resp.ResponseMetaData.NextCursor),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported slack action: %s", action)
	}
}

func (b *bridge) slackClient() (*slack.Client, error) {
	token := strings.TrimSpace(b.cfg.SlackBotToken)
	if token == "" {
		return nil, errors.New("missing SLACK_BOT_TOKEN")
	}
	base := strings.TrimSpace(b.cfg.SlackAPIBase)
	if base == "" {
		base = "https://slack.com/api"
	}
	base = strings.TrimRight(base, "/") + "/"
	return slack.New(token, slack.OptionHTTPClient(b.client), slack.OptionAPIURL(base)), nil
}

func (b *bridge) slackClientWithAppToken(appToken string) (*slack.Client, error) {
	token := strings.TrimSpace(b.cfg.SlackBotToken)
	if token == "" {
		return nil, errors.New("missing SLACK_BOT_TOKEN")
	}
	appToken = strings.TrimSpace(appToken)
	if appToken == "" {
		return nil, errors.New("missing SLACK_APP_TOKEN")
	}
	base := strings.TrimSpace(b.cfg.SlackAPIBase)
	if base == "" {
		base = "https://slack.com/api"
	}
	base = strings.TrimRight(base, "/") + "/"
	return slack.New(
		token,
		slack.OptionHTTPClient(b.client),
		slack.OptionAPIURL(base),
		slack.OptionAppLevelToken(appToken),
	), nil
}

func (b *bridge) slackRetryDecision(err error) (bool, error) {
	if err == nil {
		return false, nil
	}
	var rle *slack.RateLimitedError
	if errors.As(err, &rle) && rle != nil {
		if rle.RetryAfter > 0 {
			time.Sleep(rle.RetryAfter)
		}
		return true, err
	}
	return false, err
}

func (b *bridge) slackUploadMedia(channelID, threadID, mediaURL, caption string) error {
	token := strings.TrimSpace(b.cfg.SlackBotToken)
	if token == "" {
		return errors.New("missing SLACK_BOT_TOKEN")
	}
	data, filename, err := b.downloadMedia(mediaURL)
	if err != nil {
		return err
	}
	return withRetry(3, 200*time.Millisecond, func() (bool, error) {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		_ = w.WriteField("channel_id", channelID)
		if strings.TrimSpace(threadID) != "" {
			_ = w.WriteField("thread_ts", strings.TrimSpace(threadID))
		}
		if strings.TrimSpace(caption) != "" {
			_ = w.WriteField("initial_comment", strings.TrimSpace(caption))
		}
		_ = w.WriteField("filename", filename)
		part, err := w.CreateFormFile("file", filename)
		if err != nil {
			return false, err
		}
		if _, err := part.Write(data); err != nil {
			return false, err
		}
		if err := w.Close(); err != nil {
			return false, err
		}

		u := strings.TrimRight(b.cfg.SlackAPIBase, "/") + "/files.uploadV2"
		req, err := http.NewRequest(http.MethodPost, u, &body)
		if err != nil {
			return false, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", w.FormDataContentType())
		resp, err := b.client.Do(req)
		if err != nil {
			return true, err
		}
		defer resp.Body.Close()
		var out struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if out.OK {
			return false, nil
		}
		if d := parseRetryAfter(resp.Header.Get("Retry-After")); d > 0 {
			time.Sleep(d)
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if out.Error == "" {
			out.Error = "files.uploadV2 failed"
		}
		return retryable, errors.New(out.Error)
	})
}

func verifyBearer(r *http.Request, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return got == expected
}

func (b *bridge) verifyTeamsJWTRequest(r *http.Request, serviceURL string) error {
	if b.jwt == nil || strings.TrimSpace(b.cfg.MSTeamsAppID) == "" {
		return nil
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return errors.New("missing authorization header")
	}
	return b.jwt.Verify(auth, time.Now(), strings.TrimSpace(serviceURL))
}

func (b *bridge) handleTeamsMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !verifyBearer(r, b.cfg.MSTeamsInboundBearer) {
		b.noteInboundAuthRejected()
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
		return
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var activity map[string]any
	if err := json.Unmarshal(rawBody, &activity); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := b.verifyTeamsJWTRequest(r, strings.TrimSpace(asString(activity["serviceUrl"]))); err != nil {
		b.noteInboundAuthRejected()
		http.Error(w, "invalid teams jwt", http.StatusUnauthorized)
		return
	}
	if strings.ToLower(asString(activity["type"])) != "message" {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	inbound := normalizeTeamsInbound(activity)
	if inbound.senderID == "" || inbound.chatID == "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		return
	}
	if b.handleTeamsPollVote(inbound.chatID, inbound.senderID, activity) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "poll_vote_recorded": true})
		return
	}
	if inbound.messageID != "" && b.seenInboundEvent("teams:msg:"+inbound.chatID+":"+inbound.messageID, time.Now()) {
		b.noteInboundDeduped(false)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "deduped": true})
		return
	}

	ref := teamsConversationRef{ServiceURL: inbound.serviceURL, ConversationID: inbound.chatID, UserID: inbound.userID}
	b.teamsMu.Lock()
	b.teamsConvByID[inbound.chatID] = ref
	if inbound.userID != "" {
		b.teamsConvByUserID[inbound.userID] = ref
	}
	b.teamsMu.Unlock()
	_ = b.saveState()

	err = b.postInbound("/api/v1/channels/msteams/inbound", b.cfg.KafclawMSTeamsInboundToken, map[string]any{
		"account_id":         strings.TrimSpace(b.cfg.MSTeamsAccountID),
		"sender_id":          inbound.senderID,
		"user_id":            inbound.userID,
		"chat_id":            inbound.chatID,
		"thread_id":          inbound.threadID,
		"message_id":         inbound.messageID,
		"text":               inbound.text,
		"is_group":           inbound.isGroup,
		"was_mentioned":      inbound.wasMentioned,
		"conversation_type":  inbound.conversationType,
		"group_id":           inbound.teamID,
		"channel_id":         inbound.channelID,
		"tenant_id":          inbound.tenantID,
		"service_url":        inbound.serviceURL,
		"service_url_domain": inbound.serviceDomain,
	})
	if err != nil {
		b.noteInboundForward(false, err)
		log.Printf("teams inbound forward failed: %v", err)
		http.Error(w, "forward failed", http.StatusBadGateway)
		return
	}
	b.metricsMu.Lock()
	b.metrics.TeamsInboundForwarded++
	b.metricsMu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

type teamsInbound struct {
	senderID         string
	userID           string
	chatID           string
	threadID         string
	messageID        string
	text             string
	serviceURL       string
	serviceDomain    string
	conversationType string
	teamID           string
	channelID        string
	tenantID         string
	isGroup          bool
	wasMentioned     bool
}

func normalizeTeamsInbound(activity map[string]any) teamsInbound {
	from, _ := activity["from"].(map[string]any)
	conv, _ := activity["conversation"].(map[string]any)
	recipient, _ := activity["recipient"].(map[string]any)
	channelData, _ := activity["channelData"].(map[string]any)
	team, _ := channelData["team"].(map[string]any)
	channel, _ := channelData["channel"].(map[string]any)
	tenant, _ := channelData["tenant"].(map[string]any)

	out := teamsInbound{
		senderID:         strings.TrimSpace(asString(from["id"])),
		userID:           strings.TrimSpace(asString(from["aadObjectId"])),
		chatID:           strings.TrimSpace(asString(conv["id"])),
		threadID:         strings.TrimSpace(asString(activity["replyToId"])),
		messageID:        strings.TrimSpace(asString(activity["id"])),
		text:             strings.TrimSpace(cleanTeamsMentionText(asString(activity["text"]))),
		serviceURL:       strings.TrimSpace(asString(activity["serviceUrl"])),
		conversationType: strings.ToLower(strings.TrimSpace(asString(conv["conversationType"]))),
		teamID:           strings.TrimSpace(asString(team["id"])),
		channelID:        strings.TrimSpace(asString(channel["id"])),
		tenantID:         strings.TrimSpace(asString(tenant["id"])),
	}
	if out.userID == "" {
		out.userID = out.senderID
	}
	if out.conversationType == "" && out.channelID != "" {
		out.conversationType = "channel"
	}
	if out.chatID == "" {
		if out.channelID != "" && out.teamID != "" {
			out.chatID = out.teamID + ":" + out.channelID
		} else if out.channelID != "" {
			out.chatID = out.channelID
		}
	}
	if out.tenantID == "" {
		out.tenantID = strings.TrimSpace(asString(conv["tenantId"]))
	}
	out.isGroup = out.conversationType != "" && out.conversationType != "personal"
	if out.channelID != "" || out.teamID != "" {
		out.isGroup = true
	}
	out.serviceDomain = hostOnly(out.serviceURL)

	botID := strings.TrimSpace(asString(recipient["id"]))
	botName := strings.TrimSpace(asString(recipient["name"]))
	out.wasMentioned = teamsWasMentioned(activity, botID, botName)
	return out
}

func teamsWasMentioned(activity map[string]any, botID, botName string) bool {
	botID = strings.TrimSpace(botID)
	botName = strings.TrimSpace(strings.ToLower(botName))
	if ents, ok := activity["entities"].([]any); ok {
		for _, e := range ents {
			m, _ := e.(map[string]any)
			if strings.ToLower(strings.TrimSpace(asString(m["type"]))) != "mention" {
				continue
			}
			mentioned, _ := m["mentioned"].(map[string]any)
			mentionedID := strings.TrimSpace(asString(mentioned["id"]))
			if botID != "" && mentionedID == botID {
				return true
			}
			mentionedName := strings.TrimSpace(strings.ToLower(asString(mentioned["name"])))
			if botName != "" && mentionedName != "" && mentionedName == botName {
				return true
			}
		}
	}
	text := strings.ToLower(asString(activity["text"]))
	return strings.Contains(text, "<at>")
}

func cleanTeamsMentionText(text string) string {
	trimmed := strings.TrimSpace(text)
	for {
		start := strings.Index(strings.ToLower(trimmed), "<at>")
		if start < 0 {
			break
		}
		rest := trimmed[start:]
		endRel := strings.Index(strings.ToLower(rest), "</at>")
		if endRel < 0 {
			break
		}
		end := start + endRel + len("</at>")
		trimmed = strings.TrimSpace(trimmed[:start] + " " + trimmed[end:])
	}
	return strings.Join(strings.Fields(trimmed), " ")
}

func hostOnly(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(u.Host))
}

func (b *bridge) seenInboundEvent(key string, now time.Time) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	shouldPersist := false
	b.inboundMu.Lock()
	if b.inboundSeen == nil {
		b.inboundSeen = map[string]time.Time{}
	}
	b.pruneInboundSeenLocked(now)
	if _, ok := b.inboundSeen[key]; ok {
		b.inboundMu.Unlock()
		return true
	}
	b.inboundSeen[key] = now.Add(b.inboundTTL)
	shouldPersist = true
	b.inboundMu.Unlock()
	if shouldPersist {
		if err := b.saveState(); err != nil {
			log.Printf("channelbridge state save warning: %v", err)
		}
	}
	return false
}

func (b *bridge) pruneInboundSeenLocked(now time.Time) {
	ttl := b.inboundTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	for k, exp := range b.inboundSeen {
		if now.After(exp) {
			delete(b.inboundSeen, k)
		}
	}
}

func (b *bridge) handleTeamsOutbound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AccountID         string         `json:"account_id"`
		ChatID            string         `json:"chat_id"`
		ThreadID          string         `json:"thread_id"`
		ReplyMode         string         `json:"reply_mode"`
		Content           string         `json:"content"`
		MediaURLs         []string       `json:"media_urls"`
		Card              map[string]any `json:"card"`
		Action            string         `json:"action"`
		ActionParams      map[string]any `json:"action_params"`
		PollQuestion      string         `json:"poll_question"`
		PollOptions       []string       `json:"poll_options"`
		PollMaxSelections int            `json:"poll_max_selections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ChatID) == "" {
		http.Error(w, "chat_id required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Content) == "" && len(req.MediaURLs) == 0 && len(req.Card) == 0 && strings.TrimSpace(req.PollQuestion) == "" {
		http.Error(w, "content, media_urls, card or poll required", http.StatusBadRequest)
		return
	}
	accountID := strings.TrimSpace(req.AccountID)
	if accountID == "" {
		accountID = "default"
	}
	threadID := b.resolveReplyThread("msteams", accountID, req.ChatID, req.ThreadID, req.ReplyMode, b.cfg.MSTeamsReplyMode)
	ref, err := b.resolveTeamsConversation(req.ChatID)
	if err != nil {
		b.noteOutbound(false, false, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	token, err := b.getTeamsAccessToken()
	if err != nil {
		b.noteOutbound(false, false, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	mediaURL := ""
	if len(req.MediaURLs) > 0 {
		mediaURL = req.MediaURLs[0]
	}
	pollCard := req.Card
	if strings.TrimSpace(req.PollQuestion) != "" {
		pollCard = buildTeamsPollCard(strings.TrimSpace(req.PollQuestion), req.PollOptions, req.PollMaxSelections)
		b.recordTeamsPoll(strings.TrimSpace(req.ChatID), strings.TrimSpace(req.PollQuestion), req.PollOptions, req.PollMaxSelections)
	}
	if err := b.teamsSend(ref, token, threadID, req.Content, mediaURL, pollCard); err != nil {
		b.noteOutbound(false, false, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	b.noteOutbound(true, false, nil)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (b *bridge) handleTeamsResolveUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Entries []string `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	results, err := b.teamsResolveUsers(req.Entries)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func (b *bridge) handleTeamsResolveChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Entries []string `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	results, err := b.teamsResolveChannels(req.Entries)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}

func (b *bridge) teamsResolveUsers(entries []string) ([]map[string]any, error) {
	users, err := b.teamsGraphUsers()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(entries))
	for _, raw := range entries {
		q := strings.TrimSpace(raw)
		entry := map[string]any{"input": raw, "resolved": false}
		if q == "" {
			entry["note"] = "empty input"
			out = append(out, entry)
			continue
		}
		qNorm := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(q), "user:"), "msteams:")
		if looksLikeGUID(qNorm) || strings.Contains(qNorm, "@") {
			entry["resolved"] = true
			entry["id"] = qNorm
			out = append(out, entry)
			continue
		}
		for _, u := range users {
			id := strings.TrimSpace(asString(u["id"]))
			display := strings.ToLower(strings.TrimSpace(asString(u["displayName"])))
			mail := strings.ToLower(strings.TrimSpace(asString(u["mail"])))
			upn := strings.ToLower(strings.TrimSpace(asString(u["userPrincipalName"])))
			if qNorm == display || qNorm == mail || qNorm == upn {
				entry["resolved"] = true
				entry["id"] = id
				if display != "" {
					entry["name"] = display
				}
				break
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

func (b *bridge) teamsResolveChannels(entries []string) ([]map[string]any, error) {
	teams, err := b.teamsGraphTeams()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(entries))
	for _, raw := range entries {
		q := strings.TrimSpace(raw)
		entry := map[string]any{"input": raw, "resolved": false}
		if q == "" {
			entry["note"] = "empty input"
			out = append(out, entry)
			continue
		}
		qNorm := strings.TrimPrefix(strings.ToLower(q), "conversation:")
		if strings.Contains(qNorm, "@thread.") || looksLikeGUID(qNorm) {
			entry["resolved"] = true
			entry["id"] = qNorm
			out = append(out, entry)
			continue
		}
		if strings.Contains(qNorm, "/") {
			parts := strings.SplitN(qNorm, "/", 2)
			teamName := strings.TrimSpace(parts[0])
			channelName := strings.TrimSpace(parts[1])
			for _, tm := range teams {
				tid := strings.TrimSpace(asString(tm["id"]))
				tdn := strings.ToLower(strings.TrimSpace(asString(tm["displayName"])))
				if teamName != tdn {
					continue
				}
				chs, _ := b.teamsGraphTeamChannels(tid)
				for _, ch := range chs {
					cid := strings.TrimSpace(asString(ch["id"]))
					cdn := strings.ToLower(strings.TrimSpace(asString(ch["displayName"])))
					if channelName == cdn {
						entry["resolved"] = true
						entry["id"] = tid + "/" + cid
						entry["name"] = tdn + "/" + cdn
						break
					}
				}
				if entry["resolved"] == true {
					break
				}
			}
			out = append(out, entry)
			continue
		}
		for _, tm := range teams {
			tid := strings.TrimSpace(asString(tm["id"]))
			tdn := strings.ToLower(strings.TrimSpace(asString(tm["displayName"])))
			if qNorm == tdn {
				entry["resolved"] = true
				entry["id"] = tid
				entry["name"] = tdn
				break
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

func (b *bridge) handleTeamsProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	now := time.Now()
	botToken, err := b.getTeamsAccessToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	botClaims := decodeJWTPayloadMap(botToken)
	graphToken, gErr := b.getTeamsGraphToken()
	graph := map[string]any{"ok": gErr == nil}
	if gErr == nil {
		graphClaims := decodeJWTPayloadMap(graphToken)
		graph["claims"] = graphClaims
		graph["diagnostics"] = b.teamsGraphDiagnostics(graphToken, graphClaims, now)
	} else {
		graph["error"] = gErr.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true,
		"bot": map[string]any{
			"claims":      botClaims,
			"diagnostics": tokenDiagnostics(botClaims, now, "api.botframework.com"),
		},
		"graph": graph,
	})
}

func (b *bridge) teamsGraphDiagnostics(graphToken string, claims map[string]any, now time.Time) map[string]any {
	diag := tokenDiagnostics(claims, now, "graph.microsoft.com")
	perm := collectTokenPermissions(claims)
	diag["permissions"] = map[string]any{
		"roles":  perm["roles"],
		"scopes": perm["scopes"],
	}
	diag["required_permissions"] = map[string]any{
		"resolve_users":        []string{"User.Read.All", "Directory.Read.All"},
		"resolve_teams":        []string{"Team.ReadBasic.All", "Group.Read.All"},
		"resolve_channels":     []string{"Channel.ReadBasic.All", "ChannelSettings.Read.All", "Team.ReadBasic.All"},
		"tenant_visibility":    []string{"Organization.Read.All"},
		"identity_diagnostics": []string{"Application.Read.All"},
	}
	diag["permission_coverage"] = map[string]any{
		"resolve_users":     hasAnyPermission(perm, "User.Read.All", "Directory.Read.All"),
		"resolve_teams":     hasAnyPermission(perm, "Team.ReadBasic.All", "Group.Read.All"),
		"resolve_channels":  hasAnyPermission(perm, "Channel.ReadBasic.All", "ChannelSettings.Read.All", "Team.ReadBasic.All"),
		"tenant_visibility": hasAnyPermission(perm, "Organization.Read.All"),
	}
	diag["identity_checks"] = map[string]any{
		"tenant_id_present": strings.TrimSpace(anyToString(claims["tid"])) != "",
		"app_id_present":    strings.TrimSpace(anyToString(claims["appid"])) != "" || strings.TrimSpace(anyToString(claims["azp"])) != "",
		"aud_is_graph":      strings.Contains(strings.ToLower(strings.TrimSpace(anyToString(claims["aud"]))), "graph.microsoft.com"),
	}

	capabilities := map[string]any{}
	statusUsers, _, errUsers := b.graphProbeGET(graphToken, "/users?$top=1&$select=id")
	capabilities["users"] = graphCapabilityResult(statusUsers, errUsers, "User.Read.All or Directory.Read.All")

	statusTeams, teamsBody, errTeams := b.graphProbeGET(graphToken, "/teams?$top=1&$select=id")
	capabilities["teams"] = graphCapabilityResult(statusTeams, errTeams, "Team.ReadBasic.All or Group.Read.All")

	firstTeamID := graphFirstValueID(teamsBody)
	if firstTeamID != "" {
		statusChannels, _, errChannels := b.graphProbeGET(graphToken, "/teams/"+url.PathEscape(firstTeamID)+"/channels?$top=1&$select=id")
		capabilities["channels"] = graphCapabilityResult(statusChannels, errChannels, "Channel.ReadBasic.All or Team.ReadBasic.All")
	} else {
		capabilities["channels"] = map[string]any{
			"ok":               false,
			"status_code":      0,
			"error":            "no_team_id_available_for_channel_probe",
			"required_any_of":  []string{"Channel.ReadBasic.All", "ChannelSettings.Read.All", "Team.ReadBasic.All"},
			"remediation_hint": "ensure Graph app can list teams or manually validate channel endpoint permissions",
		}
	}

	statusOrg, _, errOrg := b.graphProbeGET(graphToken, "/organization?$top=1&$select=id")
	capabilities["organization"] = graphCapabilityResult(statusOrg, errOrg, "Organization.Read.All")

	diag["capabilities"] = capabilities
	return diag
}

func collectTokenPermissions(claims map[string]any) map[string][]string {
	out := map[string][]string{
		"roles":  {},
		"scopes": {},
	}
	if roles, ok := claims["roles"].([]any); ok {
		r := make([]string, 0, len(roles))
		for _, role := range roles {
			v := strings.TrimSpace(anyToString(role))
			if v != "" {
				r = append(r, v)
			}
		}
		out["roles"] = r
	}
	scopeRaw := strings.TrimSpace(anyToString(claims["scp"]))
	if scopeRaw != "" {
		parts := strings.Fields(scopeRaw)
		s := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				s = append(s, p)
			}
		}
		out["scopes"] = s
	}
	return out
}

func hasAnyPermission(perms map[string][]string, expected ...string) bool {
	have := map[string]struct{}{}
	for _, v := range perms["roles"] {
		have[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
	}
	for _, v := range perms["scopes"] {
		have[strings.ToLower(strings.TrimSpace(v))] = struct{}{}
	}
	for _, e := range expected {
		if _, ok := have[strings.ToLower(strings.TrimSpace(e))]; ok {
			return true
		}
	}
	return false
}

func graphCapabilityResult(statusCode int, err error, requiredPerms string) map[string]any {
	ok := err == nil && statusCode >= 200 && statusCode < 300
	out := map[string]any{
		"ok":              ok,
		"status_code":     statusCode,
		"required_any_of": requiredPerms,
	}
	if err != nil {
		out["error"] = err.Error()
		if statusCode == http.StatusForbidden || statusCode == http.StatusUnauthorized {
			out["remediation_hint"] = "grant Graph application permissions and admin-consent them, then refresh token"
		}
	}
	return out
}

func (b *bridge) graphProbeGET(token, pathSuffix string) (int, []byte, error) {
	u := strings.TrimRight(b.cfg.MSTeamsGraphBase, "/") + "/" + strings.TrimLeft(pathSuffix, "/")
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	resp, err := b.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return resp.StatusCode, body, fmt.Errorf("graph status %d", resp.StatusCode)
	}
	return resp.StatusCode, body, nil
}

func graphFirstValueID(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var out struct {
		Value []map[string]any `json:"value"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return ""
	}
	if len(out.Value) == 0 {
		return ""
	}
	return strings.TrimSpace(asString(out.Value[0]["id"]))
}

func (b *bridge) teamsGraphUsers() ([]map[string]any, error) {
	token, err := b.getTeamsGraphToken()
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(b.cfg.MSTeamsGraphBase, "/") + "/users?$top=999&$select=id,displayName,userPrincipalName,mail"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("graph users status %d: %s", resp.StatusCode, strings.TrimSpace(string(bb)))
	}
	var out struct {
		Value []map[string]any `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Value, nil
}

func (b *bridge) teamsGraphTeams() ([]map[string]any, error) {
	token, err := b.getTeamsGraphToken()
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(b.cfg.MSTeamsGraphBase, "/") + "/teams?$top=200&$select=id,displayName"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("graph teams status %d: %s", resp.StatusCode, strings.TrimSpace(string(bb)))
	}
	var out struct {
		Value []map[string]any `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Value, nil
}

func (b *bridge) teamsGraphTeamChannels(teamID string) ([]map[string]any, error) {
	token, err := b.getTeamsGraphToken()
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(b.cfg.MSTeamsGraphBase, "/") + "/teams/" + url.PathEscape(strings.TrimSpace(teamID)) + "/channels?$top=200&$select=id,displayName"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		bb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("graph channels status %d: %s", resp.StatusCode, strings.TrimSpace(string(bb)))
	}
	var out struct {
		Value []map[string]any `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Value, nil
}

func looksLikeGUID(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) < 16 {
		return false
	}
	for _, r := range v {
		if (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func (b *bridge) resolveTeamsConversation(chatID string) (teamsConversationRef, error) {
	id := normalizeTeamsTarget(chatID)
	b.teamsMu.RLock()
	defer b.teamsMu.RUnlock()
	if ref, ok := b.teamsConvByID[id]; ok && ref.ServiceURL != "" {
		return ref, nil
	}
	if ref, ok := b.teamsConvByUserID[id]; ok && ref.ServiceURL != "" {
		return ref, nil
	}
	return teamsConversationRef{}, fmt.Errorf("no teams conversation reference for %s", id)
}

func normalizeTeamsTarget(v string) string {
	s := strings.TrimSpace(v)
	l := strings.ToLower(s)
	switch {
	case strings.HasPrefix(l, "conversation:"):
		return strings.TrimSpace(s[len("conversation:"):])
	case strings.HasPrefix(l, "user:"):
		return strings.TrimSpace(s[len("user:"):])
	case strings.HasPrefix(l, "msteams:user:"):
		return strings.TrimSpace(s[len("msteams:user:"):])
	default:
		return s
	}
}

func buildTeamsPollCard(question string, options []string, maxSel int) map[string]any {
	if maxSel <= 0 {
		maxSel = 1
	}
	choices := make([]map[string]any, 0, len(options))
	for i, o := range options {
		opt := strings.TrimSpace(o)
		if opt == "" {
			continue
		}
		choices = append(choices, map[string]any{"title": opt, "value": fmt.Sprintf("opt_%d", i+1)})
	}
	return map[string]any{
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body": []map[string]any{
			{"type": "TextBlock", "text": question, "weight": "Bolder", "wrap": true},
			{"type": "Input.ChoiceSet", "id": "poll_choice", "isMultiSelect": maxSel > 1, "choices": choices},
		},
		"actions": []map[string]any{
			{"type": "Action.Submit", "title": "Vote"},
		},
	}
}

func (b *bridge) recordTeamsPoll(chatID, question string, options []string, maxSel int) {
	if strings.TrimSpace(chatID) == "" || strings.TrimSpace(question) == "" {
		return
	}
	b.pollMu.Lock()
	if b.teamsPolls == nil {
		b.teamsPolls = map[string]map[string]any{}
	}
	key := fmt.Sprintf("%s:%d", strings.TrimSpace(chatID), time.Now().UnixNano())
	b.teamsPolls[key] = map[string]any{
		"chat_id":            strings.TrimSpace(chatID),
		"question":           question,
		"options":            options,
		"max_selections":     maxSel,
		"created_at_rfc3339": time.Now().UTC().Format(time.RFC3339),
	}
	b.pollMu.Unlock()
	_ = b.saveState()
}

func (b *bridge) handleTeamsPollVote(chatID, senderID string, activity map[string]any) bool {
	val, _ := activity["value"].(map[string]any)
	if val == nil {
		return false
	}
	raw := strings.TrimSpace(asString(val["poll_choice"]))
	if raw == "" {
		choices, ok := val["choices"].([]any)
		if ok && len(choices) > 0 {
			raw = strings.TrimSpace(asString(choices[0]))
		}
	}
	if raw == "" {
		return false
	}
	chatID = strings.TrimSpace(chatID)
	senderID = strings.TrimSpace(senderID)
	if chatID == "" || senderID == "" {
		return false
	}
	b.pollMu.Lock()
	defer b.pollMu.Unlock()
	for k, p := range b.teamsPolls {
		c, _ := p["chat_id"].(string)
		if strings.TrimSpace(c) != chatID {
			continue
		}
		votes, _ := p["votes"].(map[string]any)
		if votes == nil {
			votes = map[string]any{}
		}
		votes[senderID] = raw
		p["votes"] = votes
		p["updated_at_rfc3339"] = time.Now().UTC().Format(time.RFC3339)
		b.teamsPolls[k] = p
		go func() {
			_ = b.saveState()
		}()
		return true
	}
	return false
}

func (b *bridge) getTeamsAccessToken() (string, error) {
	return b.getTeamsTokenForScope("https://api.botframework.com/.default", false)
}

func (b *bridge) getTeamsGraphToken() (string, error) {
	return b.getTeamsTokenForScope("https://graph.microsoft.com/.default", true)
}

func (b *bridge) getTeamsTokenForScope(scope string, graph bool) (string, error) {
	b.teamsMu.RLock()
	cache := b.teamsToken
	if graph {
		cache = b.teamsGraphToken
	}
	b.teamsMu.RUnlock()
	if cache.accessToken != "" && time.Until(cache.expiresAt) > 2*time.Minute {
		return cache.accessToken, nil
	}

	appID := strings.TrimSpace(b.cfg.MSTeamsAppID)
	secret := strings.TrimSpace(b.cfg.MSTeamsAppPassword)
	tenant := strings.TrimSpace(b.cfg.MSTeamsTenantID)
	if appID == "" || secret == "" || tenant == "" {
		return "", errors.New("missing teams app credentials")
	}

	var token string
	var exp time.Time
	err := withRetry(3, 300*time.Millisecond, func() (bool, error) {
		form := url.Values{}
		form.Set("grant_type", "client_credentials")
		form.Set("client_id", appID)
		form.Set("client_secret", secret)
		form.Set("scope", strings.TrimSpace(scope))
		endpoint := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenant)
		req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return false, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := b.client.Do(req)
		if err != nil {
			return true, err
		}
		defer resp.Body.Close()
		var out struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
			Error       string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if strings.TrimSpace(out.AccessToken) == "" {
			if d := parseRetryAfter(resp.Header.Get("Retry-After")); d > 0 {
				time.Sleep(d)
			}
			retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
			if out.Error == "" {
				out.Error = "token response missing access_token"
			}
			return retryable, errors.New(out.Error)
		}
		token = out.AccessToken
		exp = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
		return false, nil
	})
	if err != nil {
		return "", err
	}
	b.teamsMu.Lock()
	if graph {
		b.teamsGraphToken = tokenCache{accessToken: token, expiresAt: exp}
	} else {
		b.teamsToken = tokenCache{accessToken: token, expiresAt: exp}
	}
	b.teamsMu.Unlock()
	return token, nil
}

func (b *bridge) teamsSend(ref teamsConversationRef, accessToken, replyToID, text, mediaURL string, card map[string]any) error {
	return withRetry(3, 300*time.Millisecond, func() (bool, error) {
		payload := map[string]any{"type": "message", "text": text}
		if rid := strings.TrimSpace(replyToID); rid != "" {
			payload["replyToId"] = rid
		}
		attachments := make([]map[string]any, 0, 2)
		if strings.TrimSpace(mediaURL) != "" {
			name := path.Base(strings.TrimSpace(mediaURL))
			if name == "." || name == "/" || name == "" {
				name = "attachment"
			}
			attachments = append(attachments, map[string]any{
				"contentType": "application/octet-stream",
				"contentUrl":  strings.TrimSpace(mediaURL),
				"name":        name,
			})
		}
		if len(card) > 0 {
			attachments = append(attachments, map[string]any{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content":     card,
			})
		}
		if len(attachments) > 0 {
			payload["attachments"] = attachments
		}
		body, _ := json.Marshal(payload)
		serviceURL := strings.TrimRight(ref.ServiceURL, "/")
		base := strings.TrimSpace(b.cfg.MSTeamsAPIBase)
		u := fmt.Sprintf("%s/v3/conversations/%s/activities", serviceURL, url.PathEscape(ref.ConversationID))
		if base != "" {
			u = fmt.Sprintf("%s/v3/conversations/%s/activities", strings.TrimRight(base, "/"), url.PathEscape(ref.ConversationID))
		}
		req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return false, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := b.client.Do(req)
		if err != nil {
			return true, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 300 {
			return false, nil
		}
		bb, _ := io.ReadAll(resp.Body)
		if d := parseRetryAfter(resp.Header.Get("Retry-After")); d > 0 {
			time.Sleep(d)
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return retryable, fmt.Errorf("teams send failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bb)))
	})
}

func (b *bridge) postInbound(path, token string, payload map[string]any) error {
	return withRetry(3, 200*time.Millisecond, func() (bool, error) {
		data, _ := json.Marshal(payload)
		u := strings.TrimRight(b.cfg.KafclawBase, "/") + path
		req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(data))
		if err != nil {
			return false, err
		}
		req.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(token) != "" {
			req.Header.Set("X-Channel-Token", strings.TrimSpace(token))
		}
		resp, err := b.client.Do(req)
		if err != nil {
			return true, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 300 {
			return false, nil
		}
		body, _ := io.ReadAll(resp.Body)
		if d := parseRetryAfter(resp.Header.Get("Retry-After")); d > 0 {
			time.Sleep(d)
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return retryable, fmt.Errorf("kafclaw inbound rejected: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	})
}

func withRetry(attempts int, baseDelay time.Duration, fn func() (retryable bool, err error)) error {
	if attempts <= 0 {
		attempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		retryable, err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !retryable || i == attempts-1 {
			break
		}
		time.Sleep(baseDelay * time.Duration(1<<i))
	}
	return lastErr
}

func (b *bridge) loadState() error {
	path := strings.TrimSpace(b.cfg.StatePath)
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var st bridgeState
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	b.teamsMu.Lock()
	if st.TeamsConvByID != nil {
		b.teamsConvByID = st.TeamsConvByID
	}
	if st.TeamsConvByUserID != nil {
		b.teamsConvByUserID = st.TeamsConvByUserID
	}
	b.teamsMu.Unlock()
	b.inboundMu.Lock()
	if b.inboundSeen == nil {
		b.inboundSeen = map[string]time.Time{}
	}
	for k, exp := range st.InboundSeen {
		if time.Now().Before(exp) {
			b.inboundSeen[k] = exp
		}
	}
	b.inboundMu.Unlock()
	b.pollMu.Lock()
	if b.teamsPolls == nil {
		b.teamsPolls = map[string]map[string]any{}
	}
	for k, v := range st.TeamsPolls {
		b.teamsPolls[k] = v
	}
	b.pollMu.Unlock()
	return nil
}

func (b *bridge) saveState() error {
	path := strings.TrimSpace(b.cfg.StatePath)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b.teamsMu.RLock()
	convByID := make(map[string]teamsConversationRef, len(b.teamsConvByID))
	for k, v := range b.teamsConvByID {
		convByID[k] = v
	}
	convByUserID := make(map[string]teamsConversationRef, len(b.teamsConvByUserID))
	for k, v := range b.teamsConvByUserID {
		convByUserID[k] = v
	}
	b.teamsMu.RUnlock()
	b.inboundMu.Lock()
	b.pruneInboundSeenLocked(time.Now())
	inboundSeen := make(map[string]time.Time, len(b.inboundSeen))
	for k, v := range b.inboundSeen {
		inboundSeen[k] = v
	}
	b.inboundMu.Unlock()
	b.pollMu.Lock()
	teamsPolls := make(map[string]map[string]any, len(b.teamsPolls))
	for k, v := range b.teamsPolls {
		teamsPolls[k] = v
	}
	b.pollMu.Unlock()

	st := bridgeState{
		TeamsConvByID:     convByID,
		TeamsConvByUserID: convByUserID,
		InboundSeen:       inboundSeen,
		TeamsPolls:        teamsPolls,
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func newTeamsJWTVerifier(client *http.Client, cfgURL, appID string) *teamsJWTVerifier {
	return &teamsJWTVerifier{
		client:    client,
		cfgURL:    strings.TrimSpace(cfgURL),
		appID:     strings.TrimSpace(appID),
		keysByKid: map[string]*rsa.PublicKey{},
	}
}

func (v *teamsJWTVerifier) Verify(authHeader string, now time.Time, serviceURL string) error {
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(authHeader), "Bearer "))
	if token == "" {
		return errors.New("missing bearer token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("invalid jwt format")
	}

	headerBytes, err := decodeB64URL(parts[0])
	if err != nil {
		return fmt.Errorf("decode jwt header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return fmt.Errorf("parse jwt header: %w", err)
	}
	if strings.TrimSpace(header.Alg) != "RS256" {
		return fmt.Errorf("unsupported jwt alg: %s", header.Alg)
	}
	kid := strings.TrimSpace(header.Kid)
	if kid == "" {
		return errors.New("jwt missing kid")
	}

	claimsBytes, err := decodeB64URL(parts[1])
	if err != nil {
		return fmt.Errorf("decode jwt claims: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return fmt.Errorf("parse jwt claims: %w", err)
	}
	if err := v.validateClaims(claims, now, serviceURL); err != nil {
		return err
	}

	key, err := v.resolveKey(kid, now)
	if err != nil {
		return err
	}
	sig, err := decodeB64URL(parts[2])
	if err != nil {
		return fmt.Errorf("decode jwt signature: %w", err)
	}
	signingInput := parts[0] + "." + parts[1]
	sum := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return fmt.Errorf("verify jwt signature: %w", err)
	}
	return nil
}

func (v *teamsJWTVerifier) validateClaims(claims map[string]any, now time.Time, serviceURL string) error {
	appID := strings.TrimSpace(v.appID)
	if appID == "" {
		return nil
	}
	if !matchesJWTAudience(claims["aud"], appID) {
		return fmt.Errorf("jwt audience mismatch")
	}
	iss := strings.TrimSpace(anyToString(claims["iss"]))
	if iss == "" {
		return errors.New("jwt missing issuer")
	}
	exp := anyToUnix(claims["exp"])
	if exp <= 0 || now.Unix() >= exp {
		return errors.New("jwt expired")
	}
	nbf := anyToUnix(claims["nbf"])
	if nbf > 0 && now.Unix()+60 < nbf {
		return errors.New("jwt not yet valid")
	}
	issuer := strings.TrimSpace(v.currentIssuer(now))
	if issuer != "" && !sameIssuer(iss, issuer) {
		return fmt.Errorf("jwt issuer mismatch")
	}
	claimSvc := strings.TrimSpace(anyToString(claims["serviceurl"]))
	if claimSvc == "" {
		claimSvc = strings.TrimSpace(anyToString(claims["serviceUrl"]))
	}
	if !isTrustedTeamsServiceURL(serviceURL) {
		return errors.New("jwt serviceurl not trusted")
	}
	if claimSvc != "" && !isTrustedTeamsServiceURL(claimSvc) {
		return errors.New("jwt claim serviceurl not trusted")
	}
	if claimSvc != "" && strings.TrimSpace(serviceURL) != "" {
		if !sameServiceURL(serviceURL, claimSvc) {
			return errors.New("jwt serviceurl mismatch")
		}
	}
	return nil
}

func matchesJWTAudience(aud any, appID string) bool {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return true
	}
	switch t := aud.(type) {
	case string:
		return strings.TrimSpace(t) == appID
	case []any:
		for _, v := range t {
			if strings.TrimSpace(anyToString(v)) == appID {
				return true
			}
		}
	}
	return false
}

func isTrustedTeamsServiceURL(rawURL string) bool {
	host := hostOnly(rawURL)
	if host == "" {
		return false
	}
	if strings.HasSuffix(host, ".trafficmanager.net") {
		return true
	}
	if strings.HasSuffix(host, ".botframework.com") {
		return true
	}
	if strings.HasSuffix(host, ".teams.microsoft.com") {
		return true
	}
	return false
}

func tokenDiagnostics(claims map[string]any, now time.Time, expectedAudSubstr string) map[string]any {
	exp := anyToUnix(claims["exp"])
	nbf := anyToUnix(claims["nbf"])
	aud := strings.TrimSpace(anyToString(claims["aud"]))
	if aud == "" {
		if arr, ok := claims["aud"].([]any); ok && len(arr) > 0 {
			aud = strings.TrimSpace(anyToString(arr[0]))
		}
	}
	out := map[string]any{
		"issuer":      strings.TrimSpace(anyToString(claims["iss"])),
		"audience":    aud,
		"service_url": strings.TrimSpace(anyToString(claims["serviceurl"])),
		"scopes":      strings.TrimSpace(anyToString(claims["scp"])),
	}
	if out["service_url"] == "" {
		out["service_url"] = strings.TrimSpace(anyToString(claims["serviceUrl"]))
	}
	if roles, ok := claims["roles"].([]any); ok {
		r := make([]string, 0, len(roles))
		for _, role := range roles {
			v := strings.TrimSpace(anyToString(role))
			if v != "" {
				r = append(r, v)
			}
		}
		out["roles"] = r
	}
	if exp > 0 {
		expiresAt := time.Unix(exp, 0).UTC()
		out["expires_unix"] = exp
		out["expires_at"] = expiresAt.Format(time.RFC3339)
		out["expires_in_seconds"] = int64(time.Until(expiresAt).Seconds())
		out["expired"] = now.Unix() >= exp
	} else {
		out["expired"] = true
	}
	if nbf > 0 {
		out["not_before_unix"] = nbf
		out["not_before_at"] = time.Unix(nbf, 0).UTC().Format(time.RFC3339)
		out["active"] = now.Unix()+60 >= nbf
	} else {
		out["active"] = true
	}
	expectedAudSubstr = strings.ToLower(strings.TrimSpace(expectedAudSubstr))
	if expectedAudSubstr != "" {
		out["audience_matches"] = strings.Contains(strings.ToLower(aud), expectedAudSubstr)
	}
	return out
}

func sameIssuer(a, b string) bool {
	trim := func(s string) string {
		return strings.TrimRight(strings.TrimSpace(s), "/")
	}
	return strings.EqualFold(trim(a), trim(b))
}

func sameServiceURL(a, b string) bool {
	ua, errA := url.Parse(strings.TrimSpace(a))
	ub, errB := url.Parse(strings.TrimSpace(b))
	if errA != nil || errB != nil {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	ha := strings.TrimSpace(strings.ToLower(ua.Host))
	hb := strings.TrimSpace(strings.ToLower(ub.Host))
	return ha != "" && ha == hb
}

func (v *teamsJWTVerifier) resolveKey(kid string, now time.Time) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if key := v.keysByKid[kid]; key != nil && now.Before(v.cacheUntil) {
		return key, nil
	}
	if err := v.refreshLocked(now); err != nil {
		return nil, err
	}
	if key := v.keysByKid[kid]; key != nil {
		return key, nil
	}
	return nil, errors.New("jwt kid not found in jwks")
}

func (v *teamsJWTVerifier) currentIssuer(now time.Time) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	if strings.TrimSpace(v.issuer) == "" || !now.Before(v.cacheUntil) {
		_ = v.refreshLocked(now)
	}
	return v.issuer
}

func (v *teamsJWTVerifier) refreshLocked(now time.Time) error {
	cfgURL := strings.TrimSpace(v.cfgURL)
	if cfgURL == "" {
		return errors.New("missing openid config url")
	}
	req, err := http.NewRequest(http.MethodGet, cfgURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("openid config status %d", resp.StatusCode)
	}
	var oc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&oc); err != nil {
		return err
	}
	if strings.TrimSpace(oc.JWKSURI) == "" {
		return errors.New("openid config missing jwks_uri")
	}
	keys, err := fetchJWKS(v.client, oc.JWKSURI)
	if err != nil {
		return err
	}
	v.issuer = strings.TrimSpace(oc.Issuer)
	v.jwksURI = strings.TrimSpace(oc.JWKSURI)
	v.keysByKid = keys
	v.cacheUntil = now.Add(30 * time.Minute)
	return nil
}

func fetchJWKS(client *http.Client, uri string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimSpace(uri), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if strings.TrimSpace(k.Kty) != "RSA" || strings.TrimSpace(k.Kid) == "" {
			continue
		}
		pub, err := jwkToRSAPublicKey(k.N, k.E)
		if err != nil {
			continue
		}
		out[strings.TrimSpace(k.Kid)] = pub
	}
	if len(out) == 0 {
		return nil, errors.New("no usable jwks rsa keys")
	}
	return out, nil
}

func (b *bridge) downloadMedia(mediaURL string) ([]byte, string, error) {
	mediaURL = strings.TrimSpace(mediaURL)
	if mediaURL == "" {
		return nil, "", errors.New("empty media url")
	}
	parsed, err := validateMediaDownloadURL(mediaURL)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequest(http.MethodGet, "https://files.slack.com/", nil)
	if err != nil {
		return nil, "", err
	}
	req.URL.Path = parsed.Path
	req.URL.RawQuery = parsed.RawQuery
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("media fetch status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	name := path.Base(req.URL.Path)
	if name == "." || name == "/" || name == "" {
		name = "upload.bin"
	}
	return data, name, nil
}

func validateMediaDownloadURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid media url: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(u.Scheme), "https") {
		return nil, errors.New("media url must use https")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return nil, errors.New("media url host is missing")
	}
	if !isAllowedMediaHost(host) {
		return nil, fmt.Errorf("media url host not allowed: %s", host)
	}
	if strings.TrimSpace(u.User.String()) != "" {
		return nil, errors.New("media url user info is not allowed")
	}
	if strings.Contains(u.Path, "..") {
		return nil, errors.New("media url path is invalid")
	}
	u.Scheme = "https"
	u.Host = "files.slack.com"
	u.User = nil
	u.Fragment = ""
	return u, nil
}

func isAllowedMediaHost(host string) bool {
	// Strict host allowlist for media download path to avoid uncontrolled egress.
	return host == "files.slack.com"
}

func jwkToRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := decodeB64URL(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := decodeB64URL(eB64)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nBytes)
	eBig := new(big.Int).SetBytes(eBytes)
	if n.Sign() <= 0 || eBig.Sign() <= 0 || !eBig.IsInt64() {
		return nil, errors.New("invalid rsa jwk components")
	}
	e := int(eBig.Int64())
	return &rsa.PublicKey{N: n, E: e}, nil
}

func decodeB64URL(v string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimSpace(v))
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}

func anyToUnix(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case int:
		return int64(t)
	case json.Number:
		i, _ := t.Int64()
		return i
	default:
		return 0
	}
}

func decodeJWTPayloadMap(token string) map[string]any {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return map[string]any{}
	}
	b, err := decodeB64URL(parts[1])
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if when, err := http.ParseTime(v); err == nil {
		d := time.Until(when)
		if d > 0 {
			return d
		}
	}
	return 0
}

func normalizeReplyMode(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off":
		return "off"
	case "first":
		return "first"
	default:
		return "all"
	}
}

func (b *bridge) resolveReplyThread(channel, accountID, chatID, requestedThreadID, requestedMode, defaultMode string) string {
	threadID := strings.TrimSpace(requestedThreadID)
	if threadID == "" {
		return ""
	}
	mode := normalizeReplyMode(requestedMode)
	if strings.TrimSpace(requestedMode) == "" {
		mode = normalizeReplyMode(defaultMode)
	}
	if mode == "off" {
		return ""
	}
	if mode != "first" {
		return threadID
	}
	key := strings.ToLower(strings.TrimSpace(channel)) + "|" + strings.ToLower(bridgeAccountIDOrDefault(accountID)) + "|" + strings.TrimSpace(chatID)
	b.replyMu.Lock()
	defer b.replyMu.Unlock()
	if b.replySeen == nil {
		b.replySeen = map[string]bool{}
	}
	if b.replySeen[key] {
		return ""
	}
	b.replySeen[key] = true
	return threadID
}

func bridgeAccountIDOrDefault(accountID string) string {
	if s := strings.TrimSpace(accountID); s != "" {
		return s
	}
	return "default"
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
