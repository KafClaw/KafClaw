package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// IndexItem represents a piece of content to be indexed into semantic memory.
type IndexItem struct {
	Content string
	Source  string // e.g. "conversation:whatsapp", "tool:read_file"
	Tags    string
}

// AutoIndexerConfig holds configuration for the AutoIndexer.
type AutoIndexerConfig struct {
	MinLength     int           // skip content shorter than this (default: 100)
	BatchSize     int           // flush after N items (default: 5)
	FlushInterval time.Duration // flush on timer (default: 30s)
	QueueSize     int           // channel buffer size (default: 100)
}

// AutoIndexer runs a background goroutine that batches and indexes content
// into the memory system. It provides a non-blocking Enqueue method.
type AutoIndexer struct {
	service  *MemoryService
	config   AutoIndexerConfig
	queue    chan IndexItem
	stopOnce sync.Once
	done     chan struct{}
}

// NewAutoIndexer creates a new AutoIndexer. If service is nil, Enqueue is a no-op.
func NewAutoIndexer(service *MemoryService, cfg AutoIndexerConfig) *AutoIndexer {
	if cfg.MinLength <= 0 {
		cfg.MinLength = 100
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 5
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 30 * time.Second
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 100
	}

	return &AutoIndexer{
		service: service,
		config:  cfg,
		queue:   make(chan IndexItem, cfg.QueueSize),
		done:    make(chan struct{}),
	}
}

// Enqueue adds an item to the indexing queue. Non-blocking; drops items if
// the queue is full or the service is nil.
func (a *AutoIndexer) Enqueue(item IndexItem) {
	if a == nil || a.service == nil {
		return
	}
	if len(item.Content) < a.config.MinLength {
		return
	}
	if shouldSkip(item.Content) {
		return
	}

	select {
	case a.queue <- item:
	default:
		slog.Debug("AutoIndexer queue full, dropping item", "source", item.Source)
	}
}

// Run starts the background indexing loop. Blocks until ctx is cancelled.
func (a *AutoIndexer) Run(ctx context.Context) {
	if a == nil || a.service == nil {
		close(a.done)
		return
	}

	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()
	defer close(a.done)

	var batch []IndexItem

	flush := func() {
		if len(batch) == 0 {
			return
		}
		items := batch
		batch = nil
		a.indexBatch(ctx, items)
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case item := <-a.queue:
			batch = append(batch, item)
			if len(batch) >= a.config.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// Stop waits for the background goroutine to finish (after ctx cancel).
func (a *AutoIndexer) Stop() {
	if a == nil {
		return
	}
	<-a.done
}

func (a *AutoIndexer) indexBatch(ctx context.Context, items []IndexItem) {
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		id, err := a.service.Store(ctx, item.Content, item.Source, item.Tags)
		if err != nil {
			slog.Warn("AutoIndexer store failed", "source", item.Source, "error", err)
			continue
		}
		slog.Debug("AutoIndexer indexed", "id", id, "source", item.Source, "len", len(item.Content))
	}
}

// shouldSkip returns true for content that is not worth indexing.
func shouldSkip(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))

	// Skip pure greetings
	greetings := []string{"hi", "hello", "hey", "hallo", "moin", "servus", "ok", "yes", "no", "ja", "nein", "danke", "thanks"}
	for _, g := range greetings {
		if lower == g {
			return true
		}
	}

	// Skip if it looks like raw JSON tool call output
	if strings.HasPrefix(lower, "{\"tool_call") || strings.HasPrefix(lower, "[{\"id\"") {
		return true
	}

	// Skip error-only responses
	if strings.HasPrefix(lower, "error:") && len(lower) < 200 {
		return true
	}

	return false
}

// FormatConversationPair formats a user message + agent response for indexing.
func FormatConversationPair(userMsg, agentResponse, channel, chatID string) IndexItem {
	return IndexItem{
		Content: fmt.Sprintf("Q: %s\nA: %s", userMsg, agentResponse),
		Source:  "conversation:" + channel,
		Tags:    chatID,
	}
}

// FormatToolResult formats a tool execution result for indexing.
func FormatToolResult(toolName string, args map[string]any, result string) IndexItem {
	// Extract file path from arguments if available
	filePath, _ := args["path"].(string)
	if filePath == "" {
		filePath, _ = args["file_path"].(string)
	}

	content := result
	if filePath != "" {
		content = fmt.Sprintf("File: %s\n%s", filePath, truncateContent(result, 500))
	} else {
		content = truncateContent(result, 500)
	}

	return IndexItem{
		Content: content,
		Source:  "tool:" + toolName,
		Tags:    filePath,
	}
}

func truncateContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
