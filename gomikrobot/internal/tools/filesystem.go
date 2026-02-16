package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadFileTool reads the contents of a file.
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Tier() int    { return TierReadOnly }

func (t *ReadFileTool) Description() string {
	return "Read the contents of a file at the specified path."
}

func (t *ReadFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The path to the file to read",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	path := GetString(params, "path", "")
	if path == "" {
		return "Error: path is required", nil
	}

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file not found: %s", path), nil
		}
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: permission denied: %s", path), nil
		}
		return fmt.Sprintf("Error reading file: %v", err), nil
	}

	return string(content), nil
}

// WriteFileTool writes content to a file.
type WriteFileTool struct {
	workRepoRoot func() string
}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Tier() int    { return TierWrite }

func (t *WriteFileTool) Description() string {
	return "Write content to a file at the specified path. Creates parent directories if needed. Writes are restricted to the work repo."
}

func (t *WriteFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The path to the file to write",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	path := GetString(params, "path", "")
	content := GetString(params, "content", "")

	if path == "" {
		return "Error: path is required", nil
	}

	path = expandPath(path)
	root := ""
	if t.workRepoRoot != nil {
		root = t.workRepoRoot()
	}
	if root != "" && !isWithin(root, path) {
		return "Error: path outside work repo.", nil
	}

	// Create parent directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("Error creating directory: %v", err), nil
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: permission denied: %s", path), nil
		}
		return fmt.Sprintf("Error writing file: %v", err), nil
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// EditFileTool replaces text in a file.
type EditFileTool struct {
	workRepoRoot func() string
}

func (t *EditFileTool) Name() string { return "edit_file" }
func (t *EditFileTool) Tier() int    { return TierWrite }

func (t *EditFileTool) Description() string {
	return "Edit a file by replacing text. Useful for making targeted changes. Edits are restricted to the work repo."
}

func (t *EditFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The path to the file to edit",
			},
			"old_text": map[string]any{
				"type":        "string",
				"description": "The text to find and replace",
			},
			"new_text": map[string]any{
				"type":        "string",
				"description": "The replacement text",
			},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
}

func (t *EditFileTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	path := GetString(params, "path", "")
	oldText := GetString(params, "old_text", "")
	newText := GetString(params, "new_text", "")

	if path == "" {
		return "Error: path is required", nil
	}
	if oldText == "" {
		return "Error: old_text is required", nil
	}

	path = expandPath(path)
	root := ""
	if t.workRepoRoot != nil {
		root = t.workRepoRoot()
	}
	if root != "" && !isWithin(root, path) {
		return "Error: path outside work repo.", nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file not found: %s", path), nil
		}
		return fmt.Sprintf("Error reading file: %v", err), nil
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, oldText) {
		return fmt.Sprintf("Error: text not found in file: %s", path), nil
	}

	newContent := strings.Replace(contentStr, oldText, newText, 1)

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Sprintf("Error writing file: %v", err), nil
	}

	return fmt.Sprintf("Successfully edited %s", path), nil
}

// ListDirTool lists directory contents.
type ListDirTool struct{}

func (t *ListDirTool) Name() string { return "list_dir" }
func (t *ListDirTool) Tier() int    { return TierReadOnly }

func (t *ListDirTool) Description() string {
	return "List the contents of a directory."
}

func (t *ListDirTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The directory path to list",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	path := GetString(params, "path", ".")
	path = expandPath(path)

	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: directory not found: %s", path), nil
		}
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: permission denied: %s", path), nil
		}
		return fmt.Sprintf("Error reading directory: %v", err), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Contents of %s:\n", path))

	for _, entry := range entries {
		info, _ := entry.Info()
		if entry.IsDir() {
			result.WriteString(fmt.Sprintf("  [DIR]  %s/\n", entry.Name()))
		} else if info != nil {
			result.WriteString(fmt.Sprintf("  [FILE] %s (%d bytes)\n", entry.Name(), info.Size()))
		} else {
			result.WriteString(fmt.Sprintf("  [FILE] %s\n", entry.Name()))
		}
	}

	return result.String(), nil
}

// NewReadFileTool creates a new ReadFileTool.
func NewReadFileTool() *ReadFileTool { return &ReadFileTool{} }

// NewWriteFileTool creates a new WriteFileTool.
func NewWriteFileTool(workRepoGetter func() string) *WriteFileTool {
	if workRepoGetter == nil {
		workRepoGetter = func() string { return "" }
	}
	return &WriteFileTool{workRepoRoot: func() string { return normalizeRoot(workRepoGetter()) }}
}

// NewEditFileTool creates a new EditFileTool.
func NewEditFileTool(workRepoGetter func() string) *EditFileTool {
	if workRepoGetter == nil {
		workRepoGetter = func() string { return "" }
	}
	return &EditFileTool{workRepoRoot: func() string { return normalizeRoot(workRepoGetter()) }}
}

// NewListDirTool creates a new ListDirTool.
func NewListDirTool() *ListDirTool { return &ListDirTool{} }

// ResolvePathTool resolves a default path inside the work repo.
type ResolvePathTool struct {
	workRepoRoot func() string
}

func (t *ResolvePathTool) Name() string { return "resolve_path" }
func (t *ResolvePathTool) Tier() int    { return TierReadOnly }

func (t *ResolvePathTool) Description() string {
	return "Resolve a path inside the work repo for requirements/tasks/docs. Provide kind and filename."
}

func (t *ResolvePathTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind": map[string]any{
				"type":        "string",
				"description": "One of: requirements, tasks, docs",
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "Filename to use (default: artifact.md)",
			},
		},
	}
}

func (t *ResolvePathTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	kind := GetString(params, "kind", "docs")
	filename := GetString(params, "filename", "artifact.md")
	root := ""
	if t.workRepoRoot != nil {
		root = t.workRepoRoot()
	}
	root = normalizeRoot(root)
	if root == "" {
		return "Error: work repo path not configured", nil
	}
	path := filepath.Join(root, kind, filename)
	return path, nil
}

// NewResolvePathTool creates a new ResolvePathTool.
func NewResolvePathTool(workRepoGetter func() string) *ResolvePathTool {
	if workRepoGetter == nil {
		workRepoGetter = func() string { return "" }
	}
	return &ResolvePathTool{workRepoRoot: func() string { return normalizeRoot(workRepoGetter()) }}
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return path
}

func normalizeRoot(root string) string {
	if root == "" {
		return ""
	}
	return expandPath(root)
}

func isWithin(root, path string) bool {
	if root == "" {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}
