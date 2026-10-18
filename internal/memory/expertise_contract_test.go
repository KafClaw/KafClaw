package memory

import (
	"testing"
	"time"
)

func TestExpertiseTrendWindowsAndStorageFailure(t *testing.T) {
	for _, tc := range []struct {
		name             string
		previous, recent float64
		want             string
	}{
		{"improving", .2, .9, "improving"}, {"declining", .9, .2, "declining"}, {"stable", .6, .65, "stable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupExpertiseDB(t)
			defer db.Close()
			for i := 0; i < 20; i++ {
				quality := tc.previous
				if i >= 10 {
					quality = tc.recent
				}
				if _, err := db.Exec("INSERT INTO skill_events(skill_name,action,quality,created_at) VALUES(?,'user_feedback',?,?)", "research", quality, time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC)); err != nil {
					t.Fatal(err)
				}
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if got := computeTrend(tx, "research"); got != tc.want {
				t.Fatalf("trend=%s want=%s", got, tc.want)
			}
		})
	}
	db := setupExpertiseDB(t)
	tracker := NewExpertiseTracker(db)
	tracker.RecordToolUse("web_search", "task", 20, false)
	tracker.RecordTaskCompletion("research", "task", 30, .8)
	summary, err := tracker.GetExpertise("research")
	if err != nil || summary.SuccessCount != 1 || summary.FailureCount != 1 || summary.TotalDurMs != 50 {
		t.Fatalf("summary: %+v %v", summary, err)
	}
	db.Close()
	if _, err := tracker.GetExpertise("research"); err == nil {
		t.Fatal("closed database accepted")
	}
	if _, err := tracker.ListExpertise(); err == nil {
		t.Fatal("closed database accepted")
	}
	tracker.RecordToolUse("exec", "task", 1, true)
	tracker.RecordTaskCompletion("shell", "task", 1, .9)
}
