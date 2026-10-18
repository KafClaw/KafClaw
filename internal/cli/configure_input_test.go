package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/spf13/cobra"
)

func TestConfigureConsumesConsecutivePromptAnswers(t *testing.T) {
	oldInteractive, oldAgents, oldClear := configureNonInteractive, configureSubagentsAllowAgents, configureClearSubagentsAllowAgents
	defer func() {
		configureNonInteractive, configureSubagentsAllowAgents, configureClearSubagentsAllowAgents = oldInteractive, oldAgents, oldClear
	}()
	configureNonInteractive, configureSubagentsAllowAgents, configureClearSubagentsAllowAgents = false, "", false
	for _, answer := range []string{"enable", "disable", "keep"} {
		t.Run(answer, func(t *testing.T) {
			commandHome(t)
			cmd := &cobra.Command{}
			cmd.SetIn(strings.NewReader("worker-a,worker-b\n" + answer + "\n"))
			cmd.SetOut(io.Discard)
			if err := runConfigure(cmd, nil); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(cfg.Tools.Subagents.AllowAgents, ",") != "worker-a,worker-b" {
				t.Fatal("allowlist answer lost")
			}
			enabled, exists := getAgentCascadeEnabled(cfg, resolveConfigureTargetAgentID(cfg, ""))
			if answer != "keep" && (!exists || enabled != (answer == "enable")) {
				t.Fatalf("cascade answer lost: enabled=%v exists=%v", enabled, exists)
			}
		})
	}
	for _, input := range []string{"", "\n"} {
		commandHome(t)
		cmd := &cobra.Command{}
		cmd.SetIn(strings.NewReader(input))
		cmd.SetOut(io.Discard)
		if err := runConfigure(cmd, nil); err == nil || !strings.Contains(err.Error(), "read input") {
			t.Fatalf("input exhaustion: %v", err)
		}
	}
}
