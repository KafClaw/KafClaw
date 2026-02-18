package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateBundledArtifacts checks that all bundled skills and docs are present.
func ValidateBundledArtifacts(repoRoot string) error {
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		var err error
		root, err = findRepoRootFromWD()
		if err != nil {
			return err
		}
	}
	var missing []string
	for _, s := range BundledCatalog {
		skillPath := filepath.Join(root, "skills", s.Name, "SKILL.md")
		if fi, err := os.Stat(skillPath); err != nil || fi.IsDir() {
			missing = append(missing, skillPath)
		}
		docPath := filepath.Join(root, "docs", "skills", s.Name+".md")
		if fi, err := os.Stat(docPath); err != nil || fi.IsDir() {
			missing = append(missing, docPath)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing bundled skill artifacts:\n- %s", strings.Join(missing, "\n- "))
	}
	return nil
}

func findRepoRootFromWD() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cur := wd
	for {
		if fi, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil && !fi.IsDir() {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("repo root not found from %s", wd)
		}
		cur = parent
	}
}
