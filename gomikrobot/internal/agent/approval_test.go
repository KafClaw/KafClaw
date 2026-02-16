package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kamir/gomikrobot/internal/approval"
	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/policy"
	"github.com/kamir/gomikrobot/internal/provider"
)

// TestApprovalFlowApproved exercises the full approval gate when the user approves.
//
//	LLM returns exec tool call (tier 2) → policy RequiresApproval → prompt sent → user approves → tool runs
func TestApprovalFlowApproved(t *testing.T) {
	tl := newTestTimeline(t)
	msgBus := bus.NewMessageBus()
	tmpDir := t.TempDir()

	// Mock provider:
	// 1) LLM requests exec tool call (tier 2)
	// 2) LLM returns final answer after tool result
	mock := &mockProvider{
		responses: []provider.ChatResponse{
			{
				Content: "",
				ToolCalls: []provider.ToolCall{{
					ID:        "call_exec_1",
					Name:      "exec",
					Arguments: map[string]any{"command": "echo hello"},
				}},
				Usage: provider.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
			},
			{
				Content: "Command executed successfully.",
				Usage:   provider.Usage{PromptTokens: 150, CompletionTokens: 30, TotalTokens: 180},
			},
		},
	}

	// Policy: MaxAutoTier=1, so tier 2 (exec) requires approval
	policyEngine := policy.NewDefaultEngine()
	policyEngine.MaxAutoTier = 1

	loop := NewLoop(LoopOptions{
		Bus:           msgBus,
		Provider:      mock,
		Timeline:      tl,
		Policy:        policyEngine,
		Workspace:     tmpDir,
		WorkRepo:      tmpDir,
		Model:         "mock-model",
		MaxIterations: 5,
	})

	// Capture outbound messages
	var outbound []*bus.OutboundMessage
	msgBus.Subscribe("whatsapp", func(msg *bus.OutboundMessage) {
		outbound = append(outbound, msg)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go msgBus.DispatchOutbound(ctx)

	// Simulate: user sends a message that triggers exec
	msg := &bus.InboundMessage{
		Channel:        "whatsapp",
		SenderID:       "owner@s.whatsapp.net",
		ChatID:         "owner@s.whatsapp.net",
		TraceID:        "trace-approval-001",
		IdempotencyKey: "wa:APPR001",
		Content:        "Run echo hello",
		Timestamp:      time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType: bus.MessageTypeInternal,
		},
	}

	// processMessage will block when it hits the approval gate.
	// We need to listen for the approval prompt on outbound and respond.
	done := make(chan struct{})
	var response string
	var taskID string
	var processErr error

	go func() {
		response, taskID, processErr = loop.processMessage(ctx, msg)
		close(done)
	}()

	// Wait for the approval prompt to appear
	var approvalID string
	deadline := time.After(5 * time.Second)
	for approvalID == "" {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for approval prompt")
		default:
			time.Sleep(50 * time.Millisecond)
			for _, o := range outbound {
				if strings.Contains(o.Content, "requires approval") {
					// Extract approval ID from "approve:<id>"
					idx := strings.Index(o.Content, "approve:")
					if idx >= 0 {
						rest := o.Content[idx+len("approve:"):]
						// ID ends at space or newline
						end := strings.IndexAny(rest, " \n\r")
						if end < 0 {
							approvalID = rest
						} else {
							approvalID = rest[:end]
						}
					}
				}
			}
		}
	}

	t.Logf("Got approval prompt, ID=%s", approvalID)

	// Simulate user approving
	err := loop.approvalMgr.Respond(approvalID, true)
	if err != nil {
		t.Fatalf("respond failed: %v", err)
	}

	// Wait for processMessage to finish
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("processMessage did not complete after approval")
	}

	if processErr != nil {
		t.Fatalf("processMessage error: %v", processErr)
	}

	if response != "Command executed successfully." {
		t.Errorf("unexpected response: %s", response)
	}

	if taskID == "" {
		t.Error("expected a task ID")
	}

	// Verify the tool was actually called (mock.calls == 2: tool call + final answer)
	if mock.calls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", mock.calls)
	}

	// Verify policy decisions: exec should be logged
	decisions, err := tl.ListPolicyDecisions("trace-approval-001")
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	var foundExec bool
	for _, d := range decisions {
		if d.Tool == "exec" {
			foundExec = true
			// The policy initially denied it (before approval), so Allowed=false in the log
			if d.Allowed {
				t.Error("policy decision should log initial denial (before interactive approval)")
			}
		}
	}
	if !foundExec {
		t.Error("expected policy decision for exec tool")
	}

	// Verify approval record in DB
	approvals, err := tl.GetPendingApprovals()
	if err != nil {
		t.Fatalf("get pending: %v", err)
	}
	if len(approvals) != 0 {
		t.Errorf("expected 0 pending approvals after response, got %d", len(approvals))
	}

	t.Logf("Approval flow (approved) passed: response=%q taskID=%s", response, taskID)
}

// TestApprovalFlowDenied exercises the approval gate when the user denies.
func TestApprovalFlowDenied(t *testing.T) {
	tl := newTestTimeline(t)
	msgBus := bus.NewMessageBus()
	tmpDir := t.TempDir()

	mock := &mockProvider{
		responses: []provider.ChatResponse{
			// 1) LLM requests exec
			{
				Content: "",
				ToolCalls: []provider.ToolCall{{
					ID:        "call_exec_deny",
					Name:      "exec",
					Arguments: map[string]any{"command": "rm -rf /"},
				}},
				Usage: provider.Usage{TotalTokens: 100},
			},
			// 2) LLM response after denial
			{
				Content: "The command was not approved.",
				Usage:   provider.Usage{TotalTokens: 80},
			},
		},
	}

	policyEngine := policy.NewDefaultEngine()
	policyEngine.MaxAutoTier = 1

	loop := NewLoop(LoopOptions{
		Bus:           msgBus,
		Provider:      mock,
		Timeline:      tl,
		Policy:        policyEngine,
		Workspace:     tmpDir,
		WorkRepo:      tmpDir,
		Model:         "mock-model",
		MaxIterations: 5,
	})

	var outbound []*bus.OutboundMessage
	msgBus.Subscribe("whatsapp", func(msg *bus.OutboundMessage) {
		outbound = append(outbound, msg)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go msgBus.DispatchOutbound(ctx)

	msg := &bus.InboundMessage{
		Channel:        "whatsapp",
		SenderID:       "owner@s.whatsapp.net",
		ChatID:         "owner@s.whatsapp.net",
		TraceID:        "trace-approval-deny",
		IdempotencyKey: "wa:APPR_DENY",
		Content:        "Delete everything",
		Timestamp:      time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType: bus.MessageTypeInternal,
		},
	}

	done := make(chan struct{})
	var response string
	var processErr error

	go func() {
		response, _, processErr = loop.processMessage(ctx, msg)
		close(done)
	}()

	// Wait for approval prompt
	var approvalID string
	deadline := time.After(5 * time.Second)
	for approvalID == "" {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for approval prompt")
		default:
			time.Sleep(50 * time.Millisecond)
			for _, o := range outbound {
				if strings.Contains(o.Content, "requires approval") {
					idx := strings.Index(o.Content, "approve:")
					if idx >= 0 {
						rest := o.Content[idx+len("approve:"):]
						end := strings.IndexAny(rest, " \n\r")
						if end < 0 {
							approvalID = rest
						} else {
							approvalID = rest[:end]
						}
					}
				}
			}
		}
	}

	// Deny
	if err := loop.approvalMgr.Respond(approvalID, false); err != nil {
		t.Fatalf("respond (deny) failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("processMessage did not complete after denial")
	}

	if processErr != nil {
		t.Fatalf("processMessage error: %v", processErr)
	}

	// After denial, the LLM should see "Policy denied: approval_denied" and produce a response
	if response != "The command was not approved." {
		t.Errorf("unexpected response: %s", response)
	}

	t.Logf("Approval flow (denied) passed: response=%q", response)
}

// TestApprovalFlowTimeout exercises the approval gate when the user does not respond.
func TestApprovalFlowTimeout(t *testing.T) {
	tl := newTestTimeline(t)
	msgBus := bus.NewMessageBus()
	tmpDir := t.TempDir()

	mock := &mockProvider{
		responses: []provider.ChatResponse{
			{
				Content: "",
				ToolCalls: []provider.ToolCall{{
					ID:        "call_exec_timeout",
					Name:      "exec",
					Arguments: map[string]any{"command": "echo timeout"},
				}},
				Usage: provider.Usage{TotalTokens: 100},
			},
			{
				Content: "Request timed out.",
				Usage:   provider.Usage{TotalTokens: 50},
			},
		},
	}

	policyEngine := policy.NewDefaultEngine()
	policyEngine.MaxAutoTier = 1

	loop := NewLoop(LoopOptions{
		Bus:           msgBus,
		Provider:      mock,
		Timeline:      tl,
		Policy:        policyEngine,
		Workspace:     tmpDir,
		WorkRepo:      tmpDir,
		Model:         "mock-model",
		MaxIterations: 5,
	})

	msgBus.Subscribe("whatsapp", func(msg *bus.OutboundMessage) {})

	// Use a short context so timeout is fast
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go msgBus.DispatchOutbound(ctx)

	msg := &bus.InboundMessage{
		Channel:        "whatsapp",
		SenderID:       "owner@s.whatsapp.net",
		ChatID:         "owner@s.whatsapp.net",
		TraceID:        "trace-approval-timeout",
		IdempotencyKey: "wa:APPR_TMO",
		Content:        "Run something",
		Timestamp:      time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType: bus.MessageTypeInternal,
		},
	}

	response, _, err := loop.processMessage(ctx, msg)
	if err != nil {
		t.Fatalf("processMessage error: %v", err)
	}

	// After timeout, the tool is denied and the LLM responds
	if response != "Request timed out." {
		t.Errorf("unexpected response after timeout: %s", response)
	}

	t.Logf("Approval flow (timeout) passed: response=%q", response)
}

// TestParseApprovalResponse tests the approval response parser.
func TestParseApprovalResponse(t *testing.T) {
	tests := []struct {
		input    string
		wantID   string
		wantOK   bool
		wantAppr bool
	}{
		{"approve:abc123", "abc123", true, true},
		{"deny:abc123", "abc123", true, false},
		{"  approve:def456  ", "def456", true, true},
		{"  deny:def456  ", "def456", true, false},
		{"hello world", "", false, false},
		{"approve:", "", false, false},
		{"deny:", "", false, false},
		{"", "", false, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			id, approved, ok := parseApprovalResponse(tt.input)
			if ok != tt.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tt.wantOK)
			}
			if id != tt.wantID {
				t.Errorf("id: got %q, want %q", id, tt.wantID)
			}
			if approved != tt.wantAppr {
				t.Errorf("approved: got %v, want %v", approved, tt.wantAppr)
			}
		})
	}
}

// TestApprovalRunInterception verifies that the Run() loop intercepts
// approve:/deny: messages and routes them to the approval manager.
func TestApprovalRunInterception(t *testing.T) {
	tl := newTestTimeline(t)
	msgBus := bus.NewMessageBus()
	tmpDir := t.TempDir()

	mock := &mockProvider{}

	loop := NewLoop(LoopOptions{
		Bus:           msgBus,
		Provider:      mock,
		Timeline:      tl,
		Policy:        policy.NewDefaultEngine(),
		Workspace:     tmpDir,
		WorkRepo:      tmpDir,
		Model:         "mock-model",
		MaxIterations: 5,
	})

	// Pre-create a pending approval
	req := &approval.ApprovalRequest{Tool: "exec", Tier: 2}
	id := loop.approvalMgr.Create(req)

	// Capture outbound
	var outbound []*bus.OutboundMessage
	msgBus.Subscribe("whatsapp", func(msg *bus.OutboundMessage) {
		outbound = append(outbound, msg)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go msgBus.DispatchOutbound(ctx)

	// Start Run() in background
	go loop.Run(ctx)

	// Send the approval response via bus
	msgBus.PublishInbound(&bus.InboundMessage{
		Channel:   "whatsapp",
		SenderID:  "owner@s.whatsapp.net",
		ChatID:    "owner@s.whatsapp.net",
		Content:   fmt.Sprintf("approve:%s", id),
		Timestamp: time.Now(),
	})

	// Wait a bit for processing
	time.Sleep(200 * time.Millisecond)

	// Check outbound for confirmation message
	var found bool
	for _, o := range outbound {
		if strings.Contains(o.Content, id) && strings.Contains(o.Content, "approved") {
			found = true
		}
	}
	if !found {
		t.Error("expected outbound confirmation message for approval")
	}

	loop.Stop()
	t.Logf("Run() interception test passed for approval ID=%s", id)
}
