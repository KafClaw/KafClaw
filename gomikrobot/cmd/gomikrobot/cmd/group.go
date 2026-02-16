package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kamir/gomikrobot/internal/agent"
	"github.com/kamir/gomikrobot/internal/bus"
	"github.com/kamir/gomikrobot/internal/config"
	"github.com/kamir/gomikrobot/internal/group"
	"github.com/kamir/gomikrobot/internal/timeline"
	"github.com/kamir/gomikrobot/internal/tools"
	"github.com/spf13/cobra"
)

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "Manage group collaboration",
	Long:  "Join, leave, and inspect multi-agent collaboration groups via Kafka.",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var groupJoinCmd = &cobra.Command{
	Use:   "join <group-name>",
	Short: "Join a collaboration group",
	Args:  cobra.ExactArgs(1),
	Run:   runGroupJoin,
}

var groupLeaveCmd = &cobra.Command{
	Use:   "leave",
	Short: "Leave the current collaboration group",
	Run:   runGroupLeave,
}

var groupStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show group status",
	Run:   runGroupStatus,
}

var groupMembersCmd = &cobra.Command{
	Use:   "members",
	Short: "List current group members",
	Run:   runGroupMembers,
}

func init() {
	groupCmd.AddCommand(groupJoinCmd)
	groupCmd.AddCommand(groupLeaveCmd)
	groupCmd.AddCommand(groupStatusCmd)
	groupCmd.AddCommand(groupMembersCmd)
}

func loadGroupTimeline() (*timeline.TimelineService, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	timelinePath := filepath.Join(home, ".gomikrobot", "timeline.db")
	return timeline.NewTimelineService(timelinePath)
}

func buildGroupManager(cfg *config.Config, timeSvc *timeline.TimelineService) *group.Manager {
	// Build identity from context builder
	registry := tools.NewRegistry()
	ctxBuilder := agent.NewContextBuilder(cfg.Paths.Workspace, cfg.Paths.WorkRepoPath, cfg.Paths.SystemRepoPath, registry)

	agentID := cfg.Group.AgentID
	if agentID == "" {
		hostname, _ := os.Hostname()
		agentID = fmt.Sprintf("gomikrobot-%s", hostname)
	}

	identity := ctxBuilder.BuildIdentityEnvelope(agentID, "GoMikroBot", cfg.Model.Name)

	return group.NewManager(cfg.Group, timeSvc, identity)
}

func runGroupJoin(cmd *cobra.Command, args []string) {
	printHeader("🤝 GoMikroBot Group Join")

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		os.Exit(1)
	}

	groupName := strings.TrimSpace(args[0])
	if groupName == "" {
		fmt.Println("Error: group name required")
		os.Exit(1)
	}

	cfg.Group.GroupName = groupName
	cfg.Group.Enabled = true

	// Load LFS proxy URL from settings if not in config
	timeSvc, err := loadGroupTimeline()
	if err != nil {
		fmt.Printf("Timeline error: %v\n", err)
		os.Exit(1)
	}
	defer timeSvc.Close()

	if cfg.Group.LFSProxyURL == "" || cfg.Group.LFSProxyURL == "http://localhost:8080" {
		if url, err := timeSvc.GetSetting("kafscale_lfs_proxy_url"); err == nil && url != "" {
			cfg.Group.LFSProxyURL = url
		}
	}

	mgr := buildGroupManager(cfg, timeSvc)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := mgr.Join(ctx); err != nil {
		fmt.Printf("Failed to join group: %v\n", err)
		os.Exit(1)
	}

	// Persist group name for other commands
	_ = timeSvc.SetSetting("group_name", groupName)
	_ = timeSvc.SetSetting("group_active", "true")

	fmt.Printf("Joined group: %s\n", groupName)
	fmt.Printf("Agent ID: %s\n", mgr.Status()["agent_id"])
	fmt.Printf("LFS Proxy: %s (healthy: %v)\n", cfg.Group.LFSProxyURL, mgr.LFSHealthy())
}

func runGroupLeave(cmd *cobra.Command, args []string) {
	printHeader("👋 GoMikroBot Group Leave")

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		os.Exit(1)
	}

	timeSvc, err := loadGroupTimeline()
	if err != nil {
		fmt.Printf("Timeline error: %v\n", err)
		os.Exit(1)
	}
	defer timeSvc.Close()

	// Get group name from settings
	groupName, _ := timeSvc.GetSetting("group_name")
	if groupName == "" {
		fmt.Println("Not in a group. Use 'gomikrobot group join <name>' first.")
		return
	}

	cfg.Group.GroupName = groupName
	cfg.Group.Enabled = true

	if cfg.Group.LFSProxyURL == "" || cfg.Group.LFSProxyURL == "http://localhost:8080" {
		if url, err := timeSvc.GetSetting("kafscale_lfs_proxy_url"); err == nil && url != "" {
			cfg.Group.LFSProxyURL = url
		}
	}

	mgr := buildGroupManager(cfg, timeSvc)
	// Force active state so leave works
	mgr.Join(context.Background()) //nolint: must join to leave cleanly

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := mgr.Leave(ctx); err != nil {
		fmt.Printf("Failed to leave group: %v\n", err)
		os.Exit(1)
	}

	_ = timeSvc.SetSetting("group_active", "false")
	fmt.Printf("Left group: %s\n", groupName)
}

func runGroupStatus(cmd *cobra.Command, args []string) {
	printHeader("📊 GoMikroBot Group Status")

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		os.Exit(1)
	}

	timeSvc, err := loadGroupTimeline()
	if err != nil {
		fmt.Printf("Timeline error: %v\n", err)
		os.Exit(1)
	}
	defer timeSvc.Close()

	groupName, _ := timeSvc.GetSetting("group_name")
	groupActive, _ := timeSvc.GetSetting("group_active")

	if groupName == "" {
		fmt.Println("Not in a group. Use 'gomikrobot group join <name>' first.")
		return
	}

	cfg.Group.GroupName = groupName
	if cfg.Group.LFSProxyURL == "" || cfg.Group.LFSProxyURL == "http://localhost:8080" {
		if url, err := timeSvc.GetSetting("kafscale_lfs_proxy_url"); err == nil && url != "" {
			cfg.Group.LFSProxyURL = url
		}
	}

	mgr := buildGroupManager(cfg, timeSvc)

	members, _ := timeSvc.ListGroupMembers()
	memberCount := len(members)

	fmt.Printf("Group:       %s\n", groupName)
	fmt.Printf("Active:      %s\n", groupActive)
	fmt.Printf("Members:     %d\n", memberCount)
	fmt.Printf("LFS Proxy:   %s\n", cfg.Group.LFSProxyURL)
	fmt.Printf("LFS Healthy: %v\n", mgr.LFSHealthy())

	// Show topics
	topics := group.Topics(groupName)
	fmt.Printf("\nTopics:\n")
	fmt.Printf("  Announce:  %s\n", topics.Announce)
	fmt.Printf("  Requests:  %s\n", topics.Requests)
	fmt.Printf("  Responses: %s\n", topics.Responses)
	fmt.Printf("  Traces:    %s\n", topics.Traces)
}

func runGroupMembers(cmd *cobra.Command, args []string) {
	printHeader("👥 GoMikroBot Group Members")

	timeSvc, err := loadGroupTimeline()
	if err != nil {
		fmt.Printf("Timeline error: %v\n", err)
		os.Exit(1)
	}
	defer timeSvc.Close()

	groupName, _ := timeSvc.GetSetting("group_name")
	if groupName == "" {
		fmt.Println("Not in a group. Use 'gomikrobot group join <name>' first.")
		return
	}

	members, err := timeSvc.ListGroupMembers()
	if err != nil {
		fmt.Printf("Error listing members: %v\n", err)
		os.Exit(1)
	}

	if len(members) == 0 {
		fmt.Println("No members in group.")
		return
	}

	fmt.Printf("Group: %s (%d members)\n\n", groupName, len(members))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT ID\tNAME\tMODEL\tSTATUS\tCAPABILITIES\tLAST SEEN")
	fmt.Fprintln(w, "--------\t----\t-----\t------\t------------\t---------")
	for _, m := range members {
		var caps []string
		json.Unmarshal([]byte(m.Capabilities), &caps)
		capStr := strings.Join(caps, ", ")
		if len(capStr) > 40 {
			capStr = capStr[:37] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			m.AgentID, m.AgentName, m.Model, m.Status,
			capStr, m.LastSeen.Format("15:04:05"))
	}
	w.Flush()
}

// Used by gateway when passing msgBus around
func setupGroupBusSubscription(mgr *group.Manager, msgBus *bus.MessageBus) {
	msgBus.Subscribe("group", func(msg *bus.OutboundMessage) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := mgr.RespondTask(ctx, msg.TaskID, msg.Content, "completed"); err != nil {
				fmt.Printf("Group outbound error: %v\n", err)
			}
		}()
	})
}
