// Package kshark provides Kafka connectivity diagnostics.
// Vendored from https://github.com/scalytics/kshark-core (Apache 2.0).
package kshark

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// LoadProperties reads a Java-style .properties file into a map.
func LoadProperties(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	props := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		sep := "="
		if !strings.Contains(line, "=") && strings.Contains(line, ":") {
			sep = ":"
		}
		parts := strings.SplitN(line, sep, 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		props[key] = val
	}
	return props, sc.Err()
}

// ApplyPreset applies a named configuration preset to the properties map.
func ApplyPreset(preset string, p map[string]string) {
	switch preset {
	case "cc-plain":
		setDefault(p, "security.protocol", "SASL_SSL")
		setDefault(p, "sasl.mechanism", "PLAIN")
	case "self-scram":
		setDefault(p, "security.protocol", "SASL_SSL")
		setDefault(p, "sasl.mechanism", "SCRAM-SHA-512")
	case "plaintext":
		setDefault(p, "security.protocol", "PLAINTEXT")
	}
}

func setDefault(p map[string]string, k, v string) {
	if _, ok := p[k]; !ok {
		p[k] = v
	}
}

// KafkaAuthKind represents SASL authentication types.
type KafkaAuthKind int

const (
	AuthNone KafkaAuthKind = iota
	AuthPLAIN
	AuthSCRAM256
	AuthSCRAM512
	AuthGSSAPI
)

// TLSConfigFromProps builds a *tls.Config from properties.
func TLSConfigFromProps(p map[string]string, serverName string) (*tls.Config, string, error) {
	secProto := strings.ToUpper(p["security.protocol"])
	if secProto == "" {
		secProto = "PLAINTEXT"
	}
	useTLS := secProto == "SSL" || secProto == "SASL_SSL"

	conf := &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12}
	desc := "no TLS"
	if !useTLS {
		return nil, desc, nil
	}

	if ca := p["ssl.ca.location"]; ca != "" {
		pem, err := os.ReadFile(ca)
		if err != nil {
			return nil, "", fmt.Errorf("load CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, "", errors.New("bad CA PEM")
		}
		conf.RootCAs = pool
	}
	certFile := p["ssl.certificate.location"]
	keyFile := p["ssl.key.location"]
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, "", fmt.Errorf("load client cert: %w", err)
		}
		conf.Certificates = []tls.Certificate{cert}
	}
	desc = "TLS enabled"
	return conf, desc, nil
}

// SASLFromProps extracts SASL auth kind and credentials from properties.
func SASLFromProps(p map[string]string) (KafkaAuthKind, map[string]string, error) {
	secProto := strings.ToUpper(p["security.protocol"])
	mech := strings.ToUpper(p["sasl.mechanism"])

	switch mech {
	case "PLAIN":
		return AuthPLAIN, map[string]string{
			"username": p["sasl.username"],
			"password": p["sasl.password"],
		}, nil
	case "SCRAM-SHA-256":
		return AuthSCRAM256, map[string]string{
			"username": p["sasl.username"],
			"password": p["sasl.password"],
		}, nil
	case "SCRAM-SHA-512":
		return AuthSCRAM512, map[string]string{
			"username": p["sasl.username"],
			"password": p["sasl.password"],
		}, nil
	case "GSSAPI", "KERBEROS":
		return AuthGSSAPI, map[string]string{
			"service.name": p["sasl.kerberos.service.name"],
			"principal":    p["sasl.kerberos.principal"],
			"realm":        p["sasl.kerberos.realm"],
		}, nil
	case "":
		if secProto == "SSL" || secProto == "PLAINTEXT" || secProto == "" {
			return AuthNone, nil, nil
		}
		return AuthNone, nil, fmt.Errorf("missing sasl.mechanism for security.protocol=%s", secProto)
	default:
		return AuthNone, nil, fmt.Errorf("unsupported sasl.mechanism: %s", mech)
	}
}

// DialerFromProps builds a kafka.Dialer with TLS and SASL configured.
func DialerFromProps(p map[string]string, hostForSNI string) (*kafka.Dialer, string, error) {
	tlsConf, tlsDesc, err := TLSConfigFromProps(p, hostForSNI)
	if err != nil {
		return nil, "", err
	}

	kind, kv, err := SASLFromProps(p)
	if err != nil {
		return nil, "", err
	}

	var mech sasl.Mechanism
	switch kind {
	case AuthPLAIN:
		mech = plain.Mechanism{Username: kv["username"], Password: kv["password"]}
	case AuthSCRAM256:
		m, e := scram.Mechanism(scram.SHA256, kv["username"], kv["password"])
		if e != nil {
			return nil, "", e
		}
		mech = m
	case AuthSCRAM512:
		m, e := scram.Mechanism(scram.SHA512, kv["username"], kv["password"])
		if e != nil {
			return nil, "", e
		}
		mech = m
	}

	d := &kafka.Dialer{
		Timeout:       8 * time.Second,
		DualStack:     true,
		TLS:           tlsConf,
		SASLMechanism: mech,
	}
	return d, tlsDesc, nil
}

// TransportFromProps builds a kafka.Transport with TLS and SASL configured.
func TransportFromProps(p map[string]string, timeout time.Duration) (*kafka.Transport, error) {
	tlsConf, _, err := TLSConfigFromProps(p, "")
	if err != nil {
		return nil, fmt.Errorf("tls config: %w", err)
	}

	kind, kv, err := SASLFromProps(p)
	if err != nil {
		return nil, fmt.Errorf("sasl config: %w", err)
	}

	var mech sasl.Mechanism
	switch kind {
	case AuthPLAIN:
		mech = plain.Mechanism{Username: kv["username"], Password: kv["password"]}
	case AuthSCRAM256:
		m, e := scram.Mechanism(scram.SHA256, kv["username"], kv["password"])
		if e != nil {
			return nil, e
		}
		mech = m
	case AuthSCRAM512:
		m, e := scram.Mechanism(scram.SHA512, kv["username"], kv["password"])
		if e != nil {
			return nil, e
		}
		mech = m
	}

	return &kafka.Transport{
		TLS:         tlsConf,
		SASL:        mech,
		DialTimeout: timeout,
	}, nil
}

// RedactProps returns a copy of the properties with sensitive values masked.
func RedactProps(p map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range p {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "password") || strings.Contains(lk, "secret") || k == "sasl.oauthbearer.token" || strings.Contains(lk, "key") || k == "basic.auth.user.info" {
			out[k] = "***"
		} else {
			out[k] = v
		}
	}
	return out
}
