// Package config provides configuration types and loading for kafclaw.
package config

import "time"

// Config is the root configuration struct.
// Top-level groups: Paths, Model, Channels, Providers, Gateway, Tools.
type Config struct {
	Paths                 PathsConfig                 `json:"paths"`
	Model                 ModelConfig                 `json:"model"`
	Agents                *AgentsConfig               `json:"agents,omitempty"`
	Channels              ChannelsConfig              `json:"channels"`
	Providers             ProvidersConfig             `json:"providers"`
	Gateway               GatewayConfig               `json:"gateway"`
	Tools                 ToolsConfig                 `json:"tools"`
	Skills                SkillsConfig                `json:"skills"`
	Group                 GroupConfig                 `json:"group"`
	Orchestrator          OrchestratorConfig          `json:"orchestrator"`
	Scheduler             SchedulerConfig             `json:"scheduler"`
	ER1                   ER1IntegrationConfig        `json:"er1"`
	Observer              ObserverMemoryConfig        `json:"observer"`
	ContentClassification ContentClassificationConfig `json:"contentClassification"`
	PromptGuard           PromptGuardConfig           `json:"promptGuard"`
	OutputSanitization    OutputSanitizationConfig    `json:"outputSanitization"`
	FinOps                FinOpsConfig                `json:"finops"`
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
	Name              string            `json:"name" envconfig:"MODEL"`
	MaxTokens         int               `json:"maxTokens" envconfig:"MAX_TOKENS"`
	Temperature       float64           `json:"temperature" envconfig:"TEMPERATURE"`
	MaxToolIterations int               `json:"maxToolIterations" envconfig:"MAX_TOOL_ITERATIONS"`
	TaskRouting       map[string]string `json:"taskRouting,omitempty"` // e.g. {"security":"claude/claude-opus-4-6","tool-heavy":"openai-codex/gpt-5.3-codex"}
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
	Slack    SlackConfig    `json:"slack"`
	MSTeams  MSTeamsConfig  `json:"msteams"`
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
	SessionScope     string   `json:"sessionScope" envconfig:"WHATSAPP_SESSION_SCOPE"`
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

// DmPolicy controls direct-message access for channels.
type DmPolicy string

const (
	DmPolicyPairing   DmPolicy = "pairing"
	DmPolicyAllowlist DmPolicy = "allowlist"
	DmPolicyOpen      DmPolicy = "open"
	DmPolicyDisabled  DmPolicy = "disabled"
)

// GroupPolicy controls group/channel access for channels.
type GroupPolicy string

const (
	GroupPolicyAllowlist GroupPolicy = "allowlist"
	GroupPolicyOpen      GroupPolicy = "open"
	GroupPolicyDisabled  GroupPolicy = "disabled"
)

// SlackConfig configures the Slack channel.
type SlackConfig struct {
	Enabled          bool                 `json:"enabled" envconfig:"SLACK_ENABLED"`
	BotToken         string               `json:"botToken" envconfig:"SLACK_BOT_TOKEN"`
	AppToken         string               `json:"appToken" envconfig:"SLACK_APP_TOKEN"`
	InboundToken     string               `json:"inboundToken" envconfig:"SLACK_INBOUND_TOKEN"`
	OutboundURL      string               `json:"outboundUrl" envconfig:"SLACK_OUTBOUND_URL"`
	NativeStreaming  bool                 `json:"nativeStreaming" envconfig:"SLACK_NATIVE_STREAMING"`
	StreamMode       string               `json:"streamMode" envconfig:"SLACK_STREAM_MODE"`
	StreamChunkChars int                  `json:"streamChunkChars" envconfig:"SLACK_STREAM_CHUNK_CHARS"`
	SessionScope     string               `json:"sessionScope" envconfig:"SLACK_SESSION_SCOPE"`
	Accounts         []SlackAccountConfig `json:"accounts,omitempty"`
	AllowFrom        []string             `json:"allowFrom"`
	DmPolicy         DmPolicy             `json:"dmPolicy"`
	GroupPolicy      GroupPolicy          `json:"groupPolicy"`
	RequireMention   bool                 `json:"requireMention" envconfig:"SLACK_REQUIRE_MENTION"`
}

// SlackAccountConfig configures one named Slack account.
type SlackAccountConfig struct {
	ID               string      `json:"id"`
	Enabled          bool        `json:"enabled"`
	BotToken         string      `json:"botToken"`
	AppToken         string      `json:"appToken"`
	InboundToken     string      `json:"inboundToken"`
	OutboundURL      string      `json:"outboundUrl"`
	NativeStreaming  *bool       `json:"nativeStreaming,omitempty"`
	StreamMode       string      `json:"streamMode"`
	StreamChunkChars int         `json:"streamChunkChars"`
	SessionScope     string      `json:"sessionScope"`
	AllowFrom        []string    `json:"allowFrom"`
	DmPolicy         DmPolicy    `json:"dmPolicy"`
	GroupPolicy      GroupPolicy `json:"groupPolicy"`
	RequireMention   bool        `json:"requireMention"`
}

// MSTeamsConfig configures the Microsoft Teams channel.
type MSTeamsConfig struct {
	Enabled        bool                   `json:"enabled" envconfig:"MSTEAMS_ENABLED"`
	AppID          string                 `json:"appId" envconfig:"MSTEAMS_APP_ID"`
	AppPassword    string                 `json:"appPassword" envconfig:"MSTEAMS_APP_PASSWORD"`
	TenantID       string                 `json:"tenantId" envconfig:"MSTEAMS_TENANT_ID"`
	InboundToken   string                 `json:"inboundToken" envconfig:"MSTEAMS_INBOUND_TOKEN"`
	OutboundURL    string                 `json:"outboundUrl" envconfig:"MSTEAMS_OUTBOUND_URL"`
	SessionScope   string                 `json:"sessionScope" envconfig:"MSTEAMS_SESSION_SCOPE"`
	Accounts       []MSTeamsAccountConfig `json:"accounts,omitempty"`
	AllowFrom      []string               `json:"allowFrom"`
	GroupAllowFrom []string               `json:"groupAllowFrom"`
	DmPolicy       DmPolicy               `json:"dmPolicy"`
	GroupPolicy    GroupPolicy            `json:"groupPolicy"`
	RequireMention bool                   `json:"requireMention" envconfig:"MSTEAMS_REQUIRE_MENTION"`
}

// MSTeamsAccountConfig configures one named Teams account.
type MSTeamsAccountConfig struct {
	ID             string      `json:"id"`
	Enabled        bool        `json:"enabled"`
	AppID          string      `json:"appId"`
	AppPassword    string      `json:"appPassword"`
	TenantID       string      `json:"tenantId"`
	InboundToken   string      `json:"inboundToken"`
	OutboundURL    string      `json:"outboundUrl"`
	SessionScope   string      `json:"sessionScope"`
	AllowFrom      []string    `json:"allowFrom"`
	GroupAllowFrom []string    `json:"groupAllowFrom"`
	DmPolicy       DmPolicy    `json:"dmPolicy"`
	GroupPolicy    GroupPolicy `json:"groupPolicy"`
	RequireMention bool        `json:"requireMention"`
}

// ---------------------------------------------------------------------------
// Providers – LLM API keys & endpoints
// ---------------------------------------------------------------------------

// ProvidersConfig contains LLM provider configurations.
type ProvidersConfig struct {
	Anthropic        ProviderConfig     `json:"anthropic"`
	OpenAI           ProviderConfig     `json:"openai"`
	LocalWhisper     LocalWhisperConfig `json:"localWhisper"`
	OpenRouter       ProviderConfig     `json:"openrouter"`
	DeepSeek         ProviderConfig     `json:"deepseek"`
	Groq             ProviderConfig     `json:"groq"`
	Gemini           ProviderConfig     `json:"gemini"`
	VLLM             ProviderConfig     `json:"vllm"`
	XAI              ProviderConfig     `json:"xai"`
	ScalyticsCopilot ProviderConfig     `json:"scalyticsCopilot"`
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
	DaemonRuntime string `json:"daemonRuntime" envconfig:"DAEMON_RUNTIME"`
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

// SkillsConfig contains skill-system settings.
type SkillsConfig struct {
	Enabled               bool                        `json:"enabled" envconfig:"ENABLED"`
	AllowSystemRepoSkills bool                        `json:"allowSystemRepoSkills"`
	AllowWorkspaceSkills  bool                        `json:"allowWorkspaceSkills"`
	ExternalInstalls      bool                        `json:"externalInstalls"`
	NodeManager           string                      `json:"nodeManager" envconfig:"NODE_MANAGER"`
	Scope                 string                      `json:"scope" envconfig:"SCOPE"`
	RuntimeIsolation      string                      `json:"runtimeIsolation" envconfig:"RUNTIME_ISOLATION"`
	LinkPolicy            SkillLinkPolicyConfig       `json:"linkPolicy"`
	Entries               map[string]SkillEntryConfig `json:"entries,omitempty"`
}

// SkillLinkPolicyConfig controls which links are permitted in skills/installers.
type SkillLinkPolicyConfig struct {
	Mode             string   `json:"mode"`
	AllowDomains     []string `json:"allowDomains,omitempty"`
	DenyDomains      []string `json:"denyDomains,omitempty"`
	AllowHTTP        bool     `json:"allowHttp"`
	MaxLinksPerSkill int      `json:"maxLinksPerSkill"`
}

// SkillEntryConfig holds per-skill toggles.
type SkillEntryConfig struct {
	Enabled      bool     `json:"enabled"`
	Capabilities []string `json:"capabilities,omitempty"`
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

// AgentModelSpec configures the primary model and fallbacks for an agent.
type AgentModelSpec struct {
	Primary   string   `json:"primary"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

// AgentSubagentSpec configures the model for subagents spawned by an agent.
type AgentSubagentSpec struct {
	Model string `json:"model"`
}

// AgentListEntry describes a configured agent identity.
type AgentListEntry struct {
	ID        string             `json:"id"`
	Name      string             `json:"name,omitempty"`
	Default   bool               `json:"default,omitempty"`
	Model     *AgentModelSpec    `json:"model,omitempty"`
	Subagents *AgentSubagentSpec `json:"subagents,omitempty"`
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
	KafkaSecurityProto string `json:"kafkaSecurityProtocol" envconfig:"KAFKA_SECURITY_PROTOCOL"` // PLAINTEXT|SSL|SASL_PLAINTEXT|SASL_SSL
	KafkaSASLMechanism string `json:"kafkaSaslMechanism" envconfig:"KAFKA_SASL_MECHANISM"`       // PLAIN|SCRAM-SHA-256|SCRAM-SHA-512
	KafkaSASLUsername  string `json:"kafkaSaslUsername" envconfig:"KAFKA_SASL_USERNAME"`
	KafkaSASLPassword  string `json:"kafkaSaslPassword" envconfig:"KAFKA_SASL_PASSWORD"`
	KafkaTLSCAFile     string `json:"kafkaTlsCAFile" envconfig:"KAFKA_TLS_CA_FILE"`
	KafkaTLSCertFile   string `json:"kafkaTlsCertFile" envconfig:"KAFKA_TLS_CERT_FILE"`
	KafkaTLSKeyFile    string `json:"kafkaTlsKeyFile" envconfig:"KAFKA_TLS_KEY_FILE"`
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

// ---------------------------------------------------------------------------
// Middleware – content classification, prompt guard, output sanitization, FinOps
// ---------------------------------------------------------------------------

// NamedPattern is a reusable named regex pattern.
type NamedPattern struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

// SensitivityLevel configures detection and routing for a sensitivity class.
type SensitivityLevel struct {
	Patterns []string `json:"patterns,omitempty"` // regex patterns
	Keywords []string `json:"keywords,omitempty"` // literal keywords (case-insensitive)
	RouteTo  string   `json:"routeTo,omitempty"`  // model string to reroute to
}

// ContentClassificationConfig controls content-aware model routing.
type ContentClassificationConfig struct {
	Enabled           bool                        `json:"enabled"`
	SensitivityLevels map[string]SensitivityLevel `json:"sensitivityLevels,omitempty"` // e.g. "pii", "confidential"
	TaskTypeRoutes    map[string]string           `json:"taskTypeRoutes,omitempty"`    // task-category → model string
}

// PromptGuardConfig controls pre-LLM PII/secret scanning.
type PromptGuardConfig struct {
	Enabled        bool           `json:"enabled"`
	Mode           string         `json:"mode,omitempty"` // "warn", "block", "redact" (default "warn")
	PII            PIIConfig      `json:"pii"`
	Secrets        SecretsConfig  `json:"secrets"`
	DenyKeywords   []string       `json:"denyKeywords,omitempty"`
	CustomPatterns []NamedPattern `json:"customPatterns,omitempty"`
}

// PIIConfig controls PII detection.
type PIIConfig struct {
	Detect         []string       `json:"detect,omitempty"` // e.g. ["email","phone","ssn","credit_card","ip_address"]
	Action         string         `json:"action,omitempty"` // "redact", "block", "warn" (default inherits from mode)
	CustomPatterns []NamedPattern `json:"customPatterns,omitempty"`
}

// SecretsConfig controls secret detection.
type SecretsConfig struct {
	Detect         []string       `json:"detect,omitempty"` // e.g. ["api_key","bearer_token","private_key","password_literal"]
	Action         string         `json:"action,omitempty"` // "redact", "block", "warn"
	CustomPatterns []NamedPattern `json:"customPatterns,omitempty"`
}

// OutputSanitizationConfig controls post-LLM response filtering.
type OutputSanitizationConfig struct {
	Enabled              bool           `json:"enabled"`
	RedactPII            bool           `json:"redactPII"`
	RedactSecrets        bool           `json:"redactSecrets"`
	CustomRedactPatterns []NamedPattern `json:"customRedactPatterns,omitempty"`
	DenyPatterns         []string       `json:"denyPatterns,omitempty"`
	MaxOutputLength      int            `json:"maxOutputLength,omitempty"`
}

// ProviderPricing holds per-1k-token pricing for a provider.
type ProviderPricing struct {
	PromptPer1kTokens     float64 `json:"promptPer1kTokens"`
	CompletionPer1kTokens float64 `json:"completionPer1kTokens"`
}

// FinOpsConfig controls cost attribution and budgets.
type FinOpsConfig struct {
	Enabled       bool                       `json:"enabled"`
	Pricing       map[string]ProviderPricing `json:"pricing,omitempty"`       // providerID → pricing
	DailyBudget   float64                    `json:"dailyBudget,omitempty"`   // max USD per day (0 = unlimited)
	MonthlyBudget float64                    `json:"monthlyBudget,omitempty"` // max USD per month (0 = unlimited)
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
			DaemonRuntime: "native",
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
		Skills: SkillsConfig{
			Enabled:               false,
			AllowSystemRepoSkills: true,
			AllowWorkspaceSkills:  false,
			ExternalInstalls:      false,
			NodeManager:           "npm",
			Scope:                 "selected",
			RuntimeIsolation:      "auto",
			LinkPolicy: SkillLinkPolicyConfig{
				Mode:             "allowlist",
				AllowDomains:     []string{"clawhub.ai"},
				AllowHTTP:        false,
				MaxLinksPerSkill: 20,
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
		Channels: ChannelsConfig{
			Slack: SlackConfig{
				DmPolicy:         DmPolicyPairing,
				GroupPolicy:      GroupPolicyAllowlist,
				RequireMention:   true,
				SessionScope:     "room",
				NativeStreaming:  true,
				StreamMode:       "replace",
				StreamChunkChars: 320,
			},
			MSTeams: MSTeamsConfig{
				DmPolicy:       DmPolicyPairing,
				GroupPolicy:    GroupPolicyAllowlist,
				RequireMention: true,
				SessionScope:   "room",
			},
			WhatsApp: WhatsAppConfig{
				SessionScope: "room",
			},
		},
	}
}
