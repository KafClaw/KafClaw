package agent

import (
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/memory"
	"github.com/KafClaw/KafClaw/internal/provider"
)

func TestMemoryLaneTopKAndMinScoreDefaults(t *testing.T) {
	var l *Loop
	if l.memoryLaneTopK() != defaultMemoryLaneTopK {
		t.Fatalf("expected default top-k %d", defaultMemoryLaneTopK)
	}
	if l.memoryMinScore() != defaultMemoryMinScore {
		t.Fatalf("expected default min score %f", defaultMemoryMinScore)
	}

	cfg := config.DefaultConfig()
	cfg.Memory.Search.MaxResults = 999
	cfg.Memory.Search.MinScore = -1
	l = &Loop{cfg: cfg}
	if l.memoryLaneTopK() != maxMemoryLaneTopK {
		t.Fatalf("expected clamped top-k %d, got %d", maxMemoryLaneTopK, l.memoryLaneTopK())
	}
	if l.memoryMinScore() != 0 {
		t.Fatalf("expected clamped min score 0, got %f", l.memoryMinScore())
	}

	cfg.Memory.Search.MaxResults = 3
	cfg.Memory.Search.MinScore = 2
	if l.memoryLaneTopK() != 3 {
		t.Fatalf("expected explicit top-k 3, got %d", l.memoryLaneTopK())
	}
	if l.memoryMinScore() != 1 {
		t.Fatalf("expected clamped min score 1, got %f", l.memoryMinScore())
	}

	cfg.Memory.Search.MinScore = 0.42
	if l.memoryMinScore() != float32(0.42) {
		t.Fatalf("expected in-range min score passthrough, got %f", l.memoryMinScore())
	}
}

func TestMemoryInjectionBudgetChars(t *testing.T) {
	l := &Loop{}
	if got := l.memoryInjectionBudgetChars(); got != defaultMemoryInjectionBudgetChars {
		t.Fatalf("expected default injection budget %d, got %d", defaultMemoryInjectionBudgetChars, got)
	}

	cfg := config.DefaultConfig()
	cfg.Memory.Search.MaxResults = 1
	l.cfg = cfg
	if got := l.memoryInjectionBudgetChars(); got != 1200 {
		t.Fatalf("expected floor budget 1200, got %d", got)
	}

	cfg.Memory.Search.MaxResults = 10
	if got := l.memoryInjectionBudgetChars(); got != defaultMemoryInjectionBudgetChars {
		t.Fatalf("expected capped budget %d, got %d", defaultMemoryInjectionBudgetChars, got)
	}
}

func TestAppendSectionWithBudget(t *testing.T) {
	msgs := []provider.Message{{Role: "system", Content: "base"}}
	updated, rem := appendSectionWithBudget(msgs, "1234567890", 5, 100)
	if rem != 95 {
		t.Fatalf("expected remaining 95, got %d", rem)
	}
	if updated[0].Content != "base12..." {
		t.Fatalf("unexpected content after cap: %q", updated[0].Content)
	}

	msgs2 := []provider.Message{{Role: "system", Content: "x"}}
	updated, rem = appendSectionWithBudget(msgs2, "abcdef", 10, 3)
	if rem != 0 {
		t.Fatalf("expected remaining 0, got %d", rem)
	}
	if updated[0].Content != "xabc" {
		t.Fatalf("unexpected content with tight budget: %q", updated[0].Content)
	}

	msgs3 := []provider.Message{{Role: "system", Content: "x"}}
	updated, rem = appendSectionWithBudget(msgs3, "", 5, 5)
	if updated[0].Content != "x" || rem != 5 {
		t.Fatalf("expected no-op on empty section, got %q rem=%d", updated[0].Content, rem)
	}

	updated, rem = appendSectionWithBudget(nil, "abc", 5, 5)
	if len(updated) != 0 || rem != 5 {
		t.Fatalf("expected no-op on empty messages, rem=%d", rem)
	}

	msgs4 := []provider.Message{{Role: "system", Content: "z"}}
	updated, rem = appendSectionWithBudget(msgs4, "abc", 0, 2)
	if updated[0].Content != "zab" || rem != 0 {
		t.Fatalf("expected cap fallback to budget, got %q rem=%d", updated[0].Content, rem)
	}

	msgs5 := []provider.Message{{Role: "system", Content: "z"}}
	updated, rem = appendSectionWithBudget(msgs5, "abc", 2, 0)
	if updated[0].Content != "z" || rem != 0 {
		t.Fatalf("expected no-op on exhausted budget, got %q rem=%d", updated[0].Content, rem)
	}
}

func TestTruncateWithEllipsis(t *testing.T) {
	if got := truncateWithEllipsis("abcdef", 0); got != "" {
		t.Fatalf("expected empty for zero max, got %q", got)
	}
	if got := truncateWithEllipsis("ab", 3); got != "ab" {
		t.Fatalf("expected unchanged short string, got %q", got)
	}
	if got := truncateWithEllipsis("abcdef", 2); got != "ab" {
		t.Fatalf("expected hard truncate without ellipsis, got %q", got)
	}
	if got := truncateWithEllipsis("abcdef", 4); got != "a..." {
		t.Fatalf("expected ellipsis truncate, got %q", got)
	}
}

func TestTrimTailObservations(t *testing.T) {
	in := []memory.Observation{
		{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"},
	}
	if got := trimTailObservations(in, 0); len(got) != 4 {
		t.Fatalf("expected no trim when max<=0")
	}
	got := trimTailObservations(in, 2)
	if len(got) != 2 || got[0].ID != "3" || got[1].ID != "4" {
		t.Fatalf("unexpected tail trim result: %+v", got)
	}
}
