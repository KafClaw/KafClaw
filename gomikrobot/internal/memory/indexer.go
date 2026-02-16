package memory

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// soulFiles are the bootstrap files that define the agent's identity.
var soulFiles = []string{
	"AGENTS.md",
	"SOUL.md",
	"USER.md",
	"TOOLS.md",
	"IDENTITY.md",
}

// SoulFileIndexer reads soul files from the workspace, chunks them by ##
// headers, and embeds+upserts each chunk into the memory system.
// Idempotent: deterministic chunk IDs mean re-indexing overwrites unchanged
// chunks without duplication.
type SoulFileIndexer struct {
	service   *MemoryService
	workspace string
}

// NewSoulFileIndexer creates a new SoulFileIndexer.
func NewSoulFileIndexer(service *MemoryService, workspace string) *SoulFileIndexer {
	return &SoulFileIndexer{service: service, workspace: workspace}
}

// IndexAll reads and indexes all soul files. Errors on individual files
// are logged but do not abort the overall indexing.
func (idx *SoulFileIndexer) IndexAll(ctx context.Context) error {
	wsPath := idx.workspace
	if strings.HasPrefix(wsPath, "~") {
		home, _ := os.UserHomeDir()
		wsPath = filepath.Join(home, wsPath[1:])
	}
	if abs, err := filepath.Abs(wsPath); err == nil {
		wsPath = abs
	}

	var indexed int
	for _, filename := range soulFiles {
		path := filepath.Join(wsPath, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				slog.Warn("Failed to read soul file", "file", filename, "error", err)
			}
			continue
		}

		content := string(data)
		source := fmt.Sprintf("soul:%s", filename)
		chunks := ChunkByHeaders(content, filename)

		for _, chunk := range chunks {
			if _, err := idx.service.Store(ctx, chunk.Body, source, chunk.Heading); err != nil {
				slog.Warn("Failed to index soul chunk", "file", filename, "heading", chunk.Heading, "error", err)
				continue
			}
			indexed++
		}
	}

	slog.Info("Soul file indexing complete", "chunks_indexed", indexed)
	return nil
}
