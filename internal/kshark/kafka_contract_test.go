package kshark

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/kafka-go/protocol"
	"github.com/segmentio/kafka-go/protocol/apiversions"
	"github.com/segmentio/kafka-go/protocol/metadata"
	"github.com/segmentio/kafka-go/protocol/produce"
)

// This fixture speaks the Kafka wire protocol for metadata and acknowledged
// writes. Reads deliberately fail, exercising the diagnostic timeout report.
func diagnosticBroker(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, portRaw, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portRaw)
	var wg sync.WaitGroup
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				for {
					version, id, _, request, err := protocol.ReadRequest(conn)
					if err != nil {
						return
					}
					var response protocol.Message
					switch req := request.(type) {
					case *apiversions.Request:
						response = &apiversions.Response{ApiKeys: []apiversions.ApiKeyResponse{{ApiKey: 0, MinVersion: 3, MaxVersion: 3}, {ApiKey: 3, MinVersion: 0, MaxVersion: 1}, {ApiKey: 18, MinVersion: 0, MaxVersion: 2}}}
					case *metadata.Request:
						response = &metadata.Response{Brokers: []metadata.ResponseBroker{{NodeID: 1, Host: host, Port: int32(port)}}, Topics: []metadata.ResponseTopic{{Name: "fixture", Partitions: []metadata.ResponsePartition{{PartitionIndex: 0, LeaderID: 1, ReplicaNodes: []int32{1}, IsrNodes: []int32{1}}}}}}
					case *produce.Request:
						out := &produce.Response{}
						for _, topic := range req.Topics {
							rt := produce.ResponseTopic{Topic: topic.Topic}
							for _, partition := range topic.Partitions {
								rt.Partitions = append(rt.Partitions, produce.ResponsePartition{Partition: partition.Partition, BaseOffset: 1})
							}
							out.Topics = append(out.Topics, rt)
						}
						response = out
					default:
						return
					}
					if err := protocol.WriteResponse(conn, version, id, response); err != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { ln.Close(); <-acceptDone; wg.Wait() })
	return ln.Addr().String()
}

func TestKafkaMetadataAndProduceDiagnostics(t *testing.T) {
	addr := diagnosticBroker(t)
	props := map[string]string{"bootstrap.servers": addr, "security.protocol": "PLAINTEXT"}
	report := &Report{}
	conn, err := kafkaConn(report, props, addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	checkTopic(report, props, addr, "fixture", time.Second)
	checkTopic(report, props, addr, "missing", time.Second)
	leaders := topicLeaders(props, addr, "fixture", time.Second)
	if len(leaders) != 1 || leaders[0].Host != "127.0.0.1" {
		t.Fatalf("leaders: %+v", leaders)
	}
	for _, group := range []string{"", "fixture-group"} {
		probe := &Report{}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		probeProduceConsume(ctx, probe, props, addr, "fixture", group, 3*time.Second, 20*time.Millisecond, "rr", time.Second, -2)
		cancel()
		produced, consumeFailed := false, false
		for _, row := range probe.Rows {
			if row.Detail == "Produce OK" {
				produced = true
			}
			if strings.Contains(row.Detail, "Consume") && row.Status == FAIL {
				consumeFailed = true
			}
		}
		if !produced || !consumeFailed {
			t.Fatalf("produce/read diagnostics: %+v", probe.Rows)
		}
	}
	t.Chdir(t.TempDir())
	old := scanLog
	t.Cleanup(func() { scanLog = old })
	got, err := Run(Options{Props: props, Timeout: time.Second, Diag: false})
	if err != nil || got.HasFailed {
		t.Fatalf("plaintext scan: %+v %v", got, err)
	}
	got, err = Run(Options{Props: map[string]string{"bootstrap.servers": addr, "security.protocol": "SSL", "ssl.ca.location": "/missing/ca"}, Timeout: time.Second})
	if err != nil || !got.HasFailed {
		t.Fatal("TLS configuration error not reported")
	}
}
