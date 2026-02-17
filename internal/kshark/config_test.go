package kshark

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPropertiesAndPresets(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.properties")
	content := "# comment\nsecurity.protocol=SASL_SSL\n// ignore\nfoo: bar\ninvalidline\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write properties: %v", err)
	}

	props, err := LoadProperties(p)
	if err != nil {
		t.Fatalf("load properties: %v", err)
	}
	if props["security.protocol"] != "SASL_SSL" || props["foo"] != "bar" {
		t.Fatalf("unexpected properties: %+v", props)
	}

	ApplyPreset("plaintext", props)
	if props["security.protocol"] != "SASL_SSL" {
		t.Fatal("preset should not override existing values")
	}

	p2 := map[string]string{}
	ApplyPreset("self-scram", p2)
	if p2["sasl.mechanism"] != "SCRAM-SHA-512" {
		t.Fatalf("unexpected self-scram config: %+v", p2)
	}
}

func TestTLSConfigFromPropsBranches(t *testing.T) {
	conf, desc, err := TLSConfigFromProps(map[string]string{"security.protocol": "PLAINTEXT"}, "localhost")
	if err != nil || conf != nil || desc == "" {
		t.Fatalf("expected plaintext no TLS, got conf=%v desc=%q err=%v", conf, desc, err)
	}

	_, _, err = TLSConfigFromProps(map[string]string{
		"security.protocol": "SASL_SSL",
		"ssl.ca.location":   "/does/not/exist.pem",
	}, "localhost")
	if err == nil {
		t.Fatal("expected CA load error")
	}
}

func TestSASLFromPropsAndDialers(t *testing.T) {
	kind, kv, err := SASLFromProps(map[string]string{
		"security.protocol": "SASL_SSL",
		"sasl.mechanism":    "PLAIN",
		"sasl.username":     "u",
		"sasl.password":     "p",
	})
	if err != nil || kind != AuthPLAIN || kv["username"] != "u" {
		t.Fatalf("unexpected sasl plain parse: kind=%v kv=%v err=%v", kind, kv, err)
	}

	if _, _, err := SASLFromProps(map[string]string{"security.protocol": "SASL_SSL"}); err == nil {
		t.Fatal("expected missing sasl mechanism error")
	}
	if _, _, err := SASLFromProps(map[string]string{"sasl.mechanism": "UNKNOWN"}); err == nil {
		t.Fatal("expected unsupported sasl mechanism error")
	}

	dialer, tlsDesc, err := DialerFromProps(map[string]string{"security.protocol": "PLAINTEXT"}, "localhost")
	if err != nil || dialer == nil || tlsDesc == "" {
		t.Fatalf("unexpected dialer: dialer=%v desc=%q err=%v", dialer, tlsDesc, err)
	}

	tr, err := TransportFromProps(map[string]string{
		"security.protocol": "SASL_SSL",
		"sasl.mechanism":    "PLAIN",
		"sasl.username":     "u",
		"sasl.password":     "p",
	}, 2*time.Second)
	if err != nil || tr == nil {
		t.Fatalf("unexpected transport: %v %v", tr, err)
	}
}

func TestRedactProps(t *testing.T) {
	in := map[string]string{
		"sasl.password":          "secret",
		"basic.auth.user.info":   "a:b",
		"public":                 "ok",
		"ssl.key.location":       "/tmp/key.pem",
		"sasl.oauthbearer.token": "tok",
	}
	out := RedactProps(in)
	if out["public"] != "ok" {
		t.Fatal("expected public value unchanged")
	}
	if out["sasl.password"] != "***" || out["basic.auth.user.info"] != "***" || out["ssl.key.location"] != "***" {
		t.Fatalf("expected sensitive values masked: %+v", out)
	}
}
