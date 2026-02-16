package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamir/gomikrobot/internal/session"
	"github.com/kamir/gomikrobot/internal/tools"
)

func TestContextBuilder(t *testing.T) {
	// Setup temp workspace
	tmpDir := t.TempDir()

	// Create identity file
	os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("Bootstrap Content"), 0644)

	// Create memory
	os.Mkdir(filepath.Join(tmpDir, "memory"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "memory", "MEMORY.md"), []byte("Test Memory"), 0644)

	// Create registry
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool())

	builder := NewContextBuilder(tmpDir, "", "", registry)
	systemPrompt := builder.BuildSystemPrompt()

	// Verify content
	if !strings.Contains(systemPrompt, "GoMikroBot") {
		t.Error("System prompt missing identity")
	}
	if !strings.Contains(systemPrompt, "Bootstrap Content") {
		t.Error("System prompt missing bootstrap content")
	}
	if !strings.Contains(systemPrompt, "Test Memory") {
		t.Error("System prompt missing memory")
	}
	if !strings.Contains(systemPrompt, "read_file") {
		t.Error("System prompt missing tools summary")
	}
}

func TestBuildMessages(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewContextBuilder(tmpDir, "", "", tools.NewRegistry())
	sess := session.NewSession("test:123")

	// Session has one previous message
	sess.AddMessage("user", "Previous msg")

	// We are processing a new message "Current msg"
	// But note: Loop.ProcessDirect ADDS the current message to the session before calling BuildMessages
	// So let's simulate that
	sess.AddMessage("user", "Current msg")

	msgs := builder.BuildMessages(sess, "Current msg", "cli", "default", "")

	// Expect:
	// 1. System
	// 2. User (Previous msg)
	// 3. User (Current msg)

	if len(msgs) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(msgs))
	}

	if msgs[0].Role != "system" {
		t.Error("First message should be system")
	}
	if msgs[1].Content != "Previous msg" {
		t.Errorf("Second message content mismatch: %s", msgs[1].Content)
	}
	if msgs[2].Content != "Current msg" {
		t.Errorf("Third message content mismatch: %s", msgs[2].Content)
	}
}

func TestBuildMessagesInternalContext(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewContextBuilder(tmpDir, "", "", tools.NewRegistry())
	sess := session.NewSession("test:int")
	sess.AddMessage("user", "hello")

	msgs := builder.BuildMessages(sess, "hello", "whatsapp", "owner@s.whatsapp.net", "internal")

	if len(msgs) == 0 {
		t.Fatal("Expected messages")
	}
	system := msgs[0].Content
	if !strings.Contains(system, "INTERNAL message from the bot owner") {
		t.Error("System prompt should contain internal request context")
	}
	if strings.Contains(system, "EXTERNAL request") {
		t.Error("System prompt should not contain external request context")
	}
}

func TestBuildMessagesExternalContext(t *testing.T) {
	tmpDir := t.TempDir()
	builder := NewContextBuilder(tmpDir, "", "", tools.NewRegistry())
	sess := session.NewSession("test:ext")
	sess.AddMessage("user", "hello")

	msgs := builder.BuildMessages(sess, "hello", "whatsapp", "user@s.whatsapp.net", "external")

	if len(msgs) == 0 {
		t.Fatal("Expected messages")
	}
	system := msgs[0].Content
	if !strings.Contains(system, "EXTERNAL request from an authorized user") {
		t.Error("System prompt should contain external request context")
	}
	if strings.Contains(system, "INTERNAL message") {
		t.Error("System prompt should not contain internal request context")
	}
}
