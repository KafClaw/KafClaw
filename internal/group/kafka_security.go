package group

import (
	"fmt"
	"net"
	"strings"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/kshark"
	"github.com/segmentio/kafka-go"
)

// BuildKafkaPropsFromGroupConfig maps group config into kafka client properties.
// Defaults keep plaintext installs working; security settings are opt-in.
func BuildKafkaPropsFromGroupConfig(cfg config.GroupConfig) map[string]string {
	props := map[string]string{}

	sec := strings.ToUpper(strings.TrimSpace(cfg.KafkaSecurityProto))
	if sec != "" {
		props["security.protocol"] = sec
	} else if strings.TrimSpace(cfg.LFSProxyAPIKey) != "" {
		// KafScale convention fallback when only proxy key is present.
		props["security.protocol"] = "SASL_SSL"
	}

	mech := strings.ToUpper(strings.TrimSpace(cfg.KafkaSASLMechanism))
	user := strings.TrimSpace(cfg.KafkaSASLUsername)
	pass := strings.TrimSpace(cfg.KafkaSASLPassword)
	if mech != "" {
		props["sasl.mechanism"] = mech
	}
	if user != "" {
		props["sasl.username"] = user
	}
	if pass != "" {
		props["sasl.password"] = pass
	}

	if strings.TrimSpace(cfg.LFSProxyAPIKey) != "" && props["sasl.password"] == "" {
		// Backward-compatible auth fallback.
		props["sasl.mechanism"] = "PLAIN"
		props["sasl.username"] = "token"
		props["sasl.password"] = strings.TrimSpace(cfg.LFSProxyAPIKey)
	}

	if v := strings.TrimSpace(cfg.KafkaTLSCAFile); v != "" {
		props["ssl.ca.location"] = v
	}
	if v := strings.TrimSpace(cfg.KafkaTLSCertFile); v != "" {
		props["ssl.certificate.location"] = v
	}
	if v := strings.TrimSpace(cfg.KafkaTLSKeyFile); v != "" {
		props["ssl.key.location"] = v
	}

	return props
}

// BuildKafkaDialerFromGroupConfig creates a kafka dialer using optional TLS/SASL settings.
func BuildKafkaDialerFromGroupConfig(cfg config.GroupConfig) (*kafka.Dialer, error) {
	props := BuildKafkaPropsFromGroupConfig(cfg)
	host := firstBrokerHost(cfg.KafkaBrokers)
	dialer, _, err := kshark.DialerFromProps(props, host)
	if err != nil {
		return nil, fmt.Errorf("kafka dialer config invalid: %w", err)
	}
	return dialer, nil
}

func firstBrokerHost(brokers string) string {
	first := strings.TrimSpace(strings.Split(brokers, ",")[0])
	if first == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(first)
	if err == nil {
		return host
	}
	// tolerate host without port
	return first
}
