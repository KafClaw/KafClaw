package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

const (
	// ConfigDir is the default config directory name.
	ConfigDir = ".gomikrobot"
	// ConfigFile is the default config file name.
	ConfigFile = "config.json"
)

// ConfigPath returns the path to the config file.
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ConfigDir, ConfigFile), nil
}

// legacyConfig mirrors the old config layout so we can detect and migrate it.
type legacyConfig struct {
	Agents *struct {
		Defaults *struct {
			Workspace         string  `json:"workspace"`
			WorkRepoPath      string  `json:"workRepoPath"`
			SystemRepoPath    string  `json:"systemRepoPath"`
			Model             string  `json:"model"`
			MaxTokens         int     `json:"maxTokens"`
			Temperature       float64 `json:"temperature"`
			MaxToolIterations int     `json:"maxToolIterations"`
		} `json:"defaults"`
	} `json:"agents"`
}

// migrateIfNeeded detects the old "agents.defaults" layout and copies values
// into the new Paths/Model groups. Returns true if a migration happened.
func migrateIfNeeded(data []byte, cfg *Config) bool {
	var old legacyConfig
	if err := json.Unmarshal(data, &old); err != nil {
		return false
	}
	if old.Agents == nil || old.Agents.Defaults == nil {
		return false
	}
	d := old.Agents.Defaults

	// Only migrate if the new fields are still at their defaults (i.e. the
	// file didn't also contain the new keys).
	defaults := DefaultConfig()

	if cfg.Paths.Workspace == defaults.Paths.Workspace && d.Workspace != "" {
		cfg.Paths.Workspace = d.Workspace
	}
	if cfg.Paths.WorkRepoPath == defaults.Paths.WorkRepoPath && d.WorkRepoPath != "" {
		cfg.Paths.WorkRepoPath = d.WorkRepoPath
	}
	if cfg.Paths.SystemRepoPath == defaults.Paths.SystemRepoPath && d.SystemRepoPath != "" {
		cfg.Paths.SystemRepoPath = d.SystemRepoPath
	}
	if cfg.Model.Name == defaults.Model.Name && d.Model != "" {
		cfg.Model.Name = d.Model
	}
	if cfg.Model.MaxTokens == defaults.Model.MaxTokens && d.MaxTokens != 0 {
		cfg.Model.MaxTokens = d.MaxTokens
	}
	if cfg.Model.Temperature == defaults.Model.Temperature && d.Temperature != 0 {
		cfg.Model.Temperature = d.Temperature
	}
	if cfg.Model.MaxToolIterations == defaults.Model.MaxToolIterations && d.MaxToolIterations != 0 {
		cfg.Model.MaxToolIterations = d.MaxToolIterations
	}
	return true
}

// Load loads the configuration from file and environment variables.
// Priority: environment > file > defaults.
// Backward-compatible: if the file still uses the old "agents.defaults"
// layout the values are migrated into the new Paths/Model groups and the
// file is rewritten in the new format.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Load from file
	path, err := ConfigPath()
	if err != nil {
		return cfg, nil // Use defaults if we can't find config path
	}

	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
		// Migrate old "agents.defaults" → new "paths" + "model" groups.
		if migrateIfNeeded(data, cfg) {
			// Persist the migrated config so the old layout is replaced.
			if err := Save(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "config: auto-migration save failed: %v\n", err)
			}
		}
	}
	// If file doesn't exist, continue with defaults

	// Override with environment variables for each group
	envconfig.Process("MIKROBOT_PATHS", &cfg.Paths)
	envconfig.Process("MIKROBOT_MODEL", &cfg.Model)
	envconfig.Process("MIKROBOT_OPENAI", &cfg.Providers.OpenAI)
	envconfig.Process("MIKROBOT_CHANNELS_TELEGRAM", &cfg.Channels.Telegram)
	envconfig.Process("MIKROBOT_CHANNELS_DISCORD", &cfg.Channels.Discord)
	envconfig.Process("MIKROBOT_CHANNELS_WHATSAPP", &cfg.Channels.WhatsApp)
	envconfig.Process("MIKROBOT_CHANNELS_FEISHU", &cfg.Channels.Feishu)
	envconfig.Process("MIKROBOT_GATEWAY", &cfg.Gateway)
	envconfig.Process("MIKROBOT_TOOLS_EXEC", &cfg.Tools.Exec)
	envconfig.Process("MIKROBOT_TOOLS_WEB_SEARCH", &cfg.Tools.Web.Search)
	envconfig.Process("MIKROBOT_GROUP", &cfg.Group)
	envconfig.Process("MIKROBOT_ORCHESTRATOR", &cfg.Orchestrator)
	envconfig.Process("MIKROBOT_SCHEDULER", &cfg.Scheduler)

	// Legacy env var compatibility
	envconfig.Process("MIKROBOT_AGENTS", &cfg.Paths)
	envconfig.Process("MIKROBOT_AGENTS", &cfg.Model)

	// Fallback for API Key
	if cfg.Providers.OpenAI.APIKey == "" {
		if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			cfg.Providers.OpenAI.APIKey = key
		} else if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
			cfg.Providers.OpenAI.APIKey = key
		}
	}

	// Expand ~ in paths
	expandHome := func(p *string) {
		if strings.HasPrefix(*p, "~") {
			if home, err := os.UserHomeDir(); err == nil {
				*p = filepath.Join(home, (*p)[1:])
			}
		}
	}
	expandHome(&cfg.Paths.Workspace)
	expandHome(&cfg.Paths.WorkRepoPath)
	expandHome(&cfg.Paths.SystemRepoPath)

	// Force workspace to GoMikroBot-Workspace (state lives here, independent of work repo)
	if home, err := os.UserHomeDir(); err == nil {
		cfg.Paths.Workspace = filepath.Join(home, "GoMikroBot-Workspace")
	}

	return cfg, nil
}

// Save writes the configuration to the config file.
func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// EnsureDir ensures a directory exists with proper permissions.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
