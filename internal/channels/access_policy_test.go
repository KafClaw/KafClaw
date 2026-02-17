package channels

import (
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestEvaluateAccessDMPolicies(t *testing.T) {
	tests := []struct {
		name   string
		cfg    AccessConfig
		sender string
		expect AccessDecision
	}{
		{
			name: "pairing for unknown sender",
			cfg: AccessConfig{
				DmPolicy:  config.DmPolicyPairing,
				AllowFrom: []string{"u1"},
			},
			sender: "u2",
			expect: AccessDecision{Allowed: false, RequiresPairing: true, Reason: "dm_pairing_required"},
		},
		{
			name: "allowlist allows known sender",
			cfg: AccessConfig{
				DmPolicy:  config.DmPolicyAllowlist,
				AllowFrom: []string{"u1"},
			},
			sender: "u1",
			expect: AccessDecision{Allowed: true, Reason: "dm_allowlist_match"},
		},
		{
			name: "allowlist normalization handles slack prefixes",
			cfg: AccessConfig{
				Channel:   "slack",
				DmPolicy:  config.DmPolicyAllowlist,
				AllowFrom: []string{"slack:user:u1"},
			},
			sender: "U1",
			expect: AccessDecision{Allowed: true, Reason: "dm_allowlist_match"},
		},
		{
			name: "open allows sender",
			cfg: AccessConfig{
				DmPolicy: config.DmPolicyOpen,
			},
			sender: "u1",
			expect: AccessDecision{Allowed: true, Reason: "dm_policy_open"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateAccess(AccessContext{SenderID: tt.sender}, tt.cfg)
			if got.Allowed != tt.expect.Allowed || got.RequiresPairing != tt.expect.RequiresPairing || got.Reason != tt.expect.Reason {
				t.Fatalf("unexpected decision: %+v", got)
			}
		})
	}
}

func TestEvaluateAccessGroupPolicyRequireMention(t *testing.T) {
	got := EvaluateAccess(AccessContext{
		SenderID:     "u1",
		IsGroup:      true,
		WasMentioned: false,
	}, AccessConfig{
		GroupPolicy:    config.GroupPolicyOpen,
		RequireMention: true,
	})
	if got.Allowed || got.Reason != "mention_required" {
		t.Fatalf("expected mention_required, got %+v", got)
	}
}

func TestEvaluateAccessCriticalBranches(t *testing.T) {
	tests := []struct {
		name   string
		ctx    AccessContext
		cfg    AccessConfig
		reason string
		allow  bool
		pair   bool
	}{
		{
			name:   "missing sender",
			ctx:    AccessContext{SenderID: ""},
			cfg:    AccessConfig{DmPolicy: config.DmPolicyOpen},
			reason: "missing_sender_id",
		},
		{
			name:   "dm disabled",
			ctx:    AccessContext{SenderID: "u1"},
			cfg:    AccessConfig{DmPolicy: config.DmPolicyDisabled},
			reason: "dm_policy_disabled",
		},
		{
			name:   "dm allowlist block",
			ctx:    AccessContext{SenderID: "u2"},
			cfg:    AccessConfig{DmPolicy: config.DmPolicyAllowlist, AllowFrom: []string{"u1"}},
			reason: "dm_allowlist_block",
		},
		{
			name:   "dm invalid policy",
			ctx:    AccessContext{SenderID: "u1"},
			cfg:    AccessConfig{DmPolicy: config.DmPolicy("bogus")},
			reason: "invalid_dm_policy",
		},
		{
			name:   "group disabled",
			ctx:    AccessContext{SenderID: "u1", IsGroup: true, WasMentioned: true},
			cfg:    AccessConfig{GroupPolicy: config.GroupPolicyDisabled},
			reason: "group_policy_disabled",
		},
		{
			name:   "group open",
			ctx:    AccessContext{SenderID: "u1", IsGroup: true, WasMentioned: true},
			cfg:    AccessConfig{GroupPolicy: config.GroupPolicyOpen},
			reason: "group_policy_open",
			allow:  true,
		},
		{
			name:   "group allowlist fallback to allowfrom",
			ctx:    AccessContext{SenderID: "u1", IsGroup: true, WasMentioned: true},
			cfg:    AccessConfig{GroupPolicy: config.GroupPolicyAllowlist, AllowFrom: []string{"u1"}},
			reason: "group_allowlist_match",
			allow:  true,
		},
		{
			name:   "group allowlist wildcard",
			ctx:    AccessContext{SenderID: "u2", IsGroup: true, WasMentioned: true},
			cfg:    AccessConfig{GroupPolicy: config.GroupPolicyAllowlist, GroupAllowFrom: []string{"*"}},
			reason: "group_allowlist_match",
			allow:  true,
		},
		{
			name:   "group invalid policy",
			ctx:    AccessContext{SenderID: "u1", IsGroup: true, WasMentioned: true},
			cfg:    AccessConfig{GroupPolicy: config.GroupPolicy("bogus")},
			reason: "invalid_group_policy",
		},
		{
			name:   "group allowlist block",
			ctx:    AccessContext{SenderID: "u9", IsGroup: true, WasMentioned: true},
			cfg:    AccessConfig{GroupPolicy: config.GroupPolicyAllowlist, GroupAllowFrom: []string{"u1"}},
			reason: "group_allowlist_block",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluateAccess(tt.ctx, tt.cfg)
			if got.Reason != tt.reason || got.Allowed != tt.allow || got.RequiresPairing != tt.pair {
				t.Fatalf("unexpected decision: %+v", got)
			}
		})
	}
}
