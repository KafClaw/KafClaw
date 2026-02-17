package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/provider"
	"github.com/KafClaw/KafClaw/internal/session"
)

func writeDay2DayFile(t *testing.T, root string, date time.Time, contents string) string {
	t.Helper()
	path := filepath.Join(root, "operations", "day2day", "tasks", date.Format("2006-01-02")+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestParseAndMutateDay2DayHelpers(t *testing.T) {
	cmd, ok := parseDay2DayCommand("dtu   one\ntwo")
	if !ok || cmd.Kind != "dtu" || !strings.Contains(cmd.Text, "one") {
		t.Fatalf("unexpected parse cmd: %+v ok=%v", cmd, ok)
	}
	if _, ok := parseDay2DayCommand("hello there"); ok {
		t.Fatal("expected invalid command")
	}

	tasks := extractTasksFromText("- alpha\n* beta\n+ gamma\n\n")
	if len(tasks) != 3 || tasks[0] != "alpha" || tasks[2] != "gamma" {
		t.Fatalf("unexpected extracted tasks: %#v", tasks)
	}

	base := "# Day2Day\n\n## Tasks\n- [ ] one\n- [x] done-one\n\n## Progress Log\n\n## Notes / Context\n\n## Consolidated State\n\n## Next Step\n\n"
	open, done := parseTasks(base)
	if len(open) != 1 || open[0] != "one" || len(done) != 1 || done[0] != "done-one" {
		t.Fatalf("unexpected parseTasks open=%#v done=%#v", open, done)
	}

	added := addTasks(base, []string{"two", "one"})
	if !strings.Contains(added, "- [ ] two") {
		t.Fatalf("expected inserted open task, got:\n%s", added)
	}

	replaced := setTasks(base, []string{"a"}, []string{"b"})
	if !strings.Contains(replaced, "- [ ] a") || !strings.Contains(replaced, "- [x] b") {
		t.Fatalf("setTasks failed:\n%s", replaced)
	}

	consolidated := setConsolidatedState(base, 3, 1, time.Now())
	if !strings.Contains(consolidated, "- Open: 3") || !strings.Contains(consolidated, "- Done: 1") {
		t.Fatalf("setConsolidatedState failed:\n%s", consolidated)
	}

	next := setNextStep(base, "ship it")
	if !strings.Contains(next, "## Next Step\n- ship it") {
		t.Fatalf("setNextStep failed:\n%s", next)
	}
	nextNone := setNextStep(base, "")
	if !strings.Contains(nextNone, "## Next Step\n- none") {
		t.Fatalf("setNextStep default failed:\n%s", nextNone)
	}

	inserted := insertIntoSection(base, "## Tasks", "- [ ] zed\n")
	if !strings.Contains(inserted, "- [ ] zed") {
		t.Fatalf("insertIntoSection failed:\n%s", inserted)
	}
	insertAppended := insertIntoSection("# x\n", "## Missing", "line\n")
	if !strings.Contains(insertAppended, "## Missing\nline") {
		t.Fatalf("insertIntoSection append failed:\n%s", insertAppended)
	}

	repl := replaceSection(base, "## Next Step", "## Next Step\n- n1\n")
	if !strings.Contains(repl, "## Next Step\n- n1") {
		t.Fatalf("replaceSection failed:\n%s", repl)
	}
	replAppend := replaceSection("# x\n", "## Unknown", "## Unknown\n- x\n")
	if !strings.Contains(replAppend, "## Unknown\n- x") {
		t.Fatalf("replaceSection append failed:\n%s", replAppend)
	}

	u := uniqueTasks([]string{"One", " one ", "", "TWO", "two"})
	if len(u) != 2 || u[0] != "One" || u[1] != "TWO" {
		t.Fatalf("uniqueTasks failed: %#v", u)
	}

	p := appendProgress(base, "- 10:00: PROGRESS\n")
	if !strings.Contains(p, "## Progress Log\n\n- 10:00: PROGRESS") {
		t.Fatalf("appendProgress failed:\n%s", p)
	}
	p2 := appendProgress("# no progress\n", "- line\n")
	if !strings.Contains(p2, "## Progress Log\n- line") {
		t.Fatalf("appendProgress create failed:\n%s", p2)
	}

	marked := markDone(base, "one")
	if !strings.Contains(marked, "- [x] one") {
		t.Fatalf("markDone failed:\n%s", marked)
	}
	if s := nextSuggestion(base); s != "one" {
		t.Fatalf("nextSuggestion expected one, got %q", s)
	}
}

func TestParseStatusDate(t *testing.T) {
	explicit, ok := parseStatusDate("show day2day task status 2026-01-05")
	if !ok || explicit.Format("2006-01-02") != "2026-01-05" {
		t.Fatalf("expected explicit date parse, got %v ok=%v", explicit, ok)
	}
	if _, ok := parseStatusDate("status only please"); ok {
		t.Fatal("expected no date parse without task/day2day keyword")
	}

	now := time.Now()
	y, ok := parseStatusDate("day2day task status yesterday")
	if !ok {
		t.Fatal("expected yesterday parse")
	}
	if y.Format("2006-01-02") != now.AddDate(0, 0, -1).Format("2006-01-02") {
		t.Fatalf("unexpected yesterday date: %s", y.Format("2006-01-02"))
	}

	tom, ok := parseStatusDate("day2day aufgabe status tomorrow")
	if !ok {
		t.Fatal("expected tomorrow parse")
	}
	if tom.Format("2006-01-02") != now.AddDate(0, 0, 1).Format("2006-01-02") {
		t.Fatalf("unexpected tomorrow date: %s", tom.Format("2006-01-02"))
	}
}

func TestDay2DayFlowAndStatus(t *testing.T) {
	sysRepo := t.TempDir()
	loop := NewLoop(LoopOptions{
		Provider:   &mockProvider{},
		Workspace:  t.TempDir(),
		WorkRepo:   t.TempDir(),
		SystemRepo: sysRepo,
	})

	now := time.Now()
	initial := "# Day2Day\n\n## Tasks\n- [ ] old\n- [x] done-old\n\n## Progress Log\n\n## Notes / Context\n\n## Consolidated State\n\n## Next Step\n\n"
	dayFile := writeDay2DayFile(t, sysRepo, now, initial)

	msg := loop.applyDay2DayCommand("dtu", "- new-one\n- old")
	if !strings.Contains(msg, "Aktualisiert.") {
		t.Fatalf("unexpected apply msg: %s", msg)
	}
	data, err := os.ReadFile(dayFile)
	if err != nil {
		t.Fatalf("read day file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "- [ ] new-one") || !strings.Contains(content, "## Next Step\n- old") {
		t.Fatalf("unexpected updated content:\n%s", content)
	}

	msg = loop.applyDay2DayCommand("dtp", "worked on it")
	if !strings.Contains(msg, "Aktualisiert.") {
		t.Fatalf("unexpected progress msg: %s", msg)
	}

	msg = loop.consolidateDay2Day(now)
	if !strings.Contains(msg, "Konsolidiert.") {
		t.Fatalf("unexpected consolidate msg: %s", msg)
	}

	next := loop.planNextDay2Day(now)
	if !strings.Contains(next, "Vorschlag Nächster Schritt:") {
		t.Fatalf("unexpected planNext: %s", next)
	}
	all := loop.planAllDay2Day(now)
	if !strings.Contains(all, "Vorschlag Alle offenen Schritte:") || !strings.Contains(all, "- old") {
		t.Fatalf("unexpected planAll: %s", all)
	}

	status, handled := loop.handleDay2DayStatus("day2day task status")
	if !handled || !strings.Contains(status, "Day2Day Status") || !strings.Contains(status, "Open:") {
		t.Fatalf("unexpected status handled=%v text=%s", handled, status)
	}
}

func TestDay2DayCaptureFlow(t *testing.T) {
	sysRepo := t.TempDir()
	loop := NewLoop(LoopOptions{
		Provider:   &mockProvider{},
		Workspace:  t.TempDir(),
		WorkRepo:   t.TempDir(),
		SystemRepo: sysRepo,
	})
	sess := session.NewSession("cli:default")

	resp, handled := loop.handleDay2Day(sess, "dtu")
	if !handled || !strings.Contains(resp, "capture started") {
		t.Fatalf("expected capture start, got handled=%v resp=%q", handled, resp)
	}
	resp, handled = loop.handleDay2Day(sess, "- task from capture")
	if !handled || !strings.Contains(resp, "captured") {
		t.Fatalf("expected capture append, got handled=%v resp=%q", handled, resp)
	}
	resp, handled = loop.handleDay2Day(sess, "dtc")
	if !handled || !strings.Contains(resp, "Aktualisiert.") {
		t.Fatalf("expected capture close apply, got handled=%v resp=%q", handled, resp)
	}

	contents, _, err := loop.loadDay2Day(time.Now())
	if err != nil {
		t.Fatalf("load day2day: %v", err)
	}
	if !strings.Contains(contents, "task from capture") {
		t.Fatalf("expected captured task persisted, got:\n%s", contents)
	}

	resp, handled = loop.handleDay2Day(sess, "dtc")
	if !handled || !strings.Contains(resp, "no open capture") {
		t.Fatalf("expected dtc no open capture, got handled=%v resp=%q", handled, resp)
	}
}

func TestDay2DayMissingRepoHandling(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Provider:  &mockProvider{},
		Workspace: filepath.Join(t.TempDir(), "missing-workspace"),
		WorkRepo:  t.TempDir(),
	})

	if _, err := loop.day2DayTasksDir(); err == nil {
		t.Fatal("expected day2DayTasksDir error when system repo missing")
	}
	if got := loop.applyDay2DayCommand("dtu", "x"); !strings.Contains(got, "Bot-System-Repo nicht gefunden") {
		t.Fatalf("expected repo-not-found apply error, got %q", got)
	}
	if got := loop.consolidateDay2Day(time.Now()); !strings.Contains(got, "Bot-System-Repo nicht gefunden") {
		t.Fatalf("expected repo-not-found consolidate error, got %q", got)
	}
	if got := loop.planNextDay2Day(time.Now()); !strings.Contains(got, "keine Tagesdatei gefunden") {
		t.Fatalf("unexpected planNext missing text: %q", got)
	}
}

func TestProcessDirectAndStop(t *testing.T) {
	loop := NewLoop(LoopOptions{
		Provider:      &mockProvider{responses: []provider.ChatResponse{{Content: "ok", Usage: provider.Usage{TotalTokens: 1}}}},
		Workspace:     t.TempDir(),
		WorkRepo:      t.TempDir(),
		Model:         "mock-model",
		MaxIterations: 1,
	})

	resp, err := loop.ProcessDirect(context.Background(), "hello", "cli:default")
	if err != nil {
		t.Fatalf("ProcessDirect err: %v", err)
	}
	if !strings.Contains(resp, "ok") {
		t.Fatalf("unexpected direct response: %q", resp)
	}

	loop.running.Store(true)
	loop.Stop()
	if loop.running.Load() {
		t.Fatal("expected loop stopped")
	}
}
