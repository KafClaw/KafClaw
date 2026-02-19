package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseCSVList(t *testing.T) {
	got := parseCSVList(" agent-a,agent-b,agent-a,*, ")
	want := []string{"agent-a", "agent-b", "*"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected parse result: got=%v want=%v", got, want)
	}
}

func TestConfigureSkillsFlags(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"skills":{"enabled":false,"nodeManager":"npm","entries":{}}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t,
		"configure",
		"--non-interactive",
		"--skills-enabled-set",
		"--skills-enabled=true",
		"--skills-node-manager=pnpm",
		"--enable-skill=github",
		"--disable-skill=weather",
		"--google-workspace-read=mail,calendar",
		"--m365-read=files",
	); err != nil {
		t.Fatalf("configure skills flags failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cfgDir, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	skills, ok := cfg["skills"].(map[string]any)
	if !ok {
		t.Fatalf("missing skills section")
	}
	if enabled, _ := skills["enabled"].(bool); !enabled {
		t.Fatalf("expected skills enabled true, got %#v", skills["enabled"])
	}
	if nm, _ := skills["nodeManager"].(string); nm != "pnpm" {
		t.Fatalf("expected skills.nodeManager pnpm, got %q", nm)
	}
	entries, ok := skills["entries"].(map[string]any)
	if !ok {
		t.Fatalf("expected skills.entries object")
	}
	gh, ok := entries["github"].(map[string]any)
	if !ok {
		t.Fatalf("expected github entry override")
	}
	if v, _ := gh["enabled"].(bool); !v {
		t.Fatalf("expected github enabled override true")
	}
	weather, ok := entries["weather"].(map[string]any)
	if !ok {
		t.Fatalf("expected weather entry override")
	}
	if v, _ := weather["enabled"].(bool); v {
		t.Fatalf("expected weather enabled override false")
	}
	gws, ok := entries["google-workspace"].(map[string]any)
	if !ok {
		t.Fatalf("expected google-workspace entry override")
	}
	if v, _ := gws["enabled"].(bool); !v {
		t.Fatalf("expected google-workspace enabled override true")
	}
	if caps, ok := gws["capabilities"].([]any); !ok || len(caps) != 2 {
		t.Fatalf("expected google-workspace capabilities [mail calendar], got %#v", gws["capabilities"])
	}
	m365, ok := entries["m365"].(map[string]any)
	if !ok {
		t.Fatalf("expected m365 entry override")
	}
	if caps, ok := m365["capabilities"].([]any); !ok || len(caps) != 1 {
		t.Fatalf("expected m365 capabilities [files], got %#v", m365["capabilities"])
	}
}

func TestConfigureKafkaFlags(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"group":{"kafkaBrokers":"localhost:9092"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t,
		"configure",
		"--non-interactive",
		"--kafka-brokers=broker-a:9092,broker-b:9092",
		"--kafka-security-protocol=SASL_SSL",
		"--kafka-sasl-mechanism=PLAIN",
		"--kafka-sasl-username=svc",
		"--kafka-sasl-password=secret",
		"--kafka-tls-ca-file=/etc/ssl/ca.pem",
		"--kafka-tls-cert-file=/etc/ssl/client.pem",
		"--kafka-tls-key-file=/etc/ssl/client.key",
	); err != nil {
		t.Fatalf("configure kafka flags failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(cfgDir, "config.json"))
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
	if v, _ := group["kafkaSaslMechanism"].(string); v != "PLAIN" {
		t.Fatalf("expected kafkaSaslMechanism PLAIN, got %q", v)
	}
	if v, _ := group["kafkaSaslUsername"].(string); v != "svc" {
		t.Fatalf("expected kafkaSaslUsername svc, got %q", v)
	}
	if v, _ := group["kafkaTlsCAFile"].(string); v != "/etc/ssl/ca.pem" {
		t.Fatalf("expected kafkaTlsCAFile set, got %q", v)
	}
}

func TestConfigureKafkaInvalidSecurityProtocol(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"group":{"kafkaBrokers":"localhost:9092"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t, "configure", "--non-interactive", "--kafka-security-protocol=INVALID"); err == nil {
		t.Fatal("expected invalid kafka security protocol error")
	}
}

func TestConfigureKafkaInvalidSASLMechanism(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"group":{"kafkaBrokers":"localhost:9092"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t,
		"configure",
		"--non-interactive",
		"--kafka-security-protocol=SASL_SSL",
		"--kafka-sasl-mechanism=INVALID",
	); err == nil {
		t.Fatal("expected invalid kafka sasl mechanism error")
	}
}

func TestConfigureJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"group":{"kafkaBrokers":"localhost:9092"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)
	origKafkaSecurityProtocol := configureKafkaSecurityProtocol
	origKafkaSASLMechanism := configureKafkaSASLMechanism
	origKafkaSASLUsername := configureKafkaSASLUsername
	origKafkaSASLPassword := configureKafkaSASLPassword
	defer func() {
		configureKafkaSecurityProtocol = origKafkaSecurityProtocol
		configureKafkaSASLMechanism = origKafkaSASLMechanism
		configureKafkaSASLUsername = origKafkaSASLUsername
		configureKafkaSASLPassword = origKafkaSASLPassword
	}()
	configureKafkaSecurityProtocol = ""
	configureKafkaSASLMechanism = ""
	configureKafkaSASLUsername = ""
	configureKafkaSASLPassword = ""

	out, err := runRootCommand(t,
		"configure",
		"--non-interactive",
		"--json",
		"--kafka-brokers=broker-json:9092",
	)
	if err != nil {
		t.Fatalf("configure failed: %v", err)
	}
	if !strings.Contains(out, `"status": "ok"`) || !strings.Contains(out, `"command": "configure"`) {
		t.Fatalf("expected configure json output, got %q", out)
	}
}
