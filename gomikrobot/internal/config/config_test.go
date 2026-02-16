package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Model.Name != "anthropic/claude-sonnet-4-5" {
		t.Errorf("expected default model anthropic/claude-sonnet-4-5, got %s", cfg.Model.Name)
	}

	if cfg.Gateway.Host != "127.0.0.1" {
		t.Errorf("expected gateway host 127.0.0.1, got %s", cfg.Gateway.Host)
	}

	if cfg.Gateway.Port != 18790 {
		t.Errorf("expected gateway port 18790, got %d", cfg.Gateway.Port)
	}

	if !cfg.Tools.Exec.RestrictToWorkspace {
		t.Error("expected RestrictToWorkspace to be true by default")
	}

	if cfg.Tools.Exec.Timeout != 60*time.Second {
		t.Errorf("expected exec timeout 60s, got %v", cfg.Tools.Exec.Timeout)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Temporarily set HOME to a non-existent directory
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "/tmp/nonexistent-nanobot-test")
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Model.MaxTokens != 8192 {
		t.Errorf("expected maxTokens 8192, got %d", cfg.Model.MaxTokens)
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create temp config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".gomikrobot")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	configJSON := `{
		"model": {
			"name": "openai/gpt-4",
			"maxTokens": 4096
		},
		"gateway": {
			"port": 9999
		}
	}`
	os.WriteFile(configFile, []byte(configJSON), 0600)

	// Temporarily set HOME
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Model.Name != "openai/gpt-4" {
		t.Errorf("expected model openai/gpt-4, got %s", cfg.Model.Name)
	}

	if cfg.Gateway.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Gateway.Port)
	}
}

func TestLegacyConfigMigration(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".gomikrobot")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	// Old-format config with "agents.defaults"
	legacyJSON := `{
		"agents": {
			"defaults": {
				"workspace": "/custom/workspace",
				"workRepoPath": "/custom/work-repo",
				"systemRepoPath": "/custom/system-repo",
				"model": "gpt-4o",
				"maxTokens": 4096,
				"temperature": 0.5,
				"maxToolIterations": 10
			}
		},
		"gateway": {
			"port": 18790
		}
	}`
	os.WriteFile(configFile, []byte(legacyJSON), 0600)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Model fields should be migrated from agents.defaults
	if cfg.Model.Name != "gpt-4o" {
		t.Errorf("expected model gpt-4o after migration, got %s", cfg.Model.Name)
	}
	if cfg.Model.MaxTokens != 4096 {
		t.Errorf("expected maxTokens 4096 after migration, got %d", cfg.Model.MaxTokens)
	}
	if cfg.Model.Temperature != 0.5 {
		t.Errorf("expected temperature 0.5 after migration, got %f", cfg.Model.Temperature)
	}
	if cfg.Model.MaxToolIterations != 10 {
		t.Errorf("expected maxToolIterations 10 after migration, got %d", cfg.Model.MaxToolIterations)
	}

	// Path fields should be migrated (workspace gets overridden by the force logic)
	if cfg.Paths.SystemRepoPath != "/custom/system-repo" {
		t.Errorf("expected systemRepoPath /custom/system-repo after migration, got %s", cfg.Paths.SystemRepoPath)
	}
	if cfg.Paths.WorkRepoPath != "/custom/work-repo" {
		t.Errorf("expected workRepoPath /custom/work-repo after migration, got %s", cfg.Paths.WorkRepoPath)
	}

	// Verify the file was rewritten in new format (no more "agents" key)
	rewritten, _ := os.ReadFile(configFile)
	if strings.Contains(string(rewritten), `"agents"`) {
		t.Error("expected migrated config to not contain old 'agents' key")
	}
	if !strings.Contains(string(rewritten), `"paths"`) {
		t.Error("expected migrated config to contain new 'paths' key")
	}
	if !strings.Contains(string(rewritten), `"model"`) {
		t.Error("expected migrated config to contain new 'model' key")
	}
}

func TestEnvOverride(t *testing.T) {
	// Set env var with correct prefix for nested struct
	os.Setenv("MIKROBOT_GATEWAY_HOST", "0.0.0.0")
	os.Setenv("MIKROBOT_GATEWAY_PORT", "8080")
	defer func() {
		os.Unsetenv("MIKROBOT_GATEWAY_HOST")
		os.Unsetenv("MIKROBOT_GATEWAY_PORT")
	}()

	// Use temp home with no config file
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Gateway.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0 from env, got %s", cfg.Gateway.Host)
	}

	if cfg.Gateway.Port != 8080 {
		t.Errorf("expected port 8080 from env, got %d", cfg.Gateway.Port)
	}
}
