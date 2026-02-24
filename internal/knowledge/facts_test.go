package knowledge

import "testing"

func TestEvaluateFactApply_NewFactMustStartAtV1(t *testing.T) {
	in := FactPayload{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "v2", Version: 2, Source: "x", Group: "g"}
	got := EvaluateFactApply(nil, in)
	if got.Status != FactApplyConflict || got.Reason != "new_fact_must_start_at_v1" {
		t.Fatalf("unexpected result: %+v", got)
	}
	in.Version = 1
	got = EvaluateFactApply(nil, in)
	if got.Status != FactApplyAccepted {
		t.Fatalf("expected accepted new fact, got %+v", got)
	}
}

func TestEvaluateFactApply_SequentialAccepted(t *testing.T) {
	ex := &FactState{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "v1", Version: 1}
	in := FactPayload{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "v2", Version: 2, Source: "x", Group: "g"}
	got := EvaluateFactApply(ex, in)
	if got.Status != FactApplyAccepted {
		t.Fatalf("expected accepted, got %+v", got)
	}
}

func TestEvaluateFactApply_StaleIfSameContent(t *testing.T) {
	ex := &FactState{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "v2", Version: 2}
	in := FactPayload{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "v2", Version: 2, Source: "x", Group: "g"}
	got := EvaluateFactApply(ex, in)
	if got.Status != FactApplyStale {
		t.Fatalf("expected stale, got %+v", got)
	}
}

func TestEvaluateFactApply_RegressionConflictOnContentMismatch(t *testing.T) {
	ex := &FactState{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "v2", Version: 2}
	in := FactPayload{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "vX", Version: 2, Source: "x", Group: "g"}
	got := EvaluateFactApply(ex, in)
	if got.Status != FactApplyConflict {
		t.Fatalf("expected conflict, got %+v", got)
	}
}

func TestEvaluateFactApply_VersionGapConflict(t *testing.T) {
	ex := &FactState{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "v2", Version: 2}
	in := FactPayload{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "v4", Version: 4, Source: "x", Group: "g"}
	got := EvaluateFactApply(ex, in)
	if got.Status != FactApplyConflict {
		t.Fatalf("expected conflict for version gap, got %+v", got)
	}
}

func TestEvaluateFactApply_InvalidVersionConflict(t *testing.T) {
	in := FactPayload{FactID: "f1", Subject: "svc", Predicate: "runbook", Object: "v1", Version: 0, Source: "x", Group: "g"}
	got := EvaluateFactApply(nil, in)
	if got.Status != FactApplyConflict {
		t.Fatalf("expected conflict for invalid version, got %+v", got)
	}
}
