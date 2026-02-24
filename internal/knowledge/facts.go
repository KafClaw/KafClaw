package knowledge

import "fmt"

const (
	FactApplyAccepted = "accepted"
	FactApplyStale    = "stale"
	FactApplyConflict = "conflict"
)

type FactState struct {
	FactID    string
	Subject   string
	Predicate string
	Object    string
	Version   int
}

type FactApplyResult struct {
	Status string
	Reason string
}

// EvaluateFactApply enforces shared-fact version policy.
// Rules:
// - New fact must start at version 1.
// - Existing fact accepts only currentVersion+1.
// - Same-or-lower versions are stale only if content matches exactly; else conflict.
// - Version gaps (> currentVersion+1) are conflict (out-of-order).
func EvaluateFactApply(existing *FactState, incoming FactPayload) FactApplyResult {
	if incoming.Version <= 0 {
		return FactApplyResult{Status: FactApplyConflict, Reason: "invalid_version"}
	}
	if existing == nil {
		if incoming.Version != 1 {
			return FactApplyResult{Status: FactApplyConflict, Reason: "new_fact_must_start_at_v1"}
		}
		return FactApplyResult{Status: FactApplyAccepted, Reason: "new_fact"}
	}
	if incoming.Version == existing.Version+1 {
		return FactApplyResult{Status: FactApplyAccepted, Reason: "sequential_update"}
	}
	if incoming.Version <= existing.Version {
		if sameFactContent(existing, incoming) {
			return FactApplyResult{Status: FactApplyStale, Reason: "duplicate_or_stale"}
		}
		return FactApplyResult{Status: FactApplyConflict, Reason: "version_regression_content_mismatch"}
	}
	return FactApplyResult{Status: FactApplyConflict, Reason: fmt.Sprintf("version_gap_%d_to_%d", existing.Version, incoming.Version)}
}

func sameFactContent(existing *FactState, incoming FactPayload) bool {
	if existing == nil {
		return false
	}
	return existing.Subject == incoming.Subject &&
		existing.Predicate == incoming.Predicate &&
		existing.Object == incoming.Object
}
