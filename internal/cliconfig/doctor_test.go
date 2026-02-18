package cliconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	skillruntime "github.com/KafClaw/KafClaw/internal/skills"
)

func TestRunDoctorWithMissingConfigWarnsNoFailure(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunDoctor()
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("expected no failures with missing config, got %#v", report)
	}
}

func TestRunDoctorWithInvalidConfigFails(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"model":`), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunDoctor()
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	if !report.HasFailures() {
		t.Fatalf("expected failures for invalid config, got %#v", report)
	}
}

func TestRunDoctorRemoteModeRequiresAuthToken(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := `{
	  "gateway": {"host": "0.0.0.0", "port": 18790, "authToken": ""}
	}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunDoctor()
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	if !report.HasFailures() {
		t.Fatalf("expected failure for remote mode without auth token, got %#v", report)
	}
}

func TestDoctorFixMergesEnvFilesAndKeepsGatewayHost(t *testing.T) {
	tmpDir := t.TempDir()
	home := filepath.Join(tmpDir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".openclaw"), 0o755); err != nil {
		t.Fatalf("mkdir .openclaw: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".kafclaw"), 0o755); err != nil {
		t.Fatalf("mkdir .kafclaw: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".openclaw", ".env"), []byte("OPENAI_API_KEY=from_openclaw\n"), 0o600); err != nil {
		t.Fatalf("write openclaw env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".kafclaw", ".env"), []byte("MIKROBOT_GATEWAY_AUTH_TOKEN=abc123\n"), 0o600); err != nil {
		t.Fatalf("write kafclaw env: %v", err)
	}

	cfgDir := filepath.Join(home, ".kafclaw")
	cfg := `{"gateway":{"host":"0.0.0.0","authToken":"token"}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	origWD, _ := os.Getwd()
	defer os.Setenv("HOME", origHome)
	defer os.Chdir(origWD)
	_ = os.Setenv("HOME", home)
	_ = os.Chdir(tmpDir)
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("FOO=bar\n"), 0o600); err != nil {
		t.Fatalf("write cwd env: %v", err)
	}

	report, err := RunDoctorWithOptions(DoctorOptions{Fix: true})
	if err != nil {
		t.Fatalf("run doctor --fix: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("expected no failures, got %#v", report)
	}
	var tombSyncPass bool
	var scrubPass bool
	for _, c := range report.Checks {
		if c.Name == "env_tomb_sync" && c.Status == DoctorPass {
			tombSyncPass = true
		}
		if c.Name == "env_sensitive_scrub" && c.Status == DoctorPass {
			scrubPass = true
		}
	}
	if !tombSyncPass {
		t.Fatalf("expected env_tomb_sync pass check, got %#v", report.Checks)
	}
	if !scrubPass {
		t.Fatalf("expected env_sensitive_scrub pass check, got %#v", report.Checks)
	}

	target := filepath.Join(home, ".config", "kafclaw", "env")
	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat merged env file: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("expected env file mode 600, got %o", st.Mode().Perm())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read merged env file: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "FOO=bar") {
		t.Fatalf("missing non-sensitive key in merged env file: %s", text)
	}
	if strings.Contains(text, "OPENAI_API_KEY=from_openclaw") || strings.Contains(text, "MIKROBOT_GATEWAY_AUTH_TOKEN=abc123") {
		t.Fatalf("expected sensitive keys scrubbed from merged env file: %s", text)
	}
	tombSecrets, err := skillruntime.LoadEnvSecretsFromLocalTomb()
	if err != nil {
		t.Fatalf("load tomb env secrets: %v", err)
	}
	if tombSecrets["OPENAI_API_KEY"] != "from_openclaw" {
		t.Fatalf("expected OPENAI_API_KEY synced to tomb, got %#v", tombSecrets)
	}

	cfgAfter, err := os.ReadFile(filepath.Join(cfgDir, "config.json"))
	if err != nil {
		t.Fatalf("read config after doctor fix: %v", err)
	}
	if !strings.Contains(string(cfgAfter), `"host": "0.0.0.0"`) && !strings.Contains(string(cfgAfter), `"host":"0.0.0.0"`) {
		t.Fatalf("expected gateway host unchanged, got: %s", string(cfgAfter))
	}
}

func TestDoctorGenerateGatewayToken(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := `{"gateway":{"host":"127.0.0.1","authToken":""}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunDoctorWithOptions(DoctorOptions{GenerateGatewayToken: true})
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("expected no failures, got %#v", report)
	}

	cfgAfter, err := os.ReadFile(filepath.Join(cfgDir, "config.json"))
	if err != nil {
		t.Fatalf("read config after token generation: %v", err)
	}
	if strings.Contains(string(cfgAfter), `"authToken": ""`) || strings.Contains(string(cfgAfter), `"authToken":""`) {
		t.Fatalf("expected generated auth token, got: %s", string(cfgAfter))
	}
}

func TestDoctorReportsSlackTeamsAccountDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := `{
	  "gateway": {"host": "127.0.0.1", "authToken": "token"},
	  "channels": {
	    "slack": {"enabled": true, "botToken": "", "inboundToken": "", "outboundUrl": ""},
	    "msteams": {"enabled": true, "appId": "", "appPassword": "", "inboundToken": "", "outboundUrl": ""}
	  }
	}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunDoctor()
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	var slackWarn, teamsWarn bool
	for _, c := range report.Checks {
		if c.Name == "slack_account_default" && c.Status == DoctorWarn {
			slackWarn = true
		}
		if c.Name == "msteams_account_default" && c.Status == DoctorWarn {
			teamsWarn = true
		}
	}
	if !slackWarn || !teamsWarn {
		t.Fatalf("expected slack+teams account diagnostics warnings, got %#v", report.Checks)
	}
}

func TestDoctorSkillsDiagnosticsWarnWhenMissingNodeAndClawhub(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := `{
	  "skills": {"enabled": true, "externalInstalls": true},
	  "channels": {"slack": {"enabled": true}}
	}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	origPath := os.Getenv("PATH")
	defer os.Setenv("HOME", origHome)
	defer os.Setenv("PATH", origPath)
	_ = os.Setenv("HOME", tmpDir)
	_ = os.Setenv("PATH", "/definitely/not/found")

	report, err := RunDoctor()
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	var nodeWarn, clawhubWarn bool
	for _, c := range report.Checks {
		if c.Name == "skills_node" && c.Status == DoctorWarn {
			nodeWarn = true
		}
		if c.Name == "skills_clawhub" && c.Status == DoctorWarn {
			clawhubWarn = true
		}
	}
	if !nodeWarn || !clawhubWarn {
		t.Fatalf("expected skills/channel warnings, got %#v", report.Checks)
	}
}

func TestDoctorSkillsRuntimePermissionsFailOnInsecureDirs(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := `{"skills":{"enabled":true}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	dirs, err := skillruntime.ResolveStateDirs()
	if err != nil {
		t.Fatalf("resolve state dirs: %v", err)
	}
	for _, dir := range []string{dirs.Root, dirs.TmpDir, dirs.ToolsDir, dirs.Quarantine, dirs.Installed, dirs.Snapshots, dirs.AuditDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	report, err := RunDoctor()
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	foundFail := false
	for _, c := range report.Checks {
		if c.Name == "skills_runtime_permissions" && c.Status == DoctorFail {
			foundFail = true
			break
		}
	}
	if !foundFail {
		t.Fatalf("expected skills_runtime_permissions failure, got %#v", report.Checks)
	}
}

func TestDoctorWarnsWhenChannelOnboardingSkillInactive(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := `{
	  "skills": {"enabled": false},
	  "channels": {"slack": {"enabled": true}}
	}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	report, err := RunDoctor()
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	foundWarn := false
	for _, c := range report.Checks {
		if c.Name == "channel_onboarding_skill" && c.Status == DoctorWarn {
			foundWarn = true
			break
		}
	}
	if !foundWarn {
		t.Fatalf("expected channel_onboarding_skill warning, got %#v", report.Checks)
	}
}

func TestDoctorSkillsSecretPermissionsFailAndFix(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := `{"skills":{"enabled":true}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	dirs, err := skillruntime.EnsureStateDirs()
	if err != nil {
		t.Fatalf("ensure state dirs: %v", err)
	}
	authDir := filepath.Join(dirs.ToolsDir, "auth", "google-workspace", "default")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	tokenPath := filepath.Join(authDir, "token.json")
	if err := os.WriteFile(tokenPath, []byte(`{"access_token":"x"}`), 0o644); err != nil {
		t.Fatalf("write token: %v", err)
	}
	tombPath, err := skillruntime.ResolveLocalOAuthTombPath()
	if err != nil {
		t.Fatalf("resolve tomb path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(tombPath), 0o700); err != nil {
		t.Fatalf("mkdir tomb dir: %v", err)
	}
	if err := os.WriteFile(tombPath, []byte("badkey\n"), 0o644); err != nil {
		t.Fatalf("write tomb key: %v", err)
	}

	report, err := RunDoctor()
	if err != nil {
		t.Fatalf("run doctor: %v", err)
	}
	foundFail := false
	for _, c := range report.Checks {
		if c.Name == "skills_secret_permissions" && c.Status == DoctorFail {
			foundFail = true
			break
		}
	}
	if !foundFail {
		t.Fatalf("expected skills_secret_permissions failure, got %#v", report.Checks)
	}

	report, err = RunDoctorWithOptions(DoctorOptions{Fix: true})
	if err != nil {
		t.Fatalf("run doctor --fix: %v", err)
	}
	foundPass := false
	for _, c := range report.Checks {
		if c.Name == "skills_secret_permissions" && c.Status == DoctorPass {
			foundPass = true
			break
		}
	}
	if !foundPass {
		t.Fatalf("expected skills_secret_permissions pass after fix, got %#v", report.Checks)
	}
	st, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("stat token path: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("expected token file mode 600 after fix, got %o", st.Mode().Perm())
	}
	tombSt, err := os.Stat(tombPath)
	if err != nil {
		t.Fatalf("stat tomb path: %v", err)
	}
	if tombSt.Mode().Perm() != 0o600 {
		t.Fatalf("expected tomb key mode 600 after fix, got %o", tombSt.Mode().Perm())
	}
}

func TestEndpointLooksRemote(t *testing.T) {
	cases := []struct {
		endpoint string
		remote   bool
	}{
		{endpoint: "", remote: false},
		{endpoint: "http://127.0.0.1:8080", remote: false},
		{endpoint: "http://localhost:9000", remote: false},
		{endpoint: "https://example.com/api", remote: true},
		{endpoint: "definitely-not-a-url", remote: true},
	}
	for _, tc := range cases {
		got := endpointLooksRemote(tc.endpoint)
		if got != tc.remote {
			t.Fatalf("endpointLooksRemote(%q)=%v, want %v", tc.endpoint, got, tc.remote)
		}
	}
}

func TestTrimEnvQuotes(t *testing.T) {
	if got := trimEnvQuotes(`"abc"`); got != "abc" {
		t.Fatalf("expected double quotes trimmed, got %q", got)
	}
	if got := trimEnvQuotes(`'abc'`); got != "abc" {
		t.Fatalf("expected single quotes trimmed, got %q", got)
	}
	if got := trimEnvQuotes("abc"); got != "abc" {
		t.Fatalf("expected bare value unchanged, got %q", got)
	}
}
