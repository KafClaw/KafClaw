// Package session provides conversation session management.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Message represents a chat message in a session.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// Session represents a conversation session.
type Session struct {
	Key       string         `json:"key"`
	Messages  []Message      `json:"messages"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	mu        sync.RWMutex
}

// NewSession creates a new session with the given key.
func NewSession(key string) *Session {
	now := time.Now()
	return &Session{
		Key:       key,
		Messages:  []Message{},
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  map[string]any{},
	}
}

// AddMessage adds a message to the session.
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = append(s.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	s.UpdatedAt = time.Now()
}

// GetHistory returns the recent message history.
func (s *Session) GetHistory(maxMessages int) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Messages) <= maxMessages {
		result := make([]Message, len(s.Messages))
		copy(result, s.Messages)
		return result
	}
	result := make([]Message, maxMessages)
	copy(result, s.Messages[len(s.Messages)-maxMessages:])
	return result
}

// Clear removes all messages from the session.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = []Message{}
	s.UpdatedAt = time.Now()
}

// GetMetadata returns a metadata value by key.
func (s *Session) GetMetadata(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return nil, false
	}
	val, ok := s.Metadata[key]
	return val, ok
}

// SetMetadata sets a metadata value by key.
func (s *Session) SetMetadata(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = map[string]any{}
	}
	s.Metadata[key] = value
	s.UpdatedAt = time.Now()
}

// DeleteMetadata deletes a metadata key.
func (s *Session) DeleteMetadata(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		return
	}
	delete(s.Metadata, key)
	s.UpdatedAt = time.Now()
}

// Manager manages session persistence.
type Manager struct {
	sessionsDir string
	cache       map[string]*Session
	mu          sync.RWMutex
}

// NewManager creates a new session manager.
func NewManager(workspace string) *Manager {
	home, _ := os.UserHomeDir()
	sessionsDir := filepath.Join(home, ".kafclaw", "sessions")
	os.MkdirAll(sessionsDir, 0755)

	return &Manager{
		sessionsDir: sessionsDir,
		cache:       make(map[string]*Session),
	}
}

// GetOrCreate returns an existing session or creates a new one.
func (m *Manager) GetOrCreate(key string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check cache
	if session, ok := m.cache[key]; ok {
		return session
	}

	// Try to load from disk
	session := m.load(key)
	if session == nil {
		session = NewSession(key)
	}

	m.cache[key] = session
	return session
}

// Save persists a session to disk.
func (m *Manager) Save(session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.sessionPath(session.Key)

	session.mu.RLock()
	defer session.mu.RUnlock()

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create session file: %w", err)
	}
	defer file.Close()

	// Write metadata as first line
	meta := map[string]any{
		"_type":      "metadata",
		"created_at": session.CreatedAt.Format(time.RFC3339),
		"updated_at": session.UpdatedAt.Format(time.RFC3339),
		"metadata":   session.Metadata,
	}
	metaLine, _ := json.Marshal(meta)
	file.WriteString(string(metaLine) + "\n")

	// Write messages as subsequent lines
	for _, msg := range session.Messages {
		msgLine, _ := json.Marshal(msg)
		file.WriteString(string(msgLine) + "\n")
	}

	m.cache[session.Key] = session
	return nil
}

// Delete removes a session.
func (m *Manager) Delete(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.cache, key)

	path := m.sessionPath(key)
	if err := os.Remove(path); err != nil {
		return false
	}
	return true
}

// SessionInfo contains metadata about a session.
type SessionInfo struct {
	Key       string
	CreatedAt time.Time
	UpdatedAt time.Time
	Path      string
}

// List returns information about all sessions.
func (m *Manager) List() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sessions []SessionInfo

	entries, err := os.ReadDir(m.sessionsDir)
	if err != nil {
		return sessions
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		path := filepath.Join(m.sessionsDir, entry.Name())
		key := strings.TrimSuffix(entry.Name(), ".jsonl")
		key = strings.ReplaceAll(key, "_", ":")

		// Read metadata from first line
		info := SessionInfo{
			Key:  key,
			Path: path,
		}

		if file, err := os.Open(path); err == nil {
			var firstLine []byte
			buf := make([]byte, 1)
			for {
				n, _ := file.Read(buf)
				if n == 0 || buf[0] == '\n' {
					break
				}
				firstLine = append(firstLine, buf[0])
			}
			file.Close()

			var meta map[string]any
			if json.Unmarshal(firstLine, &meta) == nil {
				if created, ok := meta["created_at"].(string); ok {
					info.CreatedAt, _ = time.Parse(time.RFC3339, created)
				}
				if updated, ok := meta["updated_at"].(string); ok {
					info.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
				}
			}
		}

		sessions = append(sessions, info)
	}

	return sessions
}

func (m *Manager) sessionPath(key string) string {
	safeKey := strings.ReplaceAll(key, ":", "_")
	// Strip path separators and traversal components to prevent path injection.
	safeKey = strings.ReplaceAll(safeKey, "/", "_")
	safeKey = strings.ReplaceAll(safeKey, "\\", "_")
	safeKey = strings.ReplaceAll(safeKey, "..", "_")
	return filepath.Join(m.sessionsDir, filepath.Base(safeKey)+".jsonl")
}

func (m *Manager) load(key string) *Session {
	path := m.sessionPath(key)

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	session := NewSession(key)
	decoder := json.NewDecoder(file)

	for decoder.More() {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			break
		}

		// Check if it's metadata
		var check map[string]any
		if json.Unmarshal(raw, &check) == nil {
			if check["_type"] == "metadata" {
				if created, ok := check["created_at"].(string); ok {
					session.CreatedAt, _ = time.Parse(time.RFC3339, created)
				}
				if updated, ok := check["updated_at"].(string); ok {
					session.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
				}
				if meta, ok := check["metadata"].(map[string]any); ok {
					session.Metadata = meta
				}
				continue
			}
		}

		// It's a message
		var msg Message
		if json.Unmarshal(raw, &msg) == nil {
			session.Messages = append(session.Messages, msg)
		}
	}

	return session
}
