package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	// Test register and get
	tool := NewReadFileTool()
	r.Register(tool)

	got, ok := r.Get("read_file")
	if !ok {
		t.Error("expected to find read_file tool")
	}
	if got.Name() != "read_file" {
		t.Errorf("expected name 'read_file', got '%s'", got.Name())
	}

	// Test not found
	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent tool")
	}

	// Test list
	tools := r.List()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(tools))
	}

	// Test definitions
	defs := r.Definitions()
	if len(defs) != 1 {
		t.Errorf("expected 1 definition, got %d", len(defs))
	}
}

func TestReadFileTool(t *testing.T) {
	tool := NewReadFileTool()

	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(tmpFile, []byte("Hello, World!"), 0644)

	// Test successful read
	result, err := tool.Execute(context.Background(), map[string]any{"path": tmpFile})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got '%s'", result)
	}

	// Test file not found
	result, _ = tool.Execute(context.Background(), map[string]any{"path": "/nonexistent/file.txt"})
	if !strings.Contains(result, "Error") {
		t.Error("expected error for nonexistent file")
	}

	// Test missing path
	result, _ = tool.Execute(context.Background(), map[string]any{})
	if !strings.Contains(result, "Error") {
		t.Error("expected error for missing path")
	}
}

func TestWriteFileTool(t *testing.T) {
	tool := NewWriteFileTool(func() string { return "" })
	tmpDir := t.TempDir()

	// Test write new file
	newFile := filepath.Join(tmpDir, "subdir", "new.txt")
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    newFile,
		"content": "New content",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "Successfully wrote") {
		t.Errorf("expected success message, got '%s'", result)
	}

	// Verify file was created
	content, _ := os.ReadFile(newFile)
	if string(content) != "New content" {
		t.Errorf("expected 'New content', got '%s'", string(content))
	}
}

func TestEditFileTool(t *testing.T) {
	tool := NewEditFileTool(func() string { return "" })
	tmpDir := t.TempDir()

	// Create file to edit
	testFile := filepath.Join(tmpDir, "edit.txt")
	os.WriteFile(testFile, []byte("Hello, World!"), 0644)

	// Test edit
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":     testFile,
		"old_text": "World",
		"new_text": "Go",
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if !strings.Contains(result, "Successfully edited") {
		t.Errorf("expected success message, got '%s'", result)
	}

	// Verify edit
	content, _ := os.ReadFile(testFile)
	if string(content) != "Hello, Go!" {
		t.Errorf("expected 'Hello, Go!', got '%s'", string(content))
	}

	// Test text not found
	result, _ = tool.Execute(context.Background(), map[string]any{
		"path":     testFile,
		"old_text": "nonexistent",
		"new_text": "replacement",
	})
	if !strings.Contains(result, "text not found") {
		t.Errorf("expected 'text not found' error, got '%s'", result)
	}
}

func TestListDirTool(t *testing.T) {
	tool := NewListDirTool()
	tmpDir := t.TempDir()

	// Create some files and dirs
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("more"), 0644)
	os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755)

	result, err := tool.Execute(context.Background(), map[string]any{"path": tmpDir})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if !strings.Contains(result, "file1.txt") {
		t.Error("expected to find file1.txt in output")
	}
	if !strings.Contains(result, "[DIR]") && !strings.Contains(result, "subdir") {
		t.Error("expected to find subdir in output")
	}
}

func TestGetHelpers(t *testing.T) {
	params := map[string]any{
		"str":   "hello",
		"int":   42,
		"float": 3.14,
		"bool":  true,
	}

	if GetString(params, "str", "") != "hello" {
		t.Error("GetString failed")
	}
	if GetString(params, "missing", "default") != "default" {
		t.Error("GetString default failed")
	}

	if GetInt(params, "int", 0) != 42 {
		t.Error("GetInt failed for int")
	}
	if GetInt(params, "float", 0) != 3 {
		t.Error("GetInt failed for float")
	}
	if GetInt(params, "missing", 99) != 99 {
		t.Error("GetInt default failed")
	}

	if GetBool(params, "bool", false) != true {
		t.Error("GetBool failed")
	}
	if GetBool(params, "missing", true) != true {
		t.Error("GetBool default failed")
	}
}
