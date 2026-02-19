package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorCommandPassesWithMissingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	out, err := runRootCommand(t, "doctor")
	if err != nil {
		t.Fatalf("doctor command failed unexpectedly: %v", err)
	}
	if !strings.Contains(out, "[WARN] config_file:") {
		t.Fatalf("expected config_file warning in output, got %q", out)
	}
}

func TestDoctorCommandFailsOnInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"gateway":`), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	out, err := runRootCommand(t, "doctor")
	if err == nil {
		t.Fatal("expected doctor command failure for invalid config")
	}
	if !strings.Contains(out, "[FAIL] config_load:") {
		t.Fatalf("expected config_load failure in output, got %q", out)
	}
}

func TestDoctorFixDoesNotChangeGatewayHost(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"gateway":{"host":"0.0.0.0","authToken":"token"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t, "doctor", "--fix"); err != nil {
		t.Fatalf("doctor --fix failed unexpectedly: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cfgDir, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), `"host":"0.0.0.0"`) && !strings.Contains(string(data), `"host": "0.0.0.0"`) {
		t.Fatalf("expected host unchanged after --fix, got %s", string(data))
	}
}

func TestDoctorJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	out, err := runRootCommand(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor --json failed unexpectedly: %v", err)
	}
	if !strings.Contains(out, `"command": "doctor"`) || !strings.Contains(out, `"result"`) {
		t.Fatalf("expected doctor json output, got %q", out)
	}
}
