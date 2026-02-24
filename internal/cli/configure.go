package cli

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"github.com/spf13/cobra"
)

var configureSubagentsAllowAgents string
var configureClearSubagentsAllowAgents bool
var configureSubagentsMemoryShareMode string
var configureNonInteractive bool
var configureSkillsEnabled bool
var configureSkillsNodeManager string
var configureSkillsScope string
var configureEnableSkills []string
var configureDisableSkills []string
var configureGoogleWorkspaceRead string
var configureM365Read string
var configureKafkaBrokers string
var configureKafkaSecurityProtocol string
var configureKafkaSASLMechanism string
var configureKafkaSASLUsername string
var configureKafkaSASLPassword string
var configureKafkaTLSCAFile string
var configureKafkaTLSCertFile string
var configureKafkaTLSKeyFile string
var configureMemoryEmbeddingEnabled bool
var configureMemoryEmbeddingProvider string
var configureMemoryEmbeddingModel string
var configureMemoryEmbeddingDimension int
var configureConfirmMemoryWipe bool
var configureJSON bool

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Guided configuration updates",
	RunE:  runConfigure,
}

func init() {
	configureCmd.Flags().StringVar(&configureSubagentsAllowAgents, "subagents-allow-agents", "", "Comma-separated agent IDs allowed for sessions_spawn agentId (use '*' for any)")
	configureCmd.Flags().BoolVar(&configureClearSubagentsAllowAgents, "clear-subagents-allow-agents", false, "Clear subagent allowlist (default behavior: current agent only)")
	configureCmd.Flags().StringVar(&configureSubagentsMemoryShareMode, "subagents-memory-share-mode", "", "Set subagent memory share mode (isolated|handoff|inherit-readonly)")
	configureCmd.Flags().BoolVar(&configureSkillsEnabled, "skills-enabled", false, "Enable or disable global skills system (with --skills-enabled-set)")
	configureCmd.Flags().StringVar(&configureSkillsNodeManager, "skills-node-manager", "", "Set skills node manager (npm|pnpm|bun)")
	configureCmd.Flags().StringVar(&configureSkillsScope, "skills-scope", "", "Set skills scope (all|selected)")
	configureCmd.Flags().StringSliceVar(&configureEnableSkills, "enable-skill", nil, "Enable one or more skills (repeatable)")
	configureCmd.Flags().StringSliceVar(&configureDisableSkills, "disable-skill", nil, "Disable one or more skills (repeatable)")
	configureCmd.Flags().StringVar(&configureGoogleWorkspaceRead, "google-workspace-read", "", "Set google-workspace capabilities (mail,calendar,drive,all)")
	configureCmd.Flags().StringVar(&configureM365Read, "m365-read", "", "Set m365 capabilities (mail,calendar,files,all)")
	configureCmd.Flags().StringVar(&configureKafkaBrokers, "kafka-brokers", "", "Set group.kafkaBrokers (comma-separated host:port)")
	configureCmd.Flags().StringVar(&configureKafkaSecurityProtocol, "kafka-security-protocol", "", "Set group.kafkaSecurityProtocol (PLAINTEXT|SSL|SASL_PLAINTEXT|SASL_SSL)")
	configureCmd.Flags().StringVar(&configureKafkaSASLMechanism, "kafka-sasl-mechanism", "", "Set group.kafkaSaslMechanism (PLAIN|SCRAM-SHA-256|SCRAM-SHA-512)")
	configureCmd.Flags().StringVar(&configureKafkaSASLUsername, "kafka-sasl-username", "", "Set group.kafkaSaslUsername")
	configureCmd.Flags().StringVar(&configureKafkaSASLPassword, "kafka-sasl-password", "", "Set group.kafkaSaslPassword")
	configureCmd.Flags().StringVar(&configureKafkaTLSCAFile, "kafka-tls-ca-file", "", "Set group.kafkaTlsCAFile")
	configureCmd.Flags().StringVar(&configureKafkaTLSCertFile, "kafka-tls-cert-file", "", "Set group.kafkaTlsCertFile")
	configureCmd.Flags().StringVar(&configureKafkaTLSKeyFile, "kafka-tls-key-file", "", "Set group.kafkaTlsKeyFile")
	configureCmd.Flags().BoolVar(&configureMemoryEmbeddingEnabled, "memory-embedding-enabled", false, "Enable or disable memory embeddings (requires --memory-embedding-enabled-set)")
	configureCmd.Flags().StringVar(&configureMemoryEmbeddingProvider, "memory-embedding-provider", "", "Set memory.embedding.provider (e.g. local-hf|openai)")
	configureCmd.Flags().StringVar(&configureMemoryEmbeddingModel, "memory-embedding-model", "", "Set memory.embedding.model")
	configureCmd.Flags().IntVar(&configureMemoryEmbeddingDimension, "memory-embedding-dimension", 0, "Set memory.embedding.dimension")
	configureCmd.Flags().BoolVar(&configureConfirmMemoryWipe, "confirm-memory-wipe", false, "Confirm destructive memory wipe when switching an already-used embedding")
	configureCmd.Flags().BoolVar(&configureNonInteractive, "non-interactive", false, "Apply flags only and skip prompts")
	configureCmd.Flags().BoolVar(&configureJSON, "json", false, "Output machine-readable JSON summary")
	configureCmd.Flags().Bool("skills-enabled-set", false, "Apply --skills-enabled value")
	configureCmd.Flags().Bool("memory-embedding-enabled-set", false, "Apply --memory-embedding-enabled value")
	rootCmd.AddCommand(configureCmd)
}

func runConfigure(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	beforeFingerprint := memoryEmbeddingFingerprint(cfg)

	updatedAllowAgents := cfg.Tools.Subagents.AllowAgents
	hasFlagUpdate := configureClearSubagentsAllowAgents || strings.TrimSpace(configureSubagentsAllowAgents) != ""
	if configureClearSubagentsAllowAgents {
		updatedAllowAgents = nil
	} else if strings.TrimSpace(configureSubagentsAllowAgents) != "" {
		updatedAllowAgents = parseCSVList(configureSubagentsAllowAgents)
	}

	if !hasFlagUpdate && !configureNonInteractive {
		reader := bufio.NewReader(cmd.InOrStdin())
		current := strings.Join(cfg.Tools.Subagents.AllowAgents, ",")
		if current == "" {
			current = "(current agent only)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Current subagent allowlist: %s\n", current)
		fmt.Fprint(cmd.OutOrStdout(), "Set subagent allowlist (comma-separated, '*' for any, empty to keep): ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			return fmt.Errorf("read input: %w", readErr)
		}
		line = strings.TrimSpace(line)
		if line != "" {
			updatedAllowAgents = parseCSVList(line)
		}
	}

	cfg.Tools.Subagents.AllowAgents = updatedAllowAgents
	if strings.TrimSpace(configureSubagentsMemoryShareMode) != "" {
		mode := strings.ToLower(strings.TrimSpace(configureSubagentsMemoryShareMode))
		switch mode {
		case "isolated", "handoff", "inherit-readonly":
			cfg.Tools.Subagents.MemoryShareMode = mode
		default:
			return fmt.Errorf("invalid --subagents-memory-share-mode: %s (expected isolated|handoff|inherit-readonly)", mode)
		}
	}

	if cmd.Flags().Changed("skills-enabled-set") {
		cfg.Skills.Enabled = configureSkillsEnabled
	}
	if strings.TrimSpace(configureSkillsNodeManager) != "" {
		val := strings.ToLower(strings.TrimSpace(configureSkillsNodeManager))
		switch val {
		case "npm", "pnpm", "bun":
			cfg.Skills.NodeManager = val
		default:
			return fmt.Errorf("invalid --skills-node-manager: %s (expected npm|pnpm|bun)", val)
		}
	}
	if strings.TrimSpace(configureSkillsScope) != "" {
		scope := strings.ToLower(strings.TrimSpace(configureSkillsScope))
		switch scope {
		case "all", "selected":
			cfg.Skills.Scope = scope
		default:
			return fmt.Errorf("invalid --skills-scope: %s (expected all|selected)", scope)
		}
	}
	if cfg.Skills.Entries == nil {
		cfg.Skills.Entries = map[string]config.SkillEntryConfig{}
	}
	for _, name := range configureEnableSkills {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		entry := cfg.Skills.Entries[name]
		entry.Enabled = true
		cfg.Skills.Entries[name] = entry
	}
	for _, name := range configureDisableSkills {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		entry := cfg.Skills.Entries[name]
		entry.Enabled = false
		cfg.Skills.Entries[name] = entry
	}

	if strings.TrimSpace(configureGoogleWorkspaceRead) != "" {
		caps, err := parseCapabilitySelection(configureGoogleWorkspaceRead, map[string]struct{}{
			"mail": {}, "calendar": {}, "drive": {}, "all": {},
		})
		if err != nil {
			return fmt.Errorf("invalid --google-workspace-read: %w", err)
		}
		entry := cfg.Skills.Entries["google-workspace"]
		entry.Enabled = true
		entry.Capabilities = caps
		cfg.Skills.Entries["google-workspace"] = entry
	}
	if strings.TrimSpace(configureM365Read) != "" {
		caps, err := parseCapabilitySelection(configureM365Read, map[string]struct{}{
			"mail": {}, "calendar": {}, "files": {}, "all": {},
		})
		if err != nil {
			return fmt.Errorf("invalid --m365-read: %w", err)
		}
		entry := cfg.Skills.Entries["m365"]
		entry.Enabled = true
		entry.Capabilities = caps
		cfg.Skills.Entries["m365"] = entry
	}

	if strings.TrimSpace(configureKafkaBrokers) != "" {
		cfg.Group.KafkaBrokers = strings.TrimSpace(configureKafkaBrokers)
	}
	if strings.TrimSpace(configureKafkaSecurityProtocol) != "" {
		proto := strings.ToUpper(strings.TrimSpace(configureKafkaSecurityProtocol))
		switch proto {
		case "PLAINTEXT", "SSL", "SASL_PLAINTEXT", "SASL_SSL":
			cfg.Group.KafkaSecurityProto = proto
		default:
			return fmt.Errorf("invalid --kafka-security-protocol: %s (expected PLAINTEXT|SSL|SASL_PLAINTEXT|SASL_SSL)", proto)
		}
	}
	if strings.TrimSpace(configureKafkaSASLMechanism) != "" {
		mech := strings.ToUpper(strings.TrimSpace(configureKafkaSASLMechanism))
		switch mech {
		case "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512":
			cfg.Group.KafkaSASLMechanism = mech
		default:
			return fmt.Errorf("invalid --kafka-sasl-mechanism: %s (expected PLAIN|SCRAM-SHA-256|SCRAM-SHA-512)", mech)
		}
	}
	if strings.TrimSpace(configureKafkaSASLUsername) != "" {
		cfg.Group.KafkaSASLUsername = strings.TrimSpace(configureKafkaSASLUsername)
	}
	if strings.TrimSpace(configureKafkaSASLPassword) != "" {
		cfg.Group.KafkaSASLPassword = strings.TrimSpace(configureKafkaSASLPassword)
	}
	if strings.TrimSpace(configureKafkaTLSCAFile) != "" {
		cfg.Group.KafkaTLSCAFile = strings.TrimSpace(configureKafkaTLSCAFile)
	}
	if strings.TrimSpace(configureKafkaTLSCertFile) != "" {
		cfg.Group.KafkaTLSCertFile = strings.TrimSpace(configureKafkaTLSCertFile)
	}
	if strings.TrimSpace(configureKafkaTLSKeyFile) != "" {
		cfg.Group.KafkaTLSKeyFile = strings.TrimSpace(configureKafkaTLSKeyFile)
	}
	if cmd.Flags().Changed("memory-embedding-enabled-set") {
		cfg.Memory.Embedding.Enabled = configureMemoryEmbeddingEnabled
	}
	if strings.TrimSpace(configureMemoryEmbeddingProvider) != "" {
		cfg.Memory.Embedding.Provider = strings.TrimSpace(configureMemoryEmbeddingProvider)
	}
	if strings.TrimSpace(configureMemoryEmbeddingModel) != "" {
		cfg.Memory.Embedding.Model = strings.TrimSpace(configureMemoryEmbeddingModel)
	}
	if cmd.Flags().Changed("memory-embedding-dimension") {
		cfg.Memory.Embedding.Dimension = configureMemoryEmbeddingDimension
	}
	if err := validateEmbeddingHardGate(cfg); err != nil {
		return err
	}

	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(cfg.Group.KafkaSecurityProto)), "SASL_") {
		if strings.TrimSpace(cfg.Group.KafkaSASLMechanism) == "" || strings.TrimSpace(cfg.Group.KafkaSASLUsername) == "" || strings.TrimSpace(cfg.Group.KafkaSASLPassword) == "" {
			return fmt.Errorf("group.kafkaSecurityProtocol=%s requires kafka sasl mechanism, username, and password", cfg.Group.KafkaSecurityProto)
		}
	}
	afterFingerprint := memoryEmbeddingFingerprint(cfg)
	wipedCount := int64(0)
	if beforeFingerprint != afterFingerprint {
		count, err := countEmbeddedMemoryChunks()
		if err != nil {
			return fmt.Errorf("check existing embedded memory before switch: %w", err)
		}
		if count > 0 {
			if !configureConfirmMemoryWipe {
				return fmt.Errorf("embedding switch detected with %d embedded memory chunk(s); rerun with --confirm-memory-wipe to wipe memory_chunks and continue", count)
			}
			wipedCount, err = wipeAllMemoryChunks()
			if err != nil {
				return fmt.Errorf("wipe memory for embedding switch: %w", err)
			}
			if err := logEmbeddingSwitchAudit(beforeFingerprint, afterFingerprint, wipedCount); err != nil {
				return fmt.Errorf("write embedding switch audit event: %w", err)
			}
		}
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if configureJSON {
		summary := map[string]any{
			"status":  "ok",
			"command": "configure",
			"result": map[string]any{
				"allowAgents":              cfg.Tools.Subagents.AllowAgents,
				"subagentsMemoryShareMode": cfg.Tools.Subagents.MemoryShareMode,
				"skillsEnabled":            cfg.Skills.Enabled,
				"skillsNodeManager":        cfg.Skills.NodeManager,
				"skillsScope":              cfg.Skills.Scope,
				"kafkaBrokers":             cfg.Group.KafkaBrokers,
				"kafkaSecurityProtocol":    cfg.Group.KafkaSecurityProto,
				"kafkaSaslMechanism":       cfg.Group.KafkaSASLMechanism,
				"kafkaSaslUsername":        cfg.Group.KafkaSASLUsername,
				"kafkaTlsCAFile":           cfg.Group.KafkaTLSCAFile,
				"kafkaTlsCertFile":         cfg.Group.KafkaTLSCertFile,
				"kafkaTlsKeyFile":          cfg.Group.KafkaTLSKeyFile,
				"memoryEmbedding": map[string]any{
					"enabled":   cfg.Memory.Embedding.Enabled,
					"provider":  cfg.Memory.Embedding.Provider,
					"model":     cfg.Memory.Embedding.Model,
					"dimension": cfg.Memory.Embedding.Dimension,
				},
				"memoryWipedChunks": wipedCount,
			},
		}
		b, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		return nil
	}

	state := "(current agent only)"
	if len(cfg.Tools.Subagents.AllowAgents) > 0 {
		state = strings.Join(cfg.Tools.Subagents.AllowAgents, ",")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Updated tools.subagents.allowAgents: %s\n", state)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated tools.subagents.memoryShareMode: %s\n", cfg.Tools.Subagents.MemoryShareMode)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated skills.enabled: %v\n", cfg.Skills.Enabled)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated skills.nodeManager: %s\n", cfg.Skills.NodeManager)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated skills.scope: %s\n", cfg.Skills.Scope)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated group.kafkaBrokers: %s\n", cfg.Group.KafkaBrokers)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated group.kafkaSecurityProtocol: %s\n", cfg.Group.KafkaSecurityProto)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated group.kafkaSaslMechanism: %s\n", cfg.Group.KafkaSASLMechanism)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated group.kafkaSaslUsername: %s\n", cfg.Group.KafkaSASLUsername)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated group.kafkaTlsCAFile: %s\n", cfg.Group.KafkaTLSCAFile)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated group.kafkaTlsCertFile: %s\n", cfg.Group.KafkaTLSCertFile)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated group.kafkaTlsKeyFile: %s\n", cfg.Group.KafkaTLSKeyFile)
	fmt.Fprintf(cmd.OutOrStdout(), "Updated memory.embedding: enabled=%v provider=%s model=%s dimension=%d\n",
		cfg.Memory.Embedding.Enabled,
		cfg.Memory.Embedding.Provider,
		cfg.Memory.Embedding.Model,
		cfg.Memory.Embedding.Dimension,
	)
	if wipedCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Wiped memory_chunks due to embedding switch: %d\n", wipedCount)
	}
	return nil
}

func validateEmbeddingHardGate(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("memory embedding must be configured: config is nil")
	}
	emb := cfg.Memory.Embedding
	providerID := strings.ToLower(strings.TrimSpace(emb.Provider))
	model := strings.TrimSpace(emb.Model)
	if !emb.Enabled || providerID == "" || providerID == "disabled" || model == "" || emb.Dimension <= 0 {
		return fmt.Errorf("memory embedding is mandatory; configure memory.embedding.enabled=true, provider, model, and dimension (or run `kafclaw doctor --fix`)")
	}
	return nil
}

func memoryEmbeddingFingerprint(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	emb := cfg.Memory.Embedding
	providerID := strings.ToLower(strings.TrimSpace(emb.Provider))
	model := strings.TrimSpace(emb.Model)
	if !emb.Enabled || providerID == "" || providerID == "disabled" || model == "" || emb.Dimension <= 0 {
		return ""
	}
	return fmt.Sprintf("%s|%s|%d|%t", providerID, model, emb.Dimension, emb.Normalize)
}

func countEmbeddedMemoryChunks() (int64, error) {
	db, closeFn, err := openTimelineDB()
	if err != nil {
		return 0, err
	}
	defer closeFn()
	var count int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM memory_chunks WHERE embedding IS NOT NULL`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func wipeAllMemoryChunks() (int64, error) {
	db, closeFn, err := openTimelineDB()
	if err != nil {
		return 0, err
	}
	defer closeFn()
	res, err := db.Exec(`DELETE FROM memory_chunks`)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return n, nil
}

func openTimelineDB() (*sql.DB, func(), error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(home, ".kafclaw", "timeline.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, err
	}
	svc, err := timeline.NewTimelineService(path)
	if err != nil {
		return nil, nil, err
	}
	return svc.DB(), func() { _ = svc.Close() }, nil
}

func logEmbeddingSwitchAudit(beforeFingerprint, afterFingerprint string, wipedCount int64) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".kafclaw", "timeline.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	svc, err := timeline.NewTimelineService(path)
	if err != nil {
		return err
	}
	defer svc.Close()
	return svc.AddEvent(&timeline.TimelineEvent{
		EventID:        fmt.Sprintf("MEMORY_EMBED_SWITCH_%d", time.Now().UnixNano()),
		Timestamp:      time.Now(),
		SenderID:       "system",
		SenderName:     "KafClaw",
		EventType:      "SYSTEM",
		ContentText:    fmt.Sprintf("memory embedding switched (%s -> %s), wiped_chunks=%d", beforeFingerprint, afterFingerprint, wipedCount),
		Classification: "MEMORY_EMBEDDING_SWITCH",
		Authorized:     true,
	})
}

func parseCapabilitySelection(raw string, allowed map[string]struct{}) ([]string, error) {
	parts := parseCSVList(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("expected comma-separated capability list")
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.ToLower(strings.TrimSpace(p))
		if v == "" {
			continue
		}
		if _, ok := allowed[v]; !ok {
			return nil, fmt.Errorf("unsupported capability %q", v)
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid capabilities provided")
	}
	return out, nil
}

func parseCSVList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		v := strings.TrimSpace(part)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
