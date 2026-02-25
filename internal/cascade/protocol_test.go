package cascade

import "testing"

func TestValidateContract(t *testing.T) {
	if err := ValidateContract(TaskContract{TaskID: "t1", Sequence: 1}); err != nil {
		t.Fatalf("expected valid contract, got %v", err)
	}
	if err := ValidateContract(TaskContract{TaskID: "", Sequence: 1}); err == nil {
		t.Fatal("expected error for empty task_id")
	}
	if err := ValidateContract(TaskContract{TaskID: "t1", Sequence: 0}); err == nil {
		t.Fatal("expected error for sequence<=0")
	}
}

func TestCanTransition(t *testing.T) {
	if !CanTransition(StatePending, StateRunning) {
		t.Fatal("pending->running should be valid")
	}
	if !CanTransition(StateSelfTest, StatePending) {
		t.Fatal("self_test->pending should be valid")
	}
	if CanTransition(StateCommitted, StateValidated) {
		t.Fatal("committed->validated should be invalid")
	}
	if CanTransition(StateFailed, StatePending) {
		t.Fatal("failed should be terminal")
	}
}

func TestCanStart(t *testing.T) {
	start, reason := CanStart(TaskSnapshot{
		TaskID:     "t1",
		Sequence:   1,
		State:      StatePending,
		RetryCount: 0,
		MaxRetries: 2,
	}, nil)
	if !start || reason != "" {
		t.Fatalf("expected start allowed, got start=%v reason=%q", start, reason)
	}

	start, reason = CanStart(TaskSnapshot{
		TaskID:     "t2",
		Sequence:   2,
		State:      StatePending,
		RetryCount: 0,
		MaxRetries: 2,
	}, &TaskSnapshot{TaskID: "t1", State: StateValidated})
	if start || reason == "" {
		t.Fatalf("expected block until predecessor committed, got start=%v reason=%q", start, reason)
	}

	start, reason = CanStart(TaskSnapshot{
		TaskID:     "t2",
		Sequence:   2,
		State:      StatePending,
		RetryCount: 1,
		MaxRetries: 2,
	}, &TaskSnapshot{TaskID: "t1", State: StateCommitted})
	if !start || reason != "" {
		t.Fatalf("expected start allowed after predecessor commit, got start=%v reason=%q", start, reason)
	}
}

func TestCanStartRetryBudgetAndState(t *testing.T) {
	start, reason := CanStart(TaskSnapshot{
		TaskID:     "t1",
		Sequence:   1,
		State:      StateRunning,
		RetryCount: 0,
		MaxRetries: 1,
	}, nil)
	if start || reason == "" {
		t.Fatalf("expected block when not pending, got start=%v reason=%q", start, reason)
	}

	start, reason = CanStart(TaskSnapshot{
		TaskID:     "t1",
		Sequence:   1,
		State:      StatePending,
		RetryCount: 1,
		MaxRetries: 1,
	}, nil)
	if start || reason == "" {
		t.Fatalf("expected retry budget exhaustion, got start=%v reason=%q", start, reason)
	}
}

func TestValidateIO(t *testing.T) {
	contract := TaskContract{
		TaskID:          "t1",
		Sequence:        1,
		RequiredInput:   []string{"source_doc", "ticket_id"},
		ProducedOutput:  []string{"summary", "risk_level"},
		ValidationRules: []string{"non_empty:summary", "equals:risk_level:low"},
	}
	ok := ValidateIO(contract,
		map[string]string{"source_doc": "doc", "ticket_id": "T-1"},
		map[string]string{"summary": "all good", "risk_level": "low"},
	)
	if !ok.OK {
		t.Fatalf("expected valid io, got %+v", ok)
	}

	bad := ValidateIO(contract,
		map[string]string{"source_doc": "doc"},
		map[string]string{"summary": "", "risk_level": "high"},
	)
	if bad.OK {
		t.Fatalf("expected invalid io, got %+v", bad)
	}
	if len(bad.MissingInput) != 1 || bad.MissingInput[0] != "ticket_id" {
		t.Fatalf("unexpected missing input: %+v", bad.MissingInput)
	}
	if len(bad.MissingOutput) != 1 || bad.MissingOutput[0] != "summary" {
		t.Fatalf("unexpected missing output: %+v", bad.MissingOutput)
	}
	if len(bad.InvalidRules) != 2 {
		t.Fatalf("expected two invalid rules, got %+v", bad.InvalidRules)
	}
	if bad.RemediationMsg == "" {
		t.Fatal("expected remediation message")
	}
}

func TestValidateIOUnknownRule(t *testing.T) {
	contract := TaskContract{
		TaskID:          "t1",
		Sequence:        1,
		ValidationRules: []string{"unknown_rule"},
	}
	res := ValidateIO(contract, nil, map[string]string{})
	if res.OK {
		t.Fatalf("expected invalid result for unknown rule, got %+v", res)
	}
}

func TestNextStateAfterValidation(t *testing.T) {
	task := TaskSnapshot{TaskID: "t1", RetryCount: 0, MaxRetries: 2}
	if got := NextStateAfterValidation(task, ValidationResult{OK: true}); got != StateValidated {
		t.Fatalf("expected validated, got %s", got)
	}
	if got := NextStateAfterValidation(task, ValidationResult{OK: false}); got != StatePending {
		t.Fatalf("expected pending for retry, got %s", got)
	}
	if got := NextStateAfterValidation(TaskSnapshot{TaskID: "t1", RetryCount: 1, MaxRetries: 2}, ValidationResult{OK: false}); got != StateFailed {
		t.Fatalf("expected failed when retry budget reached, got %s", got)
	}
}

func TestEvaluateEscalation(t *testing.T) {
	policy := EscalationPolicy{
		RetryThreshold:  1,
		FallbackModel:   "m1",
		FallbackTooling: "t1",
	}
	no := EvaluateEscalation(1, policy)
	if no.Escalate || no.Action != EscalationActionNone {
		t.Fatalf("expected no escalation at threshold, got %+v", no)
	}
	yes := EvaluateEscalation(2, policy)
	if !yes.Escalate {
		t.Fatalf("expected escalation above threshold, got %+v", yes)
	}
	if yes.Action != EscalationActionFallbackModelTooling {
		t.Fatalf("unexpected escalation action: %+v", yes)
	}
	if yes.Reason == "" {
		t.Fatalf("expected escalation reason, got %+v", yes)
	}
}

func TestDefaultEscalationPolicy(t *testing.T) {
	p := DefaultEscalationPolicy(2)
	if p.RetryThreshold != 1 {
		t.Fatalf("expected threshold 1 for maxRetries=2, got %+v", p)
	}
	if p.FallbackModel == "" || p.FallbackTooling == "" {
		t.Fatalf("expected fallback defaults, got %+v", p)
	}
}
