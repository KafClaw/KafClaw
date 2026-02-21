package onboarding

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
)

type RuntimeMode string

const (
	ModeLocal      RuntimeMode = "local"
	ModeLocalKafka RuntimeMode = "local-kafka"
	ModeRemote     RuntimeMode = "remote"
)

type LLMPreset string

const (
	LLMPresetSkip             LLMPreset = "skip"
	LLMPresetCLIToken         LLMPreset = "cli-token"
	LLMPresetOpenAICompatible LLMPreset = "openai-compatible"
	LLMPresetClaude           LLMPreset = "claude"
	LLMPresetGemini           LLMPreset = "gemini"
	LLMPresetGeminiCLI        LLMPreset = "gemini-cli"
	LLMPresetCodex            LLMPreset = "openai-codex"
	LLMPresetXAI              LLMPreset = "xai"
	LLMPresetScalyticsCopilot LLMPreset = "scalytics-copilot"
	LLMPresetOpenRouter       LLMPreset = "openrouter"
	LLMPresetDeepSeek         LLMPreset = "deepseek"
	LLMPresetGroq             LLMPreset = "groq"
)

type WizardParams struct {
	Mode             string
	Profile          string
	LLMPreset        string
	LLMToken         string
	LLMAPIBase       string
	LLMModel         string
	KafkaBrokers     string
	KafkaSecurity    string
	KafkaSASLMech    string
	KafkaSASLUser    string
	KafkaSASLPass    string
	KafkaTLSCAFile   string
	KafkaTLSCertFile string
	KafkaTLSKeyFile  string
	GroupName        string
	AgentID          string
	Role             string
	RemoteAuth       string
	SubMaxSpawnDepth int
	SubMaxChildren   int
	SubMaxConcurrent int
	SubArchiveMins   int
	SubModel         string
	SubThinking      string
	SubAllowAgents   string
	NonInteractive   bool
}

func RunProfileWizard(cfg *config.Config, in io.Reader, out io.Writer, p WizardParams) error {
	reader := bufio.NewReader(in)

	mode, err := resolveMode(reader, out, p)
	if err != nil {
		return err
	}
	if err := applyMode(cfg, mode, p); err != nil {
		return err
	}
	applySubagentTuning(cfg, p)

	preset, err := resolveLLMPreset(reader, out, p)
	if err != nil {
		return err
	}
	if err := applyLLM(cfg, preset, reader, out, p); err != nil {
		return err
	}
	return nil
}

func applySubagentTuning(cfg *config.Config, p WizardParams) {
	if p.SubMaxSpawnDepth > 0 {
		cfg.Tools.Subagents.MaxSpawnDepth = p.SubMaxSpawnDepth
	}
	if p.SubMaxChildren > 0 {
		cfg.Tools.Subagents.MaxChildrenPerAgent = p.SubMaxChildren
	}
	if p.SubMaxConcurrent > 0 {
		cfg.Tools.Subagents.MaxConcurrent = p.SubMaxConcurrent
	}
	if p.SubArchiveMins > 0 {
		cfg.Tools.Subagents.ArchiveAfterMinutes = p.SubArchiveMins
	}
	if strings.TrimSpace(p.SubModel) != "" {
		cfg.Tools.Subagents.Model = strings.TrimSpace(p.SubModel)
	}
	if strings.TrimSpace(p.SubThinking) != "" {
		cfg.Tools.Subagents.Thinking = strings.TrimSpace(p.SubThinking)
	}
	if strings.TrimSpace(p.SubAllowAgents) != "" {
		cfg.Tools.Subagents.AllowAgents = parseCSV(p.SubAllowAgents)
	}
}

func resolveMode(reader *bufio.Reader, out io.Writer, p WizardParams) (RuntimeMode, error) {
	mode := normalizeMode(firstNonEmpty(strings.TrimSpace(p.Profile), strings.TrimSpace(p.Mode)))
	if mode != "" {
		return mode, nil
	}
	if p.NonInteractive {
		return ModeLocal, nil
	}

	fmt.Fprintln(out, "\nSelect runtime mode:")
	fmt.Fprintln(out, "1) local (personal assistant)")
	fmt.Fprintln(out, "2) local+kafka (group/orchestrator on local Kafka)")
	fmt.Fprintln(out, "3) remote (headless gateway)")
	choice, err := prompt(reader, out, "Mode [1/2/3]", "1")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(choice) {
	case "1":
		return ModeLocal, nil
	case "2":
		return ModeLocalKafka, nil
	case "3":
		return ModeRemote, nil
	default:
		return "", fmt.Errorf("invalid mode choice: %s", choice)
	}
}

func resolveLLMPreset(reader *bufio.Reader, out io.Writer, p WizardParams) (LLMPreset, error) {
	preset := normalizeLLMPreset(p.LLMPreset)
	if preset != "" {
		return preset, nil
	}
	if p.NonInteractive {
		return LLMPresetSkip, nil
	}
	fmt.Fprintln(out, "\nSelect LLM provider:")
	fmt.Fprintln(out, " 1) claude          (Anthropic API key)")
	fmt.Fprintln(out, " 2) openai          (OpenAI API key)")
	fmt.Fprintln(out, " 3) gemini          (Google Gemini API key)")
	fmt.Fprintln(out, " 4) gemini-cli      (Google Gemini via OAuth CLI)")
	fmt.Fprintln(out, " 5) openai-codex    (OpenAI Codex via OAuth CLI)")
	fmt.Fprintln(out, " 6) xai             (xAI/Grok API key)")
	fmt.Fprintln(out, " 7) openrouter      (OpenRouter API key)")
	fmt.Fprintln(out, " 8) deepseek        (DeepSeek API key)")
	fmt.Fprintln(out, " 9) groq            (Groq API key)")
	fmt.Fprintln(out, "10) scalytics-copilot (Scalytics Copilot API key + URL)")
	fmt.Fprintln(out, "11) openai-compatible (vLLM/Ollama/custom endpoint)")
	fmt.Fprintln(out, "12) skip            (keep current)")
	choice, err := prompt(reader, out, "LLM provider [1-12]", "12")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(choice) {
	case "1":
		return LLMPresetClaude, nil
	case "2":
		return LLMPresetCLIToken, nil
	case "3":
		return LLMPresetGemini, nil
	case "4":
		return LLMPresetGeminiCLI, nil
	case "5":
		return LLMPresetCodex, nil
	case "6":
		return LLMPresetXAI, nil
	case "7":
		return LLMPresetOpenRouter, nil
	case "8":
		return LLMPresetDeepSeek, nil
	case "9":
		return LLMPresetGroq, nil
	case "10":
		return LLMPresetScalyticsCopilot, nil
	case "11":
		return LLMPresetOpenAICompatible, nil
	case "12":
		return LLMPresetSkip, nil
	default:
		return "", fmt.Errorf("invalid llm provider choice: %s", choice)
	}
}

func applyMode(cfg *config.Config, mode RuntimeMode, p WizardParams) error {
	switch mode {
	case ModeLocal:
		cfg.Gateway.Host = "127.0.0.1"
		cfg.Group.Enabled = false
		cfg.Orchestrator.Enabled = false
		cfg.Gateway.AuthToken = ""
		return nil
	case ModeLocalKafka:
		cfg.Gateway.Host = "127.0.0.1"
		cfg.Group.Enabled = true
		cfg.Orchestrator.Enabled = true
		cfg.Group.KafkaBrokers = firstNonEmpty(strings.TrimSpace(p.KafkaBrokers), "localhost:9092")
		if err := applyKafkaSecurity(cfg, p); err != nil {
			return err
		}
		cfg.Group.GroupName = firstNonEmpty(strings.TrimSpace(p.GroupName), "kafclaw")
		cfg.Group.AgentID = firstNonEmpty(strings.TrimSpace(p.AgentID), defaultAgentID())
		cfg.Group.ConsumerGroup = firstNonEmpty(strings.TrimSpace(cfg.Group.ConsumerGroup), cfg.Group.GroupName+"-workers")
		cfg.Orchestrator.Role = firstNonEmpty(strings.TrimSpace(p.Role), "worker")
		cfg.Gateway.AuthToken = ""
		return nil
	case ModeRemote:
		cfg.Gateway.Host = "0.0.0.0"
		cfg.Group.Enabled = false
		cfg.Orchestrator.Enabled = false
		token := strings.TrimSpace(p.RemoteAuth)
		if token == "" {
			token = cfg.Gateway.AuthToken
		}
		if strings.TrimSpace(token) == "" {
			generated, err := generateToken()
			if err != nil {
				return err
			}
			token = generated
		}
		cfg.Gateway.AuthToken = token
		return nil
	default:
		return fmt.Errorf("unsupported runtime mode: %s", mode)
	}
}

func applyKafkaSecurity(cfg *config.Config, p WizardParams) error {
	rawRequested := strings.TrimSpace(p.KafkaSecurity)
	securityProtocol := normalizeKafkaSecurityProtocol(firstNonEmpty(rawRequested, cfg.Group.KafkaSecurityProto))
	if rawRequested != "" && securityProtocol == "" {
		return fmt.Errorf("unsupported kafka security protocol: %s", rawRequested)
	}
	if securityProtocol == "" {
		securityProtocol = "PLAINTEXT"
	}
	if !isAllowedKafkaSecurityProtocol(securityProtocol) {
		return fmt.Errorf("unsupported kafka security protocol: %s", securityProtocol)
	}
	cfg.Group.KafkaSecurityProto = securityProtocol

	saslMech := strings.ToUpper(strings.TrimSpace(firstNonEmpty(p.KafkaSASLMech, cfg.Group.KafkaSASLMechanism)))
	if saslMech != "" && !isAllowedKafkaSASLMechanism(saslMech) {
		return fmt.Errorf("unsupported kafka sasl mechanism: %s", saslMech)
	}
	cfg.Group.KafkaSASLMechanism = saslMech
	cfg.Group.KafkaSASLUsername = strings.TrimSpace(firstNonEmpty(p.KafkaSASLUser, cfg.Group.KafkaSASLUsername))
	cfg.Group.KafkaSASLPassword = strings.TrimSpace(firstNonEmpty(p.KafkaSASLPass, cfg.Group.KafkaSASLPassword))
	cfg.Group.KafkaTLSCAFile = strings.TrimSpace(firstNonEmpty(p.KafkaTLSCAFile, cfg.Group.KafkaTLSCAFile))
	cfg.Group.KafkaTLSCertFile = strings.TrimSpace(firstNonEmpty(p.KafkaTLSCertFile, cfg.Group.KafkaTLSCertFile))
	cfg.Group.KafkaTLSKeyFile = strings.TrimSpace(firstNonEmpty(p.KafkaTLSKeyFile, cfg.Group.KafkaTLSKeyFile))

	if strings.HasPrefix(securityProtocol, "SASL_") {
		if cfg.Group.KafkaSASLMechanism == "" || cfg.Group.KafkaSASLUsername == "" || cfg.Group.KafkaSASLPassword == "" {
			return fmt.Errorf("kafka sasl requires mechanism, username, and password")
		}
	}
	return nil
}

func applyLLM(cfg *config.Config, preset LLMPreset, reader *bufio.Reader, out io.Writer, p WizardParams) error {
	switch preset {
	case LLMPresetSkip:
		return nil

	case LLMPresetCLIToken:
		token := strings.TrimSpace(p.LLMToken)
		model := strings.TrimSpace(p.LLMModel)
		if token == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "OpenAI API key", os.Getenv("OPENAI_API_KEY"))
			if err != nil {
				return err
			}
			token = strings.TrimSpace(val)
		}
		if model == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "Model", "openai/gpt-4.1")
			if err != nil {
				return err
			}
			model = strings.TrimSpace(val)
		}
		cfg.Providers.OpenAI.APIKey = token
		if model != "" {
			cfg.Model.Name = model
		}
		return nil

	case LLMPresetClaude:
		return applyAPIKeyPreset(cfg, reader, out, p, applyAPIKeyOpts{
			providerLabel: "Claude/Anthropic",
			envVar:        "ANTHROPIC_API_KEY",
			defaultModel:  "claude/claude-sonnet-4-5-20250514",
			setKey:        func(c *config.Config, k string) { c.Providers.Anthropic.APIKey = k },
			modelPrefix:   "claude",
		})

	case LLMPresetGemini:
		return applyAPIKeyPreset(cfg, reader, out, p, applyAPIKeyOpts{
			providerLabel: "Gemini",
			envVar:        "GEMINI_API_KEY",
			defaultModel:  "gemini/gemini-2.5-pro",
			setKey:        func(c *config.Config, k string) { c.Providers.Gemini.APIKey = k },
			modelPrefix:   "gemini",
		})

	case LLMPresetGeminiCLI:
		cfg.Model.Name = firstNonEmpty(strings.TrimSpace(p.LLMModel), "gemini-cli/gemini-2.5-pro")
		fmt.Fprintln(out, "Gemini CLI uses OAuth — run 'kafclaw models auth login --provider gemini' to authenticate.")
		return nil

	case LLMPresetCodex:
		cfg.Model.Name = firstNonEmpty(strings.TrimSpace(p.LLMModel), "openai-codex/gpt-5.3-codex")
		fmt.Fprintln(out, "Codex uses OAuth — run 'kafclaw models auth login --provider openai-codex' to authenticate.")
		return nil

	case LLMPresetXAI:
		return applyAPIKeyPreset(cfg, reader, out, p, applyAPIKeyOpts{
			providerLabel: "xAI/Grok",
			envVar:        "XAI_API_KEY",
			defaultModel:  "xai/grok-3",
			setKey:        func(c *config.Config, k string) { c.Providers.XAI.APIKey = k },
			modelPrefix:   "xai",
		})

	case LLMPresetOpenRouter:
		return applyAPIKeyPreset(cfg, reader, out, p, applyAPIKeyOpts{
			providerLabel: "OpenRouter",
			envVar:        "OPENROUTER_API_KEY",
			defaultModel:  "openrouter/anthropic/claude-sonnet-4-5",
			setKey:        func(c *config.Config, k string) { c.Providers.OpenRouter.APIKey = k },
			modelPrefix:   "openrouter",
		})

	case LLMPresetDeepSeek:
		return applyAPIKeyPreset(cfg, reader, out, p, applyAPIKeyOpts{
			providerLabel: "DeepSeek",
			envVar:        "DEEPSEEK_API_KEY",
			defaultModel:  "deepseek/deepseek-chat",
			setKey:        func(c *config.Config, k string) { c.Providers.DeepSeek.APIKey = k },
			modelPrefix:   "deepseek",
		})

	case LLMPresetGroq:
		return applyAPIKeyPreset(cfg, reader, out, p, applyAPIKeyOpts{
			providerLabel: "Groq",
			envVar:        "GROQ_API_KEY",
			defaultModel:  "groq/llama-3.3-70b-versatile",
			setKey:        func(c *config.Config, k string) { c.Providers.Groq.APIKey = k },
			modelPrefix:   "groq",
		})

	case LLMPresetScalyticsCopilot:
		token := strings.TrimSpace(p.LLMToken)
		base := strings.TrimSpace(p.LLMAPIBase)
		model := strings.TrimSpace(p.LLMModel)
		if token == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "Scalytics Copilot API key", "")
			if err != nil {
				return err
			}
			token = strings.TrimSpace(val)
		}
		if base == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "Scalytics Copilot API base URL", "https://copilot.scalytics.io/v1")
			if err != nil {
				return err
			}
			base = strings.TrimSpace(val)
		}
		if base == "" {
			return fmt.Errorf("scalytics-copilot requires API base URL")
		}
		if model == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "Model", "scalytics-copilot/default")
			if err != nil {
				return err
			}
			model = strings.TrimSpace(val)
		}
		cfg.Providers.ScalyticsCopilot.APIKey = token
		cfg.Providers.ScalyticsCopilot.APIBase = base
		if model != "" {
			cfg.Model.Name = model
		}
		return nil

	case LLMPresetOpenAICompatible:
		token := strings.TrimSpace(p.LLMToken)
		base := strings.TrimSpace(p.LLMAPIBase)
		model := strings.TrimSpace(p.LLMModel)
		if base == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "OpenAI-compatible API base", "http://localhost:11434/v1")
			if err != nil {
				return err
			}
			base = strings.TrimSpace(val)
		}
		if base == "" {
			return fmt.Errorf("openai-compatible setup requires api base")
		}
		if token == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "API token (optional)", "")
			if err != nil {
				return err
			}
			token = strings.TrimSpace(val)
		}
		if model == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "Model", cfg.Model.Name)
			if err != nil {
				return err
			}
			model = strings.TrimSpace(val)
		}
		cfg.Providers.OpenAI.APIBase = base
		cfg.Providers.OpenAI.APIKey = token
		if model != "" {
			cfg.Model.Name = model
		}
		return nil

	default:
		return fmt.Errorf("unsupported llm preset: %s", preset)
	}
}

// applyAPIKeyOpts holds parameters for the common API-key preset pattern.
type applyAPIKeyOpts struct {
	providerLabel string
	envVar        string
	defaultModel  string
	setKey        func(*config.Config, string)
	modelPrefix   string
}

// applyAPIKeyPreset handles the common flow: prompt for API key, prompt for model, set config.
func applyAPIKeyPreset(cfg *config.Config, reader *bufio.Reader, out io.Writer, p WizardParams, opts applyAPIKeyOpts) error {
	token := strings.TrimSpace(p.LLMToken)
	model := strings.TrimSpace(p.LLMModel)
	if token == "" && !p.NonInteractive {
		val, err := prompt(reader, out, opts.providerLabel+" API key", os.Getenv(opts.envVar))
		if err != nil {
			return err
		}
		token = strings.TrimSpace(val)
	}
	if model == "" && !p.NonInteractive {
		val, err := prompt(reader, out, "Model", opts.defaultModel)
		if err != nil {
			return err
		}
		model = strings.TrimSpace(val)
	}
	if token != "" {
		opts.setKey(cfg, token)
	}
	if model != "" {
		cfg.Model.Name = model
	}
	return nil
}

func prompt(r *bufio.Reader, out io.Writer, label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}
	line, err := r.ReadString('\n')
	if err != nil && !errorsIsEOF(err) {
		return "", err
	}
	val := strings.TrimSpace(line)
	if val == "" {
		return def, nil
	}
	return val, nil
}

func normalizeMode(v string) RuntimeMode {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "local":
		return ModeLocal
	case "local+kafka", "local-kafka", "kafka-local":
		return ModeLocalKafka
	case "remote", "remote-gateway":
		return ModeRemote
	default:
		return ""
	}
}

func normalizeLLMPreset(v string) LLMPreset {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "", "auto":
		return ""
	case "skip":
		return LLMPresetSkip
	case "cli-token", "token", "openai":
		return LLMPresetCLIToken
	case "openai-compatible", "compatible", "vllm", "ollama":
		return LLMPresetOpenAICompatible
	case "claude", "anthropic":
		return LLMPresetClaude
	case "gemini", "google":
		return LLMPresetGemini
	case "gemini-cli":
		return LLMPresetGeminiCLI
	case "openai-codex", "codex":
		return LLMPresetCodex
	case "xai", "grok":
		return LLMPresetXAI
	case "scalytics-copilot", "copilot":
		return LLMPresetScalyticsCopilot
	case "openrouter":
		return LLMPresetOpenRouter
	case "deepseek":
		return LLMPresetDeepSeek
	case "groq":
		return LLMPresetGroq
	default:
		return ""
	}
}

func normalizeKafkaSecurityProtocol(v string) string {
	switch strings.TrimSpace(strings.ToUpper(v)) {
	case "PLAINTEXT":
		return "PLAINTEXT"
	case "SSL":
		return "SSL"
	case "SASL_PLAINTEXT":
		return "SASL_PLAINTEXT"
	case "SASL_SSL":
		return "SASL_SSL"
	default:
		return ""
	}
}

func isAllowedKafkaSecurityProtocol(v string) bool {
	return v == "PLAINTEXT" || v == "SSL" || v == "SASL_PLAINTEXT" || v == "SASL_SSL"
}

func isAllowedKafkaSASLMechanism(v string) bool {
	return v == "PLAIN" || v == "SCRAM-SHA-256" || v == "SCRAM-SHA-512"
}

func BuildProfileSummary(cfg *config.Config) string {
	mode := detectModeFromConfig(cfg)
	authState := "not set"
	if strings.TrimSpace(cfg.Gateway.AuthToken) != "" {
		authState = "set"
	}

	lines := []string{
		"",
		"Planned configuration:",
		fmt.Sprintf("- mode: %s", mode),
		fmt.Sprintf("- gateway.host: %s", cfg.Gateway.Host),
		fmt.Sprintf("- gateway.authToken: %s", authState),
		fmt.Sprintf("- group.enabled: %t", cfg.Group.Enabled),
		fmt.Sprintf("- orchestrator.enabled: %t", cfg.Orchestrator.Enabled),
		fmt.Sprintf("- kafka.brokers: %s", firstNonEmpty(strings.TrimSpace(cfg.Group.KafkaBrokers), "(empty)")),
		fmt.Sprintf("- kafka.securityProtocol: %s", firstNonEmpty(strings.TrimSpace(cfg.Group.KafkaSecurityProto), "(default)")),
		fmt.Sprintf("- kafka.saslMechanism: %s", firstNonEmpty(strings.TrimSpace(cfg.Group.KafkaSASLMechanism), "(none)")),
		fmt.Sprintf("- kafka.saslUsername: %s", firstNonEmpty(strings.TrimSpace(cfg.Group.KafkaSASLUsername), "(none)")),
		fmt.Sprintf("- kafka.tls.caFile: %s", firstNonEmpty(strings.TrimSpace(cfg.Group.KafkaTLSCAFile), "(none)")),
		fmt.Sprintf("- kafka.tls.certFile: %s", firstNonEmpty(strings.TrimSpace(cfg.Group.KafkaTLSCertFile), "(none)")),
		fmt.Sprintf("- kafka.tls.keyFile: %s", firstNonEmpty(strings.TrimSpace(cfg.Group.KafkaTLSKeyFile), "(none)")),
		fmt.Sprintf("- llm.model: %s", firstNonEmpty(strings.TrimSpace(cfg.Model.Name), "(empty)")),
	}

	// Show configured providers
	providerEntries := []struct {
		id     string
		hasKey bool
	}{
		{"anthropic/claude", cfg.Providers.Anthropic.APIKey != ""},
		{"openai", cfg.Providers.OpenAI.APIKey != ""},
		{"gemini", cfg.Providers.Gemini.APIKey != ""},
		{"xai", cfg.Providers.XAI.APIKey != ""},
		{"openrouter", cfg.Providers.OpenRouter.APIKey != ""},
		{"deepseek", cfg.Providers.DeepSeek.APIKey != ""},
		{"groq", cfg.Providers.Groq.APIKey != ""},
		{"scalytics-copilot", cfg.Providers.ScalyticsCopilot.APIKey != ""},
		{"vllm", cfg.Providers.VLLM.APIBase != ""},
	}
	var configured []string
	for _, pe := range providerEntries {
		if pe.hasKey {
			configured = append(configured, pe.id)
		}
	}
	if len(configured) == 0 {
		configured = []string{"(none)"}
	}
	lines = append(lines, fmt.Sprintf("- llm.providers: %s", strings.Join(configured, ", ")))

	lines = append(lines,
		fmt.Sprintf("- subagents.maxSpawnDepth: %d", cfg.Tools.Subagents.MaxSpawnDepth),
		fmt.Sprintf("- subagents.maxChildrenPerAgent: %d", cfg.Tools.Subagents.MaxChildrenPerAgent),
		fmt.Sprintf("- subagents.maxConcurrent: %d", cfg.Tools.Subagents.MaxConcurrent),
		fmt.Sprintf("- subagents.archiveAfterMinutes: %d", cfg.Tools.Subagents.ArchiveAfterMinutes),
		fmt.Sprintf("- subagents.allowAgents: %s", firstNonEmpty(strings.Join(cfg.Tools.Subagents.AllowAgents, ","), "(current agent only)")),
		fmt.Sprintf("- subagents.model: %s", firstNonEmpty(strings.TrimSpace(cfg.Tools.Subagents.Model), "(inherit main model)")),
		fmt.Sprintf("- subagents.thinking: %s", firstNonEmpty(strings.TrimSpace(cfg.Tools.Subagents.Thinking), "(inherit/default)")),
		"",
	)
	return strings.Join(lines, "\n")
}

func parseCSV(raw string) []string {
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

func ConfirmApply(reader *bufio.Reader, out io.Writer) (bool, error) {
	answer, err := prompt(reader, out, "Apply this configuration? [y/N]", "N")
	if err != nil {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

func detectModeFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return "unknown"
	}
	if !isLoopback(cfg.Gateway.Host) {
		return "remote"
	}
	if cfg.Group.Enabled || cfg.Orchestrator.Enabled {
		return "local+kafka"
	}
	return "local"
}

func isLoopback(host string) bool {
	h := strings.TrimSpace(strings.ToLower(host))
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate auth token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func defaultAgentID() string {
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		return "agent-" + host
	}
	return "agent-local"
}

func firstNonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func errorsIsEOF(err error) bool {
	return err == io.EOF
}
