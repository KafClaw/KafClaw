package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/spf13/cobra"
)

func TestDirectAgentCommand(t *testing.T) {
	commandHome(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"fixture agent answer"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Model.Name = "openai/fixture"
	cfg.Providers.OpenAI.APIKey = "fixture"
	cfg.Providers.OpenAI.APIBase = server.URL
	cfg.Providers.LocalWhisper.Enabled = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	oldMessage, oldSession := agentMessage, agentSessionID
	t.Cleanup(func() { agentMessage, agentSessionID = oldMessage, oldSession })
	agentMessage = "fixture question"
	agentSessionID = "cli:fixture"
	out := captureCommandOutput(t, func() { runAgent(nil, nil) })
	if !strings.Contains(out, "fixture agent answer") {
		t.Fatal(out)
	}
}

func TestDaemonActionsWithExecutorFixture(t *testing.T) {
	oldOS, oldUID, oldExec, oldJSON := daemonOS, daemonCurrentEUID, daemonExecFn, daemonJSON
	t.Cleanup(func() { daemonOS, daemonCurrentEUID, daemonExecFn, daemonJSON = oldOS, oldUID, oldExec, oldJSON })
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	actions := []func(*cobra.Command, []string) error{runDaemonStart, runDaemonStop, runDaemonRestart}
	for _, asJSON := range []bool{false, true} {
		daemonJSON = asJSON
		daemonOS = "unsupported"
		for _, fn := range actions {
			if err := fn(cmd, nil); err == nil {
				t.Fatal("unsupported OS accepted")
			}
		}
		if err := runDaemonUninstall(cmd, nil); err == nil {
			t.Fatal("unsupported OS accepted")
		}
		daemonOS = "linux"
		daemonCurrentEUID = func() int { return 1000 }
		for _, fn := range actions {
			if err := fn(cmd, nil); err == nil {
				t.Fatal("unprivileged action accepted")
			}
		}
		if err := runDaemonUninstall(cmd, nil); err == nil {
			t.Fatal("unprivileged uninstall accepted")
		}
		daemonCurrentEUID = func() int { return 0 }
		daemonExecFn = func(name string, args ...string) ([]byte, error) {
			if name != "systemctl" {
				t.Error("unexpected executor")
			}
			return []byte("fixture"), nil
		}
		for _, fn := range actions {
			if err := fn(cmd, nil); err != nil {
				t.Fatal(err)
			}
		}
		daemonExecFn = func(string, ...string) ([]byte, error) { return []byte("fixture failure"), errors.New("denied") }
		for _, fn := range actions {
			if err := fn(cmd, nil); err == nil {
				t.Fatal("systemctl failure hidden")
			}
		}
	}
}

func TestKsharkConfigAndCommand(t *testing.T) {
	root := commandHome(t)
	t.Chdir(root)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := propsFromConfig(); err == nil {
		t.Fatal("disabled group accepted")
	}
	cfg.Group.Enabled = true
	cfg.Group.KafkaBrokers = ""
	cfg.Group.LFSProxyURL = ""
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := propsFromConfig(); err == nil {
		t.Fatal("missing brokers accepted")
	}
	cfg.Group.LFSProxyURL = "https://broker.invalid"
	cfg.Group.LFSProxyAPIKey = "fixture"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	props, err := propsFromConfig()
	if err != nil || props["bootstrap.servers"] != "broker.invalid:9092" || props["sasl.password"] != "fixture" {
		t.Fatalf("proxy fallback: %v %v", props, err)
	}
	cfg.Group.KafkaBrokers = "bad-address"
	cfg.Group.KafkaSecurityProto = "SASL_SSL"
	cfg.Group.KafkaSASLMechanism = "PLAIN"
	cfg.Group.KafkaSASLUsername = "user"
	cfg.Group.KafkaSASLPassword = "pass"
	cfg.Group.KafkaTLSCAFile = "ca"
	cfg.Group.KafkaTLSCertFile = "cert"
	cfg.Group.KafkaTLSKeyFile = "key"
	cfg.Group.GroupName = "test"
	cfg.Group.ConsumerGroup = "test"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	props, err = propsFromConfig()
	if err != nil || props["ssl.ca.location"] != "ca" || props["sasl.username"] != "user" {
		t.Fatalf("explicit properties: %v %v", props, err)
	}
	oldAuto, oldProps, oldJSON, oldYes, oldTimeout, oldDiag, oldTopic, oldPreset := ksharkAuto, ksharkProps, ksharkJSONOut, ksharkYes, ksharkTimeout, ksharkDiag, ksharkTopic, ksharkPreset
	t.Cleanup(func() {
		ksharkAuto, ksharkProps, ksharkJSONOut, ksharkYes, ksharkTimeout, ksharkDiag, ksharkTopic, ksharkPreset = oldAuto, oldProps, oldJSON, oldYes, oldTimeout, oldDiag, oldTopic, oldPreset
	})
	ksharkAuto = false
	ksharkProps = ""
	if err := runKshark(nil, nil); err == nil {
		t.Fatal("missing input accepted")
	}
	ksharkProps = filepath.Join(root, "missing")
	if err := runKshark(nil, nil); err == nil {
		t.Fatal("missing file accepted")
	}
	path := filepath.Join(root, "client.properties")
	if err := os.WriteFile(path, []byte("bootstrap.servers=bad-address\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ksharkProps = path
	ksharkYes = true
	ksharkTimeout = time.Millisecond
	ksharkJSONOut = "fixture.json"
	ksharkDiag = false
	if err := runKshark(nil, nil); err == nil {
		t.Fatal("bad broker should fail scan")
	}
	if _, err := os.Stat(filepath.Join(root, "reports/fixture.json")); err != nil {
		t.Fatal("JSON report missing", err)
	}
	ksharkAuto = true
	ksharkTopic = ""
	ksharkPreset = "plaintext"
	if err := runKshark(nil, nil); err == nil {
		t.Fatal("bad auto broker should fail")
	}
	ksharkYes = false
	input, err := os.CreateTemp(root, "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	input.WriteString("no\n")
	input.Seek(0, 0)
	oldIn := os.Stdin
	os.Stdin = input
	defer func() { os.Stdin = oldIn }()
	if err := runKshark(nil, nil); err != nil {
		t.Fatal("cancelled scan should succeed", err)
	}
}
