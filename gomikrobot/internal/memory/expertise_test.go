package memory

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupExpertiseDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	schema := `
CREATE TABLE agent_expertise (
	skill_name TEXT PRIMARY KEY,
	success_count INTEGER NOT NULL DEFAULT 0,
	failure_count INTEGER NOT NULL DEFAULT 0,
	avg_quality REAL NOT NULL DEFAULT 0.0,
	last_used DATETIME,
	total_duration_ms INTEGER NOT NULL DEFAULT 0,
	trend TEXT NOT NULL DEFAULT 'stable'
);

CREATE TABLE skill_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	skill_name TEXT NOT NULL,
	task_id TEXT,
	action TEXT NOT NULL,
	quality REAL NOT NULL DEFAULT 0.5,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	metadata TEXT DEFAULT '{}',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_skill_events_skill ON skill_events(skill_name);
`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	return db
}

func TestRecordEvent(t *testing.T) {
	db := setupExpertiseDB(t)
	defer db.Close()

	tracker := NewExpertiseTracker(db)

	err := tracker.RecordEvent(SkillEvent{
		SkillName:  "filesystem",
		TaskID:     "task-1",
		Action:     "tool_used",
		Quality:    0.8,
		DurationMs: 150,
		Metadata:   `{"tool":"read_file"}`,
	})
	if err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}

	// Check event was inserted
	var count int
	db.QueryRow("SELECT COUNT(*) FROM skill_events WHERE skill_name = 'filesystem'").Scan(&count)
	if count != 1 {
		t.Errorf("Expected 1 event, got %d", count)
	}

	// Check expertise was created
	summary, err := tracker.GetExpertise("filesystem")
	if err != nil {
		t.Fatalf("GetExpertise failed: %v", err)
	}
	if summary == nil {
		t.Fatal("Expected non-nil summary")
	}
	if summary.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", summary.SuccessCount)
	}
	if summary.AvgQuality < 0.79 || summary.AvgQuality > 0.81 {
		t.Errorf("AvgQuality = %f, want ~0.8", summary.AvgQuality)
	}
}

func TestRecordEventError(t *testing.T) {
	db := setupExpertiseDB(t)
	defer db.Close()

	tracker := NewExpertiseTracker(db)

	_ = tracker.RecordEvent(SkillEvent{
		SkillName: "shell",
		TaskID:    "task-2",
		Action:    "error",
		Quality:   0.1,
	})

	summary, _ := tracker.GetExpertise("shell")
	if summary == nil {
		t.Fatal("Expected summary")
	}
	if summary.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", summary.FailureCount)
	}
	if summary.SuccessCount != 0 {
		t.Errorf("SuccessCount = %d, want 0", summary.SuccessCount)
	}
}

func TestComputeScore(t *testing.T) {
	s := ExpertiseSummary{
		SuccessCount: 80,
		FailureCount: 20,
		AvgQuality:   0.85,
	}
	score := computeScore(s)
	// 0.6*(80/100) + 0.3*0.85 + 0.1*(100/100) = 0.48 + 0.255 + 0.1 = 0.835
	if score < 0.83 || score > 0.84 {
		t.Errorf("Score = %f, want ~0.835", score)
	}
}

func TestComputeScoreZero(t *testing.T) {
	s := ExpertiseSummary{}
	score := computeScore(s)
	if score != 0 {
		t.Errorf("Score = %f, want 0", score)
	}
}

func TestListExpertise(t *testing.T) {
	db := setupExpertiseDB(t)
	defer db.Close()

	tracker := NewExpertiseTracker(db)

	_ = tracker.RecordEvent(SkillEvent{SkillName: "filesystem", Action: "tool_used", Quality: 0.9})
	_ = tracker.RecordEvent(SkillEvent{SkillName: "shell", Action: "tool_used", Quality: 0.6})

	list, err := tracker.ListExpertise()
	if err != nil {
		t.Fatalf("ListExpertise failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("Expected 2 skills, got %d", len(list))
	}
	// Should be ordered by avg_quality DESC
	if list[0].SkillName != "filesystem" {
		t.Errorf("Expected filesystem first (higher quality), got %s", list[0].SkillName)
	}
}

func TestClassifySkill(t *testing.T) {
	tests := []struct {
		tool string
		want string
	}{
		{"read_file", "filesystem"},
		{"write_file", "filesystem"},
		{"exec", "shell"},
		{"remember", "memory"},
		{"recall", "memory"},
		{"web_search", "research"},
		{"message", "communication"},
		{"unknown_tool", "general"},
	}
	for _, tt := range tests {
		got := ClassifySkill(tt.tool)
		if got != tt.want {
			t.Errorf("ClassifySkill(%q) = %q, want %q", tt.tool, got, tt.want)
		}
	}
}

func TestNilTracker(t *testing.T) {
	var tracker *ExpertiseTracker
	tracker.RecordToolUse("read_file", "task-1", 100, true)
	tracker.RecordTaskCompletion("filesystem", "task-1", 100, 0.8)
	// No panic = pass
}

func TestGetExpertiseNotFound(t *testing.T) {
	db := setupExpertiseDB(t)
	defer db.Close()

	tracker := NewExpertiseTracker(db)
	summary, err := tracker.GetExpertise("nonexistent")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if summary != nil {
		t.Error("Expected nil summary for nonexistent skill")
	}
}
