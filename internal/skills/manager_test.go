package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureNVMRCWritesWhenMissing(t *testing.T) {
	repo := t.TempDir()
	path, err := EnsureNVMRC(repo, "22")
	if err != nil {
		t.Fatalf("EnsureNVMRC failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read nvmrc: %v", err)
	}
	if strings.TrimSpace(string(data)) != "22" {
		t.Fatalf("expected node major 22, got %q", string(data))
	}
}

func TestEnsureClawhubWithFakeBinary(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	origPath := os.Getenv("PATH")
	defer os.Setenv("PATH", origPath)

	if err := os.WriteFile(filepath.Join(bin, "clawhub"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write clawhub: %v", err)
	}
	_ = os.Setenv("PATH", bin+string(os.PathListSeparator)+origPath)
	if err := EnsureClawhub(false); err != nil {
		t.Fatalf("EnsureClawhub should succeed with fake binary: %v", err)
	}
}
