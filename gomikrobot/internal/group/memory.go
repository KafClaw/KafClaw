package group

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kamir/gomikrobot/internal/timeline"
)

// MemoryIndexer is an optional interface for indexing received group items
// into the local semantic memory. Avoids import cycle with memory package.
type MemoryIndexer interface {
	Store(ctx context.Context, content, source, tags string) (string, error)
}

// MemoryItem is a knowledge artifact stored in S3 via LFS and shared on the memory topic.
type MemoryItem struct {
	ItemID      string            `json:"item_id"`
	AuthorID    string            `json:"author_id"`
	Title       string            `json:"title"`
	ContentType string            `json:"content_type"` // "text/plain", "application/json", etc.
	Tags        []string          `json:"tags"`
	LFSEnvelope *LFSEnvelope      `json:"lfs_envelope"` // S3 pointer from LFS Proxy
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"` // nil = permanent
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ShareMemory produces content to LFS, wraps it in a MemoryItem, and publishes to memory.shared.
func (m *Manager) ShareMemory(ctx context.Context, title, contentType string, content []byte, tags []string) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}

	itemID := fmt.Sprintf("mem-%d", time.Now().UnixNano())

	// Produce content to LFS to get S3 pointer
	lfsEnv, err := m.lfs.Produce(ctx, m.extTopics.MemoryShared, itemID, content)
	if err != nil {
		return fmt.Errorf("share memory: LFS produce failed: %w", err)
	}

	item := MemoryItem{
		ItemID:      itemID,
		AuthorID:    m.identity.AgentID,
		Title:       title,
		ContentType: contentType,
		Tags:        tags,
		LFSEnvelope: lfsEnv,
		CreatedAt:   time.Now(),
	}

	env := &GroupEnvelope{
		Type:          EnvelopeMemory,
		CorrelationID: itemID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload:       item,
	}

	if err := m.lfs.ProduceEnvelope(ctx, m.extTopics.MemoryShared, env); err != nil {
		return fmt.Errorf("share memory: publish failed: %w", err)
	}

	// Store locally in timeline DB
	if m.timeline != nil {
		tagsJSON, _ := json.Marshal(tags)
		metaJSON, _ := json.Marshal(item.Metadata)
		_ = m.timeline.InsertGroupMemoryItem(&timeline.GroupMemoryItemRecord{
			ItemID:      itemID,
			AuthorID:    m.identity.AgentID,
			Title:       title,
			ContentType: contentType,
			Tags:        string(tagsJSON),
			LFSBucket:   lfsEnv.Bucket,
			LFSKey:      lfsEnv.Key,
			Metadata:    string(metaJSON),
		})
	}

	slog.Info("Memory shared", "item_id", itemID, "title", title)
	return nil
}

// ShareContext produces ephemeral content to the memory.context topic.
func (m *Manager) ShareContext(ctx context.Context, title string, content []byte, ttl time.Duration) error {
	if !m.Active() {
		return fmt.Errorf("not in a group")
	}

	itemID := fmt.Sprintf("ctx-%d", time.Now().UnixNano())
	expiresAt := time.Now().Add(ttl)

	item := MemoryItem{
		ItemID:      itemID,
		AuthorID:    m.identity.AgentID,
		Title:       title,
		ContentType: "text/plain",
		CreatedAt:   time.Now(),
		ExpiresAt:   &expiresAt,
	}

	env := &GroupEnvelope{
		Type:          EnvelopeMemory,
		CorrelationID: itemID,
		SenderID:      m.identity.AgentID,
		Timestamp:     time.Now(),
		Payload:       item,
	}

	if err := m.lfs.ProduceEnvelope(ctx, m.extTopics.MemoryContext, env); err != nil {
		return fmt.Errorf("share context: publish failed: %w", err)
	}

	slog.Info("Context shared", "item_id", itemID, "title", title, "ttl", ttl)
	return nil
}

// HandleMemoryItem processes an incoming memory item from the group.
func (m *Manager) HandleMemoryItem(env *GroupEnvelope) {
	data, err := json.Marshal(env.Payload)
	if err != nil {
		slog.Warn("HandleMemoryItem: marshal payload", "error", err)
		return
	}
	var item MemoryItem
	if err := json.Unmarshal(data, &item); err != nil {
		slog.Warn("HandleMemoryItem: unmarshal payload", "error", err)
		return
	}

	slog.Info("Memory item received",
		"item_id", item.ItemID,
		"author", item.AuthorID,
		"title", item.Title,
		"tags", item.Tags)

	// Store metadata locally
	if m.timeline != nil {
		tagsJSON, _ := json.Marshal(item.Tags)
		metaJSON, _ := json.Marshal(item.Metadata)
		bucket, key := "", ""
		if item.LFSEnvelope != nil {
			bucket = item.LFSEnvelope.Bucket
			key = item.LFSEnvelope.Key
		}
		_ = m.timeline.InsertGroupMemoryItem(&timeline.GroupMemoryItemRecord{
			ItemID:      item.ItemID,
			AuthorID:    item.AuthorID,
			Title:       item.Title,
			ContentType: item.ContentType,
			Tags:        string(tagsJSON),
			LFSBucket:   bucket,
			LFSKey:      key,
			Metadata:    string(metaJSON),
		})
	}

	// Index into local semantic memory for RAG retrieval
	if m.memoryIdx != nil && item.Title != "" {
		source := fmt.Sprintf("group:%s:%s", item.AuthorID, item.ItemID)
		tags := strings.Join(item.Tags, ",")
		if _, err := m.memoryIdx.Store(context.Background(), item.Title, source, tags); err != nil {
			slog.Debug("Failed to index group memory item", "item_id", item.ItemID, "error", err)
		}
	}
}

