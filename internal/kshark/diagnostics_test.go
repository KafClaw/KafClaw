package kshark

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

func TestDiagnosticHTTPChecks(t *testing.T) {
	for _, code := range []int{200, 401, 403, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/subjects" {
					u, p, ok := r.BasicAuth()
					if !ok || u != "test" || p != "fixture" {
						t.Error("missing registry basic auth")
					}
				}
				w.WriteHeader(code)
			}))
			defer server.Close()
			props := map[string]string{"schema.registry.url": server.URL, "basic.auth.user.info": "test:fixture"}
			report := &Report{}
			checkSchemaRegistry(context.Background(), report, props)
			checkRESTProxy(report, props, server.URL)
			if len(report.Rows) != 4 {
				t.Fatalf("expected DNS and HTTP checks: %+v", report.Rows)
			}
			want := OK
			if code == 401 || code == 403 {
				want = FAIL
			} else if code != 200 {
				want = WARN
			}
			if report.Rows[1].Status != want || report.Rows[3].Status != want {
				t.Fatalf("status: %+v", report.Rows)
			}
			server.Close()
			report = &Report{}
			checkRESTProxy(report, props, server.URL)
			checkSchemaRegistry(context.Background(), report, props)
			if !report.HasFailed {
				t.Fatal("offline server not reported")
			}
			props["security.protocol"] = "SSL"
			props["ssl.ca.location"] = "/missing/ca.pem"
			checkRESTProxy(report, props, server.URL)
			checkSchemaRegistry(context.Background(), report, props)
		})
	}
}

func TestNetworkAndTLSChecks(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	addr := strings.TrimPrefix(server.URL, "https://")
	report := &Report{}
	checkDNS(report, "127.0.0.1", "test")
	checkDNS(report, "invalid host!", "test")
	conn := checkTCP(report, addr, "test", time.Second)
	if conn == nil {
		t.Fatal("local TCP failed")
	}
	secured := wrapTLS(report, conn, &tls.Config{InsecureSkipVerify: true}, "test", addr)
	if secured == nil {
		t.Fatal("local TLS failed")
	}
	secured.Close()
	conn = checkTCP(report, addr, "test", time.Second)
	if conn == nil {
		t.Fatal("local TCP failed")
	}
	if wrapTLS(report, conn, &tls.Config{ServerName: "invalid.example"}, "test", addr) != nil {
		t.Fatal("invalid certificate accepted")
	}
	conn.Close()
	server.Close()
	if checkTCP(report, addr, "test", time.Millisecond) != nil {
		t.Fatal("closed listener accepted")
	}
	state := &tls.ConnectionState{}
	if peerCN(state) != "-" {
		t.Fatal("missing peer name")
	}
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "peer"}, NotAfter: time.Now().Add(time.Hour)}
	state.PeerCertificates = []*x509.Certificate{cert}
	if peerCN(state) != "peer" || !earliestExpiry(state).Equal(cert.NotAfter) {
		t.Fatal("certificate details wrong")
	}
	cert.DNSNames = []string{"peer.example"}
	if peerCN(state) != "peer.example" {
		t.Fatal("DNS name not preferred")
	}
}

func TestScanAndCommandDiagnostics(t *testing.T) {
	t.Chdir(t.TempDir())
	old := scanLog
	t.Cleanup(func() { scanLog = old })
	if _, err := Run(Options{}); err == nil {
		t.Fatal("nil properties accepted")
	}
	if _, err := Run(Options{Props: map[string]string{}}); err == nil {
		t.Fatal("missing bootstrap accepted")
	}
	for _, opts := range []Options{{Props: map[string]string{"bootstrap.servers": "bad-address"}, StartOffset: "invalid"}, {Props: map[string]string{"bootstrap.servers": "127.0.0.1:1"}, Topics: []string{""}, Timeout: time.Nanosecond}} {
		report, err := Run(opts)
		if err != nil || !report.HasFailed || report.FinishedAt.Before(report.StartedAt) {
			t.Fatalf("scan report: %+v %v", report, err)
		}
	}
	if f, err := initScanLog(""); err != nil || f != nil {
		t.Fatal("empty log path")
	}
	if err := os.WriteFile("blocker", []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := initScanLog("blocker/log"); err == nil {
		t.Fatal("non-directory log parent accepted")
	}
	root := t.TempDir()
	t.Setenv("PATH", root)
	script := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	report := &Report{}
	bestEffortTraceroute(report, ";invalid")
	mtuCheck(report, ";invalid")
	bestEffortTraceroute(report, "localhost")
	mtuCheck(report, "localhost")
	for _, name := range []string{"traceroute", "tracepath", "tracert"} {
		script(name, "printf 'route fixture\\n'")
		bestEffortTraceroute(report, "localhost")
		if err := os.Remove(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	script("tracepath", "printf 'pmtu 1500\\n'")
	mtuCheck(report, "localhost")
	os.Remove(filepath.Join(root, "tracepath"))
	script("ping", "printf '1 packets transmitted\\n'")
	mtuCheck(report, "localhost")
	if out, err := runCmdIfExists("ping"); err != nil || !strings.Contains(out, "packets transmitted") {
		t.Fatalf("fake ping: %s %v", out, err)
	}
	if !report.HasFailed {
		t.Fatal("invalid host not failed")
	}
	if topicLeaders(map[string]string{"sasl.mechanism": "invalid"}, "127.0.0.1:1", "test", time.Millisecond) != nil {
		t.Fatal("invalid auth leaders")
	}
	if topicLeaders(nil, "127.0.0.1:1", "test", time.Millisecond) != nil {
		t.Fatal("offline leaders")
	}
	if _, err := kafkaConn(report, map[string]string{"sasl.mechanism": "invalid"}, "127.0.0.1:1", time.Millisecond); err == nil {
		t.Fatal("invalid auth accepted")
	}
	checkTopic(report, nil, "127.0.0.1:1", "topic", time.Millisecond)
	probeProduceConsume(context.Background(), report, nil, "", "", "", 0, 0, "", 0, 0)
	probeProduceConsume(context.Background(), report, map[string]string{"sasl.mechanism": "invalid"}, "127.0.0.1:1", "topic", "", 0, 0, "", 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probeProduceConsume(ctx, report, nil, "127.0.0.1:1", "topic", "", time.Millisecond, time.Millisecond, "", time.Millisecond, 0)
	for _, status := range []CheckStatus{OK, WARN, FAIL, SKIP, "unknown"} {
		addRow(report, Row{Status: status, Layer: L7, Hint: "fixture"})
		if detailSummary(status) == "" {
			t.Fatal("empty summary")
		}
	}
	summarize(report)
	PrintPretty(report)
}

func TestKafkaErrorClassificationAndBalancing(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{{kafka.TopicAuthorizationFailed, "topic ACL"}, {kafka.GroupAuthorizationFailed, "group ACL"}, {kafka.SASLAuthenticationFailed, "SASL"}, {kafka.RequestTimedOut, "timeout"}, {kafka.LeaderNotAvailable, "leader/coord"}, {kafka.NotLeaderForPartition, "leader/coord"}, {kafka.GroupCoordinatorNotAvailable, "leader/coord"}, {kafka.NotCoordinatorForGroup, "leader/coord"}, {context.DeadlineExceeded, "timeout"}, {errors.New("TOPIC_AUTHORIZATION_FAILED"), "topic ACL"}, {errors.New("GROUP_AUTHORIZATION_FAILED"), "group ACL"}, {errors.New("SASL_AUTHENTICATION_FAILED"), "SASL"}, {errors.New("NOT_LEADER"), "leader/coord"}, {errors.New("other"), "other"}} {
		if got := policyHint("Read", fmt.Errorf("wrapped: %w", tc.err)); !strings.Contains(got, tc.want) {
			t.Fatalf("%v: %s", tc.err, got)
		}
		_ = hint(tc.err)
	}
	for _, err := range []error{nil, errors.New("authentication"), errors.New("certificate"), errors.New("AUTHORIZATION"), &net.DNSError{IsTimeout: true}, errors.New("i/o timeout")} {
		_ = hint(err)
		_ = isTimeout(err)
	}
	if policyHint("Read", nil) != "Read OK" {
		t.Fatal("success hint")
	}
	for _, name := range []string{"rr", "random", "least"} {
		b := selectBalancer(name)
		for range 5 {
			p := b.Balance(kafka.Message{}, 1, 2, 3)
			if p < 1 || p > 3 {
				t.Fatalf("invalid partition: %d", p)
			}
		}
	}
	if selectBalancer("random").Balance(kafka.Message{}) != 0 {
		t.Fatal("empty partitions")
	}
	b := &loggingBalancer{leaders: map[int]kafka.Broker{0: {Host: "localhost", Port: 9092}}}
	if b.Balance(kafka.Message{}, 0) != 0 {
		t.Fatal("partition selection")
	}
	b.leaders = nil
	b.Balance(kafka.Message{}, 0)
	for _, mechanism := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		props := map[string]string{"security.protocol": "SASL_PLAINTEXT", "sasl.mechanism": mechanism, "sasl.username": "fixture", "sasl.password": "fixture"}
		d, _, err := DialerFromProps(props, "")
		if err != nil || d.SASLMechanism == nil {
			t.Fatalf("dialer %s: %v", mechanism, err)
		}
		tr, err := TransportFromProps(props, time.Second)
		if err != nil || tr.SASL == nil {
			t.Fatalf("transport %s: %v", mechanism, err)
		}
		tr.CloseIdleConnections()
	}
}
