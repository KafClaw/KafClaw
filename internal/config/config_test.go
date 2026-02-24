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
	if cfg.Tools.Subagents.MaxConcurrent != 8 {
		t.Errorf("expected subagents maxConcurrent 8, got %d", cfg.Tools.Subagents.MaxConcurrent)
	}
	if cfg.Tools.Subagents.ArchiveAfterMinutes != 60 {
		t.Errorf("expected subagents archiveAfterMinutes 60, got %d", cfg.Tools.Subagents.ArchiveAfterMinutes)
	}
	if cfg.Tools.Subagents.MemoryShareMode != "handoff" {
		t.Errorf("expected subagents memoryShareMode handoff, got %s", cfg.Tools.Subagents.MemoryShareMode)
	}
	if cfg.Node.ClawID == "" {
		t.Error("expected default node.clawId to be set")
	}
	if !cfg.Memory.Embedding.Enabled {
		t.Error("expected memory embedding enabled by default")
	}
	if cfg.Memory.Embedding.Provider != "local-hf" {
		t.Errorf("expected memory embedding provider local-hf, got %s", cfg.Memory.Embedding.Provider)
	}
	if cfg.Knowledge.ShareMode != "proposal" {
		t.Errorf("expected knowledge shareMode proposal, got %s", cfg.Knowledge.ShareMode)
	}
	if !cfg.Knowledge.GovernanceEnabled {
		t.Error("expected knowledge.governanceEnabled true by default")
	}
}

func TestLoadDefaults(t *testing.T) {
	// Temporarily set HOME to a non-existent directory
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "/tmp/nonexistent-kafclaw-test")
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
	configDir := filepath.Join(tmpDir, ".kafclaw")
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
	configDir := filepath.Join(tmpDir, ".kafclaw")
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

func TestLoadAgentsDefaultsSubagentsCompatibility(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	configJSON := `{
		"agents": {
			"defaults": {
				"subagents": {
					"maxConcurrent": 3,
					"maxSpawnDepth": 2,
					"maxChildrenPerAgent": 4,
					"archiveAfterMinutes": 15,
					"memoryShareMode": "inherit-readonly",
					"model": "openai/gpt-4.1",
					"thinking": "medium",
					"allowAgents": ["agent-main","agent-research"]
				}
			}
		}
	}`
	os.WriteFile(configFile, []byte(configJSON), 0600)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Tools.Subagents.MaxConcurrent != 3 ||
		cfg.Tools.Subagents.MaxSpawnDepth != 2 ||
		cfg.Tools.Subagents.MaxChildrenPerAgent != 4 ||
		cfg.Tools.Subagents.ArchiveAfterMinutes != 15 {
		t.Fatalf("expected tools.subagents to inherit agents.defaults.subagents, got %+v", cfg.Tools.Subagents)
	}
	if cfg.Tools.Subagents.Model != "openai/gpt-4.1" || cfg.Tools.Subagents.Thinking != "medium" {
		t.Fatalf("expected model/thinking inherited from agents.defaults.subagents, got %+v", cfg.Tools.Subagents)
	}
	if len(cfg.Tools.Subagents.AllowAgents) != 2 {
		t.Fatalf("expected allowAgents inherited, got %+v", cfg.Tools.Subagents.AllowAgents)
	}
	if cfg.Tools.Subagents.MemoryShareMode != "inherit-readonly" {
		t.Fatalf("expected memoryShareMode inherited, got %+v", cfg.Tools.Subagents)
	}
}

func TestLoadToolsSubagentsPrecedenceOverAgentsDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	configJSON := `{
		"agents": {
			"defaults": {
				"subagents": {
					"maxConcurrent": 3,
					"maxSpawnDepth": 2,
					"maxChildrenPerAgent": 4
				}
			}
		},
		"tools": {
			"subagents": {
				"maxConcurrent": 9,
				"maxSpawnDepth": 1,
				"maxChildrenPerAgent": 7,
				"memoryShareMode": "isolated"
			}
		}
	}`
	os.WriteFile(configFile, []byte(configJSON), 0600)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Tools.Subagents.MaxConcurrent != 9 || cfg.Tools.Subagents.MaxSpawnDepth != 1 || cfg.Tools.Subagents.MaxChildrenPerAgent != 7 {
		t.Fatalf("expected tools.subagents to override agents.defaults.subagents, got %+v", cfg.Tools.Subagents)
	}
	if cfg.Tools.Subagents.MemoryShareMode != "isolated" {
		t.Fatalf("expected tools.subagents.memoryShareMode to override defaults, got %+v", cfg.Tools.Subagents)
	}
}

func TestLoadMemoryKnowledgeNormalizationAndTopics(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	configJSON := `{
		"group": {
			"groupName": "teamalpha"
		},
		"node": {
			"clawId": "",
			"instanceId": ""
		},
		"memory": {
			"embedding": {
				"enabled": true,
				"provider": "unknown",
				"model": "",
				"dimension": 0,
				"startupTimeoutSec": 0
			},
			"search": {
				"mode": "invalid",
				"maxResults": 0,
				"minScore": 2
			}
		},
		"knowledge": {
			"enabled": true,
			"group": "",
			"shareMode": "invalid",
			"topics": {},
			"publish": {},
			"voting": {
				"minPoolSize": 0,
				"quorumYes": 0,
				"quorumNo": 0,
				"timeoutSec": 0
			}
		}
	}`
	os.WriteFile(configFile, []byte(configJSON), 0600)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Node.ClawID == "" || cfg.Node.InstanceID == "" {
		t.Fatalf("expected normalized node identity defaults, got %+v", cfg.Node)
	}
	if cfg.Memory.Embedding.Provider != "local-hf" {
		t.Fatalf("expected normalized embedding provider local-hf, got %s", cfg.Memory.Embedding.Provider)
	}
	if cfg.Memory.Embedding.Model == "" || cfg.Memory.Embedding.Dimension <= 0 || cfg.Memory.Embedding.StartupTimeoutSec <= 0 {
		t.Fatalf("expected normalized embedding defaults, got %+v", cfg.Memory.Embedding)
	}
	if cfg.Memory.Search.Mode != "hybrid" || cfg.Memory.Search.MaxResults <= 0 || cfg.Memory.Search.MinScore != 1 {
		t.Fatalf("expected normalized memory search settings, got %+v", cfg.Memory.Search)
	}
	if cfg.Knowledge.Group != "teamalpha" {
		t.Fatalf("expected knowledge group fallback to group.groupName, got %s", cfg.Knowledge.Group)
	}
	if cfg.Knowledge.ShareMode != "proposal" {
		t.Fatalf("expected knowledge shareMode fallback proposal, got %s", cfg.Knowledge.ShareMode)
	}
	if !cfg.Knowledge.GovernanceEnabled {
		t.Fatalf("expected governance enabled default after normalization")
	}
	if cfg.Knowledge.Topics.Proposals != "group.teamalpha.knowledge.proposals" ||
		cfg.Knowledge.Topics.Votes != "group.teamalpha.knowledge.votes" ||
		cfg.Knowledge.Topics.Facts != "group.teamalpha.knowledge.facts" {
		t.Fatalf("expected knowledge topics derived from group, got %+v", cfg.Knowledge.Topics)
	}
	if cfg.Knowledge.Voting.MinPoolSize <= 0 || cfg.Knowledge.Voting.QuorumYes <= 0 || cfg.Knowledge.Voting.TimeoutSec <= 0 {
		t.Fatalf("expected normalized voting defaults, got %+v", cfg.Knowledge.Voting)
	}
	if len(cfg.Knowledge.Publish.DenyTags) == 0 || len(cfg.Knowledge.Publish.AllowTags) == 0 {
		t.Fatalf("expected publish tag defaults, got %+v", cfg.Knowledge.Publish)
	}
}

func TestLoadMemoryEmbeddingDisabledNormalizesProvider(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	configJSON := `{
		"memory": {
			"embedding": {
				"enabled": false,
				"provider": "openai"
			}
		}
	}`
	os.WriteFile(configFile, []byte(configJSON), 0600)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Memory.Embedding.Enabled {
		t.Fatalf("expected embedding disabled, got %+v", cfg.Memory.Embedding)
	}
	if cfg.Memory.Embedding.Provider != "disabled" {
		t.Fatalf("expected disabled embedding provider marker, got %s", cfg.Memory.Embedding.Provider)
	}
}

func TestLoadKnowledgeGovernanceFlagDefaultsAndPreservesExplicitFalse(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfgPath := filepath.Join(configDir, "config.json")
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	// Missing governanceEnabled should keep default true.
	if err := os.WriteFile(cfgPath, []byte(`{"knowledge":{"enabled":true,"group":"g1"}}`), 0o600); err != nil {
		t.Fatalf("write config missing governanceEnabled: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config missing governanceEnabled: %v", err)
	}
	if !cfg.Knowledge.GovernanceEnabled {
		t.Fatal("expected governanceEnabled=true when field is missing")
	}

	// Explicit false should be preserved.
	if err := os.WriteFile(cfgPath, []byte(`{"knowledge":{"enabled":true,"governanceEnabled":false,"group":"g1"}}`), 0o600); err != nil {
		t.Fatalf("write config explicit governanceEnabled false: %v", err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatalf("load config explicit governanceEnabled false: %v", err)
	}
	if cfg.Knowledge.GovernanceEnabled {
		t.Fatal("expected governanceEnabled=false when explicitly configured")
	}
}

func TestLoadMemoryKnowledgePreservesValidValues(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	os.MkdirAll(configDir, 0755)
	configFile := filepath.Join(configDir, "config.json")

	configJSON := `{
		"memory": {
			"embedding": {
				"enabled": true,
				"provider": "openai",
				"model": "text-embedding-3-small",
				"dimension": 1536,
				"startupTimeoutSec": 30
			},
			"search": {
				"mode": "semantic",
				"maxResults": 12,
				"minScore": -5
			}
		},
		"knowledge": {
			"enabled": true,
			"group": "kg",
			"shareMode": "direct",
			"topics": {
				"proposals": "custom.proposals",
				"votes": "custom.votes"
			},
			"voting": {
				"minPoolSize": 5,
				"quorumYes": 3,
				"quorumNo": 3,
				"timeoutSec": 300
			}
		}
	}`
	os.WriteFile(configFile, []byte(configJSON), 0600)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Memory.Embedding.Provider != "openai" {
		t.Fatalf("expected provider openai, got %s", cfg.Memory.Embedding.Provider)
	}
	if cfg.Memory.Search.Mode != "semantic" {
		t.Fatalf("expected search mode semantic, got %s", cfg.Memory.Search.Mode)
	}
	if cfg.Memory.Search.MinScore != 0 {
		t.Fatalf("expected minScore clamped to 0, got %f", cfg.Memory.Search.MinScore)
	}
	if cfg.Knowledge.ShareMode != "direct" {
		t.Fatalf("expected direct share mode, got %s", cfg.Knowledge.ShareMode)
	}
	if cfg.Knowledge.Topics.Proposals != "custom.proposals" || cfg.Knowledge.Topics.Votes != "custom.votes" {
		t.Fatalf("expected custom topics preserved, got %+v", cfg.Knowledge.Topics)
	}
	if cfg.Knowledge.Voting.MinPoolSize != 5 || cfg.Knowledge.Voting.QuorumYes != 3 || cfg.Knowledge.Voting.TimeoutSec != 300 {
		t.Fatalf("expected explicit voting settings preserved, got %+v", cfg.Knowledge.Voting)
	}
}

func TestNormalizeMemoryKnowledgeConfigBranches(t *testing.T) {
	normalizeMemoryKnowledgeConfig(nil) // nil-safe guard

	cfg := &Config{
		Group: GroupConfig{GroupName: "autogroup"},
		Node:  NodeConfig{},
		Memory: MemoryConfig{
			Embedding: MemoryEmbeddingConfig{
				Enabled:           true,
				Provider:          "",
				Model:             "",
				Dimension:         0,
				CacheDir:          "",
				Endpoint:          "",
				StartupTimeoutSec: 0,
			},
			Search: MemorySearchConfig{
				Mode:       "keyword",
				MaxResults: 0,
				MinScore:   -1,
			},
		},
		Knowledge: KnowledgeConfig{
			GovernanceEnabled: true,
			Group:             "",
			ShareMode:         "",
			Topics:            DefaultConfig().Knowledge.Topics,
			Publish:           KnowledgePublishConfig{},
			Voting: KnowledgeVotingConfig{
				MinPoolSize: 0,
				QuorumYes:   0,
				QuorumNo:    0,
				TimeoutSec:  0,
			},
		},
	}
	normalizeMemoryKnowledgeConfig(cfg)

	if cfg.Node.ClawID == "" || cfg.Node.InstanceID == "" || cfg.Node.DisplayName == "" {
		t.Fatalf("expected node defaults after normalize, got %+v", cfg.Node)
	}
	if cfg.Memory.Embedding.Provider != "local-hf" {
		t.Fatalf("expected local-hf provider default, got %s", cfg.Memory.Embedding.Provider)
	}
	if cfg.Memory.Embedding.Model == "" || cfg.Memory.Embedding.Dimension <= 0 || cfg.Memory.Embedding.StartupTimeoutSec <= 0 {
		t.Fatalf("expected embedding defaults after normalize, got %+v", cfg.Memory.Embedding)
	}
	if cfg.Memory.Search.Mode != "keyword" || cfg.Memory.Search.MaxResults <= 0 || cfg.Memory.Search.MinScore != 0 {
		t.Fatalf("expected keyword mode preserved + clamped search values, got %+v", cfg.Memory.Search)
	}
	if cfg.Knowledge.Group != "autogroup" {
		t.Fatalf("expected fallback knowledge group from group.groupName, got %s", cfg.Knowledge.Group)
	}
	if cfg.Knowledge.ShareMode != "proposal" {
		t.Fatalf("expected proposal share mode default, got %s", cfg.Knowledge.ShareMode)
	}
	if cfg.Knowledge.Topics.Proposals != "group.autogroup.knowledge.proposals" {
		t.Fatalf("expected autogroup-derived proposals topic, got %s", cfg.Knowledge.Topics.Proposals)
	}
	if len(cfg.Knowledge.Publish.AllowTags) == 0 || len(cfg.Knowledge.Publish.DenyTags) == 0 {
		t.Fatalf("expected publish tags defaulted, got %+v", cfg.Knowledge.Publish)
	}
	if cfg.Knowledge.Voting.MinPoolSize <= 0 || cfg.Knowledge.Voting.QuorumYes <= 0 || cfg.Knowledge.Voting.TimeoutSec <= 0 {
		t.Fatalf("expected voting defaults, got %+v", cfg.Knowledge.Voting)
	}

	cfg.Memory.Embedding = MemoryEmbeddingConfig{Enabled: false, Provider: "openai"}
	cfg.Memory.Search = MemorySearchConfig{Mode: "semantic", MaxResults: 3, MinScore: 2}
	cfg.Knowledge.ShareMode = "direct"
	cfg.Knowledge.Group = ""
	cfg.Group.GroupName = ""
	cfg.Knowledge.Topics = KnowledgeTopicsConfig{}
	normalizeMemoryKnowledgeConfig(cfg)

	if cfg.Memory.Embedding.Enabled || cfg.Memory.Embedding.Provider != "disabled" {
		t.Fatalf("expected disabled marker when embedding disabled, got %+v", cfg.Memory.Embedding)
	}
	if cfg.Memory.Search.Mode != "semantic" || cfg.Memory.Search.MinScore != 1 {
		t.Fatalf("expected semantic mode preserved and minScore clamped to 1, got %+v", cfg.Memory.Search)
	}
	if cfg.Knowledge.Group != DefaultConfig().Knowledge.Group {
		t.Fatalf("expected default knowledge group fallback, got %s", cfg.Knowledge.Group)
	}
	if cfg.Knowledge.ShareMode != "direct" {
		t.Fatalf("expected direct share mode preserved, got %s", cfg.Knowledge.ShareMode)
	}
	if cfg.Knowledge.Topics.Facts == "" || cfg.Knowledge.Topics.Decisions == "" {
		t.Fatalf("expected topics filled from default group, got %+v", cfg.Knowledge.Topics)
	}

	cfg.Memory.Embedding = MemoryEmbeddingConfig{Enabled: true, Provider: "openai", Model: "x", Dimension: 123, StartupTimeoutSec: 10}
	cfg.Memory.Search = MemorySearchConfig{Mode: "invalid", MaxResults: 1, MinScore: 0.5}
	normalizeMemoryKnowledgeConfig(cfg)
	if cfg.Memory.Embedding.Provider != "openai" {
		t.Fatalf("expected openai provider preserved, got %+v", cfg.Memory.Embedding)
	}
	if cfg.Memory.Search.Mode != "hybrid" {
		t.Fatalf("expected invalid search mode to normalize to hybrid, got %+v", cfg.Memory.Search)
	}
}
