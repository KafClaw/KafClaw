// Package config provides configuration types and loading for kafclaw.
package config

import "time"

// Config is the root configuration struct.
// Top-level groups: Paths, Model, Channels, Providers, Gateway, Tools.
type Config struct {
	Paths        PathsConfig          `json:"paths"`
	Model        ModelConfig          `json:"model"`
	Agents       *AgentsConfig        `json:"agents,omitempty"`
	Channels     ChannelsConfig       `json:"channels"`
	Providers    ProvidersConfig      `json:"providers"`
	Gateway      GatewayConfig        `json:"gateway"`
	Tools        ToolsConfig          `json:"tools"`
	Group        GroupConfig          `json:"group"`
	Orchestrator OrchestratorConfig   `json:"orchestrator"`
	Scheduler    SchedulerConfig      `json:"scheduler"`
	ER1          ER1IntegrationConfig `json:"er1"`
	Observer     ObserverMemoryConfig `json:"observer"`
}

// ---------------------------------------------------------------------------
// Paths – filesystem locations
// ---------------------------------------------------------------------------

// PathsConfig groups all filesystem path settings.
type PathsConfig struct {
	Workspace      string `json:"workspace" envconfig:"WORKSPACE"`
	WorkRepoPath   string `json:"workRepoPath" envconfig:"WORK_REPO_PATH"`
	SystemRepoPath string `json:"systemRepoPath" envconfig:"SYSTEM_REPO_PATH"`
}

// ---------------------------------------------------------------------------
// Model – LLM behaviour
// ---------------------------------------------------------------------------

// ModelConfig groups LLM model and agent-loop settings.
type ModelConfig struct {
	Name              string  `json:"name" envconfig:"MODEL"`
	MaxTokens         int     `json:"maxTokens" envconfig:"MAX_TOKENS"`
	Temperature       float64 `json:"temperature" envconfig:"TEMPERATURE"`
	MaxToolIterations int     `json:"maxToolIterations" envconfig:"MAX_TOOL_ITERATIONS"`
}

// ---------------------------------------------------------------------------
// Channels – messaging integrations
// ---------------------------------------------------------------------------

// ChannelsConfig contains all channel configurations.
type ChannelsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
	Discord  DiscordConfig  `json:"discord"`
	WhatsApp WhatsAppConfig `json:"whatsapp"`
	Feishu   FeishuConfig   `json:"feishu"`
}

// TelegramConfig configures the Telegram channel.
type TelegramConfig struct {
	Enabled   bool     `json:"enabled" envconfig:"TELEGRAM_ENABLED"`
	Token     string   `json:"token" envconfig:"TELEGRAM_TOKEN"`
	AllowFrom []string `json:"allowFrom"`
	Proxy     string   `json:"proxy,omitempty" envconfig:"TELEGRAM_PROXY"`
}

// DiscordConfig configures the Discord channel.
type DiscordConfig struct {
	Enabled   bool     `json:"enabled" envconfig:"DISCORD_ENABLED"`
	Token     string   `json:"token" envconfig:"DISCORD_TOKEN"`
	AllowFrom []string `json:"allowFrom"`
}

// WhatsAppConfig configures the WhatsApp channel.
type WhatsAppConfig struct {
	Enabled          bool     `json:"enabled" envconfig:"WHATSAPP_ENABLED"`
	BridgeURL        string   `json:"bridgeUrl" envconfig:"WHATSAPP_BRIDGE_URL"`
	AllowFrom        []string `json:"allowFrom"`
	DropUnauthorized bool     `json:"dropUnauthorized" envconfig:"WHATSAPP_DROP_UNAUTHORIZED"`
	IgnoreReactions  bool     `json:"ignoreReactions" envconfig:"WHATSAPP_IGNORE_REACTIONS"`
}

// FeishuConfig configures the Feishu channel.
type FeishuConfig struct {
	Enabled           bool     `json:"enabled" envconfig:"FEISHU_ENABLED"`
	AppID             string   `json:"appId" envconfig:"FEISHU_APP_ID"`
	AppSecret         string   `json:"appSecret" envconfig:"FEISHU_APP_SECRET"`
	EncryptKey        string   `json:"encryptKey" envconfig:"FEISHU_ENCRYPT_KEY"`
	VerificationToken string   `json:"verificationToken" envconfig:"FEISHU_VERIFICATION_TOKEN"`
	AllowFrom         []string `json:"allowFrom"`
}

// ---------------------------------------------------------------------------
// Providers – LLM API keys & endpoints
// ---------------------------------------------------------------------------

// ProvidersConfig contains LLM provider configurations.
type ProvidersConfig struct {
	Anthropic    ProviderConfig     `json:"anthropic"`
	OpenAI       ProviderConfig     `json:"openai"`
	LocalWhisper LocalWhisperConfig `json:"localWhisper"`
	OpenRouter   ProviderConfig     `json:"openrouter"`
	DeepSeek     ProviderConfig     `json:"deepseek"`
	Groq         ProviderConfig     `json:"groq"`
	Gemini       ProviderConfig     `json:"gemini"`
	VLLM         ProviderConfig     `json:"vllm"`
}

// ProviderConfig contains settings for a single LLM provider.
type ProviderConfig struct {
	APIKey  string `json:"apiKey" envconfig:"API_KEY"`
	APIBase string `json:"apiBase,omitempty" envconfig:"API_BASE"`
}

// LocalWhisperConfig contains settings for local Whisper transcription.
type LocalWhisperConfig struct {
	Enabled    bool   `json:"enabled" envconfig:"WHISPER_ENABLED"`
	Model      string `json:"model" envconfig:"WHISPER_MODEL"`
	BinaryPath string `json:"binaryPath" envconfig:"WHISPER_BINARY_PATH"`
}

// ---------------------------------------------------------------------------
// Gateway – HTTP server networking
// ---------------------------------------------------------------------------

// GatewayConfig contains gateway server settings.
type GatewayConfig struct {
	Host          string `json:"host" envconfig:"HOST"`
	Port          int    `json:"port" envconfig:"PORT"`
	DashboardPort int    `json:"dashboardPort" envconfig:"DASHBOARD_PORT"`
	AuthToken     string `json:"authToken" envconfig:"AUTH_TOKEN"`
	TLSCert       string `json:"tlsCert" envconfig:"TLS_CERT"`
	TLSKey        string `json:"tlsKey" envconfig:"TLS_KEY"`
}

// ---------------------------------------------------------------------------
// Orchestrator – multi-agent coordination
// ---------------------------------------------------------------------------

// OrchestratorConfig contains settings for the agent orchestrator.
type OrchestratorConfig struct {
	Enabled  bool   `json:"enabled" envconfig:"ENABLED"`
	Role     string `json:"role" envconfig:"ROLE"` // "orchestrator", "worker", "observer"
	ZoneID   string `json:"zoneId" envconfig:"ZONE_ID"`
	ParentID string `json:"parentId" envconfig:"PARENT_ID"`
	Endpoint string `json:"endpoint" envconfig:"ENDPOINT"` // This agent's remote API URL
}

// ---------------------------------------------------------------------------
// Tools – tool-specific behaviour
// ---------------------------------------------------------------------------

// ToolsConfig contains tool-specific settings.
type ToolsConfig struct {
	Exec      ExecToolConfig      `json:"exec"`
	Web       WebToolConfig       `json:"web"`
	Subagents SubagentsToolConfig `json:"subagents"`
}

// AgentsConfig mirrors OpenClaw-style defaults block for future compatibility.
type AgentsConfig struct {
	Defaults AgentDefaultsConfig `json:"defaults"`
	List     []AgentListEntry    `json:"list,omitempty"`
}

// AgentDefaultsConfig contains default agent-level settings.
type AgentDefaultsConfig struct {
	Subagents SubagentsToolConfig `json:"subagents"`
}

// AgentListEntry describes a configured agent identity.
type AgentListEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Default bool   `json:"default,omitempty"`
}

// ---------------------------------------------------------------------------
// Group – multi-agent collaboration via Kafka
// ---------------------------------------------------------------------------

// GroupConfig contains settings for group collaboration.
type GroupConfig struct {
	Enabled            bool   `json:"enabled" envconfig:"ENABLED"`
	GroupName          string `json:"groupName" envconfig:"GROUP_NAME"`
	LFSProxyURL        string `json:"lfsProxyUrl" envconfig:"KAFSCALE_LFS_PROXY_URL"`
	LFSProxyAPIKey     string `json:"lfsProxyApiKey" envconfig:"KAFSCALE_LFS_PROXY_API_KEY"`
	KafkaBrokers       string `json:"kafkaBrokers" envconfig:"KAFKA_BROKERS"`
	ConsumerGroup      string `json:"consumerGroup" envconfig:"KAFKA_CONSUMER_GROUP"`
	AgentID            string `json:"agentId" envconfig:"AGENT_ID"`
	PollIntervalMs     int    `json:"pollIntervalMs" envconfig:"POLL_INTERVAL_MS"`
	OnboardMode        string `json:"onboardMode" envconfig:"ONBOARD_MODE"` // "open" (default) or "gated"
	MaxDelegationDepth int    `json:"maxDelegationDepth" envconfig:"MAX_DELEGATION_DEPTH"`
}

// ---------------------------------------------------------------------------
// Scheduler – cron-based job scheduling
// ---------------------------------------------------------------------------

// SchedulerConfig contains settings for the cron scheduler.
type SchedulerConfig struct {
	Enabled        bool          `json:"enabled" envconfig:"ENABLED"`
	TickInterval   time.Duration `json:"tickInterval" envconfig:"TICK_INTERVAL"`
	MaxConcLLM     int           `json:"maxConcLLM" envconfig:"MAX_CONC_LLM"`
	MaxConcShell   int           `json:"maxConcShell" envconfig:"MAX_CONC_SHELL"`
	MaxConcDefault int           `json:"maxConcDefault" envconfig:"MAX_CONC_DEFAULT"`
}

// ExecToolConfig contains shell execution tool settings.
type ExecToolConfig struct {
	Timeout             time.Duration `json:"timeout"`
	RestrictToWorkspace bool          `json:"restrictToWorkspace" envconfig:"EXEC_RESTRICT_WORKSPACE"`
}

// WebToolConfig contains web tool settings.
type WebToolConfig struct {
	Search SearchConfig `json:"search"`
}

// SearchConfig contains web search settings.
type SearchConfig struct {
	APIKey     string `json:"apiKey" envconfig:"BRAVE_API_KEY"`
	MaxResults int    `json:"maxResults"`
}

// SubagentsToolConfig contains limits for spawned child agent sessions.
type SubagentsToolConfig struct {
	MaxConcurrent       int                `json:"maxConcurrent" envconfig:"MAX_CONCURRENT"`
	MaxSpawnDepth       int                `json:"maxSpawnDepth" envconfig:"MAX_SPAWN_DEPTH"`
	MaxChildrenPerAgent int                `json:"maxChildrenPerAgent" envconfig:"MAX_CHILDREN_PER_AGENT"`
	ArchiveAfterMinutes int                `json:"archiveAfterMinutes" envconfig:"ARCHIVE_AFTER_MINUTES"`
	AllowAgents         []string           `json:"allowAgents" envconfig:"ALLOW_AGENTS"`
	Model               string             `json:"model" envconfig:"MODEL"`
	Thinking            string             `json:"thinking" envconfig:"THINKING"`
	Tools               SubagentToolPolicy `json:"tools"`
}

type SubagentToolPolicy struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

// ---------------------------------------------------------------------------
// ER1 – personal memory service integration
// ---------------------------------------------------------------------------

// ER1IntegrationConfig configures the ER1 memory service connection.
type ER1IntegrationConfig struct {
	URL          string        `json:"url" envconfig:"ER1_URL"`
	APIKey       string        `json:"apiKey" envconfig:"ER1_API_KEY"`
	UserID       string        `json:"userId" envconfig:"ER1_USER_ID"`
	SyncInterval time.Duration `json:"syncInterval" envconfig:"ER1_SYNC_INTERVAL"`
}

// ---------------------------------------------------------------------------
// Observer – observational memory (LLM compression)
// ---------------------------------------------------------------------------

// ObserverMemoryConfig configures the observational memory system.
type ObserverMemoryConfig struct {
	Enabled          bool   `json:"enabled" envconfig:"OBSERVER_ENABLED"`
	Model            string `json:"model" envconfig:"OBSERVER_MODEL"`
	MessageThreshold int    `json:"messageThreshold" envconfig:"OBSERVER_MSG_THRESHOLD"`
	MaxObservations  int    `json:"maxObservations" envconfig:"OBSERVER_MAX_OBS"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Paths: PathsConfig{
			Workspace:      "~/KafClaw-Workspace",
			WorkRepoPath:   "~/KafClaw-Workspace",
			SystemRepoPath: "/Users/kamir/GITHUB.kamir/KafClaw/kafclaw",
		},
		Model: ModelConfig{
			Name:              "anthropic/claude-sonnet-4-5",
			MaxTokens:         8192,
			Temperature:       0.7,
			MaxToolIterations: 20,
		},
		Providers: ProvidersConfig{
			LocalWhisper: LocalWhisperConfig{
				Enabled:    true,
				Model:      "base",
				BinaryPath: "/opt/homebrew/bin/whisper",
			},
		},
		Gateway: GatewayConfig{
			Host:          "127.0.0.1", // Secure default
			Port:          18790,
			DashboardPort: 18791,
		},
		Tools: ToolsConfig{
			Exec: ExecToolConfig{
				Timeout:             60 * time.Second,
				RestrictToWorkspace: true, // Secure default
			},
			Web: WebToolConfig{
				Search: SearchConfig{
					MaxResults: 10,
				},
			},
			Subagents: SubagentsToolConfig{
				MaxConcurrent:       8,
				MaxSpawnDepth:       1,
				MaxChildrenPerAgent: 5,
				ArchiveAfterMinutes: 60,
			},
		},
		Group: GroupConfig{
			Enabled:            false,
			LFSProxyURL:        "http://localhost:8080",
			PollIntervalMs:     2000,
			MaxDelegationDepth: 3,
		},
		Orchestrator: OrchestratorConfig{
			Enabled: false,
			Role:    "worker",
		},
		Scheduler: SchedulerConfig{
			Enabled:        false,
			TickInterval:   60 * time.Second,
			MaxConcLLM:     3,
			MaxConcShell:   1,
			MaxConcDefault: 5,
		},
		ER1: ER1IntegrationConfig{
			SyncInterval: 5 * time.Minute,
		},
		Observer: ObserverMemoryConfig{
			Enabled:          false,
			MessageThreshold: 50,
			MaxObservations:  200,
		},
	}
}
