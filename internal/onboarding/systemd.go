package onboarding

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	currentEUID   = os.Geteuid
	lookupUserFn  = user.Lookup
	currentUserFn = user.Current
	ensureUserFn  = ensureNonRootUser
	runCommandFn  = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}
)

type SetupOptions struct {
	ServiceUser string
	ServiceHome string
	BinaryPath  string
	Port        int
	Profile     string
	Version     string
	InstallRoot string
}

type SetupResult struct {
	UserCreated  bool
	ServicePath  string
	OverridePath string
	EnvPath      string
}

func SetupSystemdGateway(opts SetupOptions) (*SetupResult, error) {
	if opts.ServiceUser == "" {
		return nil, fmt.Errorf("service user is required")
	}
	if opts.BinaryPath == "" {
		return nil, fmt.Errorf("binary path is required")
	}
	if opts.Port <= 0 {
		return nil, fmt.Errorf("port must be > 0")
	}
	if opts.InstallRoot == "" {
		opts.InstallRoot = "/"
	}
	if opts.Profile == "" {
		opts.Profile = "default"
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}

	created := false
	home := opts.ServiceHome
	if home == "" {
		var err error
		created, home, err = ensureUserFn(opts.ServiceUser)
		if err != nil {
			return nil, err
		}
	}

	servicePath := filepath.Join(opts.InstallRoot, "etc", "systemd", "system", "kafclaw-gateway.service")
	overridePath := filepath.Join(home, ".config", "systemd", "user", "kafclaw-gateway.service.d", "override.conf")
	envPath := filepath.Join(home, ".config", "kafclaw", "env")

	if err := os.MkdirAll(filepath.Dir(servicePath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(overridePath), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		return nil, err
	}

	unit := renderSystemUnit(opts, home)
	override := renderOverride(home)
	env := renderEnvFile(home)

	if err := os.WriteFile(servicePath, []byte(unit), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(overridePath, []byte(override), 0o644); err != nil {
		return nil, err
	}
	if _, err := os.Stat(envPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
			return nil, err
		}
	}

	return &SetupResult{
		UserCreated:  created,
		ServicePath:  servicePath,
		OverridePath: overridePath,
		EnvPath:      envPath,
	}, nil
}

func ensureNonRootUser(name string) (bool, string, error) {
	if currentEUID() != 0 {
		u, err := lookupUserFn(name)
		if err != nil {
			cur, curErr := currentUserFn()
			if curErr != nil {
				return false, "", fmt.Errorf("cannot resolve current user: %w", curErr)
			}
			return false, cur.HomeDir, nil
		}
		return false, u.HomeDir, nil
	}

	u, err := lookupUserFn(name)
	if err == nil {
		return false, u.HomeDir, nil
	}

	if out, err := runCommandFn("useradd", "--create-home", "--shell", "/bin/bash", name); err != nil {
		if fbOut, fbErr := runCommandFn("adduser", "--disabled-password", "--gecos", "", name); fbErr != nil {
			return false, "", fmt.Errorf("failed to create user %q: useradd=%v (%s), adduser=%v (%s)", name, err, string(out), fbErr, string(fbOut))
		}
	}

	u, err = lookupUserFn(name)
	if err != nil {
		return false, "", fmt.Errorf("user %q created but lookup failed: %w", name, err)
	}
	return true, u.HomeDir, nil
}

func renderSystemUnit(opts SetupOptions, home string) string {
	escapedExec := shellEscape(filepath.Clean(opts.BinaryPath))
	desc := fmt.Sprintf("KafClaw Gateway (profile: %s, v%s)", opts.Profile, opts.Version)
	return strings.Join([]string{
		"[Unit]",
		"Description=" + desc,
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"User=" + opts.ServiceUser,
		"Group=" + opts.ServiceUser,
		fmt.Sprintf("ExecStart=%s gateway --port %d", escapedExec, opts.Port),
		"Restart=always",
		"RestartSec=5",
		"Environment=KAFCLAW_GATEWAY_AUTH_TOKEN=",
		"Environment=MIKROBOT_GATEWAY_AUTH_TOKEN=",
		"EnvironmentFile=-" + filepath.Join(home, ".config", "kafclaw", "env"),
		"WorkingDirectory=" + home,
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	}, "\n")
}

func renderOverride(home string) string {
	return strings.Join([]string{
		"[Service]",
		"EnvironmentFile=%h/.config/kafclaw/env",
		"Environment=KAFCLAW_CONFIG=" + filepath.Join(home, ".kafclaw", "config.json"),
		"Environment=KAFCLAW_HOME=" + home,
		"Environment=HOME=" + home,
		"Environment=PATH=" + filepath.Join(home, ".local", "bin") + ":" + "/usr/local/bin:/usr/bin:/bin",
		"",
	}, "\n")
}

func renderEnvFile(home string) string {
	return strings.Join([]string{
		"# KafClaw runtime environment",
		"# Loaded via systemd EnvironmentFile",
		"KAFCLAW_GATEWAY_AUTH_TOKEN=",
		"MIKROBOT_GATEWAY_AUTH_TOKEN=",
		"KAFCLAW_CONFIG=" + filepath.Join(home, ".kafclaw", "config.json"),
		"KAFCLAW_HOME=" + home,
		"",
	}, "\n")
}

func shellEscape(v string) string {
	if v == "" {
		return "''"
	}
	if strings.IndexFunc(v, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '"' || r == '\'' || r == '\\'
	}) == -1 {
		return v
	}
	return strconv.Quote(v)
}
