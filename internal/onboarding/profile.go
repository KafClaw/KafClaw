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
)

type WizardParams struct {
	Mode             string
	Profile          string
	LLMPreset        string
	LLMToken         string
	LLMAPIBase       string
	LLMModel         string
	KafkaBrokers     string
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
	fmt.Fprintln(out, "\nSelect LLM setup:")
	fmt.Fprintln(out, "1) cli-token (OpenRouter/OpenAI-compatible token)")
	fmt.Fprintln(out, "2) openai-compatible (vLLM/Ollama/custom endpoint)")
	fmt.Fprintln(out, "3) skip (keep current)")
	choice, err := prompt(reader, out, "LLM setup [1/2/3]", "3")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(choice) {
	case "1":
		return LLMPresetCLIToken, nil
	case "2":
		return LLMPresetOpenAICompatible, nil
	case "3":
		return LLMPresetSkip, nil
	default:
		return "", fmt.Errorf("invalid llm preset choice: %s", choice)
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

func applyLLM(cfg *config.Config, preset LLMPreset, reader *bufio.Reader, out io.Writer, p WizardParams) error {
	switch preset {
	case LLMPresetSkip:
		return nil
	case LLMPresetCLIToken:
		token := strings.TrimSpace(p.LLMToken)
		base := strings.TrimSpace(p.LLMAPIBase)
		model := strings.TrimSpace(p.LLMModel)
		if token == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "API token", os.Getenv("OPENROUTER_API_KEY"))
			if err != nil {
				return err
			}
			token = strings.TrimSpace(val)
		}
		if base == "" {
			base = "https://openrouter.ai/api/v1"
		}
		if model == "" && !p.NonInteractive {
			val, err := prompt(reader, out, "Model", cfg.Model.Name)
			if err != nil {
				return err
			}
			model = strings.TrimSpace(val)
		}
		cfg.Providers.OpenAI.APIKey = token
		cfg.Providers.OpenAI.APIBase = base
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
	case "cli-token", "token":
		return LLMPresetCLIToken
	case "openai-compatible", "compatible", "vllm", "ollama":
		return LLMPresetOpenAICompatible
	default:
		return ""
	}
}

func BuildProfileSummary(cfg *config.Config) string {
	mode := detectModeFromConfig(cfg)
	llmBase := cfg.Providers.OpenAI.APIBase
	if strings.TrimSpace(llmBase) == "" {
		llmBase = "(default)"
	}
	tokenState := "not set"
	if strings.TrimSpace(cfg.Providers.OpenAI.APIKey) != "" {
		tokenState = "set"
	}
	authState := "not set"
	if strings.TrimSpace(cfg.Gateway.AuthToken) != "" {
		authState = "set"
	}
	return strings.Join([]string{
		"",
		"Planned configuration:",
		fmt.Sprintf("- mode: %s", mode),
		fmt.Sprintf("- gateway.host: %s", cfg.Gateway.Host),
		fmt.Sprintf("- gateway.authToken: %s", authState),
		fmt.Sprintf("- group.enabled: %t", cfg.Group.Enabled),
		fmt.Sprintf("- orchestrator.enabled: %t", cfg.Orchestrator.Enabled),
		fmt.Sprintf("- kafka.brokers: %s", firstNonEmpty(strings.TrimSpace(cfg.Group.KafkaBrokers), "(empty)")),
		fmt.Sprintf("- llm.model: %s", firstNonEmpty(strings.TrimSpace(cfg.Model.Name), "(empty)")),
		fmt.Sprintf("- llm.apiBase: %s", llmBase),
		fmt.Sprintf("- llm.apiKey: %s", tokenState),
		fmt.Sprintf("- subagents.maxSpawnDepth: %d", cfg.Tools.Subagents.MaxSpawnDepth),
		fmt.Sprintf("- subagents.maxChildrenPerAgent: %d", cfg.Tools.Subagents.MaxChildrenPerAgent),
		fmt.Sprintf("- subagents.maxConcurrent: %d", cfg.Tools.Subagents.MaxConcurrent),
		fmt.Sprintf("- subagents.archiveAfterMinutes: %d", cfg.Tools.Subagents.ArchiveAfterMinutes),
		fmt.Sprintf("- subagents.allowAgents: %s", firstNonEmpty(strings.Join(cfg.Tools.Subagents.AllowAgents, ","), "(current agent only)")),
		fmt.Sprintf("- subagents.model: %s", firstNonEmpty(strings.TrimSpace(cfg.Tools.Subagents.Model), "(inherit main model)")),
		fmt.Sprintf("- subagents.thinking: %s", firstNonEmpty(strings.TrimSpace(cfg.Tools.Subagents.Thinking), "(inherit/default)")),
		"",
	}, "\n")
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
