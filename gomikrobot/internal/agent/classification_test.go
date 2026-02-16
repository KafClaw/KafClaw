package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/policy"
	"github.com/kamir/gomikrobot/internal/provider"
	"github.com/kamir/gomikrobot/internal/timeline"
	"github.com/kamir/gomikrobot/internal/tools"
)

// mockProvider returns a canned response and records calls for inspection.
type mockProvider struct {
	responses []provider.ChatResponse
	calls     int
}

func (m *mockProvider) Chat(_ context.Context, req *provider.ChatRequest) (*provider.ChatResponse, error) {
	idx := m.calls
	m.calls++
	if idx < len(m.responses) {
		return &m.responses[idx], nil
	}
	return &provider.ChatResponse{Content: "mock response", Usage: provider.Usage{TotalTokens: 10}}, nil
}
func (m *mockProvider) Transcribe(_ context.Context, _ *provider.AudioRequest) (*provider.AudioResponse, error) {
	return &provider.AudioResponse{Text: ""}, nil
}
func (m *mockProvider) Speak(_ context.Context, _ *provider.TTSRequest) (*provider.TTSResponse, error) {
	return &provider.TTSResponse{}, nil
}
func (m *mockProvider) DefaultModel() string { return "mock-model" }

// TestInternalExternalClassificationE2E exercises the full pipeline:
//
//   Bus → processMessage → BuildMessages (prompt injection) → policy check → timeline task
//
// for both internal (owner) and external (other user) messages, then prints a
// trace-style report showing how the system differentiates the two.
func TestInternalExternalClassificationE2E(t *testing.T) {
	tl := newTestTimeline(t)
	msgBus := bus.NewMessageBus()
	tmpDir := t.TempDir()

	// Mock provider: first call returns a tool call (write_file), second
	// call returns the final text answer. This pattern repeats for each message.
	mock := &mockProvider{
		responses: []provider.ChatResponse{
			// --- Internal message processing ---
			// 1) LLM wants to call write_file
			{
				Content: "",
				ToolCalls: []provider.ToolCall{{
					ID:        "call_int_1",
					Name:      "write_file",
					Arguments: map[string]any{"path": "notes.md", "content": "hello"},
				}},
				Usage: provider.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
			},
			// 2) LLM returns final answer after tool result
			{
				Content: "Done — wrote notes.md for you.",
				Usage:   provider.Usage{PromptTokens: 150, CompletionTokens: 30, TotalTokens: 180},
			},
			// --- External message processing ---
			// 3) LLM wants to call write_file (will be denied by policy)
			{
				Content: "",
				ToolCalls: []provider.ToolCall{{
					ID:        "call_ext_1",
					Name:      "write_file",
					Arguments: map[string]any{"path": "hack.md", "content": "pwned"},
				}},
				Usage: provider.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
			},
			// 4) LLM gives a text answer after the policy denial
			{
				Content: "Sorry, I can only read files for you.",
				Usage:   provider.Usage{PromptTokens: 120, CompletionTokens: 25, TotalTokens: 145},
			},
		},
	}

	// Policy: owner (internal) gets tier 2, external gets tier 0.
	policyEngine := policy.NewDefaultEngine()
	policyEngine.MaxAutoTier = 2
	policyEngine.ExternalMaxTier = 0

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

	ctx := context.Background()

	// ---------------------------------------------------------------
	// Scenario 1: INTERNAL message (owner via WhatsApp IsFromMe=true)
	// ---------------------------------------------------------------
	t.Log("━━━ Scenario 1: INTERNAL message (bot owner) ━━━")

	internalMsg := &bus.InboundMessage{
		Channel:        "whatsapp",
		SenderID:       "owner@s.whatsapp.net",
		ChatID:         "owner@s.whatsapp.net",
		TraceID:        "trace-internal-001",
		IdempotencyKey: "wa:INT001",
		Content:        "Write a note about today's progress",
		Timestamp:      time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType: bus.MessageTypeInternal,
			bus.MetaKeyIsFromMe:    true,
		},
	}

	// Verify accessor
	if internalMsg.MessageType() != bus.MessageTypeInternal {
		t.Fatalf("expected internal, got %s", internalMsg.MessageType())
	}

	response1, taskID1, err := loop.processMessage(ctx, internalMsg)
	if err != nil {
		t.Fatalf("processMessage (internal) error: %v", err)
	}

	// ---------------------------------------------------------------
	// Scenario 2: EXTERNAL message (authorized user, not owner)
	// ---------------------------------------------------------------
	t.Log("━━━ Scenario 2: EXTERNAL message (authorized user) ━━━")

	// Reset activeMessageType so the next message starts clean
	loop.activeMessageType = ""

	externalMsg := &bus.InboundMessage{
		Channel:        "whatsapp",
		SenderID:       "friend@s.whatsapp.net",
		ChatID:         "friend@s.whatsapp.net",
		TraceID:        "trace-external-002",
		IdempotencyKey: "wa:EXT002",
		Content:        "Write a file to the repo",
		Timestamp:      time.Now(),
		Metadata: map[string]any{
			bus.MetaKeyMessageType: bus.MessageTypeExternal,
			bus.MetaKeyIsFromMe:    false,
		},
	}

	if externalMsg.MessageType() != bus.MessageTypeExternal {
		t.Fatalf("expected external, got %s", externalMsg.MessageType())
	}

	response2, taskID2, err := loop.processMessage(ctx, externalMsg)
	if err != nil {
		t.Fatalf("processMessage (external) error: %v", err)
	}

	// ---------------------------------------------------------------
	// Scenario 3: CLI message (always internal, no metadata)
	// ---------------------------------------------------------------
	t.Log("━━━ Scenario 3: CLI message (always internal) ━━━")

	cliMsg := &bus.InboundMessage{
		Channel:  "cli",
		SenderID: "local",
		ChatID:   "default",
		TraceID:  "trace-cli-003",
		Content:  "hello",
	}

	if cliMsg.MessageType() != bus.MessageTypeExternal {
		// No metadata → accessor defaults to external, but processMessage/ProcessDirect
		// will override to internal for CLI
		t.Fatalf("expected accessor default external, got %s", cliMsg.MessageType())
	}

	// ---------------------------------------------------------------
	// Verify tasks in timeline
	// ---------------------------------------------------------------
	t.Log("\n━━━ Timeline Task Verification ━━━")

	task1, err := tl.GetTask(taskID1)
	if err != nil {
		t.Fatalf("get task 1: %v", err)
	}
	if task1.MessageType != "internal" {
		t.Errorf("task1 message_type: want internal, got %s", task1.MessageType)
	}

	task2, err := tl.GetTask(taskID2)
	if err != nil {
		t.Fatalf("get task 2: %v", err)
	}
	if task2.MessageType != "external" {
		t.Errorf("task2 message_type: want external, got %s", task2.MessageType)
	}

	// ---------------------------------------------------------------
	// Verify policy decisions
	// ---------------------------------------------------------------
	t.Log("\n━━━ Policy Decision Verification ━━━")

	decisions1, err := tl.ListPolicyDecisions("trace-internal-001")
	if err != nil {
		t.Fatalf("list decisions 1: %v", err)
	}

	decisions2, err := tl.ListPolicyDecisions("trace-external-002")
	if err != nil {
		t.Fatalf("list decisions 2: %v", err)
	}

	// Internal: write_file (tier 1) should be ALLOWED
	var internalWriteAllowed bool
	for _, d := range decisions1 {
		if d.Tool == "write_file" {
			internalWriteAllowed = d.Allowed
		}
	}
	if !internalWriteAllowed {
		t.Error("internal message should allow write_file")
	}

	// External: write_file (tier 1) should be DENIED
	var externalWriteDenied bool
	for _, d := range decisions2 {
		if d.Tool == "write_file" {
			externalWriteDenied = !d.Allowed
		}
	}
	if !externalWriteDenied {
		t.Error("external message should deny write_file")
	}

	// ---------------------------------------------------------------
	// Print trace report
	// ---------------------------------------------------------------
	report := buildTraceReport(t, tl, task1, task2, decisions1, decisions2, response1, response2)
	t.Log(report)
}

// buildTraceReport creates a human-readable trace report for visual inspection.
func buildTraceReport(
	t *testing.T,
	tl *timeline.TimelineService,
	task1, task2 *timeline.AgentTask,
	decisions1, decisions2 []timeline.PolicyDecisionRecord,
	response1, response2 string,
) string {
	t.Helper()

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("╔══════════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║        INTERNAL vs EXTERNAL MESSAGE CLASSIFICATION TRACE        ║\n")
	sb.WriteString("╚══════════════════════════════════════════════════════════════════╝\n")

	// --- Trace 1: Internal ---
	sb.WriteString("\n")
	sb.WriteString("┌──────────────────────────────────────────────────────────────────┐\n")
	sb.WriteString("│ TRACE: trace-internal-001  (WhatsApp, IsFromMe=true)            │\n")
	sb.WriteString("├──────────────────────────────────────────────────────────────────┤\n")
	sb.WriteString(fmt.Sprintf("│ Task ID:      %s\n", task1.TaskID))
	sb.WriteString(fmt.Sprintf("│ Message Type: %s\n", task1.MessageType))
	sb.WriteString(fmt.Sprintf("│ Channel:      %s\n", task1.Channel))
	sb.WriteString(fmt.Sprintf("│ Sender:       %s\n", task1.SenderID))
	sb.WriteString(fmt.Sprintf("│ Status:       %s\n", task1.Status))
	sb.WriteString(fmt.Sprintf("│ Tokens:       %d prompt + %d completion = %d total\n",
		task1.PromptTokens, task1.CompletionTokens, task1.TotalTokens))
	sb.WriteString("│\n")
	sb.WriteString("│ Policy Decisions:\n")
	for _, d := range decisions1 {
		icon := "ALLOW"
		if !d.Allowed {
			icon = "DENY "
		}
		sb.WriteString(fmt.Sprintf("│   [%s] tool=%-12s tier=%d  reason=%s\n",
			icon, d.Tool, d.Tier, d.Reason))
	}
	sb.WriteString("│\n")
	sb.WriteString(fmt.Sprintf("│ Response: %s\n", truncate(response1, 60)))
	sb.WriteString("└──────────────────────────────────────────────────────────────────┘\n")

	// --- Trace 2: External ---
	sb.WriteString("\n")
	sb.WriteString("┌──────────────────────────────────────────────────────────────────┐\n")
	sb.WriteString("│ TRACE: trace-external-002  (WhatsApp, IsFromMe=false)           │\n")
	sb.WriteString("├──────────────────────────────────────────────────────────────────┤\n")
	sb.WriteString(fmt.Sprintf("│ Task ID:      %s\n", task2.TaskID))
	sb.WriteString(fmt.Sprintf("│ Message Type: %s\n", task2.MessageType))
	sb.WriteString(fmt.Sprintf("│ Channel:      %s\n", task2.Channel))
	sb.WriteString(fmt.Sprintf("│ Sender:       %s\n", task2.SenderID))
	sb.WriteString(fmt.Sprintf("│ Status:       %s\n", task2.Status))
	sb.WriteString(fmt.Sprintf("│ Tokens:       %d prompt + %d completion = %d total\n",
		task2.PromptTokens, task2.CompletionTokens, task2.TotalTokens))
	sb.WriteString("│\n")
	sb.WriteString("│ Policy Decisions:\n")
	for _, d := range decisions2 {
		icon := "ALLOW"
		if !d.Allowed {
			icon = "DENY "
		}
		sb.WriteString(fmt.Sprintf("│   [%s] tool=%-12s tier=%d  reason=%s\n",
			icon, d.Tool, d.Tier, d.Reason))
	}
	sb.WriteString("│\n")
	sb.WriteString(fmt.Sprintf("│ Response: %s\n", truncate(response2, 60)))
	sb.WriteString("└──────────────────────────────────────────────────────────────────┘\n")

	// --- Summary ---
	sb.WriteString("\n")
	sb.WriteString("┌──────────────────────────────────────────────────────────────────┐\n")
	sb.WriteString("│ SUMMARY                                                          │\n")
	sb.WriteString("├──────────────────────────────────────────────────────────────────┤\n")
	sb.WriteString("│ Channel→Type Mapping:                                            │\n")
	sb.WriteString("│   WhatsApp (IsFromMe=true)   → internal  (full access)          │\n")
	sb.WriteString("│   WhatsApp (IsFromMe=false)  → external  (read-only)            │\n")
	sb.WriteString("│   CLI (agent -m)             → internal  (always owner)         │\n")
	sb.WriteString("│   Web UI                     → external  (default)              │\n")
	sb.WriteString("│   Group (Kafka)              → external  (other agents)         │\n")
	sb.WriteString("│                                                                  │\n")
	sb.WriteString("│ Tier Access:                                                     │\n")
	sb.WriteString("│   internal → MaxAutoTier=2    (read + write + shell)            │\n")
	sb.WriteString("│   external → ExternalMaxTier=0 (read-only)                      │\n")
	sb.WriteString("└──────────────────────────────────────────────────────────────────┘\n")

	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// TestMessageTypeAccessorDefaults verifies the bus accessor logic.
func TestMessageTypeAccessorDefaults(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		want     string
	}{
		{"nil metadata defaults to external", nil, bus.MessageTypeExternal},
		{"empty metadata defaults to external", map[string]any{}, bus.MessageTypeExternal},
		{"explicit internal", map[string]any{bus.MetaKeyMessageType: bus.MessageTypeInternal}, bus.MessageTypeInternal},
		{"explicit external", map[string]any{bus.MetaKeyMessageType: bus.MessageTypeExternal}, bus.MessageTypeExternal},
		{"non-string value defaults to external", map[string]any{bus.MetaKeyMessageType: 42}, bus.MessageTypeExternal},
		{"empty string defaults to external", map[string]any{bus.MetaKeyMessageType: ""}, bus.MessageTypeExternal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &bus.InboundMessage{Metadata: tt.metadata}
			got := msg.MessageType()
			if got != tt.want {
				t.Errorf("MessageType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPolicyTierGatingByMessageType verifies the policy engine
// applies different tier limits for internal vs external messages.
func TestPolicyTierGatingByMessageType(t *testing.T) {
	eng := policy.NewDefaultEngine()
	eng.MaxAutoTier = 2
	eng.ExternalMaxTier = 0

	tests := []struct {
		name        string
		messageType string
		tier        int
		wantAllow   bool
	}{
		{"internal tier 0 allowed", "internal", tools.TierReadOnly, true},
		{"internal tier 1 allowed", "internal", tools.TierWrite, true},
		{"internal tier 2 allowed", "internal", tools.TierHighRisk, true},
		{"external tier 0 allowed", "external", tools.TierReadOnly, true},
		{"external tier 1 denied", "external", tools.TierWrite, false},
		{"external tier 2 denied", "external", tools.TierHighRisk, false},
		{"empty type tier 0 allowed", "", tools.TierReadOnly, true},
		{"empty type tier 1 allowed", "", tools.TierWrite, true},
		{"empty type tier 2 allowed (uses MaxAutoTier)", "", tools.TierHighRisk, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := eng.Evaluate(policy.Context{
				Tool:        "test_tool",
				Tier:        tt.tier,
				MessageType: tt.messageType,
			})
			if d.Allow != tt.wantAllow {
				t.Errorf("Allow = %v, want %v (reason: %s)", d.Allow, tt.wantAllow, d.Reason)
			}
		})
	}
}
