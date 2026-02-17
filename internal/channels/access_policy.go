package channels

import (
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
)

// AccessContext is the normalized inbound context used for channel access checks.
type AccessContext struct {
	SenderID     string
	IsGroup      bool
	WasMentioned bool
}

// AccessConfig is a channel-agnostic policy view.
type AccessConfig struct {
	Channel        string
	AllowFrom      []string
	GroupAllowFrom []string
	DmPolicy       config.DmPolicy
	GroupPolicy    config.GroupPolicy
	RequireMention bool
}

// AccessDecision is the result of evaluating a sender against access policy.
type AccessDecision struct {
	Allowed         bool
	RequiresPairing bool
	Reason          string
}

// EvaluateAccess applies OpenClaw-style DM/group policy semantics.
func EvaluateAccess(ctx AccessContext, cfg AccessConfig) AccessDecision {
	sender := strings.TrimSpace(ctx.SenderID)
	if sender == "" {
		return AccessDecision{Allowed: false, Reason: "missing_sender_id"}
	}

	if ctx.IsGroup {
		if cfg.RequireMention && !ctx.WasMentioned {
			return AccessDecision{Allowed: false, Reason: "mention_required"}
		}
		switch cfg.GroupPolicy {
		case config.GroupPolicyDisabled:
			return AccessDecision{Allowed: false, Reason: "group_policy_disabled"}
		case config.GroupPolicyOpen:
			return AccessDecision{Allowed: true, Reason: "group_policy_open"}
		case "", config.GroupPolicyAllowlist:
			allow := cfg.GroupAllowFrom
			if len(allow) == 0 {
				allow = cfg.AllowFrom
			}
			if isAllowedSender(cfg.Channel, allow, sender) {
				return AccessDecision{Allowed: true, Reason: "group_allowlist_match"}
			}
			return AccessDecision{Allowed: false, Reason: "group_allowlist_block"}
		default:
			return AccessDecision{Allowed: false, Reason: "invalid_group_policy"}
		}
	}

	switch cfg.DmPolicy {
	case config.DmPolicyDisabled:
		return AccessDecision{Allowed: false, Reason: "dm_policy_disabled"}
	case config.DmPolicyOpen:
		return AccessDecision{Allowed: true, Reason: "dm_policy_open"}
	case config.DmPolicyAllowlist:
		if isAllowedSender(cfg.Channel, cfg.AllowFrom, sender) {
			return AccessDecision{Allowed: true, Reason: "dm_allowlist_match"}
		}
		return AccessDecision{Allowed: false, Reason: "dm_allowlist_block"}
	case "", config.DmPolicyPairing:
		if isAllowedSender(cfg.Channel, cfg.AllowFrom, sender) {
			return AccessDecision{Allowed: true, Reason: "dm_allowlist_match"}
		}
		return AccessDecision{Allowed: false, RequiresPairing: true, Reason: "dm_pairing_required"}
	default:
		return AccessDecision{Allowed: false, Reason: "invalid_dm_policy"}
	}
}

func isAllowedSender(channel string, allow []string, sender string) bool {
	s := strings.ToLower(normalizeAllowEntryForChannel(channel, sender))
	if s == "" {
		return false
	}
	for _, raw := range allow {
		v := strings.ToLower(normalizeAllowEntryForChannel(channel, raw))
		if v == "" {
			continue
		}
		if v == "*" || v == s {
			return true
		}
	}
	return false
}
