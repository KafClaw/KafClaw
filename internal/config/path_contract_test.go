package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPathPrecedenceAndHomeExpansion(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("KAFCLAW_HOME", "")
	t.Setenv("MIKROBOT_HOME", "")
	t.Setenv("KAFCLAW_CONFIG", "")
	t.Setenv("MIKROBOT_CONFIG", "")
	for _, tc := range []struct {
		current, legacy string
		want            string
	}{{"~/current.json", "~/legacy.json", filepath.Join(root, "current.json")}, {"", "~/legacy.json", filepath.Join(root, "legacy.json")}, {"", "/legacy.json", "/legacy.json"}} {
		t.Setenv("KAFCLAW_CONFIG", tc.current)
		t.Setenv("MIKROBOT_CONFIG", tc.legacy)
		got, err := ConfigPath()
		if err != nil || got != tc.want {
			t.Fatalf("config precedence: %q %v want %q", got, err, tc.want)
		}
	}
	t.Setenv("KAFCLAW_CONFIG", "")
	t.Setenv("MIKROBOT_CONFIG", "")
	for _, tc := range []struct{ current, legacy, want string }{{"~/current", "~/legacy", filepath.Join(root, "current")}, {"", "~/legacy", filepath.Join(root, "legacy")}, {"", "/legacy", "/legacy"}, {"", "", root}} {
		t.Setenv("KAFCLAW_HOME", tc.current)
		t.Setenv("MIKROBOT_HOME", tc.legacy)
		got, err := resolveHomeDir()
		if err != nil || got != tc.want {
			t.Fatalf("home precedence: %q %v want %q", got, err, tc.want)
		}
	}
	t.Setenv("HOME", "")
	t.Setenv("MIKROBOT_HOME", "")
	for _, tc := range []struct{ home, current, legacy string }{{"~/current", "", ""}, {"", "~/current.json", ""}, {"", "", "~/legacy.json"}, {"", "", ""}} {
		t.Setenv("KAFCLAW_HOME", tc.home)
		t.Setenv("KAFCLAW_CONFIG", tc.current)
		t.Setenv("MIKROBOT_CONFIG", tc.legacy)
		if _, err := ConfigPath(); err == nil {
			t.Fatal("missing home accepted for relative configuration")
		}
	}
	t.Setenv("KAFCLAW_HOME", "")
	t.Setenv("MIKROBOT_HOME", "~/legacy")
	if _, err := resolveHomeDir(); err == nil {
		t.Fatal("missing home accepted for legacy expansion")
	}
	if err := Save(DefaultConfig()); err == nil {
		t.Fatal("save without resolvable home accepted")
	}
}

func TestConfigSaveFilesystemFailures(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAFCLAW_CONFIG", filepath.Join(blocker, "config.json"))
	if err := Save(DefaultConfig()); err == nil {
		t.Fatal("non-directory config parent accepted")
	}
	t.Setenv("KAFCLAW_CONFIG", root)
	if err := Save(DefaultConfig()); err == nil {
		t.Fatal("directory config destination accepted")
	}
	if _, err := EnsureWorkRepo(blocker); err == nil {
		t.Fatal("file work repo accepted")
	}
	if _, err := loadConfigObject(root, map[string]struct{}{}); err == nil {
		t.Fatal("directory config source accepted")
	}
}
