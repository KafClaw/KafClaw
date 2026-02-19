package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/onboarding"
	"github.com/spf13/cobra"
)

func TestOnboardNonInteractiveRequiresAcceptRisk(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)
	origAcceptRisk := onboardAcceptRisk
	defer func() { onboardAcceptRisk = origAcceptRisk }()
	onboardAcceptRisk = false

	_, err := runRootCommand(t, "onboard", "--non-interactive", "--mode=local", "--llm=skip", "--skip-skills")
	if err == nil {
		t.Fatal("expected onboard to fail without --accept-risk in non-interactive mode")
	}
}

func TestOnboardJSONSummary(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	out, err := runRootCommand(t, "onboard", "--non-interactive", "--accept-risk", "--mode=local", "--llm=skip", "--skip-skills", "--json")
	if err != nil {
		t.Fatalf("onboard failed: %v", err)
	}
	if !strings.Contains(out, "\"configPath\"") || !strings.Contains(out, "\"skills\"") {
		t.Fatalf("expected onboarding JSON summary in output, got %q", out)
	}
}

func TestOnboardSkillsBootstrapPath(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origPath := os.Getenv("PATH")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("PATH", origPath)
	_ = os.Setenv("HOME", tmpDir)

	bin := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	for _, name := range []string{"node", "clawhub"} {
		script := filepath.Join(bin, name)
		if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	_ = os.Setenv("PATH", bin+string(os.PathListSeparator)+origPath)

	out, err := runRootCommand(t, "onboard", "--non-interactive", "--accept-risk", "--mode=local", "--llm=skip", "--skip-skills=false", "--json")
	if err != nil {
		t.Fatalf("onboard failed: %v", err)
	}
	if !strings.Contains(out, "\"enabled\": true") && !strings.Contains(out, "\"enabled\":true") {
		t.Fatalf("expected skills enabled in onboarding output, got %q", out)
	}
}

func TestOnboardConfiguresOAuthCapabilities(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t,
		"onboard",
		"--non-interactive",
		"--accept-risk",
		"--mode=local",
		"--llm=skip",
		"--skip-skills",
		"--google-workspace-read=mail,drive",
		"--m365-read=calendar",
	); err != nil {
		t.Fatalf("onboard failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".kafclaw", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	skillsCfg, ok := cfg["skills"].(map[string]any)
	if !ok {
		t.Fatalf("missing skills config")
	}
	entries, ok := skillsCfg["entries"].(map[string]any)
	if !ok {
		t.Fatalf("missing skills.entries")
	}
	if _, ok := entries["google-workspace"].(map[string]any); !ok {
		t.Fatalf("expected google-workspace entry, got %#v", entries["google-workspace"])
	}
	if _, ok := entries["m365"].(map[string]any); !ok {
		t.Fatalf("expected m365 entry, got %#v", entries["m365"])
	}
}

func TestOnboardConfiguresKafkaSecurity(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t,
		"onboard",
		"--non-interactive",
		"--accept-risk",
		"--skip-skills",
		"--mode=local-kafka",
		"--llm=skip",
		"--kafka-brokers=broker1:9092,broker2:9092",
		"--kafka-security-protocol=SASL_SSL",
		"--kafka-sasl-mechanism=SCRAM-SHA-512",
		"--kafka-sasl-username=svc-user",
		"--kafka-sasl-password=svc-pass",
		"--kafka-tls-ca-file=/etc/ssl/kafka-ca.pem",
		"--kafka-tls-cert-file=/etc/ssl/client.pem",
		"--kafka-tls-key-file=/etc/ssl/client.key",
	); err != nil {
		t.Fatalf("onboard failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".kafclaw", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	group, ok := cfg["group"].(map[string]any)
	if !ok {
		t.Fatalf("missing group section")
	}
	if v, _ := group["kafkaSecurityProtocol"].(string); v != "SASL_SSL" {
		t.Fatalf("expected kafkaSecurityProtocol SASL_SSL, got %q", v)
	}
	if v, _ := group["kafkaSaslMechanism"].(string); v != "SCRAM-SHA-512" {
		t.Fatalf("expected kafkaSaslMechanism SCRAM-SHA-512, got %q", v)
	}
	if v, _ := group["kafkaSaslUsername"].(string); v != "svc-user" {
		t.Fatalf("expected kafkaSaslUsername svc-user, got %q", v)
	}
	if v, _ := group["kafkaTlsCAFile"].(string); v != "/etc/ssl/kafka-ca.pem" {
		t.Fatalf("expected kafkaTlsCAFile set, got %q", v)
	}
}

func TestOnboardNonInteractiveRequiresModeAndLLM(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)
	origMode := onboardMode
	origProfile := onboardProfile
	origLLM := onboardLLMPreset
	defer func() {
		onboardMode = origMode
		onboardProfile = origProfile
		onboardLLMPreset = origLLM
	}()
	onboardMode = ""
	onboardProfile = ""
	onboardLLMPreset = ""

	if _, err := runRootCommand(t, "onboard", "--non-interactive", "--accept-risk", "--skip-skills"); err == nil {
		t.Fatal("expected non-interactive onboarding to fail without mode/profile and llm")
	}
}

func TestOnboardNonInteractiveNonLoopbackRequiresAck(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t, "onboard",
		"--non-interactive",
		"--accept-risk",
		"--skip-skills",
		"--mode=remote",
		"--llm=skip",
	); err == nil {
		t.Fatal("expected remote non-loopback onboarding to require --allow-non-loopback")
	}

	if _, err := runRootCommand(t, "onboard",
		"--non-interactive",
		"--accept-risk",
		"--skip-skills",
		"--mode=remote",
		"--llm=skip",
		"--allow-non-loopback",
	); err != nil {
		t.Fatalf("expected onboarding to pass with non-loopback acknowledgement: %v", err)
	}
}

func TestOnboardPersistsGatewayPort(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t, "onboard",
		"--non-interactive",
		"--accept-risk",
		"--skip-skills",
		"--mode=local",
		"--llm=skip",
		"--gateway-port=19990",
	); err != nil {
		t.Fatalf("onboard failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".kafclaw", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	gateway, ok := cfg["gateway"].(map[string]any)
	if !ok {
		t.Fatalf("missing gateway config")
	}
	port, ok := gateway["port"].(float64)
	if !ok {
		t.Fatalf("missing gateway.port")
	}
	if int(port) != 19990 {
		t.Fatalf("expected gateway.port 19990, got %d", int(port))
	}
}

func TestValidateOnboardNonInteractiveFlags(t *testing.T) {
	origNonInteractive := onboardNonInteractive
	origMode := onboardMode
	origProfile := onboardProfile
	origLLM := onboardLLMPreset
	defer func() {
		onboardNonInteractive = origNonInteractive
		onboardMode = origMode
		onboardProfile = origProfile
		onboardLLMPreset = origLLM
	}()

	onboardNonInteractive = false
	if err := validateOnboardNonInteractiveFlags(); err != nil {
		t.Fatalf("expected nil when non-interactive is disabled, got %v", err)
	}

	onboardNonInteractive = true
	onboardMode = ""
	onboardProfile = ""
	onboardLLMPreset = "skip"
	if err := validateOnboardNonInteractiveFlags(); err == nil {
		t.Fatal("expected mode/profile validation error")
	}

	onboardMode = "local"
	onboardLLMPreset = ""
	if err := validateOnboardNonInteractiveFlags(); err == nil {
		t.Fatal("expected llm validation error")
	}

	onboardMode = "local"
	onboardLLMPreset = "skip"
	if err := validateOnboardNonInteractiveFlags(); err != nil {
		t.Fatalf("expected valid non-interactive flags, got %v", err)
	}
}

func TestPreflightOnboardingConfigValidation(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".kafclaw", "config.json")

	if err := preflightOnboardingConfig(nil, cfgPath); err == nil {
		t.Fatal("expected nil config preflight error")
	}

	cfg := config.DefaultConfig()
	cfg.Paths.Workspace = ""
	if err := preflightOnboardingConfig(cfg, cfgPath); err == nil {
		t.Fatal("expected empty workspace preflight error")
	}

	cfg = config.DefaultConfig()
	cfg.Paths.Workspace = filepath.Join(tmpDir, "workspace")
	cfg.Gateway.Port = 0
	if err := preflightOnboardingConfig(cfg, cfgPath); err == nil {
		t.Fatal("expected invalid gateway.port preflight error")
	}

	cfg = config.DefaultConfig()
	cfg.Paths.Workspace = filepath.Join(tmpDir, "workspace")
	cfg.Gateway.Port = 18790
	cfg.Gateway.DashboardPort = 18790
	if err := preflightOnboardingConfig(cfg, cfgPath); err == nil {
		t.Fatal("expected gateway port collision preflight error")
	}

	cfg = config.DefaultConfig()
	cfg.Paths.Workspace = filepath.Join(tmpDir, "workspace")
	cfg.Gateway.Port = 18790
	cfg.Gateway.DashboardPort = 0
	if err := preflightOnboardingConfig(cfg, cfgPath); err == nil {
		t.Fatal("expected invalid gateway.dashboardPort preflight error")
	}

	cfgDirAsFile := filepath.Join(tmpDir, "cfg-parent-file")
	if err := os.WriteFile(cfgDirAsFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write cfg parent file: %v", err)
	}
	cfg = config.DefaultConfig()
	cfg.Paths.Workspace = filepath.Join(tmpDir, "workspace")
	cfg.Gateway.Port = 18790
	cfg.Gateway.DashboardPort = 18791
	if err := preflightOnboardingConfig(cfg, filepath.Join(cfgDirAsFile, "config.json")); err == nil {
		t.Fatal("expected config directory creation preflight error")
	}

	wsParentAsFile := filepath.Join(tmpDir, "workspace-parent-file")
	if err := os.WriteFile(wsParentAsFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write workspace parent file: %v", err)
	}
	cfg = config.DefaultConfig()
	cfg.Paths.Workspace = filepath.Join(wsParentAsFile, "workspace")
	cfg.Gateway.Port = 18790
	cfg.Gateway.DashboardPort = 18791
	if err := preflightOnboardingConfig(cfg, cfgPath); err == nil {
		t.Fatal("expected workspace mkdir preflight error")
	}

	cfg = config.DefaultConfig()
	cfg.Paths.Workspace = filepath.Join(tmpDir, "workspace")
	cfg.Gateway.Port = 18790
	cfg.Gateway.DashboardPort = 18791
	if err := preflightOnboardingConfig(cfg, cfgPath); err != nil {
		t.Fatalf("expected valid preflight, got %v", err)
	}
}

func TestEnforceGatewayBindSafety(t *testing.T) {
	origNonInteractive := onboardNonInteractive
	origAllowNonLoopback := onboardAllowNonLoopback
	defer func() {
		onboardNonInteractive = origNonInteractive
		onboardAllowNonLoopback = origAllowNonLoopback
	}()

	cmd := &cobra.Command{}
	in := bytes.NewBufferString("")
	out := &bytes.Buffer{}
	cmd.SetIn(in)
	cmd.SetOut(out)

	if err := enforceGatewayBindSafety(nil, cmd); err != nil {
		t.Fatalf("expected nil for nil config, got %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	if err := enforceGatewayBindSafety(cfg, cmd); err != nil {
		t.Fatalf("expected nil for loopback host, got %v", err)
	}

	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.AuthToken = ""
	if err := enforceGatewayBindSafety(cfg, cmd); err == nil {
		t.Fatal("expected auth token enforcement error")
	}

	cfg.Gateway.AuthToken = "token"
	onboardAllowNonLoopback = true
	onboardNonInteractive = true
	if err := enforceGatewayBindSafety(cfg, cmd); err != nil {
		t.Fatalf("expected acknowledged non-loopback to pass, got %v", err)
	}

	onboardAllowNonLoopback = false
	onboardNonInteractive = true
	if err := enforceGatewayBindSafety(cfg, cmd); err == nil {
		t.Fatal("expected non-interactive acknowledgement error")
	}

	onboardNonInteractive = false
	cmd = &cobra.Command{}
	cmd.SetIn(bytes.NewBufferString("n\n"))
	cmd.SetOut(&bytes.Buffer{})
	if err := enforceGatewayBindSafety(cfg, cmd); err == nil {
		t.Fatal("expected interactive abort for non-loopback bind")
	}

	cmd = &cobra.Command{}
	cmd.SetIn(bytes.NewBufferString("y\n"))
	cmd.SetOut(&bytes.Buffer{})
	if err := enforceGatewayBindSafety(cfg, cmd); err != nil {
		t.Fatalf("expected interactive approval to pass, got %v", err)
	}
}

func TestApplyOnboardResetScope(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".kafclaw", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)
	envPath := filepath.Join(tmpDir, ".config", "kafclaw", "env")
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		t.Fatalf("mkdir env dir: %v", err)
	}
	if err := os.WriteFile(envPath, []byte("TOKEN=x"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}

	if err := applyOnboardResetScope("config", cfgPath); err != nil {
		t.Fatalf("config reset failed: %v", err)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("expected config removed, stat err=%v", err)
	}
	if _, err := os.Stat(envPath); err != nil {
		t.Fatalf("expected env to remain on config reset: %v", err)
	}

	if err := os.WriteFile(cfgPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := applyOnboardResetScope("full", cfgPath); err != nil {
		t.Fatalf("full reset failed: %v", err)
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatalf("expected env removed by full reset, stat err=%v", err)
	}
	if err := applyOnboardResetScope("weird", cfgPath); err == nil {
		t.Fatal("expected invalid scope error")
	}
}

func TestWaitForGatewayHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if !waitForGatewayHealth(srv.URL, 2*time.Second) {
		t.Fatal("expected health check to succeed")
	}
	if waitForGatewayHealth("", time.Second) {
		t.Fatal("expected empty URL to fail")
	}
	if waitForGatewayHealth(srv.URL, 0) {
		t.Fatal("expected zero timeout to fail")
	}
}

func TestRunOnboardSystemdAutoActivateNonRoot(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	origSetup := onboardSetupSystemdFn
	origActivate := onboardActivateSystemdFn
	origEUID := onboardCurrentEUID
	origOS := onboardOS
	defer func() {
		onboardSetupSystemdFn = origSetup
		onboardActivateSystemdFn = origActivate
		onboardCurrentEUID = origEUID
		onboardOS = origOS
	}()

	onboardSetupSystemdFn = func(opts onboarding.SetupOptions) (*onboarding.SetupResult, error) {
		return &onboarding.SetupResult{
			ServicePath:  "/etc/systemd/system/kafclaw-gateway.service",
			OverridePath: "/home/kafclaw/.config/systemd/user/kafclaw-gateway.service.d/override.conf",
			EnvPath:      "/home/kafclaw/.config/kafclaw/env",
		}, nil
	}
	activateCalls := 0
	onboardActivateSystemdFn = func() error {
		activateCalls++
		return nil
	}
	onboardOS = "linux"
	onboardCurrentEUID = func() int { return 1000 }

	out, err := runRootCommandWithStdoutCapture(t,
		"onboard",
		"--non-interactive",
		"--accept-risk",
		"--mode=local",
		"--llm=skip",
		"--skip-skills",
		"--systemd",
	)
	if err != nil {
		t.Fatalf("onboard failed: %v", err)
	}
	if !strings.Contains(out, "Auto-activate skipped (requires root).") {
		t.Fatalf("expected non-root auto-activate message, got %q", out)
	}
	if activateCalls != 0 {
		t.Fatalf("expected no activate call for non-root, got %d", activateCalls)
	}
}

func TestRunOnboardPersistsDaemonRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t,
		"onboard",
		"--non-interactive",
		"--accept-risk",
		"--mode=local",
		"--llm=skip",
		"--skip-skills",
		"--daemon-runtime=systemd",
	); err != nil {
		t.Fatalf("onboard failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := strings.TrimSpace(cfg.Gateway.DaemonRuntime); got != "systemd" {
		t.Fatalf("expected daemon runtime systemd, got %q", got)
	}
}

func TestRunOnboardingHealthGateWaitForGateway(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hostPort := strings.TrimPrefix(srv.URL, "http://")
	parts := strings.Split(hostPort, ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected test server URL: %s", srv.URL)
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	origSkip := onboardSkipHealthcheck
	origWait := onboardWaitForGateway
	origTimeout := onboardHealthTimeout
	defer func() {
		onboardSkipHealthcheck = origSkip
		onboardWaitForGateway = origWait
		onboardHealthTimeout = origTimeout
	}()

	onboardSkipHealthcheck = false
	onboardWaitForGateway = true
	onboardHealthTimeout = 2 * time.Second

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = parts[0]
	cfg.Gateway.Port = port
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	summary := runOnboardingHealthGate(cmd, cfg)
	if !summary.GatewayReachable {
		t.Fatalf("expected gateway reachable summary, got %+v", summary)
	}
}

func TestNormalizeOnboardPathsExpandsTilde(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	cfg := config.DefaultConfig()
	cfg.Paths.Workspace = "~/KafClaw-Workspace"
	cfg.Paths.WorkRepoPath = "~/KafClaw-Workspace"
	cfg.Paths.SystemRepoPath = "~/.kafclaw/system"
	if err := normalizeOnboardPaths(cfg); err != nil {
		t.Fatalf("normalize paths: %v", err)
	}
	if strings.Contains(cfg.Paths.Workspace, "~") {
		t.Fatalf("expected expanded workspace path, got %q", cfg.Paths.Workspace)
	}
	if !strings.HasPrefix(cfg.Paths.Workspace, tmpDir) {
		t.Fatalf("expected workspace path under HOME %q, got %q", tmpDir, cfg.Paths.Workspace)
	}
}

func TestRunOnboardDoesNotCreateLiteralTildeDir(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}

	origHome := os.Getenv("HOME")
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Setenv("HOME", origHome)
	defer func() { _ = os.Chdir(origWD) }()
	_ = os.Setenv("HOME", tmpDir)
	if err := os.Chdir(runDir); err != nil {
		t.Fatalf("chdir runDir: %v", err)
	}

	if _, err := runRootCommand(t,
		"onboard",
		"--non-interactive",
		"--accept-risk",
		"--mode=local",
		"--llm=skip",
		"--skip-skills",
	); err != nil {
		t.Fatalf("onboard failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(runDir, "~")); !os.IsNotExist(err) {
		t.Fatalf("expected no literal '~' directory in cwd; stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "KafClaw-Workspace")); err != nil {
		t.Fatalf("expected workspace under HOME, got err=%v", err)
	}
}
