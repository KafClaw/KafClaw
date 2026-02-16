package memory

import (
	"context"
	"testing"
	"time"
)

func TestShouldSkip(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"hi", true},
		{"Hello", true},
		{"ok", true},
		{"ja", true},
		{"danke", true},
		{`{"tool_call_id": "123"}`, true},
		{"error: something", true},
		{"This is a meaningful response about the project architecture.", false},
		{"Here are the tasks for Monday: ...", false},
		{"", false}, // empty won't even reach shouldSkip due to minLen check
	}

	for _, tt := range tests {
		got := shouldSkip(tt.content)
		if got != tt.want {
			t.Errorf("shouldSkip(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}

func TestEnqueueNil(t *testing.T) {
	// nil AutoIndexer should not panic
	var a *AutoIndexer
	a.Enqueue(IndexItem{Content: "test", Source: "test"})
}

func TestEnqueueMinLength(t *testing.T) {
	ai := NewAutoIndexer(nil, AutoIndexerConfig{MinLength: 50})
	// Service is nil, so Enqueue is a no-op anyway, but test that short content is filtered
	ai.Enqueue(IndexItem{Content: "short", Source: "test"})
	// No panic = pass
}

func TestFormatConversationPair(t *testing.T) {
	item := FormatConversationPair("What's the weather?", "It's sunny today.", "whatsapp", "chat123")
	if item.Source != "conversation:whatsapp" {
		t.Errorf("Source = %q, want conversation:whatsapp", item.Source)
	}
	if item.Tags != "chat123" {
		t.Errorf("Tags = %q, want chat123", item.Tags)
	}
	if item.Content != "Q: What's the weather?\nA: It's sunny today." {
		t.Errorf("Content = %q", item.Content)
	}
}

func TestFormatToolResult(t *testing.T) {
	item := FormatToolResult("read_file", map[string]any{"path": "/tmp/test.md"}, "file content here")
	if item.Source != "tool:read_file" {
		t.Errorf("Source = %q, want tool:read_file", item.Source)
	}
	if item.Tags != "/tmp/test.md" {
		t.Errorf("Tags = %q, want /tmp/test.md", item.Tags)
	}
}

func TestAutoIndexerRunAndStop(t *testing.T) {
	ai := NewAutoIndexer(nil, AutoIndexerConfig{
		FlushInterval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go ai.Run(ctx)

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	cancel()
	ai.Stop() // Should not hang
}

func TestTruncateContent(t *testing.T) {
	short := "hello"
	if got := truncateContent(short, 100); got != short {
		t.Errorf("truncateContent(%q, 100) = %q", short, got)
	}

	long := "abcdefghij"
	if got := truncateContent(long, 5); got != "abcde..." {
		t.Errorf("truncateContent(%q, 5) = %q", long, got)
	}
}
