package cascade

import (
	"fmt"
	"strings"
)

const (
	StatePending      = "pending"
	StateRunning      = "running"
	StateSelfTest     = "self_test"
	StateValidated    = "validated"
	StateCommitted    = "committed"
	StateReleasedNext = "released_next"
	StateFailed       = "failed"
)

// TaskContract defines explicit IO boundaries for one cascade step.
type TaskContract struct {
	TaskID          string
	Sequence        int
	RequiredInput   []string
	ProducedOutput  []string
	ValidationRules []string // supported: non_empty:<field>, equals:<field>:<value>
}

// TaskSnapshot is the runtime state used by orchestration gates.
type TaskSnapshot struct {
	TaskID          string
	Sequence        int
	State           string
	RetryCount      int
	MaxRetries      int
	BlockedByTaskID string
}

// ValidationResult is the output of self-test and manager-side validation.
type ValidationResult struct {
	OK             bool
	MissingInput   []string
	MissingOutput  []string
	InvalidRules   []string
	RemediationMsg string
}

const (
	EscalationActionNone                 = "none"
	EscalationActionFallbackModel        = "fallback_model"
	EscalationActionFallbackTooling      = "fallback_tooling"
	EscalationActionFallbackModelTooling = "fallback_model_tooling"
)

// EscalationPolicy defines fallback strategy after retry pressure.
type EscalationPolicy struct {
	RetryThreshold  int
	FallbackModel   string
	FallbackTooling string
}

// EscalationDecision is the deterministic escalation output.
type EscalationDecision struct {
	Escalate        bool
	Action          string
	FallbackModel   string
	FallbackTooling string
	Reason          string
}

func ValidateContract(c TaskContract) error {
	if strings.TrimSpace(c.TaskID) == "" {
		return fmt.Errorf("task_id is required")
	}
	if c.Sequence <= 0 {
		return fmt.Errorf("sequence must be > 0")
	}
	return nil
}

func CanTransition(fromState, toState string) bool {
	switch fromState {
	case StatePending:
		return toState == StateRunning || toState == StateFailed
	case StateRunning:
		return toState == StateSelfTest || toState == StateFailed
	case StateSelfTest:
		return toState == StateValidated || toState == StatePending || toState == StateFailed
	case StateValidated:
		return toState == StateCommitted || toState == StateFailed
	case StateCommitted:
		return toState == StateReleasedNext || toState == StateFailed
	case StateReleasedNext, StateFailed:
		return false
	default:
		return false
	}
}

func CanStart(task TaskSnapshot, predecessor *TaskSnapshot) (bool, string) {
	if task.State != StatePending {
		return false, "task must be pending"
	}
	if task.MaxRetries > 0 && task.RetryCount >= task.MaxRetries {
		return false, "retry budget exhausted"
	}

	requiresPredecessor := task.Sequence > 1 || strings.TrimSpace(task.BlockedByTaskID) != ""
	if !requiresPredecessor {
		return true, ""
	}
	if predecessor == nil {
		return false, "predecessor not found"
	}
	if predecessor.State != StateCommitted && predecessor.State != StateReleasedNext {
		return false, "predecessor output not committed"
	}
	return true, ""
}

func ValidateIO(contract TaskContract, input map[string]string, output map[string]string) ValidationResult {
	res := ValidationResult{OK: true}
	for _, key := range contract.RequiredInput {
		if strings.TrimSpace(input[strings.TrimSpace(key)]) == "" {
			res.MissingInput = append(res.MissingInput, strings.TrimSpace(key))
		}
	}
	for _, key := range contract.ProducedOutput {
		if strings.TrimSpace(output[strings.TrimSpace(key)]) == "" {
			res.MissingOutput = append(res.MissingOutput, strings.TrimSpace(key))
		}
	}
	for _, rule := range contract.ValidationRules {
		if !applyRule(strings.TrimSpace(rule), output) {
			res.InvalidRules = append(res.InvalidRules, strings.TrimSpace(rule))
		}
	}
	if len(res.MissingInput) > 0 || len(res.MissingOutput) > 0 || len(res.InvalidRules) > 0 {
		res.OK = false
		res.RemediationMsg = buildRemediation(res)
	}
	return res
}

func NextStateAfterValidation(task TaskSnapshot, validation ValidationResult) string {
	if validation.OK {
		return StateValidated
	}
	if task.MaxRetries > 0 && task.RetryCount+1 >= task.MaxRetries {
		return StateFailed
	}
	return StatePending
}

// DefaultEscalationPolicy uses a conservative threshold near retry exhaustion.
func DefaultEscalationPolicy(maxRetries int) EscalationPolicy {
	threshold := 1
	if maxRetries > 1 {
		threshold = maxRetries - 1
	}
	return EscalationPolicy{
		RetryThreshold:  threshold,
		FallbackModel:   "deterministic-fallback-model",
		FallbackTooling: "safe-fallback-toolchain",
	}
}

// EvaluateEscalation decides if fallback model/tooling should activate.
func EvaluateEscalation(nextRetryCount int, policy EscalationPolicy) EscalationDecision {
	threshold := policy.RetryThreshold
	if threshold <= 0 {
		threshold = 1
	}
	if nextRetryCount <= threshold {
		return EscalationDecision{Escalate: false, Action: EscalationActionNone}
	}
	decision := EscalationDecision{
		Escalate:        true,
		FallbackModel:   strings.TrimSpace(policy.FallbackModel),
		FallbackTooling: strings.TrimSpace(policy.FallbackTooling),
	}
	switch {
	case decision.FallbackModel != "" && decision.FallbackTooling != "":
		decision.Action = EscalationActionFallbackModelTooling
	case decision.FallbackModel != "":
		decision.Action = EscalationActionFallbackModel
	case decision.FallbackTooling != "":
		decision.Action = EscalationActionFallbackTooling
	default:
		decision.Action = EscalationActionNone
		decision.Escalate = false
		return decision
	}
	reasonParts := []string{
		fmt.Sprintf("escalation_triggered=retries_exceeded_threshold(%d>%d)", nextRetryCount, threshold),
		"action=" + decision.Action,
	}
	if decision.FallbackModel != "" {
		reasonParts = append(reasonParts, "fallback_model="+decision.FallbackModel)
	}
	if decision.FallbackTooling != "" {
		reasonParts = append(reasonParts, "fallback_tooling="+decision.FallbackTooling)
	}
	decision.Reason = strings.Join(reasonParts, "; ")
	return decision
}

func applyRule(rule string, output map[string]string) bool {
	if rule == "" {
		return true
	}
	switch {
	case strings.HasPrefix(rule, "non_empty:"):
		key := strings.TrimSpace(strings.TrimPrefix(rule, "non_empty:"))
		return strings.TrimSpace(output[key]) != ""
	case strings.HasPrefix(rule, "equals:"):
		parts := strings.SplitN(strings.TrimPrefix(rule, "equals:"), ":", 2)
		if len(parts) != 2 {
			return false
		}
		key := strings.TrimSpace(parts[0])
		want := strings.TrimSpace(parts[1])
		return strings.TrimSpace(output[key]) == want
	default:
		return false
	}
}

func buildRemediation(res ValidationResult) string {
	parts := make([]string, 0, 3)
	if len(res.MissingInput) > 0 {
		parts = append(parts, "missing_input="+strings.Join(res.MissingInput, ","))
	}
	if len(res.MissingOutput) > 0 {
		parts = append(parts, "missing_output="+strings.Join(res.MissingOutput, ","))
	}
	if len(res.InvalidRules) > 0 {
		parts = append(parts, "invalid_rules="+strings.Join(res.InvalidRules, ","))
	}
	return strings.Join(parts, "; ")
}
