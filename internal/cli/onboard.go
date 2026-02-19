package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/KafClaw/KafClaw/internal/cliconfig"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/identity"
	"github.com/KafClaw/KafClaw/internal/onboarding"
	skillruntime "github.com/KafClaw/KafClaw/internal/skills"
	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Initialize configuration and scaffold workspace",
	RunE:  runOnboard,
}

var onboardForce bool
var onboardSystemd bool
var onboardServiceUser string
var onboardServiceBinary string
var onboardServicePort int
var onboardInstallRoot string
var onboardProfile string
var onboardMode string
var onboardLLMPreset string
var onboardLLMToken string
var onboardLLMAPIBase string
var onboardLLMModel string
var onboardKafkaBrokers string
var onboardKafkaSecurityProtocol string
var onboardKafkaSASLMechanism string
var onboardKafkaSASLUsername string
var onboardKafkaSASLPassword string
var onboardKafkaTLSCAFile string
var onboardKafkaTLSCertFile string
var onboardKafkaTLSKeyFile string
var onboardGroupName string
var onboardAgentID string
var onboardRole string
var onboardRemoteAuth string
var onboardSubMaxSpawnDepth int
var onboardSubMaxChildren int
var onboardSubMaxConcurrent int
var onboardSubArchiveMins int
var onboardSubModel string
var onboardSubThinking string
var onboardSubAllowAgents string
var onboardNonInteractive bool
var onboardAcceptRisk bool
var onboardJSON bool
var onboardSkipSkills bool
var onboardInstallClawhub bool
var onboardSkillsNodeMajor string
var onboardGoogleWorkspaceRead string
var onboardM365Read string
var onboardGatewayPort int
var onboardAllowNonLoopback bool

type onboardingSkillsSummary struct {
	Enabled      bool     `json:"enabled"`
	StateDirsOK  bool     `json:"stateDirsOk"`
	NodeFound    bool     `json:"nodeFound"`
	ClawhubFound bool     `json:"clawhubFound"`
	Remediation  []string `json:"remediation,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

type onboardingSummary struct {
	ConfigPath string                  `json:"configPath"`
	Workspace  string                  `json:"workspace"`
	WorkRepo   string                  `json:"workRepo"`
	Skills     onboardingSkillsSummary `json:"skills"`
}

func init() {
	onboardCmd.Flags().BoolVarP(&onboardForce, "force", "f", false, "Overwrite existing config and soul files")
	onboardCmd.Flags().BoolVar(&onboardNonInteractive, "non-interactive", false, "Run onboarding without prompts")
	onboardCmd.Flags().BoolVar(&onboardAcceptRisk, "accept-risk", false, "Acknowledge risk for non-interactive onboarding")
	onboardCmd.Flags().BoolVar(&onboardJSON, "json", false, "Output onboarding summary as JSON")
	onboardCmd.Flags().StringVar(&onboardProfile, "profile", "", "Preset profile: local | local-kafka | remote")
	onboardCmd.Flags().StringVar(&onboardMode, "mode", "", "Runtime mode: local | local-kafka | remote")
	onboardCmd.Flags().StringVar(&onboardLLMPreset, "llm", "", "LLM setup: cli-token | openai-compatible | skip")
	onboardCmd.Flags().StringVar(&onboardLLMToken, "llm-token", "", "LLM API token")
	onboardCmd.Flags().StringVar(&onboardLLMAPIBase, "llm-api-base", "", "OpenAI-compatible API base (e.g. http://localhost:11434/v1)")
	onboardCmd.Flags().StringVar(&onboardLLMModel, "llm-model", "", "Default model name")
	onboardCmd.Flags().StringVar(&onboardKafkaBrokers, "kafka-brokers", "", "Kafka brokers for local-kafka mode")
	onboardCmd.Flags().StringVar(&onboardKafkaSecurityProtocol, "kafka-security-protocol", "", "Kafka security protocol: PLAINTEXT|SSL|SASL_PLAINTEXT|SASL_SSL")
	onboardCmd.Flags().StringVar(&onboardKafkaSASLMechanism, "kafka-sasl-mechanism", "", "Kafka SASL mechanism: PLAIN|SCRAM-SHA-256|SCRAM-SHA-512")
	onboardCmd.Flags().StringVar(&onboardKafkaSASLUsername, "kafka-sasl-username", "", "Kafka SASL username")
	onboardCmd.Flags().StringVar(&onboardKafkaSASLPassword, "kafka-sasl-password", "", "Kafka SASL password")
	onboardCmd.Flags().StringVar(&onboardKafkaTLSCAFile, "kafka-tls-ca-file", "", "Kafka TLS CA certificate file path")
	onboardCmd.Flags().StringVar(&onboardKafkaTLSCertFile, "kafka-tls-cert-file", "", "Kafka TLS client certificate file path")
	onboardCmd.Flags().StringVar(&onboardKafkaTLSKeyFile, "kafka-tls-key-file", "", "Kafka TLS client key file path")
	onboardCmd.Flags().StringVar(&onboardGroupName, "group-name", "", "Group name for local-kafka mode")
	onboardCmd.Flags().StringVar(&onboardAgentID, "agent-id", "", "Agent ID for local-kafka mode")
	onboardCmd.Flags().StringVar(&onboardRole, "role", "", "Orchestrator role for local-kafka mode")
	onboardCmd.Flags().StringVar(&onboardRemoteAuth, "remote-auth-token", "", "Gateway auth token for remote mode")
	onboardCmd.Flags().IntVar(&onboardSubMaxSpawnDepth, "subagents-max-spawn-depth", 0, "Override subagent max spawn depth")
	onboardCmd.Flags().IntVar(&onboardSubMaxChildren, "subagents-max-children", 0, "Override max active children per parent session")
	onboardCmd.Flags().IntVar(&onboardSubMaxConcurrent, "subagents-max-concurrent", 0, "Override max active subagents globally")
	onboardCmd.Flags().IntVar(&onboardSubArchiveMins, "subagents-archive-minutes", 0, "Override subagent archive-after minutes")
	onboardCmd.Flags().StringVar(&onboardSubAllowAgents, "subagents-allow-agents", "", "Comma-separated agent IDs allowed for sessions_spawn agentId (use '*' for any)")
	onboardCmd.Flags().StringVar(&onboardSubModel, "subagents-model", "", "Default model for spawned subagents")
	onboardCmd.Flags().StringVar(&onboardSubThinking, "subagents-thinking", "", "Default thinking level for spawned subagents")
	onboardCmd.Flags().BoolVar(&onboardSkipSkills, "skip-skills", false, "Skip skills bootstrap")
	onboardCmd.Flags().BoolVar(&onboardInstallClawhub, "install-clawhub", true, "Install clawhub via npm if missing during skills bootstrap")
	onboardCmd.Flags().StringVar(&onboardSkillsNodeMajor, "skills-node-major", "20", "Node major version to pin in .nvmrc for skills")
	onboardCmd.Flags().StringVar(&onboardGoogleWorkspaceRead, "google-workspace-read", "", "Preconfigure google-workspace capabilities (mail,calendar,drive,all)")
	onboardCmd.Flags().StringVar(&onboardM365Read, "m365-read", "", "Preconfigure m365 capabilities (mail,calendar,files,all)")
	onboardCmd.Flags().IntVar(&onboardGatewayPort, "gateway-port", 0, "Gateway API port to persist in config (default: keep current)")
	onboardCmd.Flags().BoolVar(&onboardAllowNonLoopback, "allow-non-loopback", false, "Acknowledge external/LAN gateway bind risk during onboarding")
	onboardCmd.Flags().BoolVar(&onboardSystemd, "systemd", false, "Install systemd service + override + env file (Linux)")
	onboardCmd.Flags().StringVar(&onboardServiceUser, "service-user", "kafclaw", "Service user for systemd setup")
	onboardCmd.Flags().StringVar(&onboardServiceBinary, "service-binary", "/usr/local/bin/kafclaw", "kafclaw binary path for systemd ExecStart")
	onboardCmd.Flags().IntVar(&onboardServicePort, "service-port", 18790, "Gateway port for systemd ExecStart")
	onboardCmd.Flags().StringVar(&onboardInstallRoot, "service-install-root", "/", "Installation root for systemd files (testing/packaging)")
	_ = onboardCmd.Flags().MarkHidden("service-install-root")
	rootCmd.AddCommand(onboardCmd)
}

func runOnboard(cmd *cobra.Command, args []string) error {
	_ = emitLifecycleEvent("onboard", "start", "info", "onboarding started", map[string]any{
		"nonInteractive": onboardNonInteractive,
		"profile":        strings.TrimSpace(onboardProfile),
		"mode":           strings.TrimSpace(onboardMode),
	})
	if onboardNonInteractive && !onboardAcceptRisk {
		_ = emitLifecycleEvent("onboard", "validate", "error", "missing --accept-risk for non-interactive mode", nil)
		return fmt.Errorf("non-interactive onboarding requires --accept-risk")
	}
	if err := validateOnboardNonInteractiveFlags(); err != nil {
		_ = emitLifecycleEvent("onboard", "validate", "error", err.Error(), nil)
		return err
	}

	printHeader("🚀 KafClaw Onboard")
	fmt.Println("Initializing KafClaw...")

	cfgPath, _ := config.ConfigPath()
	configExists := false
	cfg := config.DefaultConfig()
	if _, err := os.Stat(cfgPath); err == nil {
		configExists = true
		loaded, loadErr := config.Load()
		if loadErr != nil {
			fmt.Printf("Config warning: %v (using defaults)\n", loadErr)
		} else if loaded != nil {
			cfg = loaded
		}
		fmt.Printf("Config exists at: %s\n", cfgPath)
		fmt.Println("Onboarding will update selected fields; existing settings are kept unless changed.")
	} else {
		fmt.Printf("Config will be created at: %s\n", cfgPath)
	}

	if err := onboarding.RunProfileWizard(cfg, cmd.InOrStdin(), cmd.OutOrStdout(), onboarding.WizardParams{
		Profile:          onboardProfile,
		Mode:             onboardMode,
		LLMPreset:        onboardLLMPreset,
		LLMToken:         onboardLLMToken,
		LLMAPIBase:       onboardLLMAPIBase,
		LLMModel:         onboardLLMModel,
		KafkaBrokers:     onboardKafkaBrokers,
		KafkaSecurity:    onboardKafkaSecurityProtocol,
		KafkaSASLMech:    onboardKafkaSASLMechanism,
		KafkaSASLUser:    onboardKafkaSASLUsername,
		KafkaSASLPass:    onboardKafkaSASLPassword,
		KafkaTLSCAFile:   onboardKafkaTLSCAFile,
		KafkaTLSCertFile: onboardKafkaTLSCertFile,
		KafkaTLSKeyFile:  onboardKafkaTLSKeyFile,
		GroupName:        onboardGroupName,
		AgentID:          onboardAgentID,
		Role:             onboardRole,
		RemoteAuth:       onboardRemoteAuth,
		SubMaxSpawnDepth: onboardSubMaxSpawnDepth,
		SubMaxChildren:   onboardSubMaxChildren,
		SubMaxConcurrent: onboardSubMaxConcurrent,
		SubArchiveMins:   onboardSubArchiveMins,
		SubAllowAgents:   onboardSubAllowAgents,
		SubModel:         onboardSubModel,
		SubThinking:      onboardSubThinking,
		NonInteractive:   onboardNonInteractive,
	}); err != nil {
		_ = emitLifecycleEvent("onboard", "wizard", "error", err.Error(), nil)
		return fmt.Errorf("onboarding wizard error: %w", err)
	}

	if onboardGatewayPort > 0 {
		cfg.Gateway.Port = onboardGatewayPort
	}
	if cfg.Gateway.Port <= 0 {
		cfg.Gateway.Port = 18790
	}
	if cfg.Gateway.DashboardPort <= 0 {
		cfg.Gateway.DashboardPort = 18791
	}

	if err := preflightOnboardingConfig(cfg, cfgPath); err != nil {
		_ = emitLifecycleEvent("onboard", "preflight", "error", err.Error(), nil)
		return err
	}
	_ = emitLifecycleEvent("onboard", "preflight", "ok", "onboarding preflight passed", map[string]any{
		"gatewayHost": cfg.Gateway.Host,
		"gatewayPort": cfg.Gateway.Port,
	})

	fmt.Fprintln(cmd.OutOrStdout(), onboarding.BuildProfileSummary(cfg))
	if !onboardNonInteractive && (configExists || !onboardForce) {
		confirmReader := bufio.NewReader(cmd.InOrStdin())
		ok, err := onboarding.ConfirmApply(confirmReader, cmd.OutOrStdout())
		if err != nil {
			return fmt.Errorf("onboarding confirmation error: %w", err)
		}
		if !ok {
			fmt.Println("Onboarding aborted before writing config or scaffolding files.")
			return nil
		}
	}
	if err := enforceGatewayBindSafety(cfg, cmd); err != nil {
		_ = emitLifecycleEvent("onboard", "bind-safety", "error", err.Error(), map[string]any{
			"gatewayHost": cfg.Gateway.Host,
		})
		return err
	}
	if err := config.Save(cfg); err != nil {
		_ = emitLifecycleEvent("onboard", "save-config", "error", err.Error(), nil)
		return fmt.Errorf("error saving configured onboarding profile: %w", err)
	}
	_ = emitLifecycleEvent("onboard", "save-config", "ok", "configuration saved", map[string]any{
		"configPath": cfgPath,
	})
	fmt.Printf("Updated configuration at: %s\n", cfgPath)

	fmt.Printf("\nWorkspace: %s\n", cfg.Paths.Workspace)
	result, err := identity.ScaffoldWorkspace(cfg.Paths.Workspace, onboardForce)
	if err != nil {
		fmt.Printf("Error scaffolding workspace: %v\n", err)
	} else {
		for _, name := range result.Created {
			fmt.Printf("  + %s\n", name)
		}
		for _, name := range result.Skipped {
			fmt.Printf("  ~ %s (exists, skipped)\n", name)
		}
		for _, e := range result.Errors {
			fmt.Printf("  ! %s\n", e)
		}
	}

	if warn, err := config.EnsureWorkRepo(cfg.Paths.WorkRepoPath); err != nil {
		fmt.Printf("Work repo error: %v\n", err)
	} else {
		fmt.Printf("\nWork repo: %s\n", cfg.Paths.WorkRepoPath)
		if warn != "" {
			fmt.Printf("  Warning: %s\n", warn)
		}
	}

	skillsSummary := onboardingSkillsSummary{
		Enabled:      cfg.Skills.Enabled,
		NodeFound:    skillruntime.HasBinary("node"),
		ClawhubFound: skillruntime.HasBinary("clawhub"),
	}
	hasCapabilitySelection := strings.TrimSpace(onboardGoogleWorkspaceRead) != "" || strings.TrimSpace(onboardM365Read) != ""
	if hasCapabilitySelection {
		if cfg.Skills.Entries == nil {
			cfg.Skills.Entries = map[string]config.SkillEntryConfig{}
		}
		if strings.TrimSpace(onboardGoogleWorkspaceRead) != "" {
			caps, err := parseCapabilitySelection(onboardGoogleWorkspaceRead, map[string]struct{}{
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
		if strings.TrimSpace(onboardM365Read) != "" {
			caps, err := parseCapabilitySelection(onboardM365Read, map[string]struct{}{
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
	}
	if onboardSkipSkills {
		fmt.Println("\nSkills bootstrap: skipped (--skip-skills)")
	} else {
		enableSkills := onboardNonInteractive
		if hasCapabilitySelection {
			enableSkills = true
		}
		if !onboardNonInteractive {
			fmt.Print("\nEnable skills now? [Y/n]: ")
			confirmReader := bufio.NewReader(cmd.InOrStdin())
			line, _ := confirmReader.ReadString('\n')
			answer := strings.TrimSpace(strings.ToLower(line))
			enableSkills = answer == "" || answer == "y" || answer == "yes"
		}
		if enableSkills {
			cfg.Skills.Enabled = true
			skillsSummary.Enabled = true
			if cfg.Skills.NodeManager == "" {
				cfg.Skills.NodeManager = "npm"
			}
			if err := config.Save(cfg); err != nil {
				fmt.Printf("Skills bootstrap warning: failed saving config: %v\n", err)
				skillsSummary.Warnings = append(skillsSummary.Warnings, err.Error())
			}
			if _, err := skillruntime.EnsureStateDirs(); err != nil {
				fmt.Printf("Skills bootstrap warning: failed creating secure dirs: %v\n", err)
				skillsSummary.Warnings = append(skillsSummary.Warnings, err.Error())
			} else {
				skillsSummary.StateDirsOK = true
			}
			if _, err := skillruntime.EnsureNVMRC(cfg.Paths.WorkRepoPath, onboardSkillsNodeMajor); err != nil {
				fmt.Printf("Skills bootstrap warning: failed writing .nvmrc: %v\n", err)
				skillsSummary.Warnings = append(skillsSummary.Warnings, err.Error())
			}
			skillsSummary.NodeFound = skillruntime.HasBinary("node")
			if !skillsSummary.NodeFound {
				fmt.Println("Skills bootstrap warning: node not found in PATH; install Node.js and run 'kafclaw skills enable'.")
				skillsSummary.Remediation = append(skillsSummary.Remediation, "Install Node.js and rerun `kafclaw skills enable`.")
			} else if err := skillruntime.EnsureClawhub(onboardInstallClawhub); err != nil {
				fmt.Printf("Skills bootstrap warning: %v\n", err)
				skillsSummary.Warnings = append(skillsSummary.Warnings, err.Error())
				skillsSummary.Remediation = append(skillsSummary.Remediation, "Install clawhub (`npm install -g --ignore-scripts clawhub`) or rerun with --install-clawhub.")
			}
			skillsSummary.ClawhubFound = skillruntime.HasBinary("clawhub")
			fmt.Println("Skills bootstrap complete. Use 'kafclaw skills list' to inspect status.")
		} else {
			fmt.Println("\nSkills bootstrap: not enabled")
		}
	}
	if hasCapabilitySelection {
		if err := config.Save(cfg); err != nil {
			fmt.Printf("Skills capability configuration warning: failed saving config: %v\n", err)
			skillsSummary.Warnings = append(skillsSummary.Warnings, err.Error())
		}
	}

	fmt.Println("\nNext steps:")
	fmt.Println("1. Review ~/.kafclaw/config.json and ~/.config/kafclaw/env.")
	fmt.Println("2. Customize soul files in your workspace (SOUL.md, USER.md, etc.)")
	fmt.Println("3. Run 'kafclaw agent -m \"hello\"' to test.")

	if onboardJSON {
		summary := onboardingSummary{
			ConfigPath: cfgPath,
			Workspace:  cfg.Paths.Workspace,
			WorkRepo:   cfg.Paths.WorkRepoPath,
			Skills:     skillsSummary,
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
	}

	if onboardSystemd {
		if runtime.GOOS != "linux" {
			fmt.Println("\nSystemd setup is only supported on Linux.")
			return nil
		}
		servicePort := onboardServicePort
		if !cmd.Flags().Changed("service-port") && cfg.Gateway.Port > 0 {
			servicePort = cfg.Gateway.Port
		}
		result, err := onboarding.SetupSystemdGateway(onboarding.SetupOptions{
			ServiceUser: onboardServiceUser,
			BinaryPath:  onboardServiceBinary,
			Port:        servicePort,
			Profile:     "default",
			Version:     version,
			InstallRoot: onboardInstallRoot,
		})
		if err != nil {
			_ = emitLifecycleEvent("onboard", "systemd", "error", err.Error(), nil)
			fmt.Printf("\nSystemd setup failed: %v\n", err)
			return nil
		}

		fmt.Println("\nSystemd setup complete:")
		if result.UserCreated {
			fmt.Printf("  + Created user: %s\n", onboardServiceUser)
		}
		fmt.Printf("  + Service unit: %s\n", result.ServicePath)
		fmt.Printf("  + Override file: %s\n", result.OverridePath)
		fmt.Printf("  + Env file: %s\n", result.EnvPath)
		fmt.Printf("  + Service port: %d\n", servicePort)
		fmt.Println("  Next (as root): systemctl daemon-reload && systemctl enable --now kafclaw-gateway.service")
		_ = emitLifecycleEvent("onboard", "systemd", "ok", "systemd setup completed", map[string]any{
			"servicePort": servicePort,
			"serviceUser": onboardServiceUser,
		})
	}
	printPostOnboardingReadiness(cmd, cfg)
	_ = emitLifecycleEvent("onboard", "complete", "ok", "onboarding completed", map[string]any{
		"gatewayHost": cfg.Gateway.Host,
		"gatewayPort": cfg.Gateway.Port,
	})
	return nil
}

func validateOnboardNonInteractiveFlags() error {
	if !onboardNonInteractive {
		return nil
	}
	if strings.TrimSpace(onboardMode) == "" && strings.TrimSpace(onboardProfile) == "" {
		return fmt.Errorf("non-interactive onboarding requires explicit mode/profile (--mode or --profile)")
	}
	if strings.TrimSpace(onboardLLMPreset) == "" {
		return fmt.Errorf("non-interactive onboarding requires explicit llm setup (--llm)")
	}
	return nil
}

func preflightOnboardingConfig(cfg *config.Config, cfgPath string) error {
	if cfg == nil {
		return fmt.Errorf("onboarding preflight failed: nil config")
	}
	if strings.TrimSpace(cfg.Paths.Workspace) == "" {
		return fmt.Errorf("onboarding preflight failed: workspace path is empty")
	}
	if cfg.Gateway.Port <= 0 || cfg.Gateway.Port > 65535 {
		return fmt.Errorf("onboarding preflight failed: gateway.port must be between 1 and 65535")
	}
	if cfg.Gateway.DashboardPort <= 0 || cfg.Gateway.DashboardPort > 65535 {
		return fmt.Errorf("onboarding preflight failed: gateway.dashboardPort must be between 1 and 65535")
	}
	if cfg.Gateway.Port == cfg.Gateway.DashboardPort {
		return fmt.Errorf("onboarding preflight failed: gateway.port and gateway.dashboardPort must differ")
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		return fmt.Errorf("onboarding preflight failed: cannot create config directory: %w", err)
	}
	if err := os.MkdirAll(cfg.Paths.Workspace, 0o755); err != nil {
		return fmt.Errorf("onboarding preflight failed: cannot access workspace path %q: %w", cfg.Paths.Workspace, err)
	}
	return nil
}

func enforceGatewayBindSafety(cfg *config.Config, cmd *cobra.Command) error {
	if cfg == nil || isLoopbackHost(cfg.Gateway.Host) {
		return nil
	}
	if strings.TrimSpace(cfg.Gateway.AuthToken) == "" {
		return fmt.Errorf("non-loopback gateway bind (%s) requires gateway auth token", cfg.Gateway.Host)
	}
	if onboardAllowNonLoopback {
		fmt.Fprintf(cmd.OutOrStdout(), "Gateway bind acknowledged for non-loopback host %s.\n", cfg.Gateway.Host)
		printNetworkEncryptionRecommendation(cmd)
		return nil
	}
	if onboardNonInteractive {
		return fmt.Errorf("non-loopback gateway bind (%s) requires explicit acknowledgement: pass --allow-non-loopback", cfg.Gateway.Host)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nGateway bind is non-loopback (%s).\n", cfg.Gateway.Host)
	printNetworkEncryptionRecommendation(cmd)
	fmt.Fprint(cmd.OutOrStdout(), "Continue with non-loopback bind? [y/N]: ")
	reader := bufio.NewReader(cmd.InOrStdin())
	line, _ := reader.ReadString('\n')
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("onboarding aborted: non-loopback bind not acknowledged")
	}
	return nil
}

func printNetworkEncryptionRecommendation(cmd *cobra.Command) {
	fmt.Fprintln(cmd.OutOrStdout(), "Recommended for non-local exposure: TLS termination (reverse proxy), or private networking (Tailscale/SSH tunnel).")
}

func isLoopbackHost(host string) bool {
	normalized := strings.TrimSpace(strings.ToLower(host))
	return normalized == "127.0.0.1" || normalized == "localhost" || normalized == "::1"
}

func printPostOnboardingReadiness(cmd *cobra.Command, cfg *config.Config) {
	fmt.Fprintln(cmd.OutOrStdout(), "\nReadiness gates:")
	report, err := cliconfig.RunDoctorWithOptions(cliconfig.DoctorOptions{})
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "- doctor: warning (%v)\n", err)
	} else {
		failures := 0
		for _, check := range report.Checks {
			if check.Status == cliconfig.DoctorFail {
				failures++
			}
		}
		if failures > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "- doctor: %d failing check(s) (run 'kafclaw doctor')\n", failures)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "- doctor: ok")
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "- provider: review with 'kafclaw status'")
	if cfg != nil && cfg.Channels.WhatsApp.Enabled {
		fmt.Fprintln(cmd.OutOrStdout(), "- whatsapp auth: run 'kafclaw whatsapp-auth list'")
	}
	if cfg != nil && (cfg.Channels.Slack.Enabled || cfg.Channels.MSTeams.Enabled) {
		fmt.Fprintln(cmd.OutOrStdout(), "- channel pairing: run 'kafclaw pairing pending'")
	}
}
