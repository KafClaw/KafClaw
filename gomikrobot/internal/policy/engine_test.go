package policy

import (
	"testing"

	"github.com/kamir/gomikrobot/internal/tools"
)

func TestTier0AlwaysAllowed(t *testing.T) {
	eng := NewDefaultEngine()
	d := eng.Evaluate(Context{
		Tool: "read_file",
		Tier: tools.TierReadOnly,
	})
	if !d.Allow {
		t.Fatalf("tier 0 should always be allowed, got: %s", d.Reason)
	}
}

func TestTier1AutoApproved(t *testing.T) {
	eng := NewDefaultEngine()
	d := eng.Evaluate(Context{
		Tool: "write_file",
		Tier: tools.TierWrite,
	})
	if !d.Allow {
		t.Fatalf("tier 1 should be auto-approved by default, got: %s", d.Reason)
	}
}

func TestTier2DeniedByDefault(t *testing.T) {
	eng := NewDefaultEngine()
	d := eng.Evaluate(Context{
		Tool: "exec",
		Tier: tools.TierHighRisk,
	})
	if d.Allow {
		t.Fatal("tier 2 should be denied by default")
	}
	if d.Reason != "tier_2_requires_approval" {
		t.Fatalf("unexpected reason: %s", d.Reason)
	}
}

func TestTier2AllowedWhenMaxTierRaised(t *testing.T) {
	eng := NewDefaultEngine()
	eng.MaxAutoTier = 2
	d := eng.Evaluate(Context{
		Tool: "exec",
		Tier: tools.TierHighRisk,
	})
	if !d.Allow {
		t.Fatalf("tier 2 should be allowed when MaxAutoTier=2, got: %s", d.Reason)
	}
}

func TestSenderDeniedWhenNotInAllowlist(t *testing.T) {
	eng := NewDefaultEngine()
	eng.AllowedSenders = map[string]bool{"alice": true}
	d := eng.Evaluate(Context{
		Tool:   "write_file",
		Tier:   tools.TierWrite,
		Sender: "bob",
	})
	if d.Allow {
		t.Fatal("bob should be denied when not in allowlist")
	}
}

func TestSenderAllowedWhenInAllowlist(t *testing.T) {
	eng := NewDefaultEngine()
	eng.AllowedSenders = map[string]bool{"alice": true}
	d := eng.Evaluate(Context{
		Tool:   "write_file",
		Tier:   tools.TierWrite,
		Sender: "alice",
	})
	if !d.Allow {
		t.Fatalf("alice should be allowed, got: %s", d.Reason)
	}
}

func TestNoAllowlistMeansAllSendersAllowed(t *testing.T) {
	eng := NewDefaultEngine()
	d := eng.Evaluate(Context{
		Tool:   "write_file",
		Tier:   tools.TierWrite,
		Sender: "anyone",
	})
	if !d.Allow {
		t.Fatalf("should allow all senders when no allowlist, got: %s", d.Reason)
	}
}

func TestExternalMessageRestrictedToReadOnly(t *testing.T) {
	eng := NewDefaultEngine()
	eng.MaxAutoTier = 2       // owner can use shell
	eng.ExternalMaxTier = 0   // external users: read-only

	// External + tier 1 (write) → denied
	d := eng.Evaluate(Context{
		Tool:        "write_file",
		Tier:        tools.TierWrite,
		MessageType: "external",
	})
	if d.Allow {
		t.Fatal("external message should be denied write access")
	}
	if d.Reason != "tier_1_denied_for_external_message" {
		t.Fatalf("unexpected reason: %s", d.Reason)
	}

	// External + tier 0 (read) → allowed
	d = eng.Evaluate(Context{
		Tool:        "read_file",
		Tier:        tools.TierReadOnly,
		MessageType: "external",
	})
	if !d.Allow {
		t.Fatalf("external message should be allowed read-only, got: %s", d.Reason)
	}
}

func TestInternalMessageAllowsFullAccess(t *testing.T) {
	eng := NewDefaultEngine()
	eng.MaxAutoTier = 2
	eng.ExternalMaxTier = 0

	// Internal + tier 2 (shell) → allowed
	d := eng.Evaluate(Context{
		Tool:        "exec",
		Tier:        tools.TierHighRisk,
		MessageType: "internal",
	})
	if !d.Allow {
		t.Fatalf("internal message should allow shell access, got: %s", d.Reason)
	}

	// Empty message type (backward compat) → uses MaxAutoTier
	d = eng.Evaluate(Context{
		Tool: "exec",
		Tier: tools.TierHighRisk,
	})
	if !d.Allow {
		t.Fatalf("empty message type should use MaxAutoTier, got: %s", d.Reason)
	}
}
