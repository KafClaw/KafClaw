package identity

import (
	"fmt"
	"os"
	"path/filepath"
)

// ScaffoldResult reports which files were created, skipped, or errored.
type ScaffoldResult struct {
	Created []string
	Skipped []string
	Errors  []string
}

// ScaffoldWorkspace writes each soul-file template into the workspace directory.
// If force is false, existing files are skipped. If force is true, they are overwritten.
func ScaffoldWorkspace(path string, force bool) (*ScaffoldResult, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}

	result := &ScaffoldResult{}

	for _, name := range TemplateNames {
		dst := filepath.Join(path, name)

		if !force {
			if _, err := os.Stat(dst); err == nil {
				result.Skipped = append(result.Skipped, name)
				continue
			}
		}

		data, err := Template(name)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}

		if err := os.WriteFile(dst, data, 0644); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}

		result.Created = append(result.Created, name)
	}

	return result, nil
}
