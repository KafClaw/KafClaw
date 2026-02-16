// Package agent implements the core agent loop.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/gomikrobot/internal/approval"
	"github.com/KafClaw/KafClaw/gomikrobot/internal/bus"
	"github.com/KafClaw/KafClaw/gomikrobot/internal/memory"
	"github.com/KafClaw/KafClaw/gomikrobot/internal/policy"
	"github.com/KafClaw/KafClaw/gomikrobot/internal/provider"
	"github.com/KafClaw/KafClaw/gomikrobot/internal/session"
	"github.com/KafClaw/KafClaw/gomikrobot/internal/timeline"
	"github.com/KafClaw/KafClaw/gomikrobot/internal/tools"
)

// GroupTracePublisher can publish trace and audit data to a group.
type GroupTracePublisher interface {
	Active() bool
	PublishTrace(ctx context.Context, payload interface{}) error
	PublishAudit(ctx context.Context, eventType, traceID, detail string) error
}

// LoopOptions contains configuration for the agent loop.
type LoopOptions struct {
	Bus              *bus.MessageBus
	Provider         provider.LLMProvider
	Timeline         *timeline.TimelineService
	Policy           policy.Engine
	MemoryService    *memory.MemoryService
	AutoIndexer      *memory.AutoIndexer
	ExpertiseTracker *memory.ExpertiseTracker
	WorkingMemory    *memory.WorkingMemoryStore
	Observer         *memory.Observer
	GroupPublisher   GroupTracePublisher
	Workspace        string
	WorkRepo         string
	SystemRepo       string
	WorkRepoGetter   func() string
	Model            string
	MaxIterations    int
}

// Loop is the core agent processing engine.
type Loop struct {
	bus            *bus.MessageBus
	provider       provider.LLMProvider
	timeline       *timeline.TimelineService
	policy         policy.Engine
	memoryService    *memory.MemoryService
	autoIndexer      *memory.AutoIndexer
	expertiseTracker *memory.ExpertiseTracker
	workingMemory    *memory.WorkingMemoryStore
	observer         *memory.Observer
	groupPublisher   GroupTracePublisher
	approvalMgr    *approval.Manager
	registry       *tools.Registry
	sessions       *session.Manager
	contextBuilder *ContextBuilder
	workspace      string
	workRepo       string
	systemRepo     string
	workRepoGetter func() string
	model          string
	maxIterations  int
	running        bool
	// activeTaskID tracks the current task being processed (for token accounting).
	activeTaskID string
	// activeSender tracks the sender of the current message (for policy checks).
	activeSender      string
	activeChannel     string
	activeChatID      string
	activeTraceID     string
	activeMessageType string
}

// NewLoop creates a new agent loop.
func NewLoop(opts LoopOptions) *Loop {
	maxIter := opts.MaxIterations
	if maxIter == 0 {
		maxIter = 20
	}

	registry := tools.NewRegistry()

	// Create context builder
	ctxBuilder := NewContextBuilder(opts.Workspace, opts.WorkRepo, opts.SystemRepo, registry)

	loop := &Loop{
		bus:            opts.Bus,
		provider:       opts.Provider,
		timeline:       opts.Timeline,
		policy:         opts.Policy,
		memoryService:    opts.MemoryService,
		autoIndexer:      opts.AutoIndexer,
		expertiseTracker: opts.ExpertiseTracker,
		workingMemory:    opts.WorkingMemory,
		observer:         opts.Observer,
		groupPublisher:   opts.GroupPublisher,
		approvalMgr:    approval.NewManager(opts.Timeline),
		registry:       registry,
		sessions:       session.NewManager(opts.Workspace),
		contextBuilder: ctxBuilder,
		workspace:      opts.Workspace,
		workRepo:       opts.WorkRepo,
		systemRepo:     opts.SystemRepo,
		workRepoGetter: opts.WorkRepoGetter,
		model:          opts.Model,
		maxIterations:  maxIter,
	}

	// Register default tools
	loop.registerDefaultTools()

	return loop
}

func (l *Loop) registerDefaultTools() {
	l.registry.Register(tools.NewReadFileTool())
	repoGetter := l.workRepoGetter
	if repoGetter == nil {
		repoGetter = func() string { return l.workRepo }
	}
	l.registry.Register(tools.NewWriteFileTool(repoGetter))
	l.registry.Register(tools.NewEditFileTool(repoGetter))
	l.registry.Register(tools.NewListDirTool())
	l.registry.Register(tools.NewResolvePathTool(repoGetter))
	l.registry.Register(tools.NewExecTool(0, true, l.workspace, repoGetter))

	// Register memory tools only when memory service is available.
	if l.memoryService != nil {
		l.registry.Register(tools.NewRememberTool(l.memoryService))
		l.registry.Register(tools.NewRecallTool(l.memoryService))
	}
}

// Run starts the agent loop, processing messages from the bus.
func (l *Loop) Run(ctx context.Context) error {
	l.running = true
	slog.Info("Agent loop started")

	for l.running {
		msg, err := l.bus.ConsumeInbound(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // Context cancelled, normal shutdown
			}
			slog.Error("Failed to consume message", "error", err)
			continue
		}

		// Intercept approval responses (approve:<id> / deny:<id>)
		if id, approved, ok := parseApprovalResponse(msg.Content); ok && l.approvalMgr != nil {
			if err := l.approvalMgr.Respond(id, approved); err != nil {
				slog.Warn("Approval response failed", "id", id, "error", err)
				l.bus.PublishOutbound(&bus.OutboundMessage{
					Channel: msg.Channel,
					ChatID:  msg.ChatID,
					TraceID: msg.TraceID,
					Content: fmt.Sprintf("No pending approval found for ID %s.", id),
				})
			} else {
				action := "denied"
				if approved {
					action = "approved"
				}
				l.bus.PublishOutbound(&bus.OutboundMessage{
					Channel: msg.Channel,
					ChatID:  msg.ChatID,
					TraceID: msg.TraceID,
					Content: fmt.Sprintf("Approval %s: %s.", id, action),
				})
			}
			continue
		}

		response, taskID, err := l.processMessage(ctx, msg)
		if err != nil {
			slog.Error("Failed to process message", "error", err)
			response = fmt.Sprintf("Error: %v", err)
		}

		if response != "" {
			l.bus.PublishOutbound(&bus.OutboundMessage{
				Channel: msg.Channel,
				ChatID:  msg.ChatID,
				TraceID: msg.TraceID,
				TaskID:  taskID,
				Content: response,
			})
			// Optimistic delivery mark
			if l.timeline != nil && taskID != "" {
				_ = l.timeline.UpdateTaskDelivery(taskID, timeline.DeliverySent, nil)
			}
		}
	}

	return nil
}

// Stop signals the agent loop to stop.
func (l *Loop) Stop() {
	l.running = false
}

// ProcessDirect processes a message directly (for CLI usage).
func (l *Loop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return l.ProcessDirectWithTrace(ctx, content, sessionKey, "")
}

// ProcessDirectWithTrace processes a message with an explicit trace id.
func (l *Loop) ProcessDirectWithTrace(ctx context.Context, content, sessionKey, traceID string) (string, error) {
	// Extract channel and chatID from key if possible
	parts := strings.SplitN(sessionKey, ":", 2)
	channel, chatID := "cli", "default"
	if len(parts) == 2 {
		channel, chatID = parts[0], parts[1]
	}
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}

	// CLI direct calls are always internal (owner). Bus-routed messages
	// have activeMessageType set by processMessage before calling here.
	if l.activeMessageType == "" {
		l.activeMessageType = bus.MessageTypeInternal
	}

	// Get or create session
	sess := l.sessions.GetOrCreate(sessionKey)
	sess.AddMessage("user", content)

	if response, handled := l.handleDay2Day(sess, content); handled {
		sess.AddMessage("assistant", response)
		l.sessions.Save(sess)
		return response, nil
	}

	if isAttackIntent(content) {
		response := "Ey, du spinnst wohl? Hä? 💣 👮‍♂️ 🔒"
		sess.AddMessage("assistant", response)
		l.sessions.Save(sess)
		return response, nil
	}

	// Build messages using the context builder
	messages := l.contextBuilder.BuildMessages(sess, content, channel, chatID, l.activeMessageType)

	// Inject working memory (scoped per user/thread)
	messages = l.injectWorkingMemory(messages, chatID, sessionKey)

	// Inject observations (compressed session history)
	messages = l.injectObservations(messages, sessionKey)

	// Inject RAG context from semantic memory
	messages = l.injectRAGContext(ctx, messages, content)

	// Run the agentic loop
	response, err := l.runAgentLoop(ctx, messages)
	if err != nil {
		return "", err
	}

	// Save session with response
	sess.AddMessage("assistant", response)
	l.sessions.Save(sess)

	// Auto-index conversation pair into semantic memory
	if l.autoIndexer != nil {
		l.autoIndexer.Enqueue(memory.FormatConversationPair(content, response, channel, chatID))
	}

	// Enqueue messages for observational memory and trigger compression if needed
	if l.observer != nil {
		l.observer.EnqueueMessage(sessionKey, "user", content)
		l.observer.EnqueueMessage(sessionKey, "assistant", response)
		if l.observer.ShouldObserve(sessionKey) {
			go func() {
				if err := l.observer.Observe(context.Background(), sessionKey); err != nil {
					slog.Warn("Observer compression failed", "error", err)
				}
				// Check if reflector should run
				if l.observer.ShouldReflect(sessionKey) {
					if err := l.observer.Reflect(context.Background(), sessionKey); err != nil {
						slog.Warn("Reflector consolidation failed", "error", err)
					}
				}
			}()
		}
	}

	return response, nil
}

func isAttackIntent(content string) bool {
	lower := strings.ToLower(content)
	if lower == "" {
		return false
	}
	badPatterns := []string{
		`(?i)\bdelete\b.*\brepo\b`,
		`(?i)\brepo\b.*\bdelete\b`,
		`(?i)\bremove\b.*\brepo\b`,
		`(?i)\brepo\b.*\bremove\b`,
		`(?i)\bwipe\b.*\brepo\b`,
		`(?i)\bdelete\b.*\bcontent\b`,
		`(?i)\bdelete\b.*\ball\b.*\bfiles\b`,
		`(?i)\bremove\b.*\ball\b.*\bfiles\b`,
		`(?i)\brm\s+-rf\b`,
		`(?i)\blösch\b.*\brepo\b`,
		`(?i)\blösch\b.*\ball\b`,
		`(?i)\bdatei(en)?\b.*\blösch\b`,
	}
	for _, pattern := range badPatterns {
		if re, err := regexp.Compile(pattern); err == nil && re.MatchString(lower) {
			return true
		}
	}
	return false
}

func (l *Loop) handleDay2Day(sess *session.Session, content string) (string, bool) {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return "", false
	}

	if statusText, ok := l.handleDay2DayStatus(raw); ok {
		return statusText, true
	}

	cmd, ok := parseDay2DayCommand(raw)
	captureMode, captureBuffer := getDay2DayCapture(sess)
	if captureMode != "" {
		if ok && cmd.Kind == "dtc" {
			if strings.TrimSpace(captureBuffer) == "" {
				clearDay2DayCapture(sess)
				return "Day2Day: capture was empty. Send dtu/dtp then content, end with dtc.", true
			}
			clearDay2DayCapture(sess)
			return l.applyDay2DayCommand(captureMode, captureBuffer), true
		}
		captureBuffer = strings.TrimSpace(captureBuffer + "\n" + raw)
		setDay2DayCapture(sess, captureMode, captureBuffer)
		return "Day2Day: captured. Send dtc to close.", true
	}

	if !ok {
		return "", false
	}

	switch cmd.Kind {
	case "dtu", "dtp":
		if cmd.Text == "" {
			setDay2DayCapture(sess, cmd.Kind, "")
			return fmt.Sprintf("Day2Day: %s capture started. Send dtc to close.", cmd.Kind), true
		}
		return l.applyDay2DayCommand(cmd.Kind, cmd.Text), true
	case "dts":
		return l.consolidateDay2Day(time.Now()), true
	case "dtn":
		return l.planNextDay2Day(time.Now()), true
	case "dta":
		return l.planAllDay2Day(time.Now()), true
	case "dtc":
		return "Day2Day: no open capture. Send dtu or dtp to start.", true
	default:
		return "", false
	}
}

type day2DayCommand struct {
	Kind string
	Text string
}

func parseDay2DayCommand(input string) (day2DayCommand, bool) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return day2DayCommand{}, false
	}
	cmd := strings.ToLower(fields[0])
	switch cmd {
	case "dtu", "dtp", "dts", "dtc", "dtn", "dta":
		text := ""
		if len(fields) > 1 {
			text = strings.TrimSpace(input[len(fields[0]):])
		}
		return day2DayCommand{Kind: cmd, Text: strings.TrimSpace(text)}, true
	default:
		return day2DayCommand{}, false
	}
}

const (
	day2DayCaptureModeKey   = "day2day_capture_mode"
	day2DayCaptureBufferKey = "day2day_capture_buffer"
)

func getDay2DayCapture(sess *session.Session) (string, string) {
	modeRaw, _ := sess.GetMetadata(day2DayCaptureModeKey)
	bufRaw, _ := sess.GetMetadata(day2DayCaptureBufferKey)
	mode, _ := modeRaw.(string)
	buf, _ := bufRaw.(string)
	return strings.TrimSpace(mode), strings.TrimSpace(buf)
}

func setDay2DayCapture(sess *session.Session, mode, buffer string) {
	sess.SetMetadata(day2DayCaptureModeKey, mode)
	sess.SetMetadata(day2DayCaptureBufferKey, buffer)
}

func clearDay2DayCapture(sess *session.Session) {
	sess.DeleteMetadata(day2DayCaptureModeKey)
	sess.DeleteMetadata(day2DayCaptureBufferKey)
}

func (l *Loop) handleDay2DayStatus(input string) (string, bool) {
	date, ok := parseStatusDate(input)
	if !ok {
		return "", false
	}

	contents, path, err := l.loadDay2Day(date)
	if err != nil {
		return "Day2Day Fehler: Bot-System-Repo nicht gefunden.", true
	}
	if contents == "" {
		return fmt.Sprintf("Day2Day: keine Datei gefunden für %s (%s). Pfad: %s", date.Format("2006-01-02"), date.Weekday(), path), true
	}

	open, done := parseTasks(contents)
	next := nextSuggestion(contents)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Day2Day Status — %s (%s)\n", date.Format("2006-01-02"), date.Weekday()))
	sb.WriteString(fmt.Sprintf("Open: %d | Done: %d\n", len(open), len(done)))
	if next != "" {
		sb.WriteString(fmt.Sprintf("Next: %s\n", next))
	}
	if len(open) > 0 {
		sb.WriteString("Open Tasks:\n")
		for i, task := range open {
			if i >= 5 {
				sb.WriteString("... (more)\n")
				break
			}
			sb.WriteString(fmt.Sprintf("- %s\n", task))
		}
	}
	return strings.TrimSpace(sb.String()), true
}

func parseStatusDate(input string) (time.Time, bool) {
	lower := strings.ToLower(input)
	if !(strings.Contains(lower, "status") && (strings.Contains(lower, "task") || strings.Contains(lower, "aufgabe") || strings.Contains(lower, "day2day"))) {
		return time.Time{}, false
	}
	if m := regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`).FindString(lower); m != "" {
		if t, err := time.Parse("2006-01-02", m); err == nil {
			return t, true
		}
	}
	now := time.Now()
	switch {
	case strings.Contains(lower, "yesterday") || strings.Contains(lower, "gestern"):
		return now.AddDate(0, 0, -1), true
	case strings.Contains(lower, "tomorrow") || strings.Contains(lower, "morgen"):
		return now.AddDate(0, 0, 1), true
	default:
		return now, true
	}
}

func (l *Loop) applyDay2DayCommand(kind, text string) string {
	date := time.Now()
	contents, path, err := l.loadOrInitDay2Day(date)
	if err != nil {
		return "Day2Day Fehler: Bot-System-Repo nicht gefunden."
	}

	updated := contents
	switch kind {
	case "dtu":
		tasks := extractTasksFromText(text)
		updated = addTasks(updated, tasks)
		updated = appendProgress(updated, fmt.Sprintf("- %s: UPDATE — %s\n", time.Now().Format("15:04"), strings.TrimSpace(text)))
	case "dtp":
		updated = appendProgress(updated, fmt.Sprintf("- %s: PROGRESS — %s\n", time.Now().Format("15:04"), strings.TrimSpace(text)))
	}

	next := nextSuggestion(updated)
	updated = setNextStep(updated, next)

	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return "Day2Day Fehler: Konnte Tagesdatei nicht schreiben."
	}
	if next == "" {
		return "Aktualisiert. Keine offenen Tasks gefunden."
	}
	return fmt.Sprintf("Aktualisiert. Nächster Schritt: %s", next)
}

func (l *Loop) consolidateDay2Day(date time.Time) string {
	contents, path, err := l.loadOrInitDay2Day(date)
	if err != nil {
		return "Day2Day Fehler: Bot-System-Repo nicht gefunden."
	}

	open, done := parseTasks(contents)
	open = uniqueTasks(open)
	done = uniqueTasks(done)
	updated := setTasks(contents, open, done)
	updated = setConsolidatedState(updated, len(open), len(done), time.Now())
	next := nextSuggestion(updated)
	updated = setNextStep(updated, next)

	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return "Day2Day Fehler: Konnte Tagesdatei nicht schreiben."
	}
	return fmt.Sprintf("Konsolidiert. Open: %d | Done: %d", len(open), len(done))
}

func (l *Loop) planNextDay2Day(date time.Time) string {
	contents, _, err := l.loadDay2Day(date)
	if err != nil || contents == "" {
		return "Day2Day: keine Tagesdatei gefunden."
	}
	next := nextSuggestion(contents)
	if next == "" {
		return "Day2Day: keine offenen Tasks."
	}
	return fmt.Sprintf("Vorschlag Nächster Schritt: %s", next)
}

func (l *Loop) planAllDay2Day(date time.Time) string {
	contents, _, err := l.loadDay2Day(date)
	if err != nil || contents == "" {
		return "Day2Day: keine Tagesdatei gefunden."
	}
	open, _ := parseTasks(contents)
	if len(open) == 0 {
		return "Day2Day: keine offenen Tasks."
	}
	var sb strings.Builder
	sb.WriteString("Vorschlag Alle offenen Schritte:\n")
	for _, task := range open {
		sb.WriteString(fmt.Sprintf("- %s\n", task))
	}
	return strings.TrimSpace(sb.String())
}

func (l *Loop) day2DayTasksDir() (string, error) {
	base := l.systemRepoPath()
	if base == "" {
		return "", fmt.Errorf("system repo not found")
	}
	return filepath.Join(base, "operations", "day2day", "tasks"), nil
}

func (l *Loop) loadOrInitDay2Day(date time.Time) (string, string, error) {
	dir, err := l.day2DayTasksDir()
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, date.Format("2006-01-02")+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", "", err
	}
	if data, err := os.ReadFile(path); err == nil {
		return string(data), path, nil
	}
	header := fmt.Sprintf("# Day2Day — %s (%s)\n\n", date.Format("2006-01-02"), date.Weekday())
	template := header +
		"## Tasks\n\n" +
		"## Progress Log\n\n" +
		"## Notes / Context\n\n" +
		"## Consolidated State\n\n" +
		"## Next Step\n\n"
	return template, path, nil
}

func (l *Loop) loadDay2Day(date time.Time) (string, string, error) {
	dir, err := l.day2DayTasksDir()
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(dir, date.Format("2006-01-02")+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", path, nil
		}
		return "", path, err
	}
	return string(data), path, nil
}

func extractTasksFromText(text string) []string {
	lines := strings.Split(text, "\n")
	var tasks []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		t = strings.TrimPrefix(t, "-")
		t = strings.TrimPrefix(t, "*")
		t = strings.TrimPrefix(t, "+")
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks
}

func parseTasks(contents string) ([]string, []string) {
	var open []string
	var done []string
	lines := strings.Split(contents, "\n")
	inTasks := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			inTasks = strings.TrimSpace(line) == "## Tasks"
			continue
		}
		if !inTasks {
			continue
		}
		if strings.HasPrefix(line, "- [ ]") {
			open = append(open, strings.TrimSpace(strings.TrimPrefix(line, "- [ ]")))
		} else if strings.HasPrefix(strings.ToLower(line), "- [x]") {
			done = append(done, strings.TrimSpace(strings.TrimPrefix(line, "- [x]")))
		}
	}
	return open, done
}

func addTasks(contents string, tasks []string) string {
	if len(tasks) == 0 {
		return contents
	}
	open, done := parseTasks(contents)
	existing := map[string]bool{}
	for _, t := range open {
		existing[strings.ToLower(t)] = true
	}
	for _, t := range done {
		existing[strings.ToLower(t)] = true
	}
	var toAdd []string
	for _, t := range tasks {
		if !existing[strings.ToLower(t)] {
			toAdd = append(toAdd, t)
		}
	}
	if len(toAdd) == 0 {
		return contents
	}
	taskLines := ""
	for _, t := range toAdd {
		taskLines += fmt.Sprintf("- [ ] %s\n", t)
	}
	return insertIntoSection(contents, "## Tasks", taskLines)
}

func setTasks(contents string, open, done []string) string {
	var sb strings.Builder
	sb.WriteString("## Tasks\n")
	for _, t := range open {
		sb.WriteString(fmt.Sprintf("- [ ] %s\n", t))
	}
	for _, t := range done {
		sb.WriteString(fmt.Sprintf("- [x] %s\n", t))
	}
	return replaceSection(contents, "## Tasks", sb.String())
}

func setConsolidatedState(contents string, openCount, doneCount int, at time.Time) string {
	block := fmt.Sprintf("## Consolidated State\n- Open: %d\n- Done: %d\n- Last Consolidation: %s\n",
		openCount, doneCount, at.Format("15:04"))
	return replaceSection(contents, "## Consolidated State", block)
}

func setNextStep(contents, next string) string {
	if next == "" {
		next = "none"
	}
	block := fmt.Sprintf("## Next Step\n- %s\n", next)
	return replaceSection(contents, "## Next Step", block)
}

func insertIntoSection(contents, header, insert string) string {
	lines := strings.Split(contents, "\n")
	var out []string
	inSection := false
	inserted := false
	for i, line := range lines {
		out = append(out, line)
		if strings.TrimSpace(line) == header {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(line, "## ") && strings.TrimSpace(line) != header {
			if !inserted {
				out = append(out[:len(out)-1], strings.Split(strings.TrimRight(insert, "\n"), "\n")...)
				out = append(out, line)
				inserted = true
			}
			inSection = false
		}
		if i == len(lines)-1 && inSection && !inserted {
			out = append(out, strings.Split(strings.TrimRight(insert, "\n"), "\n")...)
			inserted = true
		}
	}
	if !inserted {
		return contents + "\n" + header + "\n" + insert
	}
	return strings.Join(out, "\n")
}

func replaceSection(contents, header, newBlock string) string {
	lines := strings.Split(contents, "\n")
	start := -1
	end := len(lines)
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			start = i
			continue
		}
		if start != -1 && strings.HasPrefix(line, "## ") && i > start {
			end = i
			break
		}
	}
	if start == -1 {
		return contents + "\n" + strings.TrimRight(newBlock, "\n") + "\n"
	}
	newLines := append([]string{}, lines[:start]...)
	newLines = append(newLines, strings.Split(strings.TrimRight(newBlock, "\n"), "\n")...)
	newLines = append(newLines, lines[end:]...)
	return strings.Join(newLines, "\n")
}

func uniqueTasks(tasks []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tasks {
		key := strings.ToLower(strings.TrimSpace(t))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, strings.TrimSpace(t))
	}
	return out
}

func appendProgress(contents, line string) string {
	if strings.Contains(contents, "## Progress Log") {
		parts := strings.Split(contents, "## Progress Log")
		if len(parts) >= 2 {
			return parts[0] + "## Progress Log\n" + "\n" + line + parts[1]
		}
	}
	return contents + "\n## Progress Log\n" + line
}

func markDone(contents, doneText string) string {
	lines := strings.Split(contents, "\n")
	lowerDone := strings.ToLower(doneText)
	for i, line := range lines {
		if strings.Contains(line, "- [ ]") {
			taskText := strings.ToLower(line)
			if lowerDone != "" && strings.Contains(taskText, lowerDone) {
				lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

func nextSuggestion(contents string) string {
	lines := strings.Split(contents, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "- [ ]") {
			return strings.TrimSpace(strings.TrimPrefix(line, "- [ ]"))
		}
	}
	return ""
}

func (l *Loop) systemRepoPath() string {
	if l.systemRepo != "" {
		path := l.systemRepo
		if strings.HasPrefix(path, "~") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[1:])
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	wsPath := l.workspace
	if strings.HasPrefix(wsPath, "~") {
		home, _ := os.UserHomeDir()
		wsPath = filepath.Join(home, wsPath[1:])
	}
	if abs, err := filepath.Abs(wsPath); err == nil {
		wsPath = abs
	}
	candidates := []string{
		filepath.Join(wsPath, "Bottibot-REPO-01"),
		filepath.Join(wsPath, "gomikrobot", "Bottibot-REPO-01"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// injectRAGContext searches semantic memory for relevant context and appends
// it to the system prompt. Returns messages unchanged if memoryService is nil
// or search returns no relevant results.
func (l *Loop) injectRAGContext(ctx context.Context, messages []provider.Message, userQuery string) []provider.Message {
	if l.memoryService == nil || len(messages) == 0 {
		return messages
	}

	chunks, err := l.memoryService.Search(ctx, userQuery, 5)
	if err != nil {
		slog.Warn("RAG search failed", "error", err)
		return messages
	}

	// Filter out low-relevance results
	var relevant []memory.MemoryChunk
	for _, c := range chunks {
		if c.Score >= 0.3 {
			relevant = append(relevant, c)
		}
	}

	if len(relevant) == 0 {
		return messages
	}

	// Build the memory section
	var sb strings.Builder
	sb.WriteString("\n\n---\n\n# Relevant Memory\n\n")
	for _, c := range relevant {
		sb.WriteString(fmt.Sprintf("- [source=%s, relevance=%.0f%%] %s\n", c.Source, c.Score*100, c.Content))
	}

	// Append to system prompt (first message)
	messages[0].Content += sb.String()
	return messages
}

// injectWorkingMemory loads scoped working memory and appends it to the system prompt.
func (l *Loop) injectWorkingMemory(messages []provider.Message, resourceID, threadID string) []provider.Message {
	if l.workingMemory == nil || len(messages) == 0 {
		return messages
	}

	resContent, thrContent, err := l.workingMemory.LoadBoth(resourceID, threadID)
	if err != nil {
		slog.Warn("Working memory load failed", "error", err)
		return messages
	}

	if resContent == "" && thrContent == "" {
		return messages
	}

	var sb strings.Builder
	sb.WriteString("\n\n---\n\n# Working Memory\n\n")
	if resContent != "" {
		sb.WriteString(resContent)
		sb.WriteString("\n")
	}
	if thrContent != "" {
		if resContent != "" {
			sb.WriteString("\n## Thread Context\n\n")
		}
		sb.WriteString(thrContent)
		sb.WriteString("\n")
	}

	messages[0].Content += sb.String()
	return messages
}

// injectObservations loads compressed observation notes and appends them to the system prompt.
func (l *Loop) injectObservations(messages []provider.Message, sessionID string) []provider.Message {
	if l.observer == nil || len(messages) == 0 {
		return messages
	}

	observations, err := l.observer.LoadObservations(sessionID)
	if err != nil {
		slog.Warn("Observations load failed", "error", err)
		return messages
	}

	formatted := memory.FormatObservations(observations)
	if formatted == "" {
		return messages
	}

	messages[0].Content += "\n\n---\n\n" + formatted
	return messages
}

func (l *Loop) processMessage(ctx context.Context, msg *bus.InboundMessage) (response string, taskID string, err error) {
	sessionKey := fmt.Sprintf("%s:%s", msg.Channel, msg.ChatID)
	if msg.TraceID == "" {
		msg.TraceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}

	// Ensure IdempotencyKey
	if msg.IdempotencyKey == "" {
		msg.IdempotencyKey = fmt.Sprintf("auto:%s:%s", msg.Channel, msg.TraceID)
	}

	// DEDUP CHECK (H-005): if timeline is available, check for existing task
	if l.timeline != nil {
		existing, lookupErr := l.timeline.GetTaskByIdempotencyKey(msg.IdempotencyKey)
		if lookupErr != nil {
			slog.Warn("Dedup lookup failed", "error", lookupErr)
		} else if existing != nil {
			switch existing.Status {
			case timeline.TaskStatusCompleted:
				slog.Info("Dedup hit: returning cached result", "task_id", existing.TaskID)
				return existing.ContentOut, existing.TaskID, nil
			case timeline.TaskStatusProcessing:
				slog.Info("Dedup hit: task still processing, skipping", "task_id", existing.TaskID)
				return "", existing.TaskID, nil
			}
		}
	}

	// CREATE TASK (H-004)
	if l.timeline != nil {
		task, createErr := l.timeline.CreateTask(&timeline.AgentTask{
			IdempotencyKey: msg.IdempotencyKey,
			TraceID:        msg.TraceID,
			Channel:        msg.Channel,
			ChatID:         msg.ChatID,
			SenderID:       msg.SenderID,
			ContentIn:      msg.Content,
			MessageType:    msg.MessageType(),
		})
		if createErr != nil {
			slog.Warn("Failed to create task", "error", createErr)
		} else {
			taskID = task.TaskID
			_ = l.timeline.UpdateTaskStatus(taskID, timeline.TaskStatusProcessing, "", "")
		}
	}

	// Set active context for policy checks and token tracking
	l.activeTaskID = taskID
	l.activeSender = msg.SenderID
	l.activeChannel = msg.Channel
	l.activeChatID = msg.ChatID
	l.activeTraceID = msg.TraceID
	l.activeMessageType = msg.MessageType()

	// PROCESS
	response, err = l.ProcessDirectWithTrace(ctx, msg.Content, sessionKey, msg.TraceID)

	// UPDATE TASK
	if l.timeline != nil && taskID != "" {
		if err != nil {
			_ = l.timeline.UpdateTaskStatus(taskID, timeline.TaskStatusFailed, "", err.Error())
		} else {
			_ = l.timeline.UpdateTaskStatus(taskID, timeline.TaskStatusCompleted, response, "")
		}
	}

	// PUBLISH TRACE to group (if active)
	if l.groupPublisher != nil && l.groupPublisher.Active() && msg.TraceID != "" {
		go func() {
			pubCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = l.groupPublisher.PublishTrace(pubCtx, map[string]string{
				"trace_id":  msg.TraceID,
				"span_type": "TASK",
				"title":     fmt.Sprintf("Task from %s via %s", msg.SenderID, msg.Channel),
				"content":   response,
			})
		}()
	}

	return response, taskID, err
}

func (l *Loop) runAgentLoop(ctx context.Context, messages []provider.Message) (string, error) {
	toolDefs := l.buildToolDefinitions()

	for i := 0; i < l.maxIterations; i++ {
		// QUOTA CHECK (H-014): check daily token limit before LLM call
		if err := l.checkTokenQuota(); err != nil {
			return err.Error(), nil
		}

		// Call LLM
		llmStart := time.Now()
		resp, err := l.provider.Chat(ctx, &provider.ChatRequest{
			Messages:    messages,
			Tools:       toolDefs,
			Model:       l.model,
			MaxTokens:   4096,
			Temperature: 0.7,
		})
		llmDuration := time.Since(llmStart)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		// TOKEN TRACKING (H-013): record usage
		l.trackTokens(resp.Usage)

		// Build LLM span summary
		toolCallSummary := ""
		if len(resp.ToolCalls) > 0 {
			names := make([]string, len(resp.ToolCalls))
			for ti, tc := range resp.ToolCalls {
				names[ti] = tc.Name
			}
			toolCallSummary = fmt.Sprintf(" → tools: %s", strings.Join(names, ", "))
		}
		llmContent := fmt.Sprintf("model=%s tokens=%d duration=%dms%s", l.model, resp.Usage.TotalTokens, llmDuration.Milliseconds(), toolCallSummary)

		// Log LLM span to timeline for end-to-end trace visibility
		if l.timeline != nil && l.activeTraceID != "" {
			// Build rich metadata for LLM span
			llmMeta := map[string]any{
				"model":             l.model,
				"temperature":       0.7,
				"max_tokens":        4096,
				"duration_ms":       llmDuration.Milliseconds(),
				"finish_reason":     resp.FinishReason,
				"prompt_tokens":     resp.Usage.PromptTokens,
				"completion_tokens": resp.Usage.CompletionTokens,
				"total_tokens":      resp.Usage.TotalTokens,
				"response_text":     truncateStr(resp.Content, 10240),
				"message_count":     len(messages),
			}
			// System prompt preview (first message if role=system)
			if len(messages) > 0 && messages[0].Role == "system" {
				llmMeta["system_prompt"] = truncateStr(messages[0].Content, 2048)
			}
			// Last user message
			for j := len(messages) - 1; j >= 0; j-- {
				if messages[j].Role == "user" {
					llmMeta["last_user_message"] = truncateStr(messages[j].Content, 2048)
					break
				}
			}
			// Tool calls requested
			if len(resp.ToolCalls) > 0 {
				tcList := make([]map[string]any, len(resp.ToolCalls))
				for ti, tc := range resp.ToolCalls {
					tcList[ti] = map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					}
				}
				llmMeta["tool_calls"] = tcList
			}
			llmMetaJSON, _ := json.Marshal(llmMeta)

			_ = l.timeline.AddEvent(&timeline.TimelineEvent{
				EventID:        fmt.Sprintf("LLM_%s_%d_%d", l.activeTraceID, i, time.Now().UnixNano()),
				TraceID:        l.activeTraceID,
				Timestamp:      llmStart,
				SenderID:       "AGENT",
				SenderName:     "LLM",
				EventType:      "SYSTEM",
				ContentText:    llmContent,
				Classification: "LLM",
				Authorized:     true,
				Metadata:       string(llmMetaJSON),
			})
		}
		// Publish LLM span to group traces topic
		if l.groupPublisher != nil && l.groupPublisher.Active() && l.activeTraceID != "" {
			go func(traceID, content string, dur time.Duration) {
				pubCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				now := time.Now()
				_ = l.groupPublisher.PublishTrace(pubCtx, map[string]string{
					"trace_id":    traceID,
					"span_type":   "LLM",
					"title":       fmt.Sprintf("LLM call: %s", l.model),
					"content":     content,
					"started_at":  now.Add(-dur).Format(time.RFC3339),
					"ended_at":    now.Format(time.RFC3339),
					"duration_ms": fmt.Sprintf("%d", dur.Milliseconds()),
				})
			}(l.activeTraceID, llmContent, llmDuration)
		}

		// Check for tool calls
		if len(resp.ToolCalls) == 0 {
			// No tool calls, return the response
			return resp.Content, nil
		}

		// Add assistant message with tool calls
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call
		for _, tc := range resp.ToolCalls {
			// POLICY CHECK (H-011): evaluate before tool execution
			if denied, reason := l.checkToolPolicy(ctx, tc.Name, tc.Arguments); denied {
				slog.Warn("Tool denied by policy", "tool", tc.Name, "reason", reason)
				messages = append(messages, provider.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("Policy denied: %s", reason),
					ToolCallID: tc.ID,
				})
				continue
			}

			toolStart := time.Now()
			result, err := l.registry.Execute(ctx, tc.Name, tc.Arguments)
			toolDuration := time.Since(toolStart)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}

			// Log tool span to timeline for end-to-end trace visibility
			toolContent := fmt.Sprintf("tool=%s duration=%dms result_len=%d", tc.Name, toolDuration.Milliseconds(), len(result))
			if l.timeline != nil && l.activeTraceID != "" {
				// Build rich metadata for TOOL span
				toolMeta := map[string]any{
					"tool_name":    tc.Name,
					"tool_call_id": tc.ID,
					"arguments":    tc.Arguments,
					"duration_ms":  toolDuration.Milliseconds(),
					"result":       truncateStr(result, 10240),
				}
				if err != nil {
					toolMeta["error"] = err.Error()
				}
				toolMetaJSON, _ := json.Marshal(toolMeta)

				_ = l.timeline.AddEvent(&timeline.TimelineEvent{
					EventID:        fmt.Sprintf("TOOL_%s_%s_%d", l.activeTraceID, tc.Name, time.Now().UnixNano()),
					TraceID:        l.activeTraceID,
					Timestamp:      toolStart,
					SenderID:       "AGENT",
					SenderName:     "Tool",
					EventType:      "SYSTEM",
					ContentText:    toolContent,
					Classification: "TOOL",
					Authorized:     true,
					Metadata:       string(toolMetaJSON),
				})
			}
			// Publish tool span to group traces topic
			if l.groupPublisher != nil && l.groupPublisher.Active() && l.activeTraceID != "" {
				go func(traceID, toolN, content string, dur time.Duration) {
					pubCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					now := time.Now()
					_ = l.groupPublisher.PublishTrace(pubCtx, map[string]string{
						"trace_id":    traceID,
						"span_type":   "TOOL",
						"title":       fmt.Sprintf("Tool: %s", toolN),
						"content":     content,
						"started_at":  now.Add(-dur).Format(time.RFC3339),
						"ended_at":    now.Format(time.RFC3339),
						"duration_ms": fmt.Sprintf("%d", dur.Milliseconds()),
					})
				}(l.activeTraceID, tc.Name, toolContent, toolDuration)
			}

			if strings.Contains(result, "Ey, du spinnst wohl? Hä?") {
				return "Ey, du spinnst wohl? Hä? 💣 👮‍♂️ 🔒", nil
			}

			// Auto-index substantive tool results
			if l.autoIndexer != nil && err == nil && len(result) > 200 {
				item := memory.FormatToolResult(tc.Name, tc.Arguments, result)
				l.autoIndexer.Enqueue(item)
			}

			// Track tool expertise
			l.expertiseTracker.RecordToolUse(tc.Name, l.activeTaskID, toolDuration.Milliseconds(), err == nil)

			// Add tool result
			messages = append(messages, provider.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})

			slog.Debug("Tool executed", "name", tc.Name, "result_length", len(result))
		}
	}

	return "Max iterations reached. Please try a simpler request.", nil
}

// truncateStr returns s trimmed to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// checkToolPolicy evaluates whether a tool call should proceed.
// Returns (denied bool, reason string).
func (l *Loop) checkToolPolicy(ctx context.Context, toolName string, args map[string]any) (bool, string) {
	if l.policy == nil {
		return false, ""
	}

	tier := tools.TierReadOnly
	if t, ok := l.registry.Get(toolName); ok {
		tier = tools.ToolTier(t)
	}

	policyCtx := policy.Context{
		Sender:      l.activeSender,
		Channel:     l.activeChannel,
		Tool:        toolName,
		Tier:        tier,
		Arguments:   args,
		TraceID:     l.activeTraceID,
		MessageType: l.activeMessageType,
	}

	decision := l.policy.Evaluate(policyCtx)

	// Log policy decision (H-015)
	if l.timeline != nil {
		_ = l.timeline.LogPolicyDecision(&timeline.PolicyDecisionRecord{
			TraceID: l.activeTraceID,
			TaskID:  l.activeTaskID,
			Tool:    toolName,
			Tier:    tier,
			Sender:  l.activeSender,
			Channel: l.activeChannel,
			Allowed: decision.Allow,
			Reason:  decision.Reason,
		})
	}
	// Publish policy decision as audit event to group
	if l.groupPublisher != nil && l.groupPublisher.Active() && l.activeTraceID != "" {
		action := "ALLOW"
		if !decision.Allow {
			action = "DENY"
		}
		detail := fmt.Sprintf("tool=%s tier=%d sender=%s action=%s reason=%s", toolName, tier, l.activeSender, action, decision.Reason)
		go func(traceID, det string) {
			pubCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = l.groupPublisher.PublishAudit(pubCtx, "policy_decision", traceID, det)
		}(l.activeTraceID, detail)
	}

	if !decision.Allow {
		// Interactive approval gate for tier 2+ internal messages
		if decision.RequiresApproval && l.approvalMgr != nil && l.bus != nil {
			req := &approval.ApprovalRequest{
				Tool:      toolName,
				Tier:      tier,
				Arguments: args,
				Sender:    l.activeSender,
				Channel: l.activeChannel,
				TraceID: l.activeTraceID,
				TaskID:  l.activeTaskID,
			}
			approvalID := l.approvalMgr.Create(req)

			// Format and send prompt to user
			argsPreview := formatArgsPreview(args)
			prompt := fmt.Sprintf("Tool \"%s\" (tier %d) requires approval.\nArgs: %s\nReply approve:%s or deny:%s",
				toolName, tier, argsPreview, approvalID, approvalID)

			l.bus.PublishOutbound(&bus.OutboundMessage{
				Channel: l.activeChannel,
				ChatID:  l.activeChatID,
				TraceID: l.activeTraceID,
				TaskID:  l.activeTaskID,
				Content: prompt,
			})

			// Block with configurable timeout (default 60s)
			timeout := l.approvalTimeout()
			waitCtx, waitCancel := context.WithTimeout(ctx, timeout)
			defer waitCancel()

			approved, err := l.approvalMgr.Wait(waitCtx, approvalID)
			if err != nil {
				slog.Warn("Approval wait failed", "id", approvalID, "error", err)
				return true, "approval_timeout"
			}
			if approved {
				return false, "" // Allow execution
			}
			return true, "approval_denied"
		}
		return true, decision.Reason
	}
	return false, ""
}

// parseApprovalResponse checks if a message is an approval response.
// Returns (id, approved, ok).
func parseApprovalResponse(content string) (string, bool, bool) {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "approve:") {
		id := strings.TrimSpace(strings.TrimPrefix(trimmed, "approve:"))
		if id != "" {
			return id, true, true
		}
	}
	if strings.HasPrefix(trimmed, "deny:") {
		id := strings.TrimSpace(strings.TrimPrefix(trimmed, "deny:"))
		if id != "" {
			return id, false, true
		}
	}
	return "", false, false
}

// formatArgsPreview returns a truncated JSON representation of tool arguments.
func formatArgsPreview(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "{...}"
	}
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// approvalTimeout returns the configured approval timeout duration.
// Reads "approval_timeout_seconds" from settings, defaults to 60s.
func (l *Loop) approvalTimeout() time.Duration {
	if l.timeline == nil {
		return 60 * time.Second
	}
	val, err := l.timeline.GetSetting("approval_timeout_seconds")
	if err != nil || val == "" {
		return 60 * time.Second
	}
	var seconds int
	if _, err := fmt.Sscanf(val, "%d", &seconds); err != nil || seconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

// trackTokens persists token usage for the active task.
func (l *Loop) trackTokens(usage provider.Usage) {
	if l.timeline == nil || l.activeTaskID == "" {
		return
	}
	if usage.TotalTokens > 0 {
		_ = l.timeline.UpdateTaskTokens(l.activeTaskID, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}
}

// checkTokenQuota checks if the daily token limit has been exceeded.
func (l *Loop) checkTokenQuota() error {
	if l.timeline == nil {
		return nil
	}
	limitStr, err := l.timeline.GetSetting("daily_token_limit")
	if err != nil || limitStr == "" {
		return nil // No quota configured
	}
	var limit int
	if _, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil || limit <= 0 {
		return nil
	}
	used, err := l.timeline.GetDailyTokenUsage()
	if err != nil {
		return nil // Fail open
	}
	if used >= limit {
		return fmt.Errorf("Daily token quota exceeded (%d/%d). Please try again tomorrow or ask an admin to increase the limit.", used, limit)
	}
	return nil
}

func (l *Loop) buildToolDefinitions() []provider.ToolDefinition {
	toolList := l.registry.List()
	defs := make([]provider.ToolDefinition, len(toolList))

	for i, tool := range toolList {
		defs[i] = provider.ToolDefinition{
			Type: "function",
			Function: provider.FunctionDef{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Parameters(),
			},
		}
	}

	return defs
}

// SessionKey builds a session key from channel and chat ID.
func SessionKey(channel, chatID string) string {
	return strings.Join([]string{channel, chatID}, ":")
}
