package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/timeline"
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

func TestConfigureEmbeddingSwitchRequiresConfirmWhenEmbeddingsExist(t *testing.T) {
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

	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{
	  "memory": {"embedding": {"enabled": true, "provider": "local-hf", "model": "old-model", "dimension": 384}}
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	tl, err := timeline.NewTimelineService(filepath.Join(cfgDir, "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	defer tl.Close()
	if _, err := tl.DB().Exec(`INSERT INTO memory_chunks (id, content, embedding, source) VALUES (?, ?, ?, ?)`,
		"chunk-1", "hello", []byte{1, 2, 3, 4}, "conversation:test"); err != nil {
		t.Fatalf("seed memory chunk: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	_, err = runRootCommand(t,
		"configure",
		"--non-interactive",
		"--memory-embedding-model=new-model",
	)
	if err == nil {
		t.Fatal("expected embedding switch to require --confirm-memory-wipe")
	}
	if !strings.Contains(err.Error(), "--confirm-memory-wipe") {
		t.Fatalf("expected confirm-memory-wipe error, got %v", err)
	}
}

func TestConfigureEmbeddingSwitchWithConfirmWipesMemory(t *testing.T) {
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

	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{
	  "memory": {"embedding": {"enabled": true, "provider": "local-hf", "model": "old-model", "dimension": 384}}
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	tl, err := timeline.NewTimelineService(filepath.Join(cfgDir, "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	defer tl.Close()
	if _, err := tl.DB().Exec(`INSERT INTO memory_chunks (id, content, embedding, source) VALUES (?, ?, ?, ?)`,
		"chunk-1", "hello", []byte{1, 2, 3, 4}, "conversation:test"); err != nil {
		t.Fatalf("seed memory chunk: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t,
		"configure",
		"--non-interactive",
		"--memory-embedding-model=new-model",
		"--confirm-memory-wipe",
	); err != nil {
		t.Fatalf("configure with confirmed wipe failed: %v", err)
	}
	var count int
	if err := tl.DB().QueryRow(`SELECT COUNT(*) FROM memory_chunks`).Scan(&count); err != nil {
		t.Fatalf("count memory chunks: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected memory chunks wiped, got count=%d", count)
	}
	var auditCount int
	if err := tl.DB().QueryRow(`SELECT COUNT(*) FROM timeline WHERE classification = 'MEMORY_EMBEDDING_SWITCH'`).Scan(&auditCount); err != nil {
		t.Fatalf("count embedding switch audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one embedding switch audit event, got %d", auditCount)
	}
}

func TestConfigureEmbeddingFirstEnableDoesNotWipeTextOnlyRows(t *testing.T) {
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

	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{
	  "memory": {"embedding": {"enabled": false, "provider": "disabled", "model": "", "dimension": 384}}
	}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	tl, err := timeline.NewTimelineService(filepath.Join(cfgDir, "timeline.db"))
	if err != nil {
		t.Fatalf("open timeline: %v", err)
	}
	defer tl.Close()
	if _, err := tl.DB().Exec(`INSERT INTO memory_chunks (id, content, embedding, source) VALUES (?, ?, ?, ?)`,
		"chunk-1", "hello", nil, "conversation:test"); err != nil {
		t.Fatalf("seed text-only memory chunk: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := runRootCommand(t,
		"configure",
		"--non-interactive",
		"--memory-embedding-enabled-set",
		"--memory-embedding-enabled=true",
		"--memory-embedding-provider=local-hf",
		"--memory-embedding-model=BAAI/bge-small-en-v1.5",
		"--memory-embedding-dimension=384",
	); err != nil {
		t.Fatalf("configure first embedding enable failed: %v", err)
	}
	var count int
	if err := tl.DB().QueryRow(`SELECT COUNT(*) FROM memory_chunks`).Scan(&count); err != nil {
		t.Fatalf("count memory chunks: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected memory rows preserved when enabling first embedding, got count=%d", count)
	}
}

func TestConfigureAgentCascadeFlag(t *testing.T) {
	origKafkaSecurityProtocol := configureKafkaSecurityProtocol
	origKafkaSASLMechanism := configureKafkaSASLMechanism
	origKafkaSASLUsername := configureKafkaSASLUsername
	origKafkaSASLPassword := configureKafkaSASLPassword
	origAgentID := configureAgentID
	origAgentCascadeEnabled := configureAgentCascadeEnabled
	origJSON := configureJSON
	origKafkaBrokers := configureKafkaBrokers
	origKafkaTLSCAFile := configureKafkaTLSCAFile
	origKafkaTLSCertFile := configureKafkaTLSCertFile
	origKafkaTLSKeyFile := configureKafkaTLSKeyFile
	origSkillsNodeManager := configureSkillsNodeManager
	origSkillsScope := configureSkillsScope
	origGoogleWorkspaceRead := configureGoogleWorkspaceRead
	origM365Read := configureM365Read
	origSubagentsAllow := configureSubagentsAllowAgents
	origSubagentsShare := configureSubagentsMemoryShareMode
	origMemoryProvider := configureMemoryEmbeddingProvider
	origMemoryModel := configureMemoryEmbeddingModel
	origEnableSkills := configureEnableSkills
	origDisableSkills := configureDisableSkills
	defer func() {
		configureKafkaSecurityProtocol = origKafkaSecurityProtocol
		configureKafkaSASLMechanism = origKafkaSASLMechanism
		configureKafkaSASLUsername = origKafkaSASLUsername
		configureKafkaSASLPassword = origKafkaSASLPassword
		configureAgentID = origAgentID
		configureAgentCascadeEnabled = origAgentCascadeEnabled
		configureJSON = origJSON
		configureKafkaBrokers = origKafkaBrokers
		configureKafkaTLSCAFile = origKafkaTLSCAFile
		configureKafkaTLSCertFile = origKafkaTLSCertFile
		configureKafkaTLSKeyFile = origKafkaTLSKeyFile
		configureSkillsNodeManager = origSkillsNodeManager
		configureSkillsScope = origSkillsScope
		configureGoogleWorkspaceRead = origGoogleWorkspaceRead
		configureM365Read = origM365Read
		configureSubagentsAllowAgents = origSubagentsAllow
		configureSubagentsMemoryShareMode = origSubagentsShare
		configureMemoryEmbeddingProvider = origMemoryProvider
		configureMemoryEmbeddingModel = origMemoryModel
		configureEnableSkills = origEnableSkills
		configureDisableSkills = origDisableSkills
	}()
	configureKafkaSecurityProtocol = ""
	configureKafkaSASLMechanism = ""
	configureKafkaSASLUsername = ""
	configureKafkaSASLPassword = ""
	configureAgentID = ""
	configureAgentCascadeEnabled = false
	configureJSON = false
	configureKafkaBrokers = ""
	configureKafkaTLSCAFile = ""
	configureKafkaTLSCertFile = ""
	configureKafkaTLSKeyFile = ""
	configureSkillsNodeManager = ""
	configureSkillsScope = ""
	configureGoogleWorkspaceRead = ""
	configureM365Read = ""
	configureSubagentsAllowAgents = ""
	configureSubagentsMemoryShareMode = ""
	configureMemoryEmbeddingProvider = ""
	configureMemoryEmbeddingModel = ""
	configureEnableSkills = nil
	configureDisableSkills = nil

	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"group":{"agentId":"ops-agent"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	out, err := runRootCommand(t,
		"configure",
		"--non-interactive",
		"--agent-cascade-enabled-set",
		"--agent-cascade-enabled=true",
	)
	if err != nil {
		t.Fatalf("configure agent cascade failed: %v", err)
	}
	if !strings.Contains(out, "deterministic tasks") {
		t.Fatalf("expected deterministic warning in output, got %q", out)
	}

	data, err := os.ReadFile(filepath.Join(cfgDir, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	agents, ok := cfg["agents"].(map[string]any)
	if !ok {
		t.Fatalf("expected agents block in config")
	}
	list, ok := agents["list"].([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("expected one agents.list entry, got %#v", agents["list"])
	}
	entry, _ := list[0].(map[string]any)
	if id, _ := entry["id"].(string); id != "ops-agent" {
		t.Fatalf("expected agent id ops-agent, got %#v", entry["id"])
	}
	cascade, _ := entry["cascade"].(map[string]any)
	if enabled, _ := cascade["enabled"].(bool); !enabled {
		t.Fatalf("expected cascade.enabled true, got %#v", cascade["enabled"])
	}
}

func TestConfigureAgentCascadeJSONSummary(t *testing.T) {
	origKafkaSecurityProtocol := configureKafkaSecurityProtocol
	origKafkaSASLMechanism := configureKafkaSASLMechanism
	origKafkaSASLUsername := configureKafkaSASLUsername
	origKafkaSASLPassword := configureKafkaSASLPassword
	origAgentID := configureAgentID
	origAgentCascadeEnabled := configureAgentCascadeEnabled
	origJSON := configureJSON
	origKafkaBrokers := configureKafkaBrokers
	origKafkaTLSCAFile := configureKafkaTLSCAFile
	origKafkaTLSCertFile := configureKafkaTLSCertFile
	origKafkaTLSKeyFile := configureKafkaTLSKeyFile
	origSkillsNodeManager := configureSkillsNodeManager
	origSkillsScope := configureSkillsScope
	origGoogleWorkspaceRead := configureGoogleWorkspaceRead
	origM365Read := configureM365Read
	origSubagentsAllow := configureSubagentsAllowAgents
	origSubagentsShare := configureSubagentsMemoryShareMode
	origMemoryProvider := configureMemoryEmbeddingProvider
	origMemoryModel := configureMemoryEmbeddingModel
	origEnableSkills := configureEnableSkills
	origDisableSkills := configureDisableSkills
	defer func() {
		configureKafkaSecurityProtocol = origKafkaSecurityProtocol
		configureKafkaSASLMechanism = origKafkaSASLMechanism
		configureKafkaSASLUsername = origKafkaSASLUsername
		configureKafkaSASLPassword = origKafkaSASLPassword
		configureAgentID = origAgentID
		configureAgentCascadeEnabled = origAgentCascadeEnabled
		configureJSON = origJSON
		configureKafkaBrokers = origKafkaBrokers
		configureKafkaTLSCAFile = origKafkaTLSCAFile
		configureKafkaTLSCertFile = origKafkaTLSCertFile
		configureKafkaTLSKeyFile = origKafkaTLSKeyFile
		configureSkillsNodeManager = origSkillsNodeManager
		configureSkillsScope = origSkillsScope
		configureGoogleWorkspaceRead = origGoogleWorkspaceRead
		configureM365Read = origM365Read
		configureSubagentsAllowAgents = origSubagentsAllow
		configureSubagentsMemoryShareMode = origSubagentsShare
		configureMemoryEmbeddingProvider = origMemoryProvider
		configureMemoryEmbeddingModel = origMemoryModel
		configureEnableSkills = origEnableSkills
		configureDisableSkills = origDisableSkills
	}()
	configureKafkaSecurityProtocol = ""
	configureKafkaSASLMechanism = ""
	configureKafkaSASLUsername = ""
	configureKafkaSASLPassword = ""
	configureAgentID = ""
	configureAgentCascadeEnabled = false
	configureJSON = false
	configureKafkaBrokers = ""
	configureKafkaTLSCAFile = ""
	configureKafkaTLSCertFile = ""
	configureKafkaTLSKeyFile = ""
	configureSkillsNodeManager = ""
	configureSkillsScope = ""
	configureGoogleWorkspaceRead = ""
	configureM365Read = ""
	configureSubagentsAllowAgents = ""
	configureSubagentsMemoryShareMode = ""
	configureMemoryEmbeddingProvider = ""
	configureMemoryEmbeddingModel = ""
	configureEnableSkills = nil
	configureDisableSkills = nil

	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"group":{"agentId":"ops-agent"}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	out, err := runRootCommand(t,
		"configure",
		"--non-interactive",
		"--json",
		"--agent-id=qa-agent",
		"--agent-cascade-enabled-set",
		"--agent-cascade-enabled=false",
	)
	if err != nil {
		t.Fatalf("configure agent cascade json failed: %v", err)
	}
	if !strings.Contains(out, `"id": "qa-agent"`) || !strings.Contains(out, `"cascadeEnabled": false`) {
		t.Fatalf("expected agent cascade json summary, got %q", out)
	}
}
