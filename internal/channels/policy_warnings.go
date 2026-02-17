package channels

import (
	"fmt"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
)

// CollectUnsafeGroupPolicyWarnings reports risky Slack/Teams group policy states.
func CollectUnsafeGroupPolicyWarnings(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	out := make([]string, 0, 4)
	out = append(out, collectUnsafeGroupPolicyWarningsForChannel(
		"slack",
		cfg.Channels.Slack.GroupPolicy,
		cfg.Channels.Slack.RequireMention,
		cfg.Channels.Slack.AllowFrom,
	)...)
	out = append(out, collectUnsafeGroupPolicyWarningsForChannel(
		"msteams",
		cfg.Channels.MSTeams.GroupPolicy,
		cfg.Channels.MSTeams.RequireMention,
		cfg.Channels.MSTeams.GroupAllowFrom,
	)...)
	return out
}

func collectUnsafeGroupPolicyWarningsForChannel(channel string, policy config.GroupPolicy, requireMention bool, groupAllow []string) []string {
	ch := strings.TrimSpace(channel)
	if ch == "" {
		ch = "channel"
	}
	p := config.GroupPolicy(strings.ToLower(strings.TrimSpace(string(policy))))
	out := make([]string, 0, 2)

	switch p {
	case config.GroupPolicyOpen:
		if requireMention {
			out = append(out, fmt.Sprintf("%s group policy is 'open': any mentioned user in group chats can trigger the agent", ch))
		} else {
			out = append(out, fmt.Sprintf("%s group policy is 'open' with mention gating disabled: any group message can trigger the agent", ch))
		}
	case config.GroupPolicyAllowlist, "":
		if !requireMention {
			out = append(out, fmt.Sprintf("%s group policy uses allowlist with mention gating disabled: allowlisted users can trigger on every group message", ch))
		}
		if hasWildcardAllow(groupAllow) {
			out = append(out, fmt.Sprintf("%s group allowlist contains '*': effectively broad group access", ch))
		}
	}
	return out
}

func hasWildcardAllow(entries []string) bool {
	for _, raw := range entries {
		v := strings.TrimSpace(strings.ToLower(raw))
		if v == "*" || strings.HasSuffix(v, ":*") {
			return true
		}
	}
	return false
}
