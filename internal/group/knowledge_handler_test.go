package group

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/knowledge"
	"github.com/KafClaw/KafClaw/internal/timeline"
)

func TestKnowledgeHandlerProcess_ValidAndIdempotent(t *testing.T) {
	tl, err := timeline.NewTimelineService(filepath.Join(t.TempDir(), "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	defer tl.Close()

	h := NewKnowledgeHandler(tl, "local-claw")
	env := knowledge.Envelope{
		SchemaVersion:  knowledge.CurrentSchemaVersion,
		Type:           knowledge.TypeProposal,
		TraceID:        "trace-1",
		Timestamp:      time.Now(),
		IdempotencyKey: "idem-1",
		ClawID:         "remote-claw",
		InstanceID:     "inst-1",
		Payload: map[string]any{
			"proposalId": "p1",
			"group":      "g1",
			"statement":  "Runbook v2",
		},
	}
	raw, _ := json.Marshal(env)
	if err := h.Process("group.g.knowledge.proposals", raw); err != nil {
		t.Fatalf("process first message: %v", err)
	}
	if err := h.Process("group.g.knowledge.proposals", raw); err != nil {
		t.Fatalf("process duplicate message: %v", err)
	}

	var n int
	if err := tl.DB().QueryRow(`SELECT COUNT(*) FROM knowledge_idempotency WHERE idempotency_key = 'idem-1'`).Scan(&n); err != nil {
		t.Fatalf("count idempotency rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected one idempotency row, got %d", n)
	}
}

func TestKnowledgeHandlerProcess_RejectsMissingIdentity(t *testing.T) {
	tl, err := timeline.NewTimelineService(filepath.Join(t.TempDir(), "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	defer tl.Close()

	h := NewKnowledgeHandler(tl, "local-claw")
	env := knowledge.Envelope{
		SchemaVersion:  knowledge.CurrentSchemaVersion,
		Type:           knowledge.TypeVote,
		TraceID:        "trace-2",
		Timestamp:      time.Now(),
		IdempotencyKey: "idem-2",
		ClawID:         "",
		InstanceID:     "inst-2",
		Payload: map[string]any{
			"proposalId": "p1",
			"vote":       "yes",
		},
	}
	raw, _ := json.Marshal(env)
	if err := h.Process("group.g.knowledge.votes", raw); err == nil {
		t.Fatal("expected validation error for missing clawId")
	}
}

func TestKnowledgeHandlerProcess_FactVersionPolicy(t *testing.T) {
	tl, err := timeline.NewTimelineService(filepath.Join(t.TempDir(), "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	defer tl.Close()

	h := NewKnowledgeHandler(tl, "local-claw")
	makeEnv := func(idem string, version int, object string) []byte {
		env := knowledge.Envelope{
			SchemaVersion:  knowledge.CurrentSchemaVersion,
			Type:           knowledge.TypeFact,
			TraceID:        "trace-fact",
			Timestamp:      time.Now(),
			IdempotencyKey: idem,
			ClawID:         "remote-claw",
			InstanceID:     "inst-1",
			Payload: knowledge.FactPayload{
				FactID:    "fact-1",
				Group:     "g1",
				Subject:   "service",
				Predicate: "runbook",
				Object:    object,
				Version:   version,
				Source:    "decision:d1",
			},
		}
		raw, _ := json.Marshal(env)
		return raw
	}

	if err := h.Process("group.g1.knowledge.facts", makeEnv("idem-f1", 1, "v1")); err != nil {
		t.Fatalf("process fact v1: %v", err)
	}
	cur, err := tl.GetKnowledgeFactLatest("fact-1")
	if err != nil {
		t.Fatalf("get fact latest after v1: %v", err)
	}
	if cur == nil || cur.Version != 1 || cur.Object != "v1" {
		t.Fatalf("unexpected fact latest after v1: %+v", cur)
	}

	// Gap version should conflict and not overwrite latest.
	if err := h.Process("group.g1.knowledge.facts", makeEnv("idem-f2", 3, "v3")); err != nil {
		t.Fatalf("process fact v3 gap: %v", err)
	}
	cur, err = tl.GetKnowledgeFactLatest("fact-1")
	if err != nil {
		t.Fatalf("get fact latest after gap: %v", err)
	}
	if cur == nil || cur.Version != 1 || cur.Object != "v1" {
		t.Fatalf("expected latest unchanged on gap conflict, got %+v", cur)
	}

	if err := h.Process("group.g1.knowledge.facts", makeEnv("idem-f3", 2, "v2")); err != nil {
		t.Fatalf("process fact v2: %v", err)
	}
	cur, err = tl.GetKnowledgeFactLatest("fact-1")
	if err != nil {
		t.Fatalf("get fact latest after v2: %v", err)
	}
	if cur == nil || cur.Version != 2 || cur.Object != "v2" {
		t.Fatalf("unexpected fact latest after v2: %+v", cur)
	}
}

func TestKnowledgeHandlerProcess_ProposalDecisionFactEndToEnd(t *testing.T) {
	tl, err := timeline.NewTimelineService(filepath.Join(t.TempDir(), "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	defer tl.Close()

	h := NewKnowledgeHandler(tl, "local-claw")
	now := time.Now()

	makeRaw := func(idem string, msgType string, payload any) []byte {
		env := knowledge.Envelope{
			SchemaVersion:  knowledge.CurrentSchemaVersion,
			Type:           msgType,
			TraceID:        "trace-e2e",
			Timestamp:      now,
			IdempotencyKey: idem,
			ClawID:         "remote-claw",
			InstanceID:     "inst-1",
			Payload:        payload,
		}
		raw, _ := json.Marshal(env)
		return raw
	}

	if err := h.Process("group.g1.knowledge.proposals", makeRaw("idem-p1", knowledge.TypeProposal, knowledge.ProposalPayload{
		ProposalID: "p-e2e",
		Group:      "g1",
		Title:      "Runbook update",
		Statement:  "Adopt runbook v2",
		Tags:       []string{"ops"},
	})); err != nil {
		t.Fatalf("process proposal: %v", err)
	}

	if err := h.Process("group.g1.knowledge.votes", makeRaw("idem-v1", knowledge.TypeVote, knowledge.VotePayload{
		ProposalID: "p-e2e",
		Vote:       "yes",
		Reason:     "approved by team",
	})); err != nil {
		t.Fatalf("process vote: %v", err)
	}

	if err := h.Process("group.g1.knowledge.decisions", makeRaw("idem-d1", knowledge.TypeDecision, knowledge.DecisionPayload{
		ProposalID: "p-e2e",
		Outcome:    "approved",
		Yes:        3,
		No:         0,
		Reason:     "quorum reached",
	})); err != nil {
		t.Fatalf("process decision: %v", err)
	}

	if err := h.Process("group.g1.knowledge.facts", makeRaw("idem-f1", knowledge.TypeFact, knowledge.FactPayload{
		FactID:     "fact-e2e-1",
		Group:      "g1",
		Subject:    "service",
		Predicate:  "runbook",
		Object:     "v2",
		Version:    1,
		Source:     "decision:p-e2e",
		ProposalID: "p-e2e",
		DecisionID: "decision:p-e2e",
		Tags:       []string{"ops"},
	})); err != nil {
		t.Fatalf("process fact: %v", err)
	}

	prop, err := tl.GetKnowledgeProposal("p-e2e")
	if err != nil {
		t.Fatalf("get proposal: %v", err)
	}
	if prop == nil || prop.Status != "approved" || prop.YesVotes != 3 || prop.NoVotes != 0 {
		t.Fatalf("unexpected proposal state: %+v", prop)
	}

	votes, err := tl.ListKnowledgeVotes("p-e2e")
	if err != nil {
		t.Fatalf("list votes: %v", err)
	}
	if len(votes) != 1 || votes[0].Vote != "yes" {
		t.Fatalf("unexpected votes: %+v", votes)
	}

	fact, err := tl.GetKnowledgeFactLatest("fact-e2e-1")
	if err != nil {
		t.Fatalf("get fact latest: %v", err)
	}
	if fact == nil || fact.Version != 1 || fact.Object != "v2" || fact.ProposalID != "p-e2e" {
		t.Fatalf("unexpected fact state: %+v", fact)
	}
}
