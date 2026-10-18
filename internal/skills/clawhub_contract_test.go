package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureClawhubInstallationFailuresAndDiscovery(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if err := EnsureClawhub(false); err == nil || !strings.Contains(err.Error(), "clawhub not found") {
		t.Fatalf("missing clawhub: %v", err)
	}
	if err := EnsureClawhub(true); err == nil || !strings.Contains(err.Error(), "npm not found") {
		t.Fatalf("missing npm: %v", err)
	}
	npm := filepath.Join(dir, "npm")
	for _, tc := range []struct{ script, want string }{
		{"#!/bin/sh\nexit 7\n", "failed to install"},
		{"#!/bin/sh\nexit 0\n", "not discoverable"},
		{"#!/bin/sh\nprintf '#!/bin/sh\\nexit 0\\n' > '" + filepath.Join(dir, "clawhub") + "'\n/bin/chmod +x '" + filepath.Join(dir, "clawhub") + "'\n", ""},
	} {
		if err := os.WriteFile(npm, []byte(tc.script), 0700); err != nil {
			t.Fatal(err)
		}
		err := EnsureClawhub(true)
		if tc.want == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("install: %v want %s", err, tc.want)
		}
	}
}
