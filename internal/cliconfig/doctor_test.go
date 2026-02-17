package cliconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if !strings.Contains(text, "FOO=bar") || !strings.Contains(text, "OPENAI_API_KEY=from_openclaw") {
		t.Fatalf("missing expected merged keys in env file: %s", text)
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
