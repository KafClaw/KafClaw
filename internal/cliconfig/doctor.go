package cliconfig

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/KafClaw/KafClaw/internal/channels"
	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/provider"
	skillruntime "github.com/KafClaw/KafClaw/internal/skills"
)

type DoctorStatus string

const (
	DoctorPass DoctorStatus = "pass"
	DoctorWarn DoctorStatus = "warn"
	DoctorFail DoctorStatus = "fail"
)

type DoctorCheck struct {
	Name    string
	Status  DoctorStatus
	Message string
}

type DoctorReport struct {
	Checks []DoctorCheck
}

type DoctorOptions struct {
	Fix                  bool
	GenerateGatewayToken bool
}

type RuntimeMode string

const (
	ModeLocalAssistant RuntimeMode = "local-assistant"
	ModeKafkaLocal     RuntimeMode = "kafka-local"
	ModeRemoteGateway  RuntimeMode = "remote-gateway"
)

func (r DoctorReport) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == DoctorFail {
			return true
		}
	}
	return false
}

func RunDoctor() (DoctorReport, error) {
	return RunDoctorWithOptions(DoctorOptions{})
}

func RunDoctorWithOptions(opts DoctorOptions) (DoctorReport, error) {
	report := DoctorReport{Checks: make([]DoctorCheck, 0, 8)}

	cfgPath, err := config.ConfigPath()
	if err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "config_path",
			Status:  DoctorFail,
			Message: fmt.Sprintf("cannot resolve config path: %v", err),
		})
		return report, nil
	}

	if _, err := os.Stat(cfgPath); err != nil {
		if os.IsNotExist(err) {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "config_file",
				Status:  DoctorWarn,
				Message: fmt.Sprintf("config file not found at %s (defaults will be used)", cfgPath),
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "config_file",
				Status:  DoctorFail,
				Message: fmt.Sprintf("cannot access config file: %v", err),
			})
		}
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "config_file",
			Status:  DoctorPass,
			Message: fmt.Sprintf("config file found at %s", cfgPath),
		})
	}

	if opts.Fix {
		envPath, mergedKeys, mergedKV, fixErr := mergeDiscoveredEnvFiles()
		if fixErr != nil {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "env_merge",
				Status:  DoctorFail,
				Message: fmt.Sprintf("failed to merge env files: %v", fixErr),
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "env_merge",
				Status:  DoctorPass,
				Message: fmt.Sprintf("merged %d env key(s) into %s", mergedKeys, envPath),
			})
			sensitive := selectSensitiveEnvKeys(mergedKV)
			written, tombErr := skillruntime.StoreEnvSecretsInLocalTomb(sensitive)
			if tombErr != nil {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "env_tomb_sync",
					Status:  DoctorFail,
					Message: fmt.Sprintf("failed syncing sensitive env keys to tomb: %v", tombErr),
				})
				if len(sensitive) > 0 {
					report.Checks = append(report.Checks, DoctorCheck{
						Name:    "env_sensitive_scrub",
						Status:  DoctorWarn,
						Message: "skipped env sensitive-key scrub because tomb sync failed",
					})
				} else {
					report.Checks = append(report.Checks, DoctorCheck{
						Name:    "env_sensitive_scrub",
						Status:  DoctorPass,
						Message: "no sensitive env keys found to scrub",
					})
				}
			} else {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "env_tomb_sync",
					Status:  DoctorPass,
					Message: fmt.Sprintf("synced %d sensitive env key(s) into tomb-managed encrypted store", written),
				})
				removed, scrubErr := scrubSensitiveEnvKeys(envPath, mergedKV, sensitive)
				if scrubErr != nil {
					report.Checks = append(report.Checks, DoctorCheck{
						Name:    "env_sensitive_scrub",
						Status:  DoctorFail,
						Message: fmt.Sprintf("failed scrubbing sensitive env keys from %s: %v", envPath, scrubErr),
					})
				} else {
					report.Checks = append(report.Checks, DoctorCheck{
						Name:    "env_sensitive_scrub",
						Status:  DoctorPass,
						Message: fmt.Sprintf("removed %d sensitive env key(s) from %s after tomb sync", removed, envPath),
					})
				}
			}
		}
	}

	cfg, err := config.Load()
	if err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "config_load",
			Status:  DoctorFail,
			Message: fmt.Sprintf("config load failed: %v", err),
		})
		return report, nil
	}
	report.Checks = append(report.Checks, DoctorCheck{
		Name:    "config_load",
		Status:  DoctorPass,
		Message: "config loaded successfully",
	})

	if opts.GenerateGatewayToken {
		token, genErr := randomToken()
		if genErr != nil {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "gateway_token",
				Status:  DoctorFail,
				Message: fmt.Sprintf("failed to generate token: %v", genErr),
			})
		} else {
			cfg.Gateway.AuthToken = token
			if saveErr := config.Save(cfg); saveErr != nil {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "gateway_token",
					Status:  DoctorFail,
					Message: fmt.Sprintf("generated token but failed to save config: %v", saveErr),
				})
			} else {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "gateway_token",
					Status:  DoctorPass,
					Message: "generated and saved gateway auth token",
				})
			}
		}
	}

	if cfg.Paths.Workspace == "" {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "workspace_path",
			Status:  DoctorFail,
			Message: "paths.workspace is empty",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "workspace_path",
			Status:  DoctorPass,
			Message: fmt.Sprintf("workspace path: %s", cfg.Paths.Workspace),
		})
	}

	if cfg.Paths.WorkRepoPath == "" {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "work_repo_path",
			Status:  DoctorFail,
			Message: "paths.workRepoPath is empty",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "work_repo_path",
			Status:  DoctorPass,
			Message: fmt.Sprintf("work repo path: %s", cfg.Paths.WorkRepoPath),
		})
	}

	appendProviderDoctorChecks(&report, cfg)

	mode := detectRuntimeMode(cfg)
	report.Checks = append(report.Checks, DoctorCheck{
		Name:    "runtime_mode",
		Status:  DoctorPass,
		Message: fmt.Sprintf("detected mode: %s", mode),
	})

	if isLoopbackHost(cfg.Gateway.Host) {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "gateway_loopback",
			Status:  DoctorPass,
			Message: fmt.Sprintf("gateway.host is loopback (%s)", cfg.Gateway.Host),
		})
	} else {
		if mode == ModeRemoteGateway {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "gateway_loopback",
				Status:  DoctorWarn,
				Message: fmt.Sprintf("gateway.host is non-loopback (%s) in remote mode", cfg.Gateway.Host),
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "gateway_loopback",
				Status:  DoctorFail,
				Message: fmt.Sprintf("gateway.host is not loopback (%s)", cfg.Gateway.Host),
			})
		}
	}

	if mode == ModeRemoteGateway {
		if strings.TrimSpace(cfg.Gateway.AuthToken) == "" {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "remote_auth_token",
				Status:  DoctorFail,
				Message: "remote gateway requires gateway.authToken (or KAFCLAW_GATEWAY_AUTH_TOKEN / legacy MIKROBOT_GATEWAY_AUTH_TOKEN)",
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "remote_auth_token",
				Status:  DoctorPass,
				Message: "remote gateway auth token is configured",
			})
		}
	}

	if endpointLooksRemote(cfg.Orchestrator.Endpoint) && mode != ModeRemoteGateway {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "orchestrator_endpoint_scope",
			Status:  DoctorWarn,
			Message: fmt.Sprintf("orchestrator.endpoint points to non-loopback host (%s)", cfg.Orchestrator.Endpoint),
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "orchestrator_endpoint_scope",
			Status:  DoctorPass,
			Message: "orchestrator.endpoint is empty or loopback",
		})
	}

	for _, d := range channels.CollectChannelAccountDiagnostics(cfg) {
		name := fmt.Sprintf("%s_account_%s", d.Channel, d.Account)
		if len(d.Issues) == 0 {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    name,
				Status:  DoctorPass,
				Message: fmt.Sprintf("%s account %s configuration is consistent", d.Channel, d.Account),
			})
			continue
		}
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    name,
			Status:  DoctorWarn,
			Message: strings.Join(d.Issues, "; "),
		})
	}

	appendRateLimitDoctorChecks(&report)
	appendSkillsDoctorChecks(&report, cfg, opts)

	return report, nil
}

func appendSkillsDoctorChecks(report *DoctorReport, cfg *config.Config, opts DoctorOptions) {
	if cfg == nil {
		return
	}
	if cfg.Skills.Enabled && opts.Fix {
		if err := fixSkillsPermissions(); err != nil {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "skills_permissions_fix",
				Status:  DoctorFail,
				Message: fmt.Sprintf("failed to enforce skills permissions: %v", err),
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "skills_permissions_fix",
				Status:  DoctorPass,
				Message: "enforced skills directory/file permissions (0700/0600)",
			})
		}
	}
	if cfg.Skills.Enabled {
		if !skillruntime.HasBinary("node") {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "skills_node",
				Status:  DoctorWarn,
				Message: "skills enabled but `node` is not in PATH",
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "skills_node",
				Status:  DoctorPass,
				Message: "node is available for skills runtime",
			})
		}
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "skills_node",
			Status:  DoctorPass,
			Message: "skills are disabled",
		})
	}

	if cfg.Skills.ExternalInstalls {
		if !skillruntime.HasBinary("clawhub") {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "skills_clawhub",
				Status:  DoctorWarn,
				Message: "skills.externalInstalls enabled but `clawhub` is not in PATH",
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "skills_clawhub",
				Status:  DoctorPass,
				Message: "clawhub is available",
			})
		}
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "skills_clawhub",
			Status:  DoctorPass,
			Message: "skills external installs are disabled",
		})
	}

	if cfg.Skills.Enabled {
		dirs, err := skillruntime.ResolveStateDirs()
		if err != nil {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "skills_runtime_permissions",
				Status:  DoctorFail,
				Message: fmt.Sprintf("failed to resolve skills runtime dirs: %v", err),
			})
		} else {
			insecure, missing := checkSkillsDirPermissions([]string{
				dirs.Root, dirs.TmpDir, dirs.ToolsDir, dirs.Quarantine, dirs.Installed, dirs.Snapshots, dirs.AuditDir,
			})
			secretInsecure, secretMissing := checkSkillsSecretFilePermissions(dirs.ToolsDir)
			if len(insecure) > 0 {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "skills_runtime_permissions",
					Status:  DoctorFail,
					Message: strings.Join(insecure, "; "),
				})
			} else if len(missing) > 0 {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "skills_runtime_permissions",
					Status:  DoctorWarn,
					Message: strings.Join(missing, "; "),
				})
			} else {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "skills_runtime_permissions",
					Status:  DoctorPass,
					Message: "skills runtime directory permissions are secure (0700)",
				})
			}
			if len(secretInsecure) > 0 {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "skills_secret_permissions",
					Status:  DoctorFail,
					Message: strings.Join(secretInsecure, "; "),
				})
			} else if len(secretMissing) > 0 {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "skills_secret_permissions",
					Status:  DoctorWarn,
					Message: strings.Join(secretMissing, "; "),
				})
			} else {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    "skills_secret_permissions",
					Status:  DoctorPass,
					Message: "skills secret file permissions are secure (0600)",
				})
			}
		}
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "skills_runtime_permissions",
			Status:  DoctorPass,
			Message: "skills are disabled",
		})
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "skills_secret_permissions",
			Status:  DoctorPass,
			Message: "skills are disabled",
		})
	}

	channelEnabled := cfg.Channels.Slack.Enabled || cfg.Channels.MSTeams.Enabled || cfg.Channels.WhatsApp.Enabled
	if channelEnabled && (!cfg.Skills.Enabled || !skillruntime.EffectiveSkillEnabled(cfg, "channel-onboarding")) {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "channel_onboarding_skill",
			Status:  DoctorWarn,
			Message: "one or more channels are enabled but `channel-onboarding` skill is not active",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "channel_onboarding_skill",
			Status:  DoctorPass,
			Message: "channel onboarding readiness is consistent",
		})
	}
}

func checkSkillsSecretFilePermissions(toolsDir string) (insecure []string, missing []string) {
	insecure = make([]string, 0)
	missing = make([]string, 0)
	authRoot := filepath.Join(toolsDir, "auth")
	if _, err := os.Stat(authRoot); err != nil {
		if os.IsNotExist(err) {
			missing = append(missing, fmt.Sprintf("%s missing", authRoot))
			return insecure, missing
		}
		insecure = append(insecure, fmt.Sprintf("%s stat error: %v", authRoot, err))
		return insecure, missing
	}
	_ = filepath.WalkDir(authRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			insecure = append(insecure, fmt.Sprintf("%s walk error: %v", path, err))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if filepath.Ext(path) != ".json" && base != "master.key" {
			return nil
		}
		st, statErr := os.Stat(path)
		if statErr != nil {
			insecure = append(insecure, fmt.Sprintf("%s stat error: %v", path, statErr))
			return nil
		}
		if st.Mode().Perm() != 0o600 {
			insecure = append(insecure, fmt.Sprintf("%s permissions are %o (expected 600)", path, st.Mode().Perm()))
		}
		return nil
	})
	if tombPath, err := skillruntime.ResolveLocalOAuthTombPath(); err == nil {
		if st, statErr := os.Stat(tombPath); statErr == nil {
			if st.IsDir() {
				insecure = append(insecure, fmt.Sprintf("%s is a directory (expected file)", tombPath))
			} else if st.Mode().Perm() != 0o600 {
				insecure = append(insecure, fmt.Sprintf("%s permissions are %o (expected 600)", tombPath, st.Mode().Perm()))
			}
		} else if !os.IsNotExist(statErr) {
			insecure = append(insecure, fmt.Sprintf("%s stat error: %v", tombPath, statErr))
		}
	}
	return insecure, missing
}

func fixSkillsPermissions() error {
	dirs, err := skillruntime.EnsureStateDirs()
	if err != nil {
		return err
	}
	authRoot := filepath.Join(dirs.ToolsDir, "auth")
	if _, err := os.Stat(authRoot); err != nil {
		if os.IsNotExist(err) {
			// Continue to tomb permission repair even when auth root is absent.
		} else {
			return err
		}
	} else {
		if err := filepath.WalkDir(authRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return os.Chmod(path, 0o700)
			}
			base := filepath.Base(path)
			if filepath.Ext(path) == ".json" || base == "master.key" {
				return os.Chmod(path, 0o600)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	tombPath, err := skillruntime.ResolveLocalOAuthTombPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(tombPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(tombPath), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(tombPath), 0o700); err != nil {
		return err
	}
	return os.Chmod(tombPath, 0o600)
}

func checkSkillsDirPermissions(dirs []string) (insecure []string, missing []string) {
	insecure = make([]string, 0)
	missing = make([]string, 0)
	for _, dir := range dirs {
		st, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, fmt.Sprintf("%s missing", dir))
				continue
			}
			insecure = append(insecure, fmt.Sprintf("%s stat error: %v", dir, err))
			continue
		}
		if !st.IsDir() {
			insecure = append(insecure, fmt.Sprintf("%s is not a directory", dir))
			continue
		}
		if st.Mode().Perm() != 0o700 {
			insecure = append(insecure, fmt.Sprintf("%s permissions are %o (expected 700)", dir, st.Mode().Perm()))
		}
	}
	return insecure, missing
}

func mergeDiscoveredEnvFiles() (string, int, map[string]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", 0, nil, err
	}
	targetPath := filepath.Join(home, ".config", "kafclaw", "env")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return "", 0, nil, err
	}

	cwd, _ := os.Getwd()
	sources := []string{
		filepath.Join(cwd, ".env"),
		filepath.Join(home, ".openclaw", ".env"),
		filepath.Join(home, ".config", "openclaw", "env"),
		filepath.Join(home, ".kafclaw", ".env"),
		filepath.Join(home, ".kafclaw", "env"),
		targetPath,
	}

	merged := map[string]string{}
	seen := map[string]struct{}{}
	for _, src := range sources {
		if _, ok := seen[src]; ok {
			continue
		}
		seen[src] = struct{}{}
		kv, err := readEnvFileKV(src)
		if err != nil {
			if errorsIsNotExist(err) {
				continue
			}
			return "", 0, nil, fmt.Errorf("read %s: %w", src, err)
		}
		for k, v := range kv {
			merged[k] = v
		}
	}

	if err := writeEnvFileKV(targetPath, merged); err != nil {
		return "", 0, nil, err
	}
	if err := os.Chmod(targetPath, 0o600); err != nil {
		return "", 0, nil, err
	}
	return targetPath, len(merged), merged, nil
}

func selectSensitiveEnvKeys(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		trimmedKey := strings.TrimSpace(k)
		key := strings.ToUpper(trimmedKey)
		if key == "" || strings.TrimSpace(v) == "" {
			continue
		}
		switch {
		case strings.Contains(key, "TOKEN"),
			strings.Contains(key, "SECRET"),
			strings.Contains(key, "PASSWORD"),
			strings.Contains(key, "API_KEY"),
			strings.Contains(key, "ACCESS_KEY"),
			strings.Contains(key, "PRIVATE_KEY"),
			strings.Contains(key, "CLIENT_SECRET"):
			out[trimmedKey] = v
		}
	}
	return out
}

func scrubSensitiveEnvKeys(path string, merged map[string]string, sensitive map[string]string) (int, error) {
	if len(sensitive) == 0 {
		return 0, nil
	}
	upperToActual := make(map[string]string, len(merged))
	for k := range merged {
		upperToActual[strings.ToUpper(strings.TrimSpace(k))] = k
	}
	removed := 0
	for key := range sensitive {
		normalized := strings.ToUpper(strings.TrimSpace(key))
		actual, ok := upperToActual[normalized]
		if !ok {
			continue
		}
		delete(merged, actual)
		removed++
	}
	if err := writeEnvFileKV(path, merged); err != nil {
		return removed, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return removed, err
	}
	return removed, nil
}

func readEnvFileKV(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	kv := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		i := strings.IndexRune(line, '=')
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		if k == "" {
			continue
		}
		v := strings.TrimSpace(line[i+1:])
		kv[k] = trimEnvQuotes(v)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return kv, nil
}

func writeEnvFileKV(path string, kv map[string]string) error {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys)+2)
	lines = append(lines, "# KafClaw runtime env (managed by doctor --fix)")
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, kv[k]))
	}
	lines = append(lines, "")
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}

func trimEnvQuotes(v string) string {
	if len(v) >= 2 {
		if strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
			return v[1 : len(v)-1]
		}
		if strings.HasPrefix(v, `'`) && strings.HasSuffix(v, `'`) {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func errorsIsNotExist(err error) bool {
	return os.IsNotExist(err)
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(strings.ToLower(host))
	if h == "" {
		return false
	}
	if h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func endpointLooksRemote(endpoint string) bool {
	e := strings.TrimSpace(endpoint)
	if e == "" {
		return false
	}
	u, err := url.Parse(e)
	if err != nil {
		return true
	}
	host := u.Hostname()
	if host == "" {
		host = strings.TrimSpace(e)
	}
	return !isLoopbackHost(host)
}

func appendProviderDoctorChecks(report *DoctorReport, cfg *config.Config) {
	if cfg == nil {
		return
	}

	// Determine which provider is needed from model.name
	modelStr := cfg.Model.Name
	if modelStr == "" {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "provider_model",
			Status:  DoctorWarn,
			Message: "model.name is empty — using legacy OpenAI fallback",
		})
		// Check if legacy OpenAI is configured
		if cfg.Providers.OpenAI.APIKey == "" {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "provider_openai",
				Status:  DoctorFail,
				Message: "no model configured and providers.openai.apiKey is empty",
			})
		} else {
			report.Checks = append(report.Checks, DoctorCheck{
				Name:    "provider_openai",
				Status:  DoctorPass,
				Message: "legacy OpenAI fallback: API key is configured",
			})
		}
		return
	}

	report.Checks = append(report.Checks, DoctorCheck{
		Name:    "provider_model",
		Status:  DoctorPass,
		Message: fmt.Sprintf("model.name: %s", modelStr),
	})

	// Check each configured provider's readiness
	type provCheck struct {
		id      string
		hasKey  bool
		hasBase bool
		needKey bool
	}
	checks := []provCheck{
		{"claude", cfg.Providers.Anthropic.APIKey != "", cfg.Providers.Anthropic.APIBase != "", true},
		{"openai", cfg.Providers.OpenAI.APIKey != "", cfg.Providers.OpenAI.APIBase != "", true},
		{"gemini", cfg.Providers.Gemini.APIKey != "", false, true},
		{"xai", cfg.Providers.XAI.APIKey != "", false, true},
		{"openrouter", cfg.Providers.OpenRouter.APIKey != "", cfg.Providers.OpenRouter.APIBase != "", true},
		{"deepseek", cfg.Providers.DeepSeek.APIKey != "", cfg.Providers.DeepSeek.APIBase != "", true},
		{"groq", cfg.Providers.Groq.APIKey != "", cfg.Providers.Groq.APIBase != "", true},
		{"scalytics-copilot", cfg.Providers.ScalyticsCopilot.APIKey != "", cfg.Providers.ScalyticsCopilot.APIBase != "", true},
		{"vllm", cfg.Providers.VLLM.APIKey != "", cfg.Providers.VLLM.APIBase != "", false},
	}

	configured := 0
	for _, pc := range checks {
		if pc.hasKey || (pc.id == "vllm" && pc.hasBase) {
			configured++
		}
	}

	if configured == 0 {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "provider_configured",
			Status:  DoctorWarn,
			Message: "no API-key providers configured — only CLI-OAuth providers (gemini-cli, openai-codex) may work",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "provider_configured",
			Status:  DoctorPass,
			Message: fmt.Sprintf("%d provider(s) with credentials configured", configured),
		})
	}

	// Validate scalytics-copilot requires base URL
	if cfg.Providers.ScalyticsCopilot.APIKey != "" && cfg.Providers.ScalyticsCopilot.APIBase == "" {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "provider_scalytics_copilot_base",
			Status:  DoctorFail,
			Message: "scalytics-copilot has API key but missing apiBase (required)",
		})
	}

	// Validate vLLM requires base URL
	if cfg.Providers.VLLM.APIKey != "" && cfg.Providers.VLLM.APIBase == "" {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "provider_vllm_base",
			Status:  DoctorFail,
			Message: "vllm has API key but missing apiBase (required)",
		})
	}

	// Check CLI tools for OAuth providers
	if skillruntime.HasBinary("gemini") {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "provider_gemini_cli",
			Status:  DoctorPass,
			Message: "gemini CLI found in PATH",
		})
	}
	if skillruntime.HasBinary("codex") {
		report.Checks = append(report.Checks, DoctorCheck{
			Name:    "provider_codex_cli",
			Status:  DoctorPass,
			Message: "codex CLI found in PATH",
		})
	}
}

func appendRateLimitDoctorChecks(report *DoctorReport) {
	snapshots := provider.AllRateLimitSnapshots()
	if len(snapshots) == 0 {
		return
	}
	for provID, snap := range snapshots {
		if snap.RemainingTokens != nil && snap.LimitTokens != nil && *snap.LimitTokens > 0 {
			pct := float64(*snap.RemainingTokens) / float64(*snap.LimitTokens) * 100
			if pct < 10 {
				report.Checks = append(report.Checks, DoctorCheck{
					Name:    fmt.Sprintf("rate_limit_%s", provID),
					Status:  DoctorWarn,
					Message: fmt.Sprintf("%s: only %d/%d tokens remaining (%.0f%%)", provID, *snap.RemainingTokens, *snap.LimitTokens, pct),
				})
			}
		}
	}
}

func detectRuntimeMode(cfg *config.Config) RuntimeMode {
	if !isLoopbackHost(cfg.Gateway.Host) {
		return ModeRemoteGateway
	}
	if cfg.Group.Enabled || cfg.Orchestrator.Enabled {
		return ModeKafkaLocal
	}
	return ModeLocalAssistant
}
