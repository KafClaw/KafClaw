package cliconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePath(t *testing.T) {
	toks, err := parsePath(" gateway.allowFrom[0] ")
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(toks))
	}
	if toks[0].key != "gateway" || toks[1].key != "allowFrom" || toks[2].index == nil || *toks[2].index != 0 {
		t.Fatalf("unexpected tokens: %#v", toks)
	}

	if _, err := parsePath("a[nope]"); err == nil {
		t.Fatal("expected parse error for non-numeric index")
	}
	if _, err := parsePath("a[1"); err == nil {
		t.Fatal("expected parse error for missing closing bracket")
	}
}

func TestParseValue(t *testing.T) {
	v := parseValue("123")
	if n, ok := v.(float64); !ok || n != 123 {
		t.Fatalf("expected numeric JSON parse, got %#v", v)
	}
	v = parseValue(`{"a":1}`)
	m, ok := v.(map[string]any)
	if !ok || m["a"].(float64) != 1 {
		t.Fatalf("expected map parse, got %#v", v)
	}
	v = parseValue("plain-string")
	if s, ok := v.(string); !ok || s != "plain-string" {
		t.Fatalf("expected plain string fallback, got %#v", v)
	}
}

func TestPathHelpersWithArrayIndex(t *testing.T) {
	m := map[string]any{}
	path, _ := parsePath("a.b[0].c")
	root, err := setAtPath(m, path, 42)
	if err != nil {
		t.Fatalf("setAtPath: %v", err)
	}
	obj := root.(map[string]any)

	v, ok := getAtPath(obj, path)
	if !ok || v.(int) != 42 {
		t.Fatalf("expected nested indexed value, got %#v ok=%v", v, ok)
	}

	root, changed, err := unsetAtPath(obj, path)
	if err != nil {
		t.Fatalf("unsetAtPath error: %v", err)
	}
	if !changed {
		t.Fatal("expected unset success")
	}
	obj = root.(map[string]any)
	if _, ok := getAtPath(obj, path); ok {
		t.Fatal("expected key removed")
	}
}

func TestSetGetUnsetRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	cfgContent := `{
	  "gateway": {"port": 18790},
	  "model": {"name": "base"},
	  "channels": {"telegram": {"allowFrom": []}}
	}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if err := Set("gateway.port", "9999"); err != nil {
		t.Fatalf("set gateway.port: %v", err)
	}
	v, err := Get("gateway.port")
	if err != nil {
		t.Fatalf("get gateway.port: %v", err)
	}
	if n, ok := v.(float64); !ok || n != 9999 {
		t.Fatalf("expected 9999, got %#v", v)
	}

	if err := Set("channels.telegram.allowFrom[0]", `"alice"`); err != nil {
		t.Fatalf("set array value: %v", err)
	}
	v, err = Get("channels.telegram.allowFrom[0]")
	if err != nil {
		t.Fatalf("get array value: %v", err)
	}
	if s, ok := v.(string); !ok || s != "alice" {
		t.Fatalf("expected alice, got %#v", v)
	}

	if err := Set("custom.section.values[0]", `"hello"`); err != nil {
		t.Fatalf("set custom.section.values[0]: %v", err)
	}
	if err := Unset("custom.section.values[0]"); err != nil {
		t.Fatalf("unset custom.section.values[0]: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		t.Fatalf("read config after unset: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal config after unset: %v", err)
	}
	custom, ok := m["custom"].(map[string]any)
	if !ok {
		t.Fatalf("expected custom object to exist")
	}
	section, ok := custom["section"].(map[string]any)
	if !ok {
		t.Fatalf("expected custom.section object to exist")
	}
	values, ok := section["values"].([]any)
	if !ok {
		t.Fatalf("expected custom.section.values array to exist")
	}
	if len(values) != 0 {
		t.Fatalf("expected custom.section.values to be empty after unset, got len=%d", len(values))
	}
}

func TestSetCreatesConfigFileWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if err := Set("group.enabled", "true"); err != nil {
		t.Fatalf("set when missing config: %v", err)
	}

	path := filepath.Join(tmpDir, ".kafclaw", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read created config: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal created config: %v", err)
	}
	group, ok := m["group"].(map[string]any)
	if !ok {
		t.Fatalf("expected group map in created config: %#v", m)
	}
	if group["enabled"] != true {
		t.Fatalf("expected group.enabled=true, got %#v", group["enabled"])
	}
}

func TestUnsetMissingPathReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if err := Unset("missing.path"); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestInvalidPathErrors(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"gateway":{"port":18790}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, err := Get(" "); err == nil {
		t.Fatal("expected get path parse error")
	}
	if err := Set("a[", "1"); err == nil {
		t.Fatal("expected set path parse error")
	}
	if err := Unset("a[-1]"); err == nil {
		t.Fatal("expected unset path parse error")
	}
}

func TestLoadFileConfigMapInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".kafclaw")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"bad":`), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	_ = os.Setenv("HOME", tmpDir)

	if _, _, err := loadFileConfigMap(); err == nil {
		t.Fatal("expected loadFileConfigMap error for invalid JSON")
	}
}

func TestSetUnsetEmptyPathGuards(t *testing.T) {
	root := map[string]any{}
	if _, err := setAtPath(root, nil, 1); err == nil {
		t.Fatal("expected setAtPath empty path error")
	}
	if _, _, err := unsetAtPath(root, nil); err == nil {
		t.Fatal("expected unsetAtPath empty path error")
	}
}

func TestSaveFileConfigMapMarshalError(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, ".kafclaw", "config.json")
	bad := map[string]any{
		"bad": func() {},
	}
	if err := saveFileConfigMap(cfgPath, bad); err == nil {
		t.Fatal("expected saveFileConfigMap to fail on non-JSON-serializable values")
	}
}
