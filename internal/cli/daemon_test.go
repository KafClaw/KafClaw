package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/onboarding"
	"github.com/spf13/cobra"
)

func TestRunDaemonInstallNonLinux(t *testing.T) {
	origOS := daemonOS
	origJSON := daemonJSON
	defer func() {
		daemonOS = origOS
		daemonJSON = origJSON
	}()

	daemonOS = "darwin"
	daemonJSON = true
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	if err := runDaemonInstall(cmd, nil); err == nil {
		t.Fatal("expected non-linux daemon install error")
	}
}

func TestRunDaemonInstallPersistsRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	origOS := daemonOS
	origJSON := daemonJSON
	origActivate := daemonActivate
	origRuntime := daemonRuntimeLabel
	origSetup := daemonSetupSystemdFn
	defer func() {
		daemonOS = origOS
		daemonJSON = origJSON
		daemonActivate = origActivate
		daemonRuntimeLabel = origRuntime
		daemonSetupSystemdFn = origSetup
	}()

	daemonOS = "linux"
	daemonJSON = true
	daemonActivate = false
	daemonRuntimeLabel = "systemd"
	daemonSetupSystemdFn = func(opts onboarding.SetupOptions) (*onboarding.SetupResult, error) {
		return &onboarding.SetupResult{
			ServicePath:  "/etc/systemd/system/kafclaw-gateway.service",
			OverridePath: "/home/kafclaw/.config/systemd/user/kafclaw-gateway.service.d/override.conf",
			EnvPath:      "/home/kafclaw/.config/kafclaw/env",
		}, nil
	}

	out := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	if err := runDaemonInstall(cmd, nil); err != nil {
		t.Fatalf("daemon install failed: %v", err)
	}
	if !strings.Contains(out.String(), `"status": "ok"`) {
		t.Fatalf("expected json ok output, got %q", out.String())
	}

	updated, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if strings.TrimSpace(updated.Gateway.DaemonRuntime) != "systemd" {
		t.Fatalf("expected persisted daemon runtime systemd, got %q", updated.Gateway.DaemonRuntime)
	}
}

func TestRunDaemonInstallActivationRequiresRoot(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	cfg := config.DefaultConfig()
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	origOS := daemonOS
	origJSON := daemonJSON
	origActivate := daemonActivate
	origSetup := daemonSetupSystemdFn
	origEUID := daemonCurrentEUID
	defer func() {
		daemonOS = origOS
		daemonJSON = origJSON
		daemonActivate = origActivate
		daemonSetupSystemdFn = origSetup
		daemonCurrentEUID = origEUID
	}()

	daemonOS = "linux"
	daemonJSON = true
	daemonActivate = true
	daemonCurrentEUID = func() int { return 1000 }
	daemonSetupSystemdFn = func(opts onboarding.SetupOptions) (*onboarding.SetupResult, error) {
		return &onboarding.SetupResult{
			ServicePath: "/etc/systemd/system/kafclaw-gateway.service",
		}, nil
	}

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	if err := runDaemonInstall(cmd, nil); err == nil {
		t.Fatal("expected activation root privilege error")
	}
}

func TestRunDaemonStatusJSON(t *testing.T) {
	origOS := daemonOS
	origJSON := daemonJSON
	origExec := daemonExecFn
	defer func() {
		daemonOS = origOS
		daemonJSON = origJSON
		daemonExecFn = origExec
	}()

	daemonOS = "linux"
	daemonJSON = true
	daemonExecFn = func(name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "is-enabled" {
			return []byte("enabled\n"), nil
		}
		return []byte("active\n"), nil
	}

	out := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	if err := runDaemonStatus(cmd, nil); err != nil {
		t.Fatalf("daemon status failed: %v", err)
	}
	if !strings.Contains(out.String(), `"enabled": "enabled"`) || !strings.Contains(out.String(), `"active": "active"`) {
		t.Fatalf("expected daemon status output fields, got %q", out.String())
	}
}
