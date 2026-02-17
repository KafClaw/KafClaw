package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

type subagentFieldPresence struct {
	MaxConcurrent       bool
	MaxSpawnDepth       bool
	MaxChildrenPerAgent bool
	ArchiveAfterMinutes bool
	AllowAgents         bool
	Model               bool
	Thinking            bool
	ToolsAllow          bool
	ToolsDeny           bool
}

const (
	// ConfigDir is the default config directory name.
	ConfigDir = ".kafclaw"
	// ConfigFile is the default config file name.
	ConfigFile = "config.json"
)

// ConfigPath returns the path to the config file.
func ConfigPath() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("KAFCLAW_CONFIG")); explicit != "" {
		if strings.HasPrefix(explicit, "~") {
			home, err := resolveHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, explicit[1:]), nil
		}
		return explicit, nil
	}
	if explicit := strings.TrimSpace(os.Getenv("MIKROBOT_CONFIG")); explicit != "" {
		if strings.HasPrefix(explicit, "~") {
			home, err := resolveHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, explicit[1:]), nil
		}
		return explicit, nil
	}
	home, err := resolveHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ConfigDir, ConfigFile), nil
}

func resolveHomeDir() (string, error) {
	if h := strings.TrimSpace(os.Getenv("KAFCLAW_HOME")); h != "" {
		if strings.HasPrefix(h, "~") {
			base, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(base, h[1:]), nil
		}
		return h, nil
	}
	if h := strings.TrimSpace(os.Getenv("MIKROBOT_HOME")); h != "" {
		if strings.HasPrefix(h, "~") {
			base, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(base, h[1:]), nil
		}
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home, nil
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
	toolsPresence := subagentFieldPresence{}

	// Load process env vars from ~/.config/kafclaw/env (and fallbacks) first.
	LoadEnvFileCandidates()

	// Load from file
	path, err := ConfigPath()
	if err != nil {
		return cfg, nil // Use defaults if we can't find config path
	}

	data, err := loadResolvedConfig(path)
	if err == nil {
		toolsPresence, _ = detectSubagentFieldPresence(data)
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
		// Migrate old "agents.defaults" → new "paths" + "model" groups.
		if migrateIfNeeded(data, cfg) {
			cleanEmptyAgents(cfg)
			// Persist the migrated config so the old layout is replaced.
			if err := Save(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "config: auto-migration save failed: %v\n", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
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
	envconfig.Process("MIKROBOT_TOOLS_SUBAGENTS", &cfg.Tools.Subagents)
	legacyAgentDefaults := SubagentsToolConfig{}
	if cfg.Agents != nil {
		legacyAgentDefaults = cfg.Agents.Defaults.Subagents
	}
	envconfig.Process("MIKROBOT_AGENTS_DEFAULTS_SUBAGENTS", &legacyAgentDefaults)
	if !isZeroSubagentsToolConfig(legacyAgentDefaults) {
		if cfg.Agents == nil {
			cfg.Agents = &AgentsConfig{}
		}
		cfg.Agents.Defaults.Subagents = legacyAgentDefaults
	}
	envconfig.Process("MIKROBOT_GROUP", &cfg.Group)
	envconfig.Process("MIKROBOT_ORCHESTRATOR", &cfg.Orchestrator)
	envconfig.Process("MIKROBOT_SCHEDULER", &cfg.Scheduler)
	envconfig.Process("MIKROBOT", &cfg.ER1)
	envconfig.Process("MIKROBOT", &cfg.Observer)
	envconfig.Process("KAFCLAW_PATHS", &cfg.Paths)
	envconfig.Process("KAFCLAW_MODEL", &cfg.Model)
	envconfig.Process("KAFCLAW_OPENAI", &cfg.Providers.OpenAI)
	envconfig.Process("KAFCLAW_CHANNELS_TELEGRAM", &cfg.Channels.Telegram)
	envconfig.Process("KAFCLAW_CHANNELS_DISCORD", &cfg.Channels.Discord)
	envconfig.Process("KAFCLAW_CHANNELS_WHATSAPP", &cfg.Channels.WhatsApp)
	envconfig.Process("KAFCLAW_CHANNELS_FEISHU", &cfg.Channels.Feishu)
	envconfig.Process("KAFCLAW_GATEWAY", &cfg.Gateway)
	envconfig.Process("KAFCLAW_TOOLS_EXEC", &cfg.Tools.Exec)
	envconfig.Process("KAFCLAW_TOOLS_WEB_SEARCH", &cfg.Tools.Web.Search)
	envconfig.Process("KAFCLAW_TOOLS_SUBAGENTS", &cfg.Tools.Subagents)
	agentDefaults := SubagentsToolConfig{}
	if cfg.Agents != nil {
		agentDefaults = cfg.Agents.Defaults.Subagents
	}
	envconfig.Process("KAFCLAW_AGENTS_DEFAULTS_SUBAGENTS", &agentDefaults)
	if !isZeroSubagentsToolConfig(agentDefaults) {
		if cfg.Agents == nil {
			cfg.Agents = &AgentsConfig{}
		}
		cfg.Agents.Defaults.Subagents = agentDefaults
	}
	envconfig.Process("KAFCLAW_GROUP", &cfg.Group)
	envconfig.Process("KAFCLAW_ORCHESTRATOR", &cfg.Orchestrator)
	envconfig.Process("KAFCLAW_SCHEDULER", &cfg.Scheduler)
	envconfig.Process("KAFCLAW", &cfg.ER1)
	envconfig.Process("KAFCLAW", &cfg.Observer)

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

	mergeAgentsSubagentDefaults(cfg, toolsPresence)

	if cfg.Tools.Subagents.MaxSpawnDepth <= 0 {
		cfg.Tools.Subagents.MaxSpawnDepth = 1
	}
	if cfg.Tools.Subagents.MaxChildrenPerAgent <= 0 {
		cfg.Tools.Subagents.MaxChildrenPerAgent = 5
	}
	if cfg.Tools.Subagents.MaxConcurrent <= 0 {
		cfg.Tools.Subagents.MaxConcurrent = 8
	}
	if cfg.Tools.Subagents.ArchiveAfterMinutes <= 0 {
		cfg.Tools.Subagents.ArchiveAfterMinutes = 60
	}
	if cfg.Agents != nil {
		cfg.Agents.Defaults.Subagents = cfg.Tools.Subagents
	}
	cleanEmptyAgents(cfg)

	// Resolve workspace path with backward compatibility:
	// 1. Use ~/KafClaw-Workspace if it exists
	// 2. Fall back to ~/KafClaw-Workspace if it exists (legacy)
	// 3. Default to ~/KafClaw-Workspace for new installs
	if home, err := os.UserHomeDir(); err == nil {
		newWS := filepath.Join(home, "KafClaw-Workspace")
		oldWS := filepath.Join(home, "KafClaw-Workspace")

		if _, err := os.Stat(newWS); err == nil {
			cfg.Paths.Workspace = newWS
		} else if _, err := os.Stat(oldWS); err == nil {
			cfg.Paths.Workspace = oldWS
			fmt.Fprintf(os.Stderr, "config: using legacy workspace %s (rename to %s to suppress this warning)\n", oldWS, newWS)
		} else {
			cfg.Paths.Workspace = newWS
		}
	}

	return cfg, nil
}

func mergeAgentsSubagentDefaults(cfg *Config, toolsPresence subagentFieldPresence) {
	if cfg == nil || cfg.Agents == nil {
		return
	}
	src := cfg.Agents.Defaults.Subagents
	dst := &cfg.Tools.Subagents
	def := DefaultConfig().Tools.Subagents
	if !toolsPresence.MaxConcurrent && dst.MaxConcurrent == def.MaxConcurrent && src.MaxConcurrent > 0 {
		dst.MaxConcurrent = src.MaxConcurrent
	}
	if !toolsPresence.MaxSpawnDepth && dst.MaxSpawnDepth == def.MaxSpawnDepth && src.MaxSpawnDepth > 0 {
		dst.MaxSpawnDepth = src.MaxSpawnDepth
	}
	if !toolsPresence.MaxChildrenPerAgent && dst.MaxChildrenPerAgent == def.MaxChildrenPerAgent && src.MaxChildrenPerAgent > 0 {
		dst.MaxChildrenPerAgent = src.MaxChildrenPerAgent
	}
	if !toolsPresence.ArchiveAfterMinutes && dst.ArchiveAfterMinutes == def.ArchiveAfterMinutes && src.ArchiveAfterMinutes > 0 {
		dst.ArchiveAfterMinutes = src.ArchiveAfterMinutes
	}
	if !toolsPresence.Model && strings.TrimSpace(dst.Model) == "" && strings.TrimSpace(src.Model) != "" {
		dst.Model = src.Model
	}
	if !toolsPresence.Thinking && strings.TrimSpace(dst.Thinking) == "" && strings.TrimSpace(src.Thinking) != "" {
		dst.Thinking = src.Thinking
	}
	if !toolsPresence.AllowAgents && len(dst.AllowAgents) == 0 && len(src.AllowAgents) > 0 {
		dst.AllowAgents = append([]string{}, src.AllowAgents...)
	}
	if !toolsPresence.ToolsAllow && len(dst.Tools.Allow) == 0 && len(src.Tools.Allow) > 0 {
		dst.Tools.Allow = append([]string{}, src.Tools.Allow...)
	}
	if !toolsPresence.ToolsDeny && len(dst.Tools.Deny) == 0 && len(src.Tools.Deny) > 0 {
		dst.Tools.Deny = append([]string{}, src.Tools.Deny...)
	}
}

func isZeroSubagentsToolConfig(c SubagentsToolConfig) bool {
	return c.MaxConcurrent == 0 &&
		c.MaxSpawnDepth == 0 &&
		c.MaxChildrenPerAgent == 0 &&
		c.ArchiveAfterMinutes == 0 &&
		strings.TrimSpace(c.Model) == "" &&
		strings.TrimSpace(c.Thinking) == "" &&
		len(c.AllowAgents) == 0 &&
		len(c.Tools.Allow) == 0 &&
		len(c.Tools.Deny) == 0
}

func cleanEmptyAgents(cfg *Config) {
	if cfg == nil || cfg.Agents == nil {
		return
	}
	if isZeroSubagentsToolConfig(cfg.Agents.Defaults.Subagents) {
		cfg.Agents = nil
	}
}

func detectSubagentFieldPresence(data []byte) (subagentFieldPresence, subagentFieldPresence) {
	var outTools subagentFieldPresence
	var outAgents subagentFieldPresence
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return outTools, outAgents
	}
	outTools = readSubagentPresence(nestedMap(root, "tools", "subagents"))
	outAgents = readSubagentPresence(nestedMap(root, "agents", "defaults", "subagents"))
	return outTools, outAgents
}

func readSubagentPresence(node map[string]any) subagentFieldPresence {
	p := subagentFieldPresence{}
	if node == nil {
		return p
	}
	_, p.MaxConcurrent = node["maxConcurrent"]
	_, p.MaxSpawnDepth = node["maxSpawnDepth"]
	_, p.MaxChildrenPerAgent = node["maxChildrenPerAgent"]
	_, p.ArchiveAfterMinutes = node["archiveAfterMinutes"]
	_, p.AllowAgents = node["allowAgents"]
	_, p.Model = node["model"]
	_, p.Thinking = node["thinking"]
	toolsNode, _ := node["tools"].(map[string]any)
	if toolsNode != nil {
		_, p.ToolsAllow = toolsNode["allow"]
		_, p.ToolsDeny = toolsNode["deny"]
	}
	return p
}

func nestedMap(root map[string]any, keys ...string) map[string]any {
	cur := root
	for _, k := range keys {
		next, _ := cur[k].(map[string]any)
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
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

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func loadResolvedConfig(path string) ([]byte, error) {
	obj, err := loadConfigObject(path, map[string]struct{}{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

func loadConfigObject(path string, visited map[string]struct{}) (map[string]any, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, seen := visited[absPath]; seen {
		return nil, fmt.Errorf("config include cycle detected at %s", absPath)
	}
	visited[absPath] = struct{}{}
	defer delete(visited, absPath)

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		raw = map[string]any{}
	}

	merged := map[string]any{}
	if includeRaw, ok := raw["$include"]; ok {
		includeFiles, err := parseIncludes(includeRaw)
		if err != nil {
			return nil, err
		}
		baseDir := filepath.Dir(absPath)
		for _, includePath := range includeFiles {
			resolvedPath := includePath
			if !filepath.IsAbs(includePath) {
				resolvedPath = filepath.Join(baseDir, includePath)
			}
			child, err := loadConfigObject(resolvedPath, visited)
			if err != nil {
				return nil, err
			}
			deepMerge(merged, child)
		}
	}
	delete(raw, "$include")
	substituteEnvValues(raw)
	deepMerge(merged, raw)
	return merged, nil
}

func parseIncludes(v any) ([]string, error) {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return nil, nil
		}
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("$include entries must be strings")
			}
			if strings.TrimSpace(s) == "" {
				continue
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("$include must be a string or array of strings")
	}
}

func deepMerge(dst, src map[string]any) {
	for key, val := range src {
		srcMap, srcIsMap := val.(map[string]any)
		if !srcIsMap {
			dst[key] = val
			continue
		}

		existing, ok := dst[key]
		if !ok {
			copyMap := map[string]any{}
			deepMerge(copyMap, srcMap)
			dst[key] = copyMap
			continue
		}
		dstMap, dstIsMap := existing.(map[string]any)
		if !dstIsMap {
			copyMap := map[string]any{}
			deepMerge(copyMap, srcMap)
			dst[key] = copyMap
			continue
		}
		deepMerge(dstMap, srcMap)
	}
}

func substituteEnvValues(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, item := range t {
			t[k] = substituteEnvValues(item)
		}
		return t
	case []any:
		for i, item := range t {
			t[i] = substituteEnvValues(item)
		}
		return t
	case string:
		return envPattern.ReplaceAllStringFunc(t, func(match string) string {
			parts := envPattern.FindStringSubmatch(match)
			if len(parts) != 2 {
				return match
			}
			if value, ok := os.LookupEnv(parts[1]); ok {
				return value
			}
			return match
		})
	default:
		return v
	}
}
