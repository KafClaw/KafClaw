package scheduler

import (
	"testing"
	"time"
)

func TestParseCronValid(t *testing.T) {
	tests := []struct {
		expr string
	}{
		{"* * * * *"},
		{"*/5 * * * *"},
		{"0 0 * * *"},
		{"30 4 1,15 * *"},
		{"0 0 1 1 0"},
		{"0-30/5 9-17 * * 1-5"},
	}
	for _, tc := range tests {
		if _, err := ParseCron(tc.expr); err != nil {
			t.Errorf("ParseCron(%q) returned error: %v", tc.expr, err)
		}
	}
}

func TestParseCronInvalid(t *testing.T) {
	tests := []struct {
		expr string
	}{
		{""},
		{"* * *"},
		{"60 * * * *"},
		{"* 25 * * *"},
		{"* * 32 * *"},
		{"* * * 13 *"},
		{"* * * * 7"},
		{"*/0 * * * *"},
		{"abc * * * *"},
	}
	for _, tc := range tests {
		if _, err := ParseCron(tc.expr); err == nil {
			t.Errorf("ParseCron(%q) should have returned error", tc.expr)
		}
	}
}

func TestMatchesEveryMinute(t *testing.T) {
	c, _ := ParseCron("* * * * *")
	now := time.Date(2026, 2, 15, 10, 30, 0, 0, time.UTC)
	if !c.Matches(now) {
		t.Error("* * * * * should match any time")
	}
}

func TestMatchesEvery5Minutes(t *testing.T) {
	c, _ := ParseCron("*/5 * * * *")

	match := time.Date(2026, 2, 15, 10, 15, 0, 0, time.UTC)
	if !c.Matches(match) {
		t.Error("*/5 should match minute 15")
	}

	noMatch := time.Date(2026, 2, 15, 10, 13, 0, 0, time.UTC)
	if c.Matches(noMatch) {
		t.Error("*/5 should not match minute 13")
	}
}

func TestMatchesRange(t *testing.T) {
	c, _ := ParseCron("0-30/5 9-17 * * 1-5")

	// Monday 10:15 → should match
	match := time.Date(2026, 2, 16, 10, 15, 0, 0, time.UTC) // Monday
	if !c.Matches(match) {
		t.Errorf("should match Monday 10:15, weekday=%d", match.Weekday())
	}

	// Saturday 10:15 → should not match (weekday 6)
	noMatch := time.Date(2026, 2, 14, 10, 15, 0, 0, time.UTC) // Saturday
	if c.Matches(noMatch) {
		t.Errorf("should not match Saturday, weekday=%d", noMatch.Weekday())
	}
}

func TestMatchesSpecificValues(t *testing.T) {
	c, _ := ParseCron("30 4 1,15 * *")

	match := time.Date(2026, 3, 1, 4, 30, 0, 0, time.UTC)
	if !c.Matches(match) {
		t.Error("should match 4:30 on the 1st")
	}

	match2 := time.Date(2026, 3, 15, 4, 30, 0, 0, time.UTC)
	if !c.Matches(match2) {
		t.Error("should match 4:30 on the 15th")
	}

	noMatch := time.Date(2026, 3, 2, 4, 30, 0, 0, time.UTC)
	if c.Matches(noMatch) {
		t.Error("should not match 4:30 on the 2nd")
	}
}

func TestNextEveryMinute(t *testing.T) {
	c, _ := ParseCron("* * * * *")
	now := time.Date(2026, 2, 15, 10, 30, 45, 0, time.UTC)
	next := c.Next(now)
	expected := time.Date(2026, 2, 15, 10, 31, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("Next = %v, want %v", next, expected)
	}
}

func TestNextEvery5Minutes(t *testing.T) {
	c, _ := ParseCron("*/5 * * * *")
	now := time.Date(2026, 2, 15, 10, 12, 0, 0, time.UTC)
	next := c.Next(now)
	expected := time.Date(2026, 2, 15, 10, 15, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("Next = %v, want %v", next, expected)
	}
}

func TestNextMidnight(t *testing.T) {
	c, _ := ParseCron("0 0 * * *")
	now := time.Date(2026, 2, 15, 23, 59, 0, 0, time.UTC)
	next := c.Next(now)
	expected := time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("Next = %v, want %v", next, expected)
	}
}
