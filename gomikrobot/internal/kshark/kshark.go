package kshark

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

// Options configures a kshark diagnostic scan.
type Options struct {
	// Props is the Kafka client properties map (from file or auto-generated).
	Props map[string]string

	// Topics to test (comma-separated in CLI, pre-split here).
	Topics []string

	// Group is the consumer group for the produce/consume probe.
	Group string

	// JSONOut is the path for JSON report output (empty = no JSON).
	JSONOut string

	// Timeout is the global scan timeout.
	Timeout time.Duration

	// KafkaTimeout is the per-operation Kafka metadata/dial timeout.
	KafkaTimeout time.Duration

	// ProduceTimeout overrides OpTimeout for produce operations.
	ProduceTimeout time.Duration

	// ConsumeTimeout overrides OpTimeout for consume operations.
	ConsumeTimeout time.Duration

	// Balancer selects the partition balancer: "least", "rr", "random".
	Balancer string

	// StartOffset is "earliest" or "latest".
	StartOffset string

	// Diag enables traceroute/MTU diagnostics.
	Diag bool
}

// ---------- Package-level logger ----------

var scanLog *log.Logger

func logf(format string, args ...any) {
	if scanLog == nil {
		return
	}
	scanLog.Printf(format, args...)
}

func initScanLog(path string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	scanLog = log.New(f, "kshark ", log.LstdFlags|log.Lmicroseconds)
	return f, nil
}

// ---------- Scan entry point ----------

// Run executes a full kshark diagnostic scan and returns the report.
func Run(opts Options) (*Report, error) {
	if opts.Props == nil {
		return nil, errors.New("kshark: properties map is nil")
	}
	bootstrap := opts.Props["bootstrap.servers"]
	if bootstrap == "" {
		return nil, errors.New("kshark: bootstrap.servers missing from properties")
	}

	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.KafkaTimeout <= 0 {
		opts.KafkaTimeout = 10 * time.Second
	}
	if opts.ProduceTimeout <= 0 {
		opts.ProduceTimeout = 10 * time.Second
	}
	if opts.ConsumeTimeout <= 0 {
		opts.ConsumeTimeout = 10 * time.Second
	}

	// Set up logging
	logPath := filepath.Join("reports", fmt.Sprintf("kshark-%s.log", time.Now().Format("20060102-150405")))
	logFile, err := initScanLog(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kshark: failed to open log file: %v\n", err)
	} else if logFile != nil {
		defer logFile.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	report := &Report{
		StartedAt:  time.Now(),
		ConfigEcho: RedactProps(opts.Props),
	}

	logf("scan start bootstrap=%s timeout=%s kafka_timeout=%s topics=%v group=%s diag=%t",
		bootstrap, opts.Timeout, opts.KafkaTimeout, opts.Topics, opts.Group, opts.Diag)

	startOffsetVal, err := parseStartOffset(opts.StartOffset)
	if err != nil {
		logf("start-offset invalid value=%s err=%v, defaulting to earliest", opts.StartOffset, err)
		startOffsetVal = kafka.FirstOffset
	}

	// Per-broker checks
	brokers := strings.Split(bootstrap, ",")
	for _, b := range brokers {
		select {
		case <-ctx.Done():
			addRow(report, Row{"kshark", "timeout", DIAG, FAIL, "Global timeout reached during broker checks", ""})
			goto endScan
		default:
		}

		b = strings.TrimSpace(b)
		host, port, err := net.SplitHostPort(b)
		if err != nil {
			addRow(report, Row{"kafka", b, L3, FAIL, "Invalid host:port", "Fix bootstrap.servers format (host:port)."})
			continue
		}
		logf("broker check start host=%s port=%s", host, port)
		checkDNS(report, host, "kafka")
		addr := net.JoinHostPort(host, port)
		conn := checkTCP(report, addr, "kafka", 8*time.Second)
		if conn == nil {
			continue
		}
		tlsConf, _, err := TLSConfigFromProps(opts.Props, host)
		if err != nil {
			addRow(report, Row{"kafka", addr, L56, FAIL, fmt.Sprintf("TLS config err: %v", err), ""})
			_ = conn.Close()
			continue
		}
		var secured net.Conn = conn
		if tlsConf != nil {
			secured = wrapTLS(report, conn, tlsConf, "kafka", addr)
			if secured == nil {
				continue
			}
		} else {
			addRow(report, Row{"kafka", addr, L56, SKIP, "PLAINTEXT (no TLS)", "Prefer SSL/SASL_SSL."})
		}
		_ = secured.Close()

		for _, t := range opts.Topics {
			checkTopic(report, opts.Props, addr, t, opts.KafkaTimeout)
		}

		if opts.Diag {
			bestEffortTraceroute(report, host)
			mtuCheck(report, host)
		}
	}

	// Produce/Consume probes
	for _, t := range opts.Topics {
		select {
		case <-ctx.Done():
			addRow(report, Row{"kshark", "timeout", DIAG, FAIL, "Global timeout reached before produce/consume", ""})
			goto endScan
		default:
			probeProduceConsume(ctx, report, opts.Props, bootstrap, t, opts.Group,
				opts.ProduceTimeout, opts.ConsumeTimeout, opts.Balancer, opts.KafkaTimeout, startOffsetVal)
		}
	}

	// Schema Registry
	select {
	case <-ctx.Done():
		addRow(report, Row{"kshark", "timeout", DIAG, FAIL, "Global timeout reached before schema registry check", ""})
		goto endScan
	default:
		checkSchemaRegistry(ctx, report, opts.Props)
	}

endScan:
	// REST Proxy
	if rest := strings.TrimSpace(opts.Props["rest.proxy.url"]); rest != "" {
		checkRESTProxy(report, opts.Props, rest)
	}

	report.FinishedAt = time.Now()
	logf("scan finished duration=%s failed=%t", report.FinishedAt.Sub(report.StartedAt), report.HasFailed)
	summarize(report)

	return report, nil
}

// ---------- DNS, TCP, TLS checks ----------

func checkDNS(r *Report, host string, component string) {
	start := time.Now()
	_, err := net.LookupHost(host)
	logf("step dns host=%s dur=%s err=%v", host, time.Since(start).Truncate(time.Millisecond), err)
	if err != nil {
		addRow(r, Row{component, host, L3, FAIL, fmt.Sprintf("DNS lookup failed: %v", err),
			"Check /etc/hosts, DNS server, split-horizon/VPN search domains."})
	} else {
		addRow(r, Row{component, host, L3, OK, "Resolved host", ""})
	}
}

func checkTCP(r *Report, addr string, component string, timeout time.Duration) net.Conn {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		logf("step tcp addr=%s dur=%s err=%v", addr, time.Since(start).Truncate(time.Millisecond), err)
		addRow(r, Row{component, addr, L4, FAIL, fmt.Sprintf("TCP connect failed: %v", err),
			"Firewall, SG/NACL/NSG, LB listeners, PodNetworkPolicy, or routing."})
		return nil
	}
	lat := time.Since(start)
	logf("step tcp addr=%s dur=%s err=nil", addr, lat.Truncate(time.Millisecond))
	addRow(r, Row{component, addr, L4, OK, fmt.Sprintf("Connected in %s", lat.Truncate(time.Millisecond)), ""})
	return conn
}

func wrapTLS(r *Report, base net.Conn, tlsConf *tls.Config, component, addr string) net.Conn {
	if tlsConf == nil {
		addRow(r, Row{component, addr, L56, SKIP, "TLS not configured (PLAINTEXT)", "Prefer SSL/SASL_SSL for encryption."})
		return base
	}
	client := tls.Client(base, tlsConf)
	start := time.Now()
	if err := client.Handshake(); err != nil {
		logf("step tls addr=%s dur=%s err=%v", addr, time.Since(start).Truncate(time.Millisecond), err)
		addRow(r, Row{component, addr, L56, FAIL, fmt.Sprintf("TLS handshake failed: %v", err),
			"Check CA chain, SNI/hostname, client cert/key, and server certificate validity."})
		return nil
	}
	state := client.ConnectionState()
	logf("step tls addr=%s dur=%s err=nil", addr, time.Since(start).Truncate(time.Millisecond))
	exp := earliestExpiry(&state)
	detail := fmt.Sprintf("TLS %x; peer=%s; expires=%s", state.Version, peerCN(&state), exp.Format("2006-01-02"))
	if time.Until(exp) < (30 * 24 * time.Hour) {
		addRow(r, Row{component, addr, L56, WARN, detail, "Server certificate expires <30 days."})
	} else {
		addRow(r, Row{component, addr, L56, OK, detail, ""})
	}
	return client
}

func peerCN(st *tls.ConnectionState) string {
	if len(st.PeerCertificates) == 0 {
		return "-"
	}
	pc := st.PeerCertificates[0]
	if len(pc.DNSNames) > 0 {
		return pc.DNSNames[0]
	}
	return pc.Subject.CommonName
}

func earliestExpiry(st *tls.ConnectionState) time.Time {
	earliest := time.Now().Add(365 * 24 * time.Hour)
	for _, c := range st.PeerCertificates {
		if c.NotAfter.Before(earliest) {
			earliest = c.NotAfter
		}
	}
	return earliest
}

// ---------- Kafka protocol checks ----------

func kafkaConn(r *Report, p map[string]string, brokerAddr string, timeout time.Duration) (*kafka.Conn, error) {
	host, _, _ := net.SplitHostPort(brokerAddr)
	dialer, _, err := DialerFromProps(p, host)
	if err != nil {
		addRow(r, Row{"kafka", brokerAddr, L7, FAIL, fmt.Sprintf("dialer error: %v", err), "Check security.protocol & sasl.* settings."})
		return nil, err
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	dialStart := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", brokerAddr)
	if err != nil {
		logf("step kafka.dial addr=%s dur=%s err=%v", brokerAddr, time.Since(dialStart).Truncate(time.Millisecond), err)
		addRow(r, Row{"kafka", brokerAddr, L7, FAIL, fmt.Sprintf("broker dial failed: %v", err), "Auth/TLS mismatch or listener not exposed."})
		return nil, err
	}
	logf("step kafka.dial addr=%s dur=%s err=nil", brokerAddr, time.Since(dialStart).Truncate(time.Millisecond))
	apiStart := time.Now()
	if _, err := conn.ApiVersions(); err != nil {
		logf("step kafka.apiversions addr=%s dur=%s err=%v", brokerAddr, time.Since(apiStart).Truncate(time.Millisecond), err)
		addRow(r, Row{"kafka", brokerAddr, L7, FAIL, fmt.Sprintf("ApiVersions failed: %v", err), "Broker incompatible or proxy interfering."})
		_ = conn.Close()
		return nil, err
	}
	logf("step kafka.apiversions addr=%s dur=%s err=nil", brokerAddr, time.Since(apiStart).Truncate(time.Millisecond))
	addRow(r, Row{"kafka", brokerAddr, L7, OK, "ApiVersions OK", ""})
	return conn, nil
}

func checkTopic(r *Report, p map[string]string, brokerAddr, topic string, timeout time.Duration) {
	conn, err := kafkaConn(r, p, brokerAddr, timeout)
	if err != nil {
		return
	}
	defer conn.Close()

	partsStart := time.Now()
	parts, err := conn.ReadPartitions()
	if err != nil {
		logf("step kafka.readpartitions addr=%s dur=%s err=%v", brokerAddr, time.Since(partsStart).Truncate(time.Millisecond), err)
		addRow(r, Row{"kafka", brokerAddr, L7, FAIL, policyHint("ReadPartitions", err), hint(err)})
		return
	}
	logf("step kafka.readpartitions addr=%s dur=%s err=nil", brokerAddr, time.Since(partsStart).Truncate(time.Millisecond))
	var found bool
	var leaders int
	for _, pt := range parts {
		if pt.Topic == topic {
			found = true
			if pt.Leader.Host != "" {
				leaders++
			}
		}
	}
	if !found {
		addRow(r, Row{"kafka", topic, L7, FAIL, "Topic not found / not authorized (DescribeTopic).", "Grant Describe on topic or create it."})
		return
	}
	addRow(r, Row{"kafka", topic, L7, OK, fmt.Sprintf("Topic visible; leader partitions=%d", leaders), ""})
}

// ---------- Produce/Consume probes ----------

func probeProduceConsume(ctx context.Context, r *Report, p map[string]string, bootstrap, topic, group string, produceTimeout, consumeTimeout time.Duration, balancer string, kafkaTimeout time.Duration, startOffset int64) {
	if topic == "" {
		addRow(r, Row{"kafka", "(no topic)", L7, SKIP, "Produce/Consume skipped", ""})
		return
	}
	dialer, _, err := DialerFromProps(p, "")
	if err != nil {
		addRow(r, Row{"kafka", topic, L7, FAIL, fmt.Sprintf("dialer: %v", err), "Check tls/sasl settings."})
		return
	}
	if produceTimeout <= 0 {
		produceTimeout = 10 * time.Second
	}
	if consumeTimeout <= 0 {
		consumeTimeout = 10 * time.Second
	}
	leaders := topicLeaders(p, bootstrap, topic, kafkaTimeout)
	baseBalancer := selectBalancer(balancer)
	transport, err := TransportFromProps(p, produceTimeout)
	if err != nil {
		addRow(r, Row{"kafka", topic, L7, FAIL, fmt.Sprintf("transport: %v", err), "Check tls/sasl settings."})
		return
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(strings.Split(bootstrap, ",")...),
		Topic:        topic,
		Balancer:     &loggingBalancer{base: baseBalancer, topic: topic, leaders: leaders},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
		Transport:    transport,
	}
	defer w.Close()

	key := fmt.Sprintf("kshark-%d", time.Now().UnixNano())
	msg := kafka.Message{
		Key:     []byte(key),
		Value:   []byte("probe"),
		Headers: []kafka.Header{{Key: "ksharkcheck", Value: []byte("1")}},
		Time:    time.Now(),
	}

	writeStart := time.Now()
	var writeErr error
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 500 * time.Millisecond
			logf("step kafka.produce.retry topic=%s attempt=%d backoff=%s", topic, attempt, backoff)
			time.Sleep(backoff)
		}
		writeCtx, writeCancel := context.WithTimeout(ctx, produceTimeout)
		writeErr = w.WriteMessages(writeCtx, msg)
		writeCancel()
		if writeErr == nil {
			logf("step kafka.produce topic=%s dur=%s err=nil", topic, time.Since(writeStart).Truncate(time.Millisecond))
			addRow(r, Row{"kafka", topic, L7, OK, "Produce OK", ""})
			break
		}
		if errors.Is(writeErr, kafka.NotLeaderForPartition) || errors.Is(writeErr, kafka.LeaderNotAvailable) {
			logf("step kafka.produce topic=%s dur=%s err=%v (retrying)", topic, time.Since(writeStart).Truncate(time.Millisecond), writeErr)
			continue
		}
		break
	}
	if writeErr != nil {
		logf("step kafka.produce topic=%s dur=%s err=%v", topic, time.Since(writeStart).Truncate(time.Millisecond), writeErr)
		addRow(r, Row{"kafka", topic, L7, FAIL, policyHint("Produce", writeErr), hint(writeErr)})
		return
	}

	if group != "" {
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:     strings.Split(bootstrap, ","),
			Topic:       topic,
			GroupID:     group,
			StartOffset: startOffset,
			MaxWait:     3 * time.Second,
			Dialer:      dialer,
		})
		defer reader.Close()
		readStart := time.Now()
		readCtx, readCancel := context.WithTimeout(ctx, consumeTimeout)
		defer readCancel()
		rec, err := reader.ReadMessage(readCtx)
		if err != nil {
			logf("step kafka.consume topic=%s group=%s dur=%s err=%v", topic, group, time.Since(readStart).Truncate(time.Millisecond), err)
			addRow(r, Row{"kafka", topic, L7, FAIL, policyHint("Consume", err), "Grant Read on topic and Group Read/Describe; check prefixes."})
			return
		}
		logf("step kafka.consume topic=%s group=%s dur=%s err=nil", topic, group, time.Since(readStart).Truncate(time.Millisecond))
		addRow(r, Row{"kafka", topic, L7, OK, fmt.Sprintf("Consume OK (offset %d)", rec.Offset), ""})
		return
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   strings.Split(bootstrap, ","),
		Topic:     topic,
		Partition: 0,
		MaxWait:   3 * time.Second,
		Dialer:    dialer,
	})
	defer reader.Close()
	if err := reader.SetOffset(kafka.FirstOffset); err != nil {
		addRow(r, Row{"kafka", topic, L7, FAIL, fmt.Sprintf("Consume seek failed: %v", err), "Check topic/partition exists."})
		return
	}
	readStart := time.Now()
	readCtx, readCancel := context.WithTimeout(ctx, consumeTimeout)
	defer readCancel()
	rec, err := reader.ReadMessage(readCtx)
	if err != nil {
		logf("step kafka.consume topic=%s dur=%s err=%v", topic, time.Since(readStart).Truncate(time.Millisecond), err)
		addRow(r, Row{"kafka", topic, L7, FAIL, policyHint("Consume", err), "Grant Read on topic and Group Read/Describe; check prefixes."})
		return
	}
	logf("step kafka.consume topic=%s dur=%s err=nil", topic, time.Since(readStart).Truncate(time.Millisecond))
	addRow(r, Row{"kafka", topic, L7, OK, fmt.Sprintf("Consume OK (offset %d)", rec.Offset), ""})
}

// ---------- Kafka helpers ----------

func firstHost(bootstrap string) string {
	first := strings.TrimSpace(strings.Split(bootstrap, ",")[0])
	h, _, err := net.SplitHostPort(first)
	if err != nil {
		return first
	}
	return h
}

func policyHint(op string, err error) string {
	if err == nil {
		return op + " OK"
	}
	if ke, ok := kafkaErrorCode(err); ok {
		switch ke {
		case kafka.TopicAuthorizationFailed:
			return op + " failed: missing topic ACL"
		case kafka.GroupAuthorizationFailed:
			return op + " failed: missing group ACL"
		case kafka.SASLAuthenticationFailed:
			return op + " failed: SASL auth failure"
		case kafka.RequestTimedOut:
			return op + " failed: broker request timeout"
		case kafka.LeaderNotAvailable, kafka.NotLeaderForPartition, kafka.GroupCoordinatorNotAvailable, kafka.NotCoordinatorForGroup:
			return op + " failed: leader/coord not available"
		}
	}
	if isTimeout(err) {
		return op + " failed: timeout (" + err.Error() + ")"
	}
	em := err.Error()
	switch {
	case containsAny(em, "TOPIC_AUTHORIZATION_FAILED", "Topic authorization failed"):
		return op + " failed: missing topic ACL"
	case containsAny(em, "GROUP_AUTHORIZATION_FAILED", "Group authorization failed"):
		return op + " failed: missing group ACL"
	case containsAny(em, "SASL_AUTHENTICATION_FAILED", "SASL"):
		return op + " failed: SASL auth failure"
	case containsAny(em, "LEADER_NOT_AVAILABLE", "NOT_LEADER", "COORDINATOR_NOT_AVAILABLE"):
		return op + " failed: leader/coord not available"
	default:
		return op + " failed: " + em
	}
}

func hint(err error) string {
	if err == nil {
		return ""
	}
	if ke, ok := kafkaErrorCode(err); ok {
		switch ke {
		case kafka.TopicAuthorizationFailed:
			return "Missing topic ACL: Write/Describe for produce; Read/Describe for consume."
		case kafka.GroupAuthorizationFailed:
			return "Missing group ACL: Read/Describe on group."
		case kafka.SASLAuthenticationFailed:
			return "Verify sasl.mechanism, credentials, clocks (JWT), and listener SASL config."
		case kafka.RequestTimedOut:
			return "Broker request timed out; check broker load, network path, and required.acks."
		case kafka.LeaderNotAvailable, kafka.NotLeaderForPartition:
			return "Leader not available; check broker health and metadata propagation."
		case kafka.GroupCoordinatorNotAvailable, kafka.NotCoordinatorForGroup:
			return "Group coordinator not available; check group/offsets topic health."
		}
	}
	if isTimeout(err) {
		return "Client timeout (10s): check network path, firewall, DNS, TLS/SNI, or advertised.listeners."
	}
	em := err.Error()
	switch {
	case containsAny(em, "authorization", "AUTHORIZATION", "AUTH"):
		return "Check ACLs: Write/Read/Describe on topic; Read/Describe on group."
	case containsAny(em, "SASL", "authentication"):
		return "Verify sasl.mechanism, credentials, clocks (JWT), and listener SASL config."
	case containsAny(em, "EOF", "tls", "handshake", "certificate"):
		return "TLS mismatch or mTLS requirements; verify CA and SNI/hostnames."
	default:
		return ""
	}
}

func containsAny(s string, subs ...string) bool {
	ls := strings.ToLower(s)
	for _, sub := range subs {
		if strings.Contains(ls, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	em := strings.ToLower(err.Error())
	return strings.Contains(em, "deadline exceeded") || strings.Contains(em, "i/o timeout")
}

func kafkaErrorCode(err error) (kafka.Error, bool) {
	var ke kafka.Error
	if errors.As(err, &ke) {
		return ke, true
	}
	return 0, false
}

type loggingBalancer struct {
	base    kafka.Balancer
	topic   string
	leaders map[int]kafka.Broker
}

func (b *loggingBalancer) Balance(msg kafka.Message, partitions ...int) int {
	if b.base == nil {
		b.base = &kafka.LeastBytes{}
	}
	partition := b.base.Balance(msg, partitions...)
	if len(b.leaders) > 0 {
		if leader, ok := b.leaders[partition]; ok {
			logf("step kafka.balance topic=%s partition=%d leader=%s:%d broker_id=%d", b.topic, partition, leader.Host, leader.Port, leader.ID)
			return partition
		}
	}
	logf("step kafka.balance topic=%s partition=%d leader=unknown", b.topic, partition)
	return partition
}

func topicLeaders(p map[string]string, bootstrap, topic string, timeout time.Duration) map[int]kafka.Broker {
	first := strings.TrimSpace(strings.Split(bootstrap, ",")[0])
	hostForSNI := firstHost(first)
	dialer, _, err := DialerFromProps(p, hostForSNI)
	if err != nil {
		logf("step kafka.leaders dialer err=%v", err)
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", first)
	if err != nil {
		logf("step kafka.leaders dial addr=%s err=%v", first, err)
		return nil
	}
	defer conn.Close()
	parts, err := conn.ReadPartitions()
	if err != nil {
		logf("step kafka.leaders readpartitions err=%v", err)
		return nil
	}
	leaders := make(map[int]kafka.Broker)
	for _, pt := range parts {
		if pt.Topic == topic {
			leaders[pt.ID] = pt.Leader
		}
	}
	return leaders
}

func selectBalancer(name string) kafka.Balancer {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rr", "roundrobin":
		return &kafka.RoundRobin{}
	case "random":
		return kafka.BalancerFunc(func(_ kafka.Message, partitions ...int) int {
			if len(partitions) == 0 {
				return 0
			}
			return partitions[rand.Intn(len(partitions))]
		})
	default:
		return &kafka.LeastBytes{}
	}
}

func parseStartOffset(s string) (int64, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "earliest", "first", "":
		return kafka.FirstOffset, nil
	case "latest", "last":
		return kafka.LastOffset, nil
	default:
		return kafka.FirstOffset, fmt.Errorf("invalid start-offset: %s", s)
	}
}

// ParseTopics splits a comma-separated topic string into a slice.
func ParseTopics(raw string) []string {
	if raw == "" {
		return nil
	}
	var topics []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			topics = append(topics, t)
		}
	}
	return topics
}

// ---------- Schema Registry / REST ----------

func httpClientFromTLS(tlsConf *tls.Config, timeout time.Duration) *http.Client {
	tr := &http.Transport{TLSClientConfig: tlsConf, Proxy: http.ProxyFromEnvironment, IdleConnTimeout: 10 * time.Second}
	return &http.Client{Transport: tr, Timeout: timeout}
}

func checkSchemaRegistry(ctx context.Context, r *Report, p map[string]string) {
	url := strings.TrimSpace(p["schema.registry.url"])
	if url == "" {
		return
	}
	host := extractHost(url)
	if _, err := net.LookupHost(host); err != nil {
		addRow(r, Row{"schema-reg", host, L3, FAIL, "DNS failed", "Fix DNS/VPN."})
	} else {
		addRow(r, Row{"schema-reg", host, L3, OK, "Resolved host", ""})
	}
	tlsConf, _, err := TLSConfigFromProps(p, host)
	if err != nil {
		addRow(r, Row{"schema-reg", url, HTTP, FAIL, fmt.Sprintf("TLS config err: %v", err), ""})
		return
	}
	client := httpClientFromTLS(tlsConf, 8*time.Second)
	req, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(url, "/")+"/subjects", nil)
	if info := p["basic.auth.user.info"]; info != "" {
		up := strings.SplitN(info, ":", 2)
		if len(up) == 2 {
			req.SetBasicAuth(up[0], up[1])
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		addRow(r, Row{"schema-reg", url, HTTP, FAIL, fmt.Sprintf("GET /subjects failed: %v", err), "TLS/host/network or auth."})
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		addRow(r, Row{"schema-reg", url, HTTP, OK, "GET /subjects OK", ""})
	case 401, 403:
		addRow(r, Row{"schema-reg", url, HTTP, FAIL, fmt.Sprintf("Auth %d", resp.StatusCode), "Check basic.auth.user.info or mTLS mapping."})
	default:
		addRow(r, Row{"schema-reg", url, HTTP, WARN, fmt.Sprintf("HTTP %d", resp.StatusCode), ""})
	}
}

func checkRESTProxy(r *Report, p map[string]string, rest string) {
	logf("rest proxy check start url=%s", rest)
	checkDNS(r, extractHost(rest), "rest-proxy")
	tlsConf, _, err := TLSConfigFromProps(p, extractHost(rest))
	if err != nil {
		addRow(r, Row{"rest-proxy", rest, HTTP, FAIL, fmt.Sprintf("TLS config err: %v", err), ""})
		return
	}
	client := httpClientFromTLS(tlsConf, 8*time.Second)
	req, _ := http.NewRequest("GET", strings.TrimRight(rest, "/")+"/topics", nil)
	resp, err := client.Do(req)
	if err != nil {
		addRow(r, Row{"rest-proxy", rest, HTTP, FAIL, fmt.Sprintf("GET /topics failed: %v", err), "Check listener/auth."})
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		addRow(r, Row{"rest-proxy", rest, HTTP, OK, "GET /topics OK", ""})
	case 401, 403:
		addRow(r, Row{"rest-proxy", rest, HTTP, FAIL, fmt.Sprintf("Auth %d", resp.StatusCode), "Check credentials or mTLS mapping."})
	default:
		addRow(r, Row{"rest-proxy", rest, HTTP, WARN, fmt.Sprintf("HTTP %d", resp.StatusCode), ""})
	}
}

func extractHost(raw string) string {
	trim := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	if idx := strings.IndexByte(trim, '/'); idx > 0 {
		trim = trim[:idx]
	}
	if h, _, err := net.SplitHostPort(trim); err == nil {
		return h
	}
	return trim
}

// ---------- Diagnostics (traceroute / MTU) ----------

func runCmdIfExists(name string, args ...string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(path, args...)
	cmd.Env = os.Environ()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Run()
	return buf.String(), err
}

func isValidHostname(host string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9\.\-]+$`)
	return re.MatchString(host)
}

func bestEffortTraceroute(r *Report, host string) {
	if !isValidHostname(host) {
		addRow(r, Row{"diag", host, DIAG, FAIL, "Invalid hostname provided", "Skipping traceroute to prevent command injection."})
		return
	}
	if out, err := runCmdIfExists("traceroute", "-n", "-w", "2", "-q", "1", host); err == nil {
		addRow(r, Row{"diag", host, DIAG, OK, "traceroute OK (see JSON)", ""})
		addRow(r, Row{"diag", host, DIAG, SKIP, trimLines(out, 15), "Full output in JSON if -json used."})
		return
	}
	if out, err := runCmdIfExists("tracepath", host); err == nil {
		addRow(r, Row{"diag", host, DIAG, OK, "tracepath OK (see JSON)", ""})
		addRow(r, Row{"diag", host, DIAG, SKIP, trimLines(out, 15), "Full output in JSON if -json used."})
		return
	}
	if out, err := runCmdIfExists("tracert", "-d", "-w", "2000", host); err == nil {
		addRow(r, Row{"diag", host, DIAG, OK, "tracert OK (see JSON)", ""})
		addRow(r, Row{"diag", host, DIAG, SKIP, trimLines(out, 15), "Full output in JSON if -json used."})
		return
	}
	addRow(r, Row{"diag", host, DIAG, SKIP, "No traceroute tool found", "Install traceroute/tracepath (Linux), or use tracert (Windows)."})
}

func mtuCheck(r *Report, host string) {
	if !isValidHostname(host) {
		addRow(r, Row{"diag", host, DIAG, FAIL, "Invalid hostname provided", "Skipping MTU check to prevent command injection."})
		return
	}
	if out, err := runCmdIfExists("tracepath", host); err == nil && strings.Contains(out, "pmtu") {
		addRow(r, Row{"diag", host, DIAG, OK, "pMTU detected via tracepath", ""})
		return
	}
	for _, sz := range []int{1472, 1464, 1452, 1400, 1200, 1000} {
		var out string
		var err error
		if runtime.GOOS == "darwin" {
			out, err = runCmdIfExists("ping", "-D", "-s", strconv.Itoa(sz), "-c", "1", host)
		} else {
			out, err = runCmdIfExists("ping", "-M", "do", "-s", strconv.Itoa(sz), "-c", "1", host)
		}
		if err == nil && strings.Contains(strings.ToLower(out), "1 packets transmitted") {
			addRow(r, Row{"diag", host, DIAG, OK, fmt.Sprintf("MTU ok at payload %d", sz), ""})
			return
		}
	}
	addRow(r, Row{"diag", host, DIAG, SKIP, "MTU probe inconclusive", "Run tracepath or adjust network MTU if you see fragmentation."})
}

func trimLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + "\n..."
}
