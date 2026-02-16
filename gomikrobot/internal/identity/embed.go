// Package identity provides embedded soul-file templates and workspace scaffolding.
package identity

import "embed"

//go:embed templates/*.md
var templateFS embed.FS

// TemplateNames is the canonical ordered list of soul files.
// This is the single source of truth — agent/context.go and memory/indexer.go
// both import this slice instead of maintaining their own copies.
var TemplateNames = []string{
	"AGENTS.md",
	"SOUL.md",
	"USER.md",
	"TOOLS.md",
	"IDENTITY.md",
}

// Template returns the embedded content of a template file.
func Template(name string) ([]byte, error) {
	return templateFS.ReadFile("templates/" + name)
}
