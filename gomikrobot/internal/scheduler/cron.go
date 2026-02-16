// Package scheduler provides a minimal cron scheduler with file-lock overlap
// prevention and channel-based concurrency caps.
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpr represents a parsed 5-field cron expression.
// Fields: minute, hour, day-of-month, month, day-of-week.
type CronExpr struct {
	Minute     []int
	Hour       []int
	DayOfMonth []int
	Month      []int
	DayOfWeek  []int
}

// ParseCron parses a standard 5-field cron expression.
// Supports: *, */N, N, N-M, comma-separated values.
func ParseCron(expr string) (*CronExpr, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}

	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("cron: minute: %w", err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("cron: hour: %w", err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-month: %w", err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("cron: month: %w", err)
	}
	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("cron: day-of-week: %w", err)
	}

	return &CronExpr{
		Minute:     minute,
		Hour:       hour,
		DayOfMonth: dom,
		Month:      month,
		DayOfWeek:  dow,
	}, nil
}

// Matches returns true if t falls within the cron expression.
func (c *CronExpr) Matches(t time.Time) bool {
	return intIn(c.Minute, t.Minute()) &&
		intIn(c.Hour, t.Hour()) &&
		intIn(c.DayOfMonth, t.Day()) &&
		intIn(c.Month, int(t.Month())) &&
		intIn(c.DayOfWeek, int(t.Weekday()))
}

// Next returns the next time after t that matches the cron expression.
// Searches up to 2 years ahead; returns zero time if not found.
func (c *CronExpr) Next(t time.Time) time.Time {
	// Start from the next minute boundary.
	candidate := t.Truncate(time.Minute).Add(time.Minute)
	limit := t.Add(2 * 365 * 24 * time.Hour)

	for candidate.Before(limit) {
		if !intIn(c.Month, int(candidate.Month())) {
			// Jump to the 1st of the next month.
			candidate = time.Date(candidate.Year(), candidate.Month()+1, 1, 0, 0, 0, 0, candidate.Location())
			continue
		}
		if !intIn(c.DayOfMonth, candidate.Day()) || !intIn(c.DayOfWeek, int(candidate.Weekday())) {
			// Jump to the next day.
			candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day()+1, 0, 0, 0, 0, candidate.Location())
			continue
		}
		if !intIn(c.Hour, candidate.Hour()) {
			// Jump to the next hour.
			candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(), candidate.Hour()+1, 0, 0, 0, candidate.Location())
			continue
		}
		if !intIn(c.Minute, candidate.Minute()) {
			candidate = candidate.Add(time.Minute)
			continue
		}
		return candidate
	}
	return time.Time{}
}

// parseField parses a single cron field into a sorted list of integers.
func parseField(field string, min, max int) ([]int, error) {
	if field == "*" {
		return rangeSlice(min, max), nil
	}

	// Handle comma-separated values.
	parts := strings.Split(field, ",")
	seen := make(map[int]bool)
	for _, part := range parts {
		vals, err := parsePart(part, min, max)
		if err != nil {
			return nil, err
		}
		for _, v := range vals {
			seen[v] = true
		}
	}

	result := make([]int, 0, len(seen))
	for v := range seen {
		result = append(result, v)
	}
	sortInts(result)
	return result, nil
}

// parsePart parses a single part: *, */N, N, N-M, N-M/S.
func parsePart(part string, min, max int) ([]int, error) {
	// */N
	if strings.HasPrefix(part, "*/") {
		step, err := strconv.Atoi(part[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step %q", part)
		}
		return stepSlice(min, max, step), nil
	}

	// N-M or N-M/S
	if strings.Contains(part, "-") {
		rangeParts := strings.SplitN(part, "/", 2)
		bounds := strings.SplitN(rangeParts[0], "-", 2)
		if len(bounds) != 2 {
			return nil, fmt.Errorf("invalid range %q", part)
		}
		lo, err := strconv.Atoi(bounds[0])
		if err != nil {
			return nil, fmt.Errorf("invalid range start %q", bounds[0])
		}
		hi, err := strconv.Atoi(bounds[1])
		if err != nil {
			return nil, fmt.Errorf("invalid range end %q", bounds[1])
		}
		if lo < min || hi > max || lo > hi {
			return nil, fmt.Errorf("range %d-%d out of bounds [%d,%d]", lo, hi, min, max)
		}
		step := 1
		if len(rangeParts) == 2 {
			step, err = strconv.Atoi(rangeParts[1])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step in %q", part)
			}
		}
		return stepSlice(lo, hi, step), nil
	}

	// Single value
	val, err := strconv.Atoi(part)
	if err != nil {
		return nil, fmt.Errorf("invalid value %q", part)
	}
	if val < min || val > max {
		return nil, fmt.Errorf("value %d out of bounds [%d,%d]", val, min, max)
	}
	return []int{val}, nil
}

func rangeSlice(min, max int) []int {
	out := make([]int, 0, max-min+1)
	for i := min; i <= max; i++ {
		out = append(out, i)
	}
	return out
}

func stepSlice(min, max, step int) []int {
	out := make([]int, 0, (max-min)/step+1)
	for i := min; i <= max; i += step {
		out = append(out, i)
	}
	return out
}

func intIn(set []int, val int) bool {
	for _, v := range set {
		if v == val {
			return true
		}
	}
	return false
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
