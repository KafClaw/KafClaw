package clinst

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCLI(t *testing.T) {
	for name, ensure := range map[string]func() error{"gemini": EnsureGeminiCLI, "codex": EnsureCodexCLI} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("PATH", root)
			if err := ensure(); err == nil || !strings.Contains(err.Error(), "install") {
				t.Fatalf("missing npm: %v", err)
			}
			npm := filepath.Join(root, "npm")
			if err := os.WriteFile(npm, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := ensure(); err == nil || !strings.Contains(err.Error(), "not found in PATH") {
				t.Fatalf("missing installed CLI: %v", err)
			}
			script := "#!/bin/sh\n[ \"$1\" = install ] && [ \"$2\" = -g ] || exit 2\nprintf '#!/bin/sh\\nexit 0\\n' > '" + filepath.Join(root, name) + "'\n/bin/chmod 700 '" + filepath.Join(root, name) + "'\n"
			if err := os.WriteFile(npm, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			if err := ensure(); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(npm); err != nil {
				t.Fatal(err)
			}
			if err := ensure(); err != nil {
				t.Fatalf("installed CLI should bypass npm: %v", err)
			}
		})
	}
}
