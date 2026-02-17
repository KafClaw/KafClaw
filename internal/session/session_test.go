package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionMessageHistoryAndMetadata(t *testing.T) {
	s := NewSession("chat:1")
	s.AddMessage("user", "hello")
	s.AddMessage("assistant", "hi")

	history := s.GetHistory(1)
	if len(history) != 1 || history[0].Role != "assistant" {
		t.Fatalf("unexpected history: %+v", history)
	}
	all := s.GetHistory(10)
	if len(all) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(all))
	}

	s.SetMetadata("lang", "en")
	if v, ok := s.GetMetadata("lang"); !ok || v.(string) != "en" {
		t.Fatalf("expected metadata lang=en, got %v (ok=%v)", v, ok)
	}
	s.DeleteMetadata("lang")
	if _, ok := s.GetMetadata("lang"); ok {
		t.Fatal("expected metadata key deleted")
	}

	s.Clear()
	if len(s.GetHistory(10)) != 0 {
		t.Fatal("expected no messages after clear")
	}

	// Nil-metadata branches should be safe.
	s.Metadata = nil
	if _, ok := s.GetMetadata("none"); ok {
		t.Fatal("expected missing metadata with nil map")
	}
	s.SetMetadata("k", "v")
	if v, ok := s.GetMetadata("k"); !ok || v.(string) != "v" {
		t.Fatalf("expected metadata k=v, got %v (ok=%v)", v, ok)
	}
	s.Metadata = nil
	s.DeleteMetadata("k") // no-op branch
}

func TestManagerSaveLoadListDelete(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{sessionsDir: dir, cache: map[string]*Session{}}

	s := NewSession("wa:123")
	s.CreatedAt = time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	s.UpdatedAt = s.CreatedAt
	s.SetMetadata("foo", "bar")
	s.AddMessage("user", "ping")
	if err := m.Save(s); err != nil {
		t.Fatalf("save session: %v", err)
	}

	cached := m.GetOrCreate("wa:123")
	if cached == nil || len(cached.Messages) != 1 {
		t.Fatalf("expected cached session with one message, got %+v", cached)
	}

	m2 := &Manager{sessionsDir: dir, cache: map[string]*Session{}}
	loaded := m2.GetOrCreate("wa:123")
	if loaded == nil {
		t.Fatal("expected loaded session")
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "ping" {
		t.Fatalf("unexpected loaded messages: %+v", loaded.Messages)
	}
	if v, ok := loaded.GetMetadata("foo"); !ok || v.(string) != "bar" {
		t.Fatalf("expected loaded metadata foo=bar, got %v (ok=%v)", v, ok)
	}

	infos := m2.List()
	if len(infos) != 1 {
		t.Fatalf("expected one listed session, got %d", len(infos))
	}
	if infos[0].Key != "wa:123" {
		t.Fatalf("unexpected listed key: %s", infos[0].Key)
	}

	if ok := m2.Delete("wa:123"); !ok {
		t.Fatal("expected delete success")
	}
	if ok := m2.Delete("wa:123"); ok {
		t.Fatal("expected second delete to return false")
	}
}

func TestManagerLoadMissingOrInvalid(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{sessionsDir: dir, cache: map[string]*Session{}}

	missing := m.GetOrCreate("missing:key")
	if missing == nil || missing.Key != "missing:key" {
		t.Fatalf("expected newly created session, got %+v", missing)
	}

	badPath := filepath.Join(dir, "bad_key.jsonl")
	if err := os.WriteFile(badPath, []byte("{not json}\n"), 0644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}
	loaded := m.load("bad:key")
	if loaded == nil {
		t.Fatal("expected non-nil session even for invalid file")
	}
}

func TestNewManagerUsesHomeSessionsDir(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	_ = os.Setenv("HOME", tmpHome)

	m := NewManager("ignored")
	if m == nil {
		t.Fatal("expected manager")
	}
	expected := filepath.Join(tmpHome, ".kafclaw", "sessions")
	if m.sessionsDir != expected {
		t.Fatalf("unexpected sessions dir: got %q want %q", m.sessionsDir, expected)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected sessions dir created: %v", err)
	}
}

func TestManagerSaveAndListErrorPaths(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	m := &Manager{sessionsDir: blocker, cache: map[string]*Session{}}

	if err := m.Save(NewSession("bad:key")); err == nil {
		t.Fatal("expected save error when sessionsDir is not a directory")
	}

	infos := m.List()
	if len(infos) != 0 {
		t.Fatalf("expected empty list when readdir fails, got %d", len(infos))
	}
}

func TestManagerListSkipsNonJSONLFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatalf("write non-jsonl file: %v", err)
	}
	m := &Manager{sessionsDir: dir, cache: map[string]*Session{}}
	infos := m.List()
	if len(infos) != 0 {
		t.Fatalf("expected non-jsonl files to be ignored, got %d entries", len(infos))
	}
}
