package agent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SubagentLimits struct {
	MaxSpawnDepth       int
	MaxChildrenPerAgent int
	MaxConcurrent       int
}

type subagentRun struct {
	RunID            string
	AnnounceID       string
	ParentSession    string
	RootSession      string
	RequestedBy      string
	RequesterChan    string
	RequesterChatID  string
	RequesterTrace   string
	ChildSessionKey  string
	AgentID          string
	Task             string
	Label            string
	Model            string
	Thinking         string
	Cleanup          string
	Status           string
	Depth            int
	CreatedAt        time.Time
	StartedAt        *time.Time
	EndedAt          *time.Time
	ArchiveAt        *time.Time
	AnnouncedAt      *time.Time
	LastAnnounceAt   *time.Time
	NextAnnounceAt   *time.Time
	AnnounceAttempts int
	CompletionOutput string
	Error            string
	cancel           context.CancelFunc
}

func buildSubagentAnnounceID(childSessionKey, runID string) string {
	payload := strings.TrimSpace(childSessionKey) + "|" + strings.TrimSpace(runID)
	sum := sha1.Sum([]byte(payload))
	return "subannounce:" + hex.EncodeToString(sum[:8])
}

type subagentManager struct {
	mu           sync.Mutex
	runs         map[string]*subagentRun
	sessionDepth map[string]int
	sessionRoot  map[string]string
	limits       SubagentLimits
	storePath    string
	archiveAfter time.Duration
}

func newSubagentManager(limits SubagentLimits, storePath string, archiveAfterMinutes int) *subagentManager {
	if limits.MaxSpawnDepth <= 0 {
		limits.MaxSpawnDepth = 1
	}
	if limits.MaxChildrenPerAgent <= 0 {
		limits.MaxChildrenPerAgent = 5
	}
	if limits.MaxConcurrent <= 0 {
		limits.MaxConcurrent = 8
	}
	if archiveAfterMinutes <= 0 {
		archiveAfterMinutes = 60
	}
	m := &subagentManager{
		runs:         make(map[string]*subagentRun),
		sessionDepth: make(map[string]int),
		sessionRoot:  make(map[string]string),
		limits:       limits,
		storePath:    strings.TrimSpace(storePath),
		archiveAfter: time.Duration(archiveAfterMinutes) * time.Minute,
	}
	m.restoreFromDisk()
	return m
}

func (m *subagentManager) archiveTime(now time.Time) *time.Time {
	if m.archiveAfter <= 0 {
		return nil
	}
	at := now.Add(m.archiveAfter)
	return &at
}

func (m *subagentManager) restoreFromDisk() {
	if m.storePath == "" {
		return
	}
	data, err := os.ReadFile(m.storePath)
	if err != nil {
		return
	}
	var persisted []subagentRun
	if err := json.Unmarshal(data, &persisted); err != nil {
		return
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range persisted {
		run := persisted[i]
		run.cancel = nil
		if run.EndedAt == nil {
			// The process was restarted; mark in-flight runs as failed.
			run.Status = "failed"
			run.Error = "gateway restarted before subagent completion"
			run.EndedAt = &now
			run.ArchiveAt = m.archiveTime(now)
		}
		copied := cloneSubagentRun(&run)
		if copied == nil {
			continue
		}
		if strings.TrimSpace(copied.AnnounceID) == "" {
			copied.AnnounceID = buildSubagentAnnounceID(copied.ChildSessionKey, copied.RunID)
		}
		if strings.TrimSpace(copied.RequestedBy) == "" {
			copied.RequestedBy = copied.ParentSession
		}
		if strings.TrimSpace(copied.RootSession) == "" {
			copied.RootSession = strings.TrimSpace(copied.ParentSession)
			if copied.RootSession == "" {
				copied.RootSession = copied.ChildSessionKey
			}
		}
		if strings.TrimSpace(copied.Cleanup) == "" {
			copied.Cleanup = "keep"
		}
		m.runs[copied.RunID] = copied
		m.sessionDepth[copied.ChildSessionKey] = copied.Depth
		m.sessionRoot[copied.ChildSessionKey] = copied.RootSession
	}
	m.sweepExpiredLocked(now)
	m.persistLocked()
}

func (m *subagentManager) persist() {
	if m.storePath == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistLocked()
}

func (m *subagentManager) persistLocked() {
	if m.storePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.storePath), 0o700); err != nil {
		return
	}
	snapshot := make([]subagentRun, 0, len(m.runs))
	for _, run := range m.runs {
		c := cloneSubagentRun(run)
		if c == nil {
			continue
		}
		c.cancel = nil
		snapshot = append(snapshot, *c)
	}
	sort.Slice(snapshot, func(i, j int) bool {
		return snapshot[i].CreatedAt.Before(snapshot[j].CreatedAt)
	})
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return
	}
	tmp := m.storePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, m.storePath)
}

func (m *subagentManager) sweepExpiredLocked(now time.Time) {
	for runID, run := range m.runs {
		if run.EndedAt == nil || run.ArchiveAt == nil {
			continue
		}
		if now.Before(*run.ArchiveAt) {
			continue
		}
		delete(m.runs, runID)
		delete(m.sessionDepth, run.ChildSessionKey)
	}
}

func (m *subagentManager) sweepExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := len(m.runs)
	m.sweepExpiredLocked(time.Now())
	if len(m.runs) != before {
		m.persistLocked()
	}
}

func (m *subagentManager) parentDepthLocked(parentSession string) int {
	if depth, ok := m.sessionDepth[parentSession]; ok {
		return depth
	}
	return 0
}

func (m *subagentManager) currentDepth(parentSession string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.parentDepthLocked(parentSession)
}

func (m *subagentManager) canSpawn(parentSession string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(time.Now())

	parentDepth := m.parentDepthLocked(parentSession)
	if parentDepth >= m.limits.MaxSpawnDepth {
		return 0, fmt.Errorf(
			"sessions_spawn is not allowed at this depth (current depth: %d, max: %d)",
			parentDepth, m.limits.MaxSpawnDepth,
		)
	}

	activeChildren := 0
	activeGlobal := 0
	for _, run := range m.runs {
		if run.EndedAt == nil {
			activeGlobal++
		}
		if run.ParentSession == parentSession && run.EndedAt == nil {
			activeChildren++
		}
	}
	if activeChildren >= m.limits.MaxChildrenPerAgent {
		return 0, fmt.Errorf(
			"sessions_spawn has reached max active children for this session (%d/%d)",
			activeChildren, m.limits.MaxChildrenPerAgent,
		)
	}
	if activeGlobal >= m.limits.MaxConcurrent {
		return 0, fmt.Errorf(
			"sessions_spawn has reached max active subagents globally (%d/%d)",
			activeGlobal, m.limits.MaxConcurrent,
		)
	}
	return parentDepth + 1, nil
}

func (m *subagentManager) register(parentSession, requesterSession, requesterChan, requesterChatID, requesterTrace, task, label, model, thinking, agentID, cleanup string, depth int, cancel context.CancelFunc) *subagentRun {
	now := time.Now()
	trimmedAgentID := strings.TrimSpace(agentID)
	trimmedParent := strings.TrimSpace(parentSession)
	if trimmedParent == "" {
		trimmedParent = "cli:default"
	}
	trimmedRequester := strings.TrimSpace(requesterSession)
	if trimmedRequester == "" {
		trimmedRequester = trimmedParent
	}
	trimmedCleanup := strings.TrimSpace(cleanup)
	if trimmedCleanup == "" {
		trimmedCleanup = "keep"
	}
	childKey := fmt.Sprintf("subagent:%s", uuid.NewString())
	if trimmedAgentID != "" {
		childKey = fmt.Sprintf("agent:%s:subagent:%s", trimmedAgentID, uuid.NewString())
	}
	m.mu.Lock()
	rootSession := m.rootSessionLocked(trimmedParent)
	parentRun := m.findByChildSessionLocked(trimmedParent)
	if parentRun != nil {
		if strings.TrimSpace(requesterChan) == "" {
			requesterChan = parentRun.RequesterChan
		}
		if strings.TrimSpace(requesterChatID) == "" {
			requesterChatID = parentRun.RequesterChatID
		}
		if strings.TrimSpace(requesterTrace) == "" {
			requesterTrace = parentRun.RequesterTrace
		}
	}
	m.mu.Unlock()
	run := &subagentRun{
		RunID:           uuid.NewString(),
		ParentSession:   trimmedParent,
		RootSession:     rootSession,
		RequestedBy:     trimmedRequester,
		RequesterChan:   strings.TrimSpace(requesterChan),
		RequesterChatID: strings.TrimSpace(requesterChatID),
		RequesterTrace:  strings.TrimSpace(requesterTrace),
		ChildSessionKey: childKey,
		AgentID:         trimmedAgentID,
		Task:            task,
		Label:           label,
		Model:           model,
		Thinking:        thinking,
		Cleanup:         trimmedCleanup,
		Status:          "accepted",
		Depth:           depth,
		CreatedAt:       now,
		cancel:          cancel,
	}
	run.AnnounceID = buildSubagentAnnounceID(run.ChildSessionKey, run.RunID)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(now)
	m.runs[run.RunID] = run
	m.sessionDepth[run.ChildSessionKey] = depth
	m.sessionRoot[run.ChildSessionKey] = run.RootSession
	m.persistLocked()
	return cloneSubagentRun(run)
}

func (m *subagentManager) markRunning(runID string) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if run, ok := m.runs[runID]; ok {
		run.Status = "running"
		run.StartedAt = &now
		m.persistLocked()
	}
}

func (m *subagentManager) markFinished(runID string, status string, err error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if run, ok := m.runs[runID]; ok {
		if run.EndedAt != nil && run.Status == "killed" {
			return
		}
		run.Status = status
		run.EndedAt = &now
		run.ArchiveAt = m.archiveTime(now)
		run.cancel = nil
		if err != nil {
			run.Error = err.Error()
		}
		m.persistLocked()
	}
}

func (m *subagentManager) markCompletionOutput(runID, output string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok {
		return
	}
	run.CompletionOutput = strings.TrimSpace(output)
	m.persistLocked()
}

func (m *subagentManager) rootSessionLocked(session string) string {
	session = strings.TrimSpace(session)
	if session == "" {
		return "cli:default"
	}
	if root, ok := m.sessionRoot[session]; ok {
		root = strings.TrimSpace(root)
		if root != "" {
			return root
		}
	}
	m.sessionRoot[session] = session
	return session
}

func (m *subagentManager) findByChildSessionLocked(childSession string) *subagentRun {
	for _, run := range m.runs {
		if run.ChildSessionKey == childSession {
			return run
		}
	}
	return nil
}

func (m *subagentManager) listByController(controllerSession string) []subagentRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(time.Now())
	controllerRoot := m.rootSessionLocked(controllerSession)
	out := make([]subagentRun, 0)
	for _, run := range m.runs {
		if run.RootSession == controllerRoot {
			out = append(out, *cloneSubagentRun(run))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (m *subagentManager) listByParent(parentSession string) []subagentRun {
	return m.listByController(parentSession)
}

func (m *subagentManager) getRun(runID string) (*subagentRun, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok {
		return nil, false
	}
	return cloneSubagentRun(run), true
}

func (m *subagentManager) markAnnounceAttempt(runID string, delivered bool) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok {
		return
	}
	if strings.TrimSpace(run.AnnounceID) == "" {
		run.AnnounceID = buildSubagentAnnounceID(run.ChildSessionKey, run.RunID)
	}
	run.AnnounceAttempts++
	run.LastAnnounceAt = &now
	if delivered {
		run.AnnouncedAt = &now
		run.NextAnnounceAt = nil
	} else {
		backoff := 2 * time.Second
		if run.AnnounceAttempts > 1 {
			backoff = time.Duration(1<<(run.AnnounceAttempts-1)) * time.Second
			if backoff > 2*time.Minute {
				backoff = 2 * time.Minute
			}
		}
		next := now.Add(backoff)
		run.NextAnnounceAt = &next
	}
	m.persistLocked()
}

func (m *subagentManager) pendingAnnounceRuns(limit int) []subagentRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(time.Now())
	now := time.Now()
	out := make([]subagentRun, 0)
	for _, run := range m.runs {
		if run.EndedAt == nil {
			continue
		}
		if run.AnnouncedAt != nil {
			continue
		}
		if strings.TrimSpace(run.AnnounceID) == "" {
			run.AnnounceID = buildSubagentAnnounceID(run.ChildSessionKey, run.RunID)
		}
		if run.NextAnnounceAt != nil && now.Before(*run.NextAnnounceAt) {
			continue
		}
		if run.AnnounceAttempts >= 6 {
			continue
		}
		out = append(out, *cloneSubagentRun(run))
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].CreatedAt
		right := out[j].CreatedAt
		if out[i].EndedAt != nil {
			left = *out[i].EndedAt
		}
		if out[j].EndedAt != nil {
			right = *out[j].EndedAt
		}
		return left.Before(right)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (m *subagentManager) canControlLocked(controllerSession string, run *subagentRun) bool {
	if run == nil {
		return false
	}
	return m.rootSessionLocked(controllerSession) == strings.TrimSpace(run.RootSession)
}

func (m *subagentManager) killByRunID(parentSession, runID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepExpiredLocked(time.Now())
	run, ok := m.runs[runID]
	if !ok {
		return false, fmt.Errorf("unknown subagent run: %s", runID)
	}
	if !m.canControlLocked(parentSession, run) {
		return false, fmt.Errorf("run does not belong to current session scope")
	}
	if run.cancel == nil || run.EndedAt != nil {
		return false, nil
	}
	m.killRunLocked(run)
	m.killDescendantsLocked(run.ChildSessionKey)
	m.persistLocked()
	return true, nil
}

func (m *subagentManager) getByRunID(parentSession, runID string) (*subagentRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.runs[runID]
	if !ok {
		return nil, fmt.Errorf("unknown subagent run: %s", runID)
	}
	if !m.canControlLocked(parentSession, run) {
		return nil, fmt.Errorf("run does not belong to current session scope")
	}
	return cloneSubagentRun(run), nil
}

func (m *subagentManager) killRunLocked(run *subagentRun) {
	if run == nil || run.cancel == nil || run.EndedAt != nil {
		return
	}
	run.cancel()
	now := time.Now()
	run.EndedAt = &now
	run.ArchiveAt = m.archiveTime(now)
	run.Status = "killed"
	run.cancel = nil
}

func (m *subagentManager) killDescendantsLocked(parentSession string) {
	for _, child := range m.runs {
		if child.ParentSession != parentSession {
			continue
		}
		m.killRunLocked(child)
		m.killDescendantsLocked(child.ChildSessionKey)
	}
}

func cloneSubagentRun(in *subagentRun) *subagentRun {
	if in == nil {
		return nil
	}
	out := *in
	if in.StartedAt != nil {
		started := *in.StartedAt
		out.StartedAt = &started
	}
	if in.EndedAt != nil {
		ended := *in.EndedAt
		out.EndedAt = &ended
	}
	if in.ArchiveAt != nil {
		archiveAt := *in.ArchiveAt
		out.ArchiveAt = &archiveAt
	}
	return &out
}
