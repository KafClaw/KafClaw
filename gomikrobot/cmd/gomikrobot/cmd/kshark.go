package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamir/gomikrobot/internal/config"
	"github.com/kamir/gomikrobot/internal/kshark"
	"github.com/spf13/cobra"
)

var ksharkCmd = &cobra.Command{
	Use:   "kshark",
	Short: "Kafka connectivity diagnostics (bundled kshark)",
	Long: `kshark systematically tests every network layer between this machine and
a Kafka cluster: DNS, TCP, TLS, SASL auth, Kafka protocol (ApiVersions,
Metadata), produce/consume probes, Schema Registry, and REST Proxy.

Use --auto to read Kafka settings from GoMikroBot's group config instead
of providing a client.properties file.`,
	RunE: runKshark,
}

var (
	ksharkProps          string
	ksharkTopic          string
	ksharkGroup          string
	ksharkPreset         string
	ksharkTimeout        time.Duration
	ksharkKafkaTimeout   time.Duration
	ksharkProduceTimeout time.Duration
	ksharkConsumeTimeout time.Duration
	ksharkBalancer       string
	ksharkStartOffset    string
	ksharkDiag           bool
	ksharkJSONOut        string
	ksharkAuto           bool
	ksharkYes            bool
)

func init() {
	f := ksharkCmd.Flags()
	f.StringVarP(&ksharkProps, "props", "p", "", "Path to client.properties file")
	f.StringVarP(&ksharkTopic, "topic", "t", "", "Topics to test (comma-separated)")
	f.StringVarP(&ksharkGroup, "group", "g", "", "Consumer group for probe")
	f.StringVar(&ksharkPreset, "preset", "", "Config preset (cc-plain, self-scram, plaintext)")
	f.DurationVar(&ksharkTimeout, "timeout", 60*time.Second, "Global scan timeout")
	f.DurationVar(&ksharkKafkaTimeout, "kafka-timeout", 10*time.Second, "Metadata/dial timeout")
	f.DurationVar(&ksharkProduceTimeout, "produce-timeout", 10*time.Second, "Produce operation timeout")
	f.DurationVar(&ksharkConsumeTimeout, "consume-timeout", 10*time.Second, "Consume operation timeout")
	f.StringVar(&ksharkBalancer, "balancer", "least", "Partition balancer: least|rr|random")
	f.StringVar(&ksharkStartOffset, "start-offset", "earliest", "Probe read start offset: earliest|latest")
	f.BoolVar(&ksharkDiag, "diag", true, "Enable traceroute/MTU diagnostics")
	f.StringVar(&ksharkJSONOut, "json", "", "Export results to JSON file")
	f.BoolVar(&ksharkAuto, "auto", false, "Use group config from GoMikroBot settings (no props file needed)")
	f.BoolVarP(&ksharkYes, "yes", "y", false, "Skip confirmation prompt")
}

func runKshark(cmd *cobra.Command, args []string) error {
	fmt.Print(`
 __      _________.__                  __
|  | __ /   _____/|  |__ _____ _______|  | __
|  |/ / \_____  \ |  |  \\__  \_  __ \  |/ /
|    <  /        \|   Y  \/ __ \|  | \/    <
|__|_ \/_______  /|___|  (____  /__|  |__|_ \
     \/        \/      \/     \/           \/
  bundled in GoMikroBot
`)

	var props map[string]string

	switch {
	case ksharkAuto:
		// Build properties from GoMikroBot config
		p, err := propsFromConfig()
		if err != nil {
			return fmt.Errorf("--auto: %w", err)
		}
		props = p
	case ksharkProps != "":
		p, err := kshark.LoadProperties(ksharkProps)
		if err != nil {
			return fmt.Errorf("failed to load properties: %w", err)
		}
		props = p
	default:
		return fmt.Errorf("provide either --props <file> or --auto")
	}

	if ksharkPreset != "" {
		kshark.ApplyPreset(ksharkPreset, props)
	}

	topics := kshark.ParseTopics(ksharkTopic)

	// Print scan plan
	fmt.Println("\n--- Scan Plan ---")
	fmt.Printf("Target Kafka Cluster: %s\n", props["bootstrap.servers"])
	if len(topics) > 0 {
		fmt.Printf("Target Topics: %s\n", strings.Join(topics, ", "))
	} else {
		fmt.Println("Target Topics: (none, metadata checks only)")
	}
	fmt.Println("\nChecks to be performed:")
	fmt.Println("  - Connectivity Checks (DNS, TCP, TLS) for each broker.")
	fmt.Println("  - Kafka Protocol Checks (ApiVersions, Topic Metadata).")
	if len(topics) > 0 {
		fmt.Println("  - Produce & Consume Probe.")
	}
	if props["schema.registry.url"] != "" {
		fmt.Printf("  - Schema Registry Check: %s\n", props["schema.registry.url"])
	}
	if ksharkDiag {
		fmt.Println("  - Network Diagnostics (Traceroute, MTU).")
	}
	if ksharkJSONOut != "" {
		fmt.Printf("JSON Report: %s\n", ksharkJSONOut)
	}
	fmt.Println("-------------------")

	// Confirmation
	if !ksharkYes {
		fmt.Print("\nContinue with the scan? (y/n): ")
		var input string
		fmt.Scanln(&input)
		input = strings.ToLower(strings.TrimSpace(input))
		if input != "y" && input != "yes" {
			fmt.Println("Scan aborted.")
			return nil
		}
	}

	opts := kshark.Options{
		Props:          props,
		Topics:         topics,
		Group:          ksharkGroup,
		JSONOut:        ksharkJSONOut,
		Timeout:        ksharkTimeout,
		KafkaTimeout:   ksharkKafkaTimeout,
		ProduceTimeout: ksharkProduceTimeout,
		ConsumeTimeout: ksharkConsumeTimeout,
		Balancer:       ksharkBalancer,
		StartOffset:    ksharkStartOffset,
		Diag:           ksharkDiag,
	}

	report, err := kshark.Run(opts)
	if err != nil {
		return err
	}

	kshark.PrintPretty(report)

	if ksharkJSONOut != "" {
		actualPath, err := kshark.WriteJSON(ksharkJSONOut, report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON report: %v\n", err)
		} else {
			absPath, _ := filepath.Abs(actualPath)
			fmt.Printf("JSON report written to %s\n", absPath)
			if md5Path, _, err := kshark.WriteReportMD5(actualPath); err == nil {
				fmt.Printf("JSON report MD5 saved to %s\n", md5Path)
			}
		}
	}

	if report.HasFailed {
		return fmt.Errorf("one or more checks failed")
	}
	return nil
}

// propsFromConfig builds a Kafka client properties map from GoMikroBot's config.
func propsFromConfig() (map[string]string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if !cfg.Group.Enabled {
		return nil, fmt.Errorf("group collaboration is not enabled in config; set group.enabled=true or use --props instead")
	}

	brokers := cfg.Group.KafkaBrokers
	if brokers == "" && cfg.Group.LFSProxyURL != "" {
		// When using KafScale LFS proxy, the proxy URL serves as the bootstrap endpoint.
		// Strip protocol prefix for Kafka bootstrap.servers format.
		proxy := cfg.Group.LFSProxyURL
		proxy = strings.TrimPrefix(proxy, "https://")
		proxy = strings.TrimPrefix(proxy, "http://")
		if !strings.Contains(proxy, ":") {
			proxy += ":9092"
		}
		brokers = proxy
	}
	if brokers == "" {
		return nil, fmt.Errorf("no Kafka brokers configured (set group.kafkaBrokers or group.lfsProxyUrl)")
	}

	props := map[string]string{
		"bootstrap.servers": brokers,
	}

	// If LFS proxy API key is set, assume SASL/PLAIN over SSL (KafScale convention).
	if cfg.Group.LFSProxyAPIKey != "" {
		props["security.protocol"] = "SASL_SSL"
		props["sasl.mechanism"] = "PLAIN"
		props["sasl.username"] = "token"
		props["sasl.password"] = cfg.Group.LFSProxyAPIKey
	}

	fmt.Printf("Auto-configured from GoMikroBot group settings:\n")
	fmt.Printf("  Brokers:        %s\n", brokers)
	if cfg.Group.GroupName != "" {
		fmt.Printf("  Group Name:     %s\n", cfg.Group.GroupName)
	}
	if cfg.Group.ConsumerGroup != "" {
		fmt.Printf("  Consumer Group: %s\n", cfg.Group.ConsumerGroup)
	}

	return props, nil
}
