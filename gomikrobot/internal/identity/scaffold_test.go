package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScaffoldCreatesAllFiles(t *testing.T) {
	dir := t.TempDir()

	result, err := ScaffoldWorkspace(dir, false)
	if err != nil {
		t.Fatalf("ScaffoldWorkspace failed: %v", err)
	}

	if len(result.Errors) > 0 {
		t.Fatalf("Unexpected errors: %v", result.Errors)
	}

	if len(result.Created) != len(TemplateNames) {
		t.Errorf("Expected %d created, got %d", len(TemplateNames), len(result.Created))
	}

	for _, name := range TemplateNames {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Expected file %s to exist: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("File %s is empty", name)
		}
	}
}

func TestScaffoldSkipsExistingFiles(t *testing.T) {
	dir := t.TempDir()

	// Pre-create SOUL.md with custom content
	customContent := []byte("# My Custom Soul")
	os.WriteFile(filepath.Join(dir, "SOUL.md"), customContent, 0644)

	result, err := ScaffoldWorkspace(dir, false)
	if err != nil {
		t.Fatalf("ScaffoldWorkspace failed: %v", err)
	}

	// SOUL.md should be skipped
	found := false
	for _, name := range result.Skipped {
		if name == "SOUL.md" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected SOUL.md to be skipped")
	}

	// Verify custom content was preserved
	data, _ := os.ReadFile(filepath.Join(dir, "SOUL.md"))
	if string(data) != string(customContent) {
		t.Error("Custom SOUL.md content was overwritten")
	}

	// Other files should be created
	if len(result.Created) != len(TemplateNames)-1 {
		t.Errorf("Expected %d created, got %d", len(TemplateNames)-1, len(result.Created))
	}
}

func TestScaffoldForceOverwrites(t *testing.T) {
	dir := t.TempDir()

	// Pre-create SOUL.md with custom content
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("custom"), 0644)

	result, err := ScaffoldWorkspace(dir, true)
	if err != nil {
		t.Fatalf("ScaffoldWorkspace failed: %v", err)
	}

	if len(result.Skipped) != 0 {
		t.Errorf("Expected 0 skipped with force, got %d", len(result.Skipped))
	}

	if len(result.Created) != len(TemplateNames) {
		t.Errorf("Expected %d created, got %d", len(TemplateNames), len(result.Created))
	}

	// Verify SOUL.md was overwritten with template content
	data, _ := os.ReadFile(filepath.Join(dir, "SOUL.md"))
	if string(data) == "custom" {
		t.Error("SOUL.md was not overwritten with force=true")
	}
}

func TestTemplateReturnsContent(t *testing.T) {
	for _, name := range TemplateNames {
		data, err := Template(name)
		if err != nil {
			t.Errorf("Template(%q) failed: %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("Template(%q) returned empty content", name)
		}
	}
}
