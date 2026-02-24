package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/KafClaw/KafClaw/internal/config"
	"github.com/KafClaw/KafClaw/internal/group"
	"github.com/KafClaw/KafClaw/internal/knowledge"
	"github.com/KafClaw/KafClaw/internal/timeline"
	"github.com/spf13/cobra"
)

var (
	knowledgeJSON bool

	knowledgeGroup      string
	knowledgeProposalID string
	knowledgeTitle      string
	knowledgeStatement  string
	knowledgeTags       string
	knowledgePublish    bool

	knowledgeVote       string
	knowledgeReason     string
	knowledgeAsClaw     string
	knowledgeAsInstance string
	knowledgePoolSize   int

	knowledgeStatusFilter string
	knowledgeLimit        int
)

var knowledgeCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Manage shared knowledge proposals, votes, decisions, and facts",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var knowledgeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show knowledge governance status",
	RunE:  runKnowledgeStatus,
}

var knowledgeProposeCmd = &cobra.Command{
	Use:   "propose",
	Short: "Create a knowledge proposal",
	RunE:  runKnowledgePropose,
}

var knowledgeVoteCmd = &cobra.Command{
	Use:   "vote",
	Short: "Cast/update a vote for a proposal and evaluate quorum",
	RunE:  runKnowledgeVote,
}

var knowledgeDecisionsCmd = &cobra.Command{
	Use:   "decisions",
	Short: "List proposal decisions",
	RunE:  runKnowledgeDecisions,
}

var knowledgeFactsCmd = &cobra.Command{
	Use:   "facts",
	Short: "List accepted shared facts",
	RunE:  runKnowledgeFacts,
}

func init() {
	knowledgeCmd.PersistentFlags().BoolVar(&knowledgeJSON, "json", false, "Output machine-readable JSON")

	knowledgeProposeCmd.Flags().StringVar(&knowledgeGroup, "group", "", "Knowledge group name (defaults to config knowledge.group)")
	knowledgeProposeCmd.Flags().StringVar(&knowledgeProposalID, "proposal-id", "", "Proposal ID (auto-generated if empty)")
	knowledgeProposeCmd.Flags().StringVar(&knowledgeTitle, "title", "", "Proposal title")
	knowledgeProposeCmd.Flags().StringVar(&knowledgeStatement, "statement", "", "Proposal statement text")
	knowledgeProposeCmd.Flags().StringVar(&knowledgeTags, "tags", "", "Comma-separated tags")
	knowledgeProposeCmd.Flags().BoolVar(&knowledgePublish, "publish", false, "Publish proposal envelope to Kafka knowledge topic")

	knowledgeVoteCmd.Flags().StringVar(&knowledgeProposalID, "proposal-id", "", "Proposal ID")
	knowledgeVoteCmd.Flags().StringVar(&knowledgeVote, "vote", "", "Vote value (yes|no)")
	knowledgeVoteCmd.Flags().StringVar(&knowledgeReason, "reason", "", "Vote reason")
	knowledgeVoteCmd.Flags().StringVar(&knowledgeAsClaw, "as-claw", "", "Override clawId for vote record/testing")
	knowledgeVoteCmd.Flags().StringVar(&knowledgeAsInstance, "as-instance", "", "Override instanceId for vote record/testing")
	knowledgeVoteCmd.Flags().IntVar(&knowledgePoolSize, "pool-size", 0, "Override pool size for quorum evaluation (0=auto)")
	knowledgeVoteCmd.Flags().BoolVar(&knowledgePublish, "publish", false, "Publish vote/decision envelope(s) to Kafka knowledge topics")

	knowledgeDecisionsCmd.Flags().StringVar(&knowledgeStatusFilter, "status", "", "Decision status filter (approved|rejected|expired)")
	knowledgeDecisionsCmd.Flags().IntVar(&knowledgeLimit, "limit", 50, "Maximum rows to return")

	knowledgeFactsCmd.Flags().StringVar(&knowledgeGroup, "group", "", "Group filter")
	knowledgeFactsCmd.Flags().IntVar(&knowledgeLimit, "limit", 50, "Maximum rows to return")

	knowledgeCmd.AddCommand(knowledgeStatusCmd, knowledgeProposeCmd, knowledgeVoteCmd, knowledgeDecisionsCmd, knowledgeFactsCmd)
	rootCmd.AddCommand(knowledgeCmd)
}

func runKnowledgeStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	timeSvc, err := loadGroupTimeline()
	if err != nil {
		return err
	}
	defer timeSvc.Close()

	pending, _ := timeSvc.ListKnowledgeProposals("pending", 1000, 0)
	approved, _ := timeSvc.ListKnowledgeProposals("approved", 1000, 0)
	rejected, _ := timeSvc.ListKnowledgeProposals("rejected", 1000, 0)
	expired, _ := timeSvc.ListKnowledgeProposals("expired", 1000, 0)
	factsCount, _ := timeSvc.CountKnowledgeFacts(strings.TrimSpace(cfg.Knowledge.Group))

	out := map[string]any{
		"enabled":    cfg.Knowledge.Enabled,
		"group":      cfg.Knowledge.Group,
		"clawId":     cfg.Node.ClawID,
		"instanceId": cfg.Node.InstanceID,
		"topics":     cfg.Knowledge.Topics,
		"counts": map[string]int{
			"pending":  len(pending),
			"approved": len(approved),
			"rejected": len(rejected),
			"expired":  len(expired),
			"facts":    factsCount,
		},
	}
	return printKnowledgeOutput(cmd.OutOrStdout(), out)
}

func runKnowledgePropose(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	timeSvc, err := loadGroupTimeline()
	if err != nil {
		return err
	}
	defer timeSvc.Close()

	groupName := strings.TrimSpace(knowledgeGroup)
	if groupName == "" {
		groupName = strings.TrimSpace(cfg.Knowledge.Group)
	}
	if groupName == "" {
		return fmt.Errorf("knowledge group is required")
	}
	statement := strings.TrimSpace(knowledgeStatement)
	if statement == "" && len(args) > 0 {
		statement = strings.TrimSpace(strings.Join(args, " "))
	}
	if statement == "" {
		return fmt.Errorf("proposal statement is required")
	}
	proposalID := strings.TrimSpace(knowledgeProposalID)
	if proposalID == "" {
		proposalID = "kp-" + randomShortID()
	}
	tagsJSON := tagsCSVToJSON(knowledgeTags)
	rec := &timeline.KnowledgeProposalRecord{
		ProposalID:         proposalID,
		GroupName:          groupName,
		Title:              strings.TrimSpace(knowledgeTitle),
		Statement:          statement,
		Tags:               tagsJSON,
		ProposerClawID:     strings.TrimSpace(cfg.Node.ClawID),
		ProposerInstanceID: strings.TrimSpace(cfg.Node.InstanceID),
		Status:             "pending",
	}
	if rec.ProposerClawID == "" || rec.ProposerInstanceID == "" {
		return fmt.Errorf("node.clawId and node.instanceId must be configured")
	}
	if err := timeSvc.CreateKnowledgeProposal(rec); err != nil {
		return err
	}

	traceID := newTraceID()
	idem := "knowledge:proposal:" + proposalID
	env := knowledge.Envelope{
		SchemaVersion:  knowledge.CurrentSchemaVersion,
		Type:           knowledge.TypeProposal,
		TraceID:        traceID,
		Timestamp:      time.Now(),
		IdempotencyKey: idem,
		ClawID:         rec.ProposerClawID,
		InstanceID:     rec.ProposerInstanceID,
		Payload: knowledge.ProposalPayload{
			ProposalID: proposalID,
			Group:      groupName,
			Title:      rec.Title,
			Statement:  rec.Statement,
			Tags:       mustParseTags(tagsJSON),
		},
	}
	if knowledgePublish {
		if err := publishKnowledgeEnvelope(cfg, timeSvc, cfg.Knowledge.Topics.Proposals, env); err != nil {
			return err
		}
	}
	return printKnowledgeOutput(cmd.OutOrStdout(), map[string]any{
		"status":      "ok",
		"action":      "propose",
		"proposalId":  proposalID,
		"group":       groupName,
		"published":   knowledgePublish,
		"idempotency": idem,
	})
}

func runKnowledgeVote(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	timeSvc, err := loadGroupTimeline()
	if err != nil {
		return err
	}
	defer timeSvc.Close()

	proposalID := strings.TrimSpace(knowledgeProposalID)
	if proposalID == "" {
		return fmt.Errorf("--proposal-id is required")
	}
	voteVal := strings.ToLower(strings.TrimSpace(knowledgeVote))
	if voteVal != "yes" && voteVal != "no" {
		return fmt.Errorf("--vote must be yes|no")
	}
	prop, err := timeSvc.GetKnowledgeProposal(proposalID)
	if err != nil {
		return err
	}
	if prop == nil {
		return fmt.Errorf("proposal %s not found", proposalID)
	}
	clawID := strings.TrimSpace(knowledgeAsClaw)
	if clawID == "" {
		clawID = strings.TrimSpace(cfg.Node.ClawID)
	}
	instanceID := strings.TrimSpace(knowledgeAsInstance)
	if instanceID == "" {
		instanceID = strings.TrimSpace(cfg.Node.InstanceID)
	}
	if clawID == "" || instanceID == "" {
		return fmt.Errorf("clawId/instanceId are required (configure node.clawId/node.instanceId or pass --as-claw/--as-instance)")
	}
	traceID := newTraceID()
	if err := timeSvc.UpsertKnowledgeVote(&timeline.KnowledgeVoteRecord{
		ProposalID: proposalID,
		ClawID:     clawID,
		InstanceID: instanceID,
		Vote:       voteVal,
		Reason:     strings.TrimSpace(knowledgeReason),
		TraceID:    traceID,
	}); err != nil {
		return err
	}
	votes, err := timeSvc.ListKnowledgeVotes(proposalID)
	if err != nil {
		return err
	}
	vmap := make(map[string]string, len(votes))
	for _, v := range votes {
		vmap[v.ClawID] = strings.ToLower(strings.TrimSpace(v.Vote))
	}
	poolSize := knowledgePoolSize
	if poolSize <= 0 {
		poolSize = estimateKnowledgePoolSize(timeSvc, cfg)
	}
	policy := knowledge.VotingPolicy{
		Enabled:       cfg.Knowledge.Voting.Enabled,
		MinPoolSize:   cfg.Knowledge.Voting.MinPoolSize,
		QuorumYes:     cfg.Knowledge.Voting.QuorumYes,
		QuorumNo:      cfg.Knowledge.Voting.QuorumNo,
		Timeout:       time.Duration(cfg.Knowledge.Voting.TimeoutSec) * time.Second,
		AllowSelfVote: cfg.Knowledge.Voting.AllowSelfVote,
	}
	decision := knowledge.EvaluateQuorum(
		prop.ProposerClawID,
		poolSize,
		vmap,
		prop.CreatedAt,
		time.Now(),
		policy,
	)
	if decision.Status != knowledge.VoteStatusPending {
		if err := timeSvc.UpdateKnowledgeProposalDecision(
			proposalID,
			decision.Status,
			decision.Yes,
			decision.No,
			decision.Reason,
		); err != nil {
			return err
		}
	}

	if knowledgePublish {
		voteEnv := knowledge.Envelope{
			SchemaVersion:  knowledge.CurrentSchemaVersion,
			Type:           knowledge.TypeVote,
			TraceID:        traceID,
			Timestamp:      time.Now(),
			IdempotencyKey: "knowledge:vote:" + proposalID + ":" + clawID,
			ClawID:         clawID,
			InstanceID:     instanceID,
			Payload: knowledge.VotePayload{
				ProposalID: proposalID,
				Vote:       voteVal,
				Reason:     strings.TrimSpace(knowledgeReason),
			},
		}
		if err := publishKnowledgeEnvelope(cfg, timeSvc, cfg.Knowledge.Topics.Votes, voteEnv); err != nil {
			return err
		}
		if decision.Status != knowledge.VoteStatusPending {
			decEnv := knowledge.Envelope{
				SchemaVersion:  knowledge.CurrentSchemaVersion,
				Type:           knowledge.TypeDecision,
				TraceID:        traceID,
				Timestamp:      time.Now(),
				IdempotencyKey: "knowledge:decision:" + proposalID,
				ClawID:         strings.TrimSpace(cfg.Node.ClawID),
				InstanceID:     strings.TrimSpace(cfg.Node.InstanceID),
				Payload: knowledge.DecisionPayload{
					ProposalID: proposalID,
					Outcome:    decision.Status,
					Yes:        decision.Yes,
					No:         decision.No,
					Reason:     decision.Reason,
				},
			}
			if err := publishKnowledgeEnvelope(cfg, timeSvc, cfg.Knowledge.Topics.Decisions, decEnv); err != nil {
				return err
			}
		}
	}

	return printKnowledgeOutput(cmd.OutOrStdout(), map[string]any{
		"status":     "ok",
		"action":     "vote",
		"proposalId": proposalID,
		"vote":       voteVal,
		"decision":   decision,
		"poolSize":   poolSize,
		"published":  knowledgePublish,
	})
}

func runKnowledgeDecisions(cmd *cobra.Command, args []string) error {
	timeSvc, err := loadGroupTimeline()
	if err != nil {
		return err
	}
	defer timeSvc.Close()
	status := strings.TrimSpace(knowledgeStatusFilter)
	if status == "" {
		status = "approved"
	}
	list, err := timeSvc.ListKnowledgeProposals(status, knowledgeLimit, 0)
	if err != nil {
		return err
	}
	return printKnowledgeOutput(cmd.OutOrStdout(), map[string]any{
		"status":    "ok",
		"action":    "decisions",
		"filter":    status,
		"count":     len(list),
		"decisions": list,
	})
}

func runKnowledgeFacts(cmd *cobra.Command, args []string) error {
	timeSvc, err := loadGroupTimeline()
	if err != nil {
		return err
	}
	defer timeSvc.Close()
	list, err := timeSvc.ListKnowledgeFacts(strings.TrimSpace(knowledgeGroup), knowledgeLimit, 0)
	if err != nil {
		return err
	}
	return printKnowledgeOutput(cmd.OutOrStdout(), map[string]any{
		"status": "ok",
		"action": "facts",
		"group":  strings.TrimSpace(knowledgeGroup),
		"count":  len(list),
		"facts":  list,
	})
}

func estimateKnowledgePoolSize(timeSvc *timeline.TimelineService, cfg *config.Config) int {
	if timeSvc != nil {
		if members, err := timeSvc.ListGroupMembers(); err == nil {
			n := len(members)
			if n > 0 {
				return n
			}
		}
	}
	if cfg != nil && cfg.Knowledge.Voting.MinPoolSize > 0 {
		return cfg.Knowledge.Voting.MinPoolSize
	}
	return 1
}

func publishKnowledgeEnvelope(cfg *config.Config, timeSvc *timeline.TimelineService, topic string, env knowledge.Envelope) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return fmt.Errorf("knowledge topic is empty")
	}
	if err := env.ValidateBase(); err != nil {
		return err
	}
	baseURL := strings.TrimSpace(cfg.Group.LFSProxyURL)
	if baseURL == "" || baseURL == "http://localhost:8080" {
		if v, err := timeSvc.GetSetting("kafscale_lfs_proxy_url"); err == nil && strings.TrimSpace(v) != "" {
			baseURL = strings.TrimSpace(v)
		}
	}
	if baseURL == "" {
		return fmt.Errorf("group.lfsProxyUrl is required for --publish")
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	client := group.NewLFSClient(baseURL, cfg.Group.LFSProxyAPIKey)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = client.Produce(ctx, topic, env.TraceID, payload)
	return err
}

func printKnowledgeOutput(w io.Writer, v map[string]any) error {
	if knowledgeJSON {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Fprintln(w, string(b))
		return nil
	}
	if action, ok := v["action"].(string); ok && action != "" {
		fmt.Fprintf(w, "knowledge %s: ok\n", action)
	}
	if proposalID, ok := v["proposalId"].(string); ok && proposalID != "" {
		fmt.Fprintf(w, "proposal: %s\n", proposalID)
	}
	if count, ok := v["count"]; ok {
		fmt.Fprintf(w, "count: %v\n", count)
	}
	return nil
}

func tagsCSVToJSON(raw string) string {
	out := parseCSVList(raw)
	b, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func mustParseTags(tagsJSON string) []string {
	var out []string
	_ = json.Unmarshal([]byte(tagsJSON), &out)
	return out
}

func randomShortID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
