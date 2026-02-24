package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/timeline"
)

func TestKnowledgeCLIProposeVoteDecisionsFacts(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := `{
	  "node": {"clawId":"local-claw","instanceId":"inst-local"},
	  "group": {"lfsProxyUrl":"http://127.0.0.1:8080"},
	  "knowledge": {
	    "enabled": true,
	    "group": "g1",
	    "topics": {
	      "capabilities":"group.g1.knowledge.capabilities",
	      "presence":"group.g1.knowledge.presence",
	      "proposals":"group.g1.knowledge.proposals",
	      "votes":"group.g1.knowledge.votes",
	      "decisions":"group.g1.knowledge.decisions",
	      "facts":"group.g1.knowledge.facts"
	    },
	    "voting": {"enabled": true, "minPoolSize": 3, "quorumYes": 2, "quorumNo": 2, "timeoutSec": 120, "allowSelfVote": false}
	  }
	}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	out, err := runRootCommand(t,
		"knowledge", "propose",
		"--proposal-id=p1",
		"--group=g1",
		"--statement=Use runbook v2",
		"--json",
	)
	if err != nil {
		t.Fatalf("knowledge propose: %v", err)
	}
	if !strings.Contains(out, `"proposalId": "p1"`) {
		t.Fatalf("unexpected propose output: %s", out)
	}

	out, err = runRootCommand(t,
		"knowledge", "vote",
		"--proposal-id=p1",
		"--vote=yes",
		"--as-claw=claw-a",
		"--as-instance=inst-a",
		"--pool-size=4",
		"--json",
	)
	if err != nil {
		t.Fatalf("knowledge vote #1: %v", err)
	}
	if !strings.Contains(out, `"Status": "pending"`) {
		t.Fatalf("expected pending decision after first vote, got: %s", out)
	}

	out, err = runRootCommand(t,
		"knowledge", "vote",
		"--proposal-id=p1",
		"--vote=yes",
		"--as-claw=claw-b",
		"--as-instance=inst-b",
		"--pool-size=4",
		"--json",
	)
	if err != nil {
		t.Fatalf("knowledge vote #2: %v", err)
	}
	if !strings.Contains(out, `"Status": "approved"`) {
		t.Fatalf("expected approved decision after second vote, got: %s", out)
	}

	out, err = runRootCommand(t, "knowledge", "decisions", "--status=approved", "--json")
	if err != nil {
		t.Fatalf("knowledge decisions: %v", err)
	}
	if !strings.Contains(out, `"proposal_id": "p1"`) {
		t.Fatalf("expected approved decision list to include p1, got: %s", out)
	}

	// Seed an accepted fact and verify facts command surfaces it.
	tl, err := timeline.NewTimelineService(filepath.Join(cfgDir, "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	if err := tl.UpsertKnowledgeFactLatest(&timeline.KnowledgeFactRecord{
		FactID:    "f1",
		GroupName: "g1",
		Subject:   "service",
		Predicate: "runbook",
		Object:    "v2",
		Version:   1,
		Source:    "decision:p1",
		Tags:      `["ops"]`,
	}); err != nil {
		t.Fatalf("upsert fact: %v", err)
	}
	_ = tl.Close()

	out, err = runRootCommand(t, "knowledge", "facts", "--group=g1", "--json")
	if err != nil {
		t.Fatalf("knowledge facts: %v", err)
	}
	if !strings.Contains(out, `"fact_id": "f1"`) {
		t.Fatalf("expected facts output to include f1, got: %s", out)
	}

	out, err = runRootCommand(t, "knowledge", "status", "--json")
	if err != nil {
		t.Fatalf("knowledge status: %v", err)
	}
	var payload map[string]any
	if json.Unmarshal([]byte(out), &payload) != nil {
		t.Fatalf("expected JSON status output, got: %s", out)
	}
}

func TestKnowledgeVoteRequiresProposalID(t *testing.T) {
	if _, err := runRootCommand(t, "knowledge", "vote", "--vote=yes"); err == nil {
		t.Fatal("expected missing proposal-id error")
	}
}
