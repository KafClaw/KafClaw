// Package policy provides tool execution authorization.
package policy

import (
	"fmt"
	"time"

	"github.com/kamir/gomikrobot/internal/tools"
)

// Context holds information about a pending tool execution.
type Context struct {
	Sender      string
	Channel     string
	Tool        string
	Tier        int
	Arguments   map[string]any
	TraceID     string
	MessageType string // "internal" or "external"
}

// Decision is the result of a policy evaluation.
type Decision struct {
	Allow            bool
	RequiresApproval bool // true when tier exceeds auto-approve but interactive approval is possible
	Reason           string
	Tier             int
	Ts               time.Time
	TraceID          string
}

// Engine evaluates whether a tool execution should proceed.
type Engine interface {
	Evaluate(ctx Context) Decision
}

// DefaultEngine is the v1 policy implementation.
// It checks tool tier against configured max tier and sender authorization.
type DefaultEngine struct {
	// MaxAutoTier is the highest tier that is auto-approved (default: 1).
	// Tools with tier > MaxAutoTier are denied.
	MaxAutoTier int
	// ExternalMaxTier is the highest tier auto-approved for external messages.
	// Defaults to 0 (read-only). Only applies when MessageType is explicitly "external".
	ExternalMaxTier int
	// AllowedSenders is the set of senders permitted to trigger tools.
	// If empty, all senders are allowed.
	AllowedSenders map[string]bool
}

// NewDefaultEngine creates a policy engine with sensible defaults.
func NewDefaultEngine() *DefaultEngine {
	return &DefaultEngine{
		MaxAutoTier: 1,
	}
}

// Evaluate checks tool tier and sender authorization.
func (e *DefaultEngine) Evaluate(ctx Context) Decision {
	d := Decision{
		Tier:    ctx.Tier,
		Ts:      time.Now(),
		TraceID: ctx.TraceID,
	}

	// Tier 0 tools are always allowed
	if ctx.Tier == tools.TierReadOnly {
		d.Allow = true
		d.Reason = "tier_0_always_allowed"
		return d
	}

	// Check sender authorization if allowlist is configured
	if len(e.AllowedSenders) > 0 && ctx.Sender != "" {
		if !e.AllowedSenders[ctx.Sender] {
			d.Allow = false
			d.Reason = fmt.Sprintf("sender_not_authorized: %s", ctx.Sender)
			return d
		}
	}

	// Determine effective max tier based on message type
	effectiveMaxTier := e.MaxAutoTier
	if ctx.MessageType == "external" {
		effectiveMaxTier = e.ExternalMaxTier
	}

	// Check tier against max auto-approved tier
	if ctx.Tier > effectiveMaxTier {
		d.Allow = false
		if ctx.MessageType == "external" {
			d.Reason = fmt.Sprintf("tier_%d_denied_for_external_message", ctx.Tier)
		} else {
			d.RequiresApproval = true
			d.Reason = fmt.Sprintf("tier_%d_requires_approval", ctx.Tier)
		}
		return d
	}

	d.Allow = true
	d.Reason = fmt.Sprintf("tier_%d_auto_approved", ctx.Tier)
	return d
}
