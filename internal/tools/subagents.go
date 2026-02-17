package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type SpawnRequest struct {
	Task              string
	Label             string
	AgentID           string
	Model             string
	Thinking          string
	Cleanup           string
	TimeoutSeconds    int
	RunTimeoutSeconds int
}

type SpawnResult struct {
	Status          string `json:"status"`
	RunID           string `json:"runId,omitempty"`
	ChildSessionKey string `json:"childSessionKey,omitempty"`
	Message         string `json:"message,omitempty"`
}

type SubagentRunView struct {
	RunID           string     `json:"runId"`
	ParentSession   string     `json:"parentSession"`
	RootSession     string     `json:"rootSession,omitempty"`
	RequestedBy     string     `json:"requestedBy,omitempty"`
	ChildSessionKey string     `json:"childSessionKey"`
	AgentID         string     `json:"agentId,omitempty"`
	Task            string     `json:"task"`
	Label           string     `json:"label,omitempty"`
	Model           string     `json:"model,omitempty"`
	Thinking        string     `json:"thinking,omitempty"`
	Cleanup         string     `json:"cleanup,omitempty"`
	Status          string     `json:"status"`
	Depth           int        `json:"depth"`
	CreatedAt       time.Time  `json:"createdAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	EndedAt         *time.Time `json:"endedAt,omitempty"`
	Error           string     `json:"error,omitempty"`
}

type AgentDiscovery struct {
	CurrentAgentID   string                `json:"currentAgentId"`
	AllowAgents      []string              `json:"allowAgents,omitempty"`
	EffectiveTargets []string              `json:"effectiveTargets,omitempty"`
	Wildcard         bool                  `json:"wildcard"`
	Agents           []AgentDiscoveryEntry `json:"agents,omitempty"`
}

type AgentDiscoveryEntry struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	Configured bool   `json:"configured"`
}

type SessionsSpawnTool struct {
	spawn func(context.Context, SpawnRequest) (SpawnResult, error)
}

func NewSessionsSpawnTool(spawnFn func(context.Context, SpawnRequest) (SpawnResult, error)) *SessionsSpawnTool {
	return &SessionsSpawnTool{spawn: spawnFn}
}

func (t *SessionsSpawnTool) Name() string { return "sessions_spawn" }
func (t *SessionsSpawnTool) Tier() int    { return TierWrite }
func (t *SessionsSpawnTool) Description() string {
	return "Spawn a background sub-agent run in a child session."
}

func (t *SessionsSpawnTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "Task instruction for the child sub-agent.",
			},
			"label": map[string]any{
				"type":        "string",
				"description": "Optional short label for the spawned run.",
			},
			"agentId": map[string]any{
				"type":        "string",
				"description": "Optional target agent ID for the spawned run (subject to allowlist).",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model override for the spawned run.",
			},
			"thinking": map[string]any{
				"type":        "string",
				"description": "Optional thinking level override for the spawned run.",
			},
			"runTimeoutSeconds": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in seconds for the spawned run (0 disables timeout).",
			},
			"timeoutSeconds": map[string]any{
				"type":        "integer",
				"description": "Alias for runTimeoutSeconds.",
			},
			"cleanup": map[string]any{
				"type":        "string",
				"description": "Session cleanup mode after run completion (keep|delete).",
				"enum":        []string{"keep", "delete"},
			},
		},
		"required": []string{"task"},
	}
}

func (t *SessionsSpawnTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	if t.spawn == nil {
		return "", fmt.Errorf("sessions_spawn unavailable")
	}
	task := strings.TrimSpace(GetString(params, "task", ""))
	if task == "" {
		return "", fmt.Errorf("task is required")
	}
	label := strings.TrimSpace(GetString(params, "label", ""))
	agentID := strings.TrimSpace(GetString(params, "agentId", ""))
	model := strings.TrimSpace(GetString(params, "model", ""))
	thinking := strings.TrimSpace(GetString(params, "thinking", ""))
	runTimeoutSeconds := GetInt(params, "runTimeoutSeconds", 0)
	timeoutSeconds := GetInt(params, "timeoutSeconds", 0)
	cleanup := strings.TrimSpace(strings.ToLower(GetString(params, "cleanup", "keep")))
	if cleanup == "" {
		cleanup = "keep"
	}
	if cleanup != "keep" && cleanup != "delete" {
		return "", fmt.Errorf("cleanup must be keep or delete")
	}
	if runTimeoutSeconds == 0 && timeoutSeconds > 0 {
		runTimeoutSeconds = timeoutSeconds
	}
	if runTimeoutSeconds < 0 {
		return "", fmt.Errorf("runTimeoutSeconds must be >= 0")
	}

	res, err := t.spawn(ctx, SpawnRequest{
		Task:              task,
		Label:             label,
		AgentID:           agentID,
		Model:             model,
		Thinking:          thinking,
		Cleanup:           cleanup,
		TimeoutSeconds:    timeoutSeconds,
		RunTimeoutSeconds: runTimeoutSeconds,
	})
	if err != nil {
		return "", err
	}
	out, marshalErr := json.Marshal(res)
	if marshalErr != nil {
		return "", marshalErr
	}
	return string(out), nil
}

type SubagentsRequest struct {
	Action        string
	Target        string
	Input         string
	RecentMinutes int
}

type SubagentsTool struct {
	listRuns func() []SubagentRunView
	killRun  func(runID string) (bool, error)
	steerRun func(runID, input string) (SpawnResult, error)
}

func NewSubagentsTool(
	listFn func() []SubagentRunView,
	killFn func(runID string) (bool, error),
	steerFn func(runID, input string) (SpawnResult, error),
) *SubagentsTool {
	return &SubagentsTool{
		listRuns: listFn,
		killRun:  killFn,
		steerRun: steerFn,
	}
}

func (t *SubagentsTool) Name() string { return "subagents" }
func (t *SubagentsTool) Tier() int    { return TierWrite }
func (t *SubagentsTool) Description() string {
	return "List, kill, or steer sub-agent runs for the current requester session."
}

func (t *SubagentsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action: list, kill, kill_all, or steer.",
				"enum":        []string{"list", "kill", "kill_all", "steer"},
			},
			"target": map[string]any{
				"type":        "string",
				"description": "Run selector when action=kill or action=steer (run ID, 'last', numeric index, or label prefix).",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "Steering input when action=steer.",
			},
			"recentMinutes": map[string]any{
				"type":        "integer",
				"description": "Recent window for numeric/run selection (default: 30).",
			},
		},
	}
}

func (t *SubagentsTool) Execute(_ context.Context, params map[string]any) (string, error) {
	action := strings.TrimSpace(GetString(params, "action", "list"))
	if action == "" {
		action = "list"
	}
	recentMinutes := GetInt(params, "recentMinutes", 30)
	if recentMinutes <= 0 {
		recentMinutes = 30
	}
	if recentMinutes > 24*60 {
		recentMinutes = 24 * 60
	}
	switch action {
	case "list":
		if t.listRuns == nil {
			return "", fmt.Errorf("subagents list unavailable")
		}
		runs := t.listRuns()
		body := map[string]any{
			"status": "ok",
			"action": "list",
			"runs":   runs,
		}
		out, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		return string(out), nil
	case "kill":
		if t.killRun == nil {
			return "", fmt.Errorf("subagents kill unavailable")
		}
		target := strings.TrimSpace(GetString(params, "target", ""))
		if target == "" {
			return "", fmt.Errorf("target is required for kill")
		}
		runs := t.listRunsOrNil()
		resolved, err := resolveSubagentTarget(runs, target, recentMinutes)
		if err != nil {
			return "", err
		}
		killed, err := t.killRun(resolved.RunID)
		if err != nil {
			return "", err
		}
		body := map[string]any{
			"status": "ok",
			"action": "kill",
			"runId":  resolved.RunID,
			"target": target,
			"killed": killed,
		}
		out, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return "", marshalErr
		}
		return string(out), nil
	case "kill_all":
		if t.killRun == nil {
			return "", fmt.Errorf("subagents kill unavailable")
		}
		runs := t.listRunsOrNil()
		killed := 0
		attempted := 0
		for _, run := range runs {
			if run.EndedAt != nil {
				continue
			}
			attempted++
			ok, err := t.killRun(run.RunID)
			if err != nil {
				return "", err
			}
			if ok {
				killed++
			}
		}
		body := map[string]any{
			"status":    "ok",
			"action":    "kill_all",
			"attempted": attempted,
			"killed":    killed,
		}
		out, err := json.Marshal(body)
		if err != nil {
			return "", err
		}
		return string(out), nil
	case "steer":
		if t.steerRun == nil {
			return "", fmt.Errorf("subagents steer unavailable")
		}
		target := strings.TrimSpace(GetString(params, "target", ""))
		if target == "" {
			return "", fmt.Errorf("target is required for steer")
		}
		input := strings.TrimSpace(GetString(params, "input", ""))
		if input == "" {
			return "", fmt.Errorf("input is required for steer")
		}
		runs := t.listRunsOrNil()
		resolved, err := resolveSubagentTarget(runs, target, recentMinutes)
		if err != nil {
			return "", err
		}
		res, err := t.steerRun(resolved.RunID, input)
		if err != nil {
			return "", err
		}
		body := map[string]any{
			"status":          "ok",
			"action":          "steer",
			"targetRunId":     resolved.RunID,
			"target":          target,
			"newRunId":        res.RunID,
			"childSessionKey": res.ChildSessionKey,
			"message":         res.Message,
		}
		out, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return "", marshalErr
		}
		return string(out), nil
	default:
		return "", fmt.Errorf("unsupported action: %s", action)
	}
}

func (t *SubagentsTool) listRunsOrNil() []SubagentRunView {
	if t.listRuns == nil {
		return nil
	}
	return t.listRuns()
}

func resolveSubagentTarget(runs []SubagentRunView, target string, recentMinutes int) (SubagentRunView, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return SubagentRunView{}, fmt.Errorf("target is required")
	}
	sorted := append([]SubagentRunView{}, runs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})
	if trimmed == "last" {
		if len(sorted) == 0 {
			return SubagentRunView{}, fmt.Errorf("no subagent runs found")
		}
		return sorted[0], nil
	}

	cutoff := time.Now().Add(-time.Duration(recentMinutes) * time.Minute)
	numericOrder := make([]SubagentRunView, 0, len(sorted))
	for _, run := range sorted {
		if run.EndedAt == nil {
			numericOrder = append(numericOrder, run)
		}
	}
	for _, run := range sorted {
		if run.EndedAt != nil && run.EndedAt.After(cutoff) {
			numericOrder = append(numericOrder, run)
		}
	}
	if isAllDigits(trimmed) {
		idx := atoi(trimmed)
		if idx <= 0 || idx > len(numericOrder) {
			return SubagentRunView{}, fmt.Errorf("invalid subagent index: %s", trimmed)
		}
		return numericOrder[idx-1], nil
	}

	if strings.Contains(trimmed, ":") {
		for _, run := range sorted {
			if run.ChildSessionKey == trimmed {
				return run, nil
			}
		}
		return SubagentRunView{}, fmt.Errorf("unknown subagent session: %s", trimmed)
	}

	lowered := strings.ToLower(trimmed)
	exactLabel := filterRuns(sorted, func(run SubagentRunView) bool {
		return strings.ToLower(strings.TrimSpace(run.Label)) == lowered
	})
	if len(exactLabel) == 1 {
		return exactLabel[0], nil
	}
	if len(exactLabel) > 1 {
		return SubagentRunView{}, fmt.Errorf("ambiguous subagent label: %s", trimmed)
	}
	prefixLabel := filterRuns(sorted, func(run SubagentRunView) bool {
		return strings.HasPrefix(strings.ToLower(strings.TrimSpace(run.Label)), lowered)
	})
	if len(prefixLabel) == 1 {
		return prefixLabel[0], nil
	}
	if len(prefixLabel) > 1 {
		return SubagentRunView{}, fmt.Errorf("ambiguous subagent label prefix: %s", trimmed)
	}

	runIDPrefix := filterRuns(sorted, func(run SubagentRunView) bool {
		return strings.HasPrefix(run.RunID, trimmed)
	})
	if len(runIDPrefix) == 1 {
		return runIDPrefix[0], nil
	}
	if len(runIDPrefix) > 1 {
		return SubagentRunView{}, fmt.Errorf("ambiguous subagent run id prefix: %s", trimmed)
	}

	return SubagentRunView{}, fmt.Errorf("unknown subagent target: %s", trimmed)
}

func filterRuns(runs []SubagentRunView, match func(SubagentRunView) bool) []SubagentRunView {
	out := make([]SubagentRunView, 0)
	for _, run := range runs {
		if match(run) {
			out = append(out, run)
		}
	}
	return out
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func atoi(s string) int {
	n := 0
	for _, ch := range s {
		n = n*10 + int(ch-'0')
	}
	return n
}

type AgentsListTool struct {
	discover func() AgentDiscovery
}

func NewAgentsListTool(discoverFn func() AgentDiscovery) *AgentsListTool {
	return &AgentsListTool{discover: discoverFn}
}

func (t *AgentsListTool) Name() string { return "agents_list" }
func (t *AgentsListTool) Tier() int    { return TierReadOnly }
func (t *AgentsListTool) Description() string {
	return "Discover allowed sub-agent spawn targets for this session."
}

func (t *AgentsListTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *AgentsListTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	if t.discover == nil {
		return "", fmt.Errorf("agents_list unavailable")
	}
	body := map[string]any{
		"status": "ok",
		"action": "agents_list",
		"agents": t.discover(),
	}
	out, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
