package group

import (
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
)

func TestBuildKafkaPropsFromGroupConfigExplicit(t *testing.T) {
	cfg := config.GroupConfig{
		KafkaSecurityProto: "SASL_SSL",
		KafkaSASLMechanism: "SCRAM-SHA-512",
		KafkaSASLUsername:  "svc-user",
		KafkaSASLPassword:  "svc-pass",
		KafkaTLSCAFile:     "/etc/ssl/ca.pem",
		KafkaTLSCertFile:   "/etc/ssl/client.pem",
		KafkaTLSKeyFile:    "/etc/ssl/client.key",
	}

	props := BuildKafkaPropsFromGroupConfig(cfg)
	if props["security.protocol"] != "SASL_SSL" {
		t.Fatalf("expected security.protocol SASL_SSL, got %q", props["security.protocol"])
	}
	if props["sasl.mechanism"] != "SCRAM-SHA-512" {
		t.Fatalf("expected sasl.mechanism SCRAM-SHA-512, got %q", props["sasl.mechanism"])
	}
	if props["sasl.username"] != "svc-user" {
		t.Fatalf("expected sasl.username svc-user, got %q", props["sasl.username"])
	}
	if props["ssl.ca.location"] != "/etc/ssl/ca.pem" {
		t.Fatalf("expected ssl.ca.location, got %q", props["ssl.ca.location"])
	}
}

func TestBuildKafkaPropsFromGroupConfigLFSFallback(t *testing.T) {
	cfg := config.GroupConfig{
		LFSProxyAPIKey: "api-key",
	}
	props := BuildKafkaPropsFromGroupConfig(cfg)
	if props["security.protocol"] != "SASL_SSL" {
		t.Fatalf("expected fallback security.protocol SASL_SSL, got %q", props["security.protocol"])
	}
	if props["sasl.mechanism"] != "PLAIN" {
		t.Fatalf("expected fallback sasl.mechanism PLAIN, got %q", props["sasl.mechanism"])
	}
	if props["sasl.username"] != "token" {
		t.Fatalf("expected fallback sasl.username token, got %q", props["sasl.username"])
	}
	if props["sasl.password"] != "api-key" {
		t.Fatalf("expected fallback sasl.password api-key, got %q", props["sasl.password"])
	}
}

func TestBuildKafkaDialerFromGroupConfig(t *testing.T) {
	cfg := config.GroupConfig{
		KafkaBrokers: "localhost:9092",
	}
	dialer, err := BuildKafkaDialerFromGroupConfig(cfg)
	if err != nil {
		t.Fatalf("expected dialer without error: %v", err)
	}
	if dialer == nil {
		t.Fatal("expected non-nil dialer")
	}
}

func TestBuildKafkaDialerFromGroupConfigInvalidSASL(t *testing.T) {
	cfg := config.GroupConfig{
		KafkaBrokers:       "localhost:9092",
		KafkaSecurityProto: "SASL_SSL",
	}
	if _, err := BuildKafkaDialerFromGroupConfig(cfg); err == nil {
		t.Fatal("expected invalid sasl configuration error")
	}
}

func TestFirstBrokerHost(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "broker-a:9092,broker-b:9092", want: "broker-a"},
		{in: "broker-a", want: "broker-a"},
		{in: "[::1]:9092", want: "::1"},
		{in: "", want: ""},
	}
	for _, tc := range cases {
		if got := firstBrokerHost(tc.in); got != tc.want {
			t.Fatalf("firstBrokerHost(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
