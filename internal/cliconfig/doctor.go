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

	"github.com/KafClaw/KafClaw/internal/config"
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
		envPath, mergedKeys, fixErr := mergeDiscoveredEnvFiles()
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

	return report, nil
}

func mergeDiscoveredEnvFiles() (string, int, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", 0, err
	}
	targetPath := filepath.Join(home, ".config", "kafclaw", "env")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return "", 0, err
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
			return "", 0, fmt.Errorf("read %s: %w", src, err)
		}
		for k, v := range kv {
			merged[k] = v
		}
	}

	if err := writeEnvFileKV(targetPath, merged); err != nil {
		return "", 0, err
	}
	if err := os.Chmod(targetPath, 0o600); err != nil {
		return "", 0, err
	}
	return targetPath, len(merged), nil
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

func detectRuntimeMode(cfg *config.Config) RuntimeMode {
	if !isLoopbackHost(cfg.Gateway.Host) {
		return ModeRemoteGateway
	}
	if cfg.Group.Enabled || cfg.Orchestrator.Enabled {
		return ModeKafkaLocal
	}
	return ModeLocalAssistant
}
