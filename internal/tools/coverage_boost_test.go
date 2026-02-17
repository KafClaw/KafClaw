package tools

import (
	"context"
	"strings"
	"testing"
)

type fakeTieredTool struct{}

func (f *fakeTieredTool) Name() string                                            { return "fake_tiered" }
func (f *fakeTieredTool) Description() string                                     { return "fake" }
func (f *fakeTieredTool) Parameters() map[string]any                              { return map[string]any{"type": "object"} }
func (f *fakeTieredTool) Execute(context.Context, map[string]any) (string, error) { return "ok", nil }
func (f *fakeTieredTool) Tier() int                                               { return TierHighRisk }

type fakeUntieredTool struct{}

func (f *fakeUntieredTool) Name() string                                            { return "fake_untiered" }
func (f *fakeUntieredTool) Description() string                                     { return "fake" }
func (f *fakeUntieredTool) Parameters() map[string]any                              { return map[string]any{"type": "object"} }
func (f *fakeUntieredTool) Execute(context.Context, map[string]any) (string, error) { return "ok", nil }

func TestToolTierAndDefaults(t *testing.T) {
	if got := ToolTier(&fakeTieredTool{}); got != TierHighRisk {
		t.Fatalf("expected tier high risk, got %d", got)
	}
	if got := ToolTier(&fakeUntieredTool{}); got != TierReadOnly {
		t.Fatalf("expected default tier read-only, got %d", got)
	}

	names := DefaultToolNames()
	want := []string{"read_file", "write_file", "edit_file", "list_dir", "resolve_path", "exec"}
	if len(names) != len(want) {
		t.Fatalf("unexpected default tools len: %d", len(names))
	}
	for _, n := range want {
		found := false
		for _, got := range names {
			if got == n {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing default tool %s", n)
		}
	}
}

func TestRegistryExecute_NotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute(context.Background(), "missing", nil)
	if err == nil || !strings.Contains(err.Error(), "tool not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestFilesystemToolMetadataAndResolvePath(t *testing.T) {
	repo := t.TempDir()

	read := NewReadFileTool()
	if read.Name() != "read_file" || read.Tier() != TierReadOnly || read.Description() == "" {
		t.Fatalf("unexpected read tool metadata")
	}
	if read.Parameters()["type"] != "object" {
		t.Fatalf("unexpected read schema")
	}

	write := NewWriteFileTool(func() string { return repo })
	if write.Name() != "write_file" || write.Tier() != TierWrite || write.Description() == "" {
		t.Fatalf("unexpected write tool metadata")
	}
	if write.Parameters()["type"] != "object" {
		t.Fatalf("unexpected write schema")
	}

	edit := NewEditFileTool(func() string { return repo })
	if edit.Name() != "edit_file" || edit.Tier() != TierWrite || edit.Description() == "" {
		t.Fatalf("unexpected edit tool metadata")
	}
	if edit.Parameters()["type"] != "object" {
		t.Fatalf("unexpected edit schema")
	}

	list := NewListDirTool()
	if list.Name() != "list_dir" || list.Tier() != TierReadOnly || list.Description() == "" {
		t.Fatalf("unexpected list tool metadata")
	}
	if list.Parameters()["type"] != "object" {
		t.Fatalf("unexpected list schema")
	}

	resolve := NewResolvePathTool(func() string { return repo })
	if resolve.Name() != "resolve_path" || resolve.Tier() != TierReadOnly || resolve.Description() == "" {
		t.Fatalf("unexpected resolve tool metadata")
	}
	if resolve.Parameters()["type"] != "object" {
		t.Fatalf("unexpected resolve schema")
	}

	path, err := resolve.Execute(context.Background(), map[string]any{
		"kind":     "docs",
		"filename": "task.md",
	})
	if err != nil {
		t.Fatalf("resolve execute err: %v", err)
	}
	if !strings.HasSuffix(path, "/docs/task.md") {
		t.Fatalf("unexpected resolved path: %s", path)
	}

	none := NewResolvePathTool(func() string { return "" })
	out, err := none.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("resolve nil repo err: %v", err)
	}
	if !strings.Contains(out, "not configured") {
		t.Fatalf("expected not configured error, got %s", out)
	}
}

func TestExecToolMetadata(t *testing.T) {
	tool := NewExecTool(0, false, "", nil)
	if tool.Name() != "exec" || tool.Tier() != TierHighRisk || tool.Description() == "" {
		t.Fatalf("unexpected exec metadata")
	}
	if tool.Parameters()["type"] != "object" {
		t.Fatalf("unexpected exec schema")
	}
}

func TestPathHelpers(t *testing.T) {
	repo := t.TempDir()
	if !isWithin(repo, repo) {
		t.Fatal("expected repo to be within itself")
	}
	if isWithin(repo, "/tmp") {
		t.Fatal("expected /tmp to be outside repo")
	}

	if normalizeRoot("") != "" {
		t.Fatal("expected empty normalize root")
	}
	if normalizeRoot(repo) == "" {
		t.Fatal("expected normalized root to be non-empty")
	}
}

func TestFilesystemConstructorsWithNilGetter(t *testing.T) {
	write := NewWriteFileTool(nil)
	if write.workRepoRoot() != "" {
		t.Fatal("expected empty repo root for nil write getter")
	}
	edit := NewEditFileTool(nil)
	if edit.workRepoRoot() != "" {
		t.Fatal("expected empty repo root for nil edit getter")
	}
	resolve := NewResolvePathTool(nil)
	if resolve.workRepoRoot() != "" {
		t.Fatal("expected empty repo root for nil resolve getter")
	}
}

func TestMemoryToolMetadataAndValidation(t *testing.T) {
	remember := NewRememberTool(nil)
	if remember.Name() != "remember" || remember.Description() == "" || remember.Tier() != TierWrite {
		t.Fatalf("unexpected remember metadata")
	}
	if remember.Parameters()["type"] != "object" {
		t.Fatalf("unexpected remember schema")
	}
	out, err := remember.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("remember execute err: %v", err)
	}
	if !strings.Contains(out, "content is required") {
		t.Fatalf("expected content validation, got %s", out)
	}

	recall := NewRecallTool(nil)
	if recall.Name() != "recall" || recall.Description() == "" || recall.Tier() != TierReadOnly {
		t.Fatalf("unexpected recall metadata")
	}
	if recall.Parameters()["type"] != "object" {
		t.Fatalf("unexpected recall schema")
	}
	out, err = recall.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("recall execute err: %v", err)
	}
	if !strings.Contains(out, "query is required") {
		t.Fatalf("expected query validation, got %s", out)
	}
}

func TestExecToolDefaultWorkDir(t *testing.T) {
	tool := NewExecTool(0, false, "/tmp/fallback", func() string { return "" })
	if tool.defaultWorkDir() != "/tmp/fallback" {
		t.Fatalf("expected fallback dir, got %s", tool.defaultWorkDir())
	}
	tool = NewExecTool(0, false, "/tmp/fallback", func() string { return "/tmp/repo" })
	if tool.defaultWorkDir() != "/tmp/repo" {
		t.Fatalf("expected repo dir, got %s", tool.defaultWorkDir())
	}
}
