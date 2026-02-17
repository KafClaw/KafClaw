package channels

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

const pairingPendingKey = "pairing_pending_v1"

const (
	pairingCodeLength   = 8
	pairingCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	defaultPairingTTL   = time.Hour
	maxPendingPairings  = 3
)

// PendingPairing stores a pending sender approval entry.
type PendingPairing struct {
	Channel   string    `json:"channel"`
	SenderID  string    `json:"sender_id"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PairingService manages pending sender approvals in timeline settings.
type PairingService struct {
	store pairingStore
}

func NewPairingService(timeSvc *timeline.TimelineService) *PairingService {
	return &PairingService{store: timeSvc}
}

type pairingStore interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

func (s *PairingService) CreateOrGetPending(channel, senderID string, ttl time.Duration) (*PendingPairing, error) {
	channel = normalizeChannel(channel)
	senderID = strings.TrimSpace(senderID)
	if channel == "" || senderID == "" {
		return nil, fmt.Errorf("channel and sender_id are required")
	}
	if ttl <= 0 {
		ttl = defaultPairingTTL
	}

	items, err := s.loadPending()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	items = filterNonExpired(items, now)
	for _, it := range items {
		if it.Channel == channel && it.SenderID == senderID {
			return &it, s.savePending(items)
		}
	}

	code, err := randomPairingCode()
	if err != nil {
		return nil, err
	}
	entry := PendingPairing{
		Channel:   channel,
		SenderID:  senderID,
		Code:      code,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	items = append(items, entry)
	if len(items) > maxPendingPairings {
		slices.SortFunc(items, func(a, b PendingPairing) int {
			if a.CreatedAt.Before(b.CreatedAt) {
				return -1
			}
			if a.CreatedAt.After(b.CreatedAt) {
				return 1
			}
			return 0
		})
		items = items[len(items)-maxPendingPairings:]
	}
	if err := s.savePending(items); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (s *PairingService) ListPending() ([]PendingPairing, error) {
	items, err := s.loadPending()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	items = filterNonExpired(items, now)
	if err := s.savePending(items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *PairingService) Approve(cfg *config.Config, channel, code string) (*PendingPairing, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	entry, remaining, err := s.takePending(channel, code)
	if err != nil {
		return nil, err
	}
	if err := addChannelAllowFrom(cfg, entry.Channel, entry.SenderID); err != nil {
		return nil, err
	}
	if err := s.savePending(remaining); err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *PairingService) Deny(channel, code string) (*PendingPairing, error) {
	entry, remaining, err := s.takePending(channel, code)
	if err != nil {
		return nil, err
	}
	if err := s.savePending(remaining); err != nil {
		return nil, err
	}
	return entry, nil
}

func (s *PairingService) takePending(channel, code string) (*PendingPairing, []PendingPairing, error) {
	channel = normalizeChannel(channel)
	code = normalizeCode(code)
	if channel == "" || code == "" {
		return nil, nil, fmt.Errorf("channel and code are required")
	}
	items, err := s.loadPending()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	items = filterNonExpired(items, now)
	out := make([]PendingPairing, 0, len(items))
	var hit *PendingPairing
	for _, it := range items {
		if hit == nil && it.Channel == channel && normalizeCode(it.Code) == code {
			tmp := it
			hit = &tmp
			continue
		}
		out = append(out, it)
	}
	if hit == nil {
		return nil, nil, fmt.Errorf("pairing code not found for channel %q", channel)
	}
	return hit, out, nil
}

func (s *PairingService) loadPending() ([]PendingPairing, error) {
	raw, err := s.store.GetSetting(pairingPendingKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return []PendingPairing{}, nil
	}
	var items []PendingPairing
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("parse pending pairings: %w", err)
	}
	return items, nil
}

func (s *PairingService) savePending(items []PendingPairing) error {
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return s.store.SetSetting(pairingPendingKey, string(data))
}

func filterNonExpired(items []PendingPairing, now time.Time) []PendingPairing {
	out := make([]PendingPairing, 0, len(items))
	for _, it := range items {
		if it.ExpiresAt.IsZero() || it.ExpiresAt.After(now) {
			it.Channel = normalizeChannel(it.Channel)
			it.Code = normalizeCode(it.Code)
			it.SenderID = strings.TrimSpace(it.SenderID)
			if it.Channel != "" && it.Code != "" && it.SenderID != "" {
				out = append(out, it)
			}
		}
	}
	return out
}

func randomPairingCode() (string, error) {
	out := make([]byte, pairingCodeLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(pairingCodeAlphabet))))
		if err != nil {
			return "", err
		}
		out[i] = pairingCodeAlphabet[n.Int64()]
	}
	return string(out), nil
}

func normalizeChannel(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "teams":
		return "msteams"
	default:
		return v
	}
}

func normalizeCode(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

func addChannelAllowFrom(cfg *config.Config, channel, senderID string) error {
	senderID = normalizeAllowEntryForChannel(channel, senderID)
	if senderID == "" {
		return fmt.Errorf("sender id is required")
	}
	switch normalizeChannel(channel) {
	case "slack":
		cfg.Channels.Slack.AllowFrom = appendUnique(cfg.Channels.Slack.AllowFrom, senderID)
	case "msteams":
		cfg.Channels.MSTeams.AllowFrom = appendUnique(cfg.Channels.MSTeams.AllowFrom, senderID)
	case "whatsapp":
		cfg.Channels.WhatsApp.AllowFrom = appendUnique(cfg.Channels.WhatsApp.AllowFrom, senderID)
	default:
		return fmt.Errorf("unsupported channel: %s", channel)
	}
	return nil
}

func appendUnique(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	for _, v := range items {
		if strings.EqualFold(strings.TrimSpace(v), value) {
			return items
		}
	}
	return append(items, value)
}

func normalizeAllowEntryForChannel(channel, raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	switch normalizeChannel(channel) {
	case "slack":
		v = strings.TrimPrefix(strings.ToLower(v), "slack:")
		v = strings.TrimPrefix(v, "user:")
	case "msteams":
		v = strings.TrimPrefix(strings.ToLower(v), "msteams:")
		v = strings.TrimPrefix(v, "teams:")
		v = strings.TrimPrefix(v, "user:")
	default:
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(v)
}
