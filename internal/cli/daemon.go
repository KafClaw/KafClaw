package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/onboarding"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage gateway daemon service lifecycle",
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and optionally activate systemd service (Linux)",
	RunE:  runDaemonInstall,
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Disable and remove systemd service (Linux)",
	RunE:  runDaemonUninstall,
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start systemd service (Linux)",
	RunE:  runDaemonStart,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop systemd service (Linux)",
	RunE:  runDaemonStop,
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart systemd service (Linux)",
	RunE:  runDaemonRestart,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show systemd service status (Linux)",
	RunE:  runDaemonStatus,
}

var daemonServiceUser string
var daemonServiceBinary string
var daemonServicePort int
var daemonInstallRoot string
var daemonRuntimeLabel string
var daemonActivate bool
var daemonJSON bool

var daemonExecFn = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
var daemonOS = runtime.GOOS
var daemonCurrentEUID = os.Geteuid
var daemonSetupSystemdFn = onboarding.SetupSystemdGateway
var daemonActivateSystemdFn = onboarding.ActivateSystemdGateway

func init() {
	daemonInstallCmd.Flags().StringVar(&daemonServiceUser, "service-user", "kafclaw", "Service user for systemd unit")
	daemonInstallCmd.Flags().StringVar(&daemonServiceBinary, "service-binary", "/usr/local/bin/kafclaw", "kafclaw binary path in ExecStart")
	daemonInstallCmd.Flags().IntVar(&daemonServicePort, "service-port", 18790, "Gateway port for systemd ExecStart")
	daemonInstallCmd.Flags().StringVar(&daemonInstallRoot, "service-install-root", "/", "Root path for systemd files (testing/packaging)")
	daemonInstallCmd.Flags().StringVar(&daemonRuntimeLabel, "daemon-runtime", "native", "Daemon runtime label persisted in config")
	daemonInstallCmd.Flags().BoolVar(&daemonActivate, "activate", true, "Run daemon-reload and enable --now after install")
	daemonInstallCmd.Flags().BoolVar(&daemonJSON, "json", false, "Output machine-readable JSON")
	_ = daemonInstallCmd.Flags().MarkHidden("service-install-root")

	daemonUninstallCmd.Flags().BoolVar(&daemonJSON, "json", false, "Output machine-readable JSON")
	daemonStartCmd.Flags().BoolVar(&daemonJSON, "json", false, "Output machine-readable JSON")
	daemonStopCmd.Flags().BoolVar(&daemonJSON, "json", false, "Output machine-readable JSON")
	daemonRestartCmd.Flags().BoolVar(&daemonJSON, "json", false, "Output machine-readable JSON")
	daemonStatusCmd.Flags().BoolVar(&daemonJSON, "json", false, "Output machine-readable JSON")

	daemonCmd.AddCommand(daemonInstallCmd, daemonUninstallCmd, daemonStartCmd, daemonStopCmd, daemonRestartCmd, daemonStatusCmd)
	rootCmd.AddCommand(daemonCmd)
}

func runDaemonInstall(cmd *cobra.Command, args []string) error {
	if daemonOS != "linux" {
		return daemonResult(cmd, "error", "install", map[string]any{"os": daemonOS}, "daemon install currently supports Linux systemd only")
	}
	cfg, err := config.Load()
	if err != nil {
		return daemonResult(cmd, "error", "install", nil, fmt.Sprintf("load config: %v", err))
	}
	if daemonServicePort <= 0 {
		if cfg.Gateway.Port > 0 {
			daemonServicePort = cfg.Gateway.Port
		} else {
			daemonServicePort = 18790
		}
	}
	cfg.Gateway.DaemonRuntime = strings.TrimSpace(daemonRuntimeLabel)
	if cfg.Gateway.DaemonRuntime == "" {
		cfg.Gateway.DaemonRuntime = "native"
	}
	if err := config.Save(cfg); err != nil {
		return daemonResult(cmd, "error", "install", nil, fmt.Sprintf("save config: %v", err))
	}

	result, err := daemonSetupSystemdFn(onboarding.SetupOptions{
		ServiceUser: daemonServiceUser,
		BinaryPath:  daemonServiceBinary,
		Port:        daemonServicePort,
		Profile:     "default",
		Version:     version,
		InstallRoot: daemonInstallRoot,
	})
	if err != nil {
		return daemonResult(cmd, "error", "install", nil, err.Error())
	}
	if daemonActivate {
		if daemonCurrentEUID() != 0 {
			return daemonResult(cmd, "error", "install", map[string]any{"servicePath": result.ServicePath}, "activation requires root privileges")
		}
		if err := daemonActivateSystemdFn(); err != nil {
			return daemonResult(cmd, "error", "install", map[string]any{"servicePath": result.ServicePath}, err.Error())
		}
	}
	return daemonResult(cmd, "ok", "install", map[string]any{
		"servicePath":  result.ServicePath,
		"overridePath": result.OverridePath,
		"envPath":      result.EnvPath,
		"userCreated":  result.UserCreated,
		"activated":    daemonActivate,
		"runtime":      cfg.Gateway.DaemonRuntime,
	}, "")
}

func runDaemonUninstall(cmd *cobra.Command, args []string) error {
	if daemonOS != "linux" {
		return daemonResult(cmd, "error", "uninstall", map[string]any{"os": daemonOS}, "daemon uninstall currently supports Linux systemd only")
	}
	if daemonCurrentEUID() != 0 {
		return daemonResult(cmd, "error", "uninstall", nil, "uninstall requires root privileges")
	}
	_, _ = daemonExecFn("systemctl", "disable", "--now", "kafclaw-gateway.service")
	_ = os.Remove(filepath.Join("/", "etc", "systemd", "system", "kafclaw-gateway.service"))
	_, _ = daemonExecFn("systemctl", "daemon-reload")
	return daemonResult(cmd, "ok", "uninstall", nil, "")
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	return runDaemonSystemctlAction(cmd, "start")
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	return runDaemonSystemctlAction(cmd, "stop")
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	return runDaemonSystemctlAction(cmd, "restart")
}

func runDaemonSystemctlAction(cmd *cobra.Command, action string) error {
	if daemonOS != "linux" {
		return daemonResult(cmd, "error", action, map[string]any{"os": daemonOS}, "daemon actions currently support Linux systemd only")
	}
	if daemonCurrentEUID() != 0 {
		return daemonResult(cmd, "error", action, nil, fmt.Sprintf("%s requires root privileges", action))
	}
	out, err := daemonExecFn("systemctl", action, "kafclaw-gateway.service")
	if err != nil {
		return daemonResult(cmd, "error", action, map[string]any{"output": strings.TrimSpace(string(out))}, err.Error())
	}
	return daemonResult(cmd, "ok", action, map[string]any{"output": strings.TrimSpace(string(out))}, "")
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	if daemonOS != "linux" {
		return daemonResult(cmd, "error", "status", map[string]any{"os": daemonOS}, "daemon status currently supports Linux systemd only")
	}
	enabledOut, enabledErr := daemonExecFn("systemctl", "is-enabled", "kafclaw-gateway.service")
	activeOut, activeErr := daemonExecFn("systemctl", "is-active", "kafclaw-gateway.service")
	result := map[string]any{
		"enabled": strings.TrimSpace(string(enabledOut)),
		"active":  strings.TrimSpace(string(activeOut)),
	}
	if enabledErr != nil || activeErr != nil {
		return daemonResult(cmd, "error", "status", result, "service not enabled/active")
	}
	return daemonResult(cmd, "ok", "status", result, "")
}

func daemonResult(cmd *cobra.Command, status, action string, result map[string]any, errMsg string) error {
	if daemonJSON {
		payload := map[string]any{
			"status":  strings.TrimSpace(status),
			"command": "daemon",
			"action":  strings.TrimSpace(action),
		}
		if len(result) > 0 {
			payload["result"] = result
		}
		if strings.TrimSpace(errMsg) != "" {
			payload["error"] = strings.TrimSpace(errMsg)
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
		if strings.EqualFold(status, "error") {
			return fmt.Errorf("%s", errMsg)
		}
		return nil
	}
	if strings.EqualFold(status, "error") {
		return fmt.Errorf("%s", errMsg)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "daemon %s: ok\n", action)
	return nil
}
