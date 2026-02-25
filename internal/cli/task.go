package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

var (
	taskCmd = &cobra.Command{
		Use:   "task",
		Short: "Task workflow utilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	taskStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show cascading protocol status for a trace",
		RunE:  runTaskStatus,
	}
)

func init() {
	taskStatusCmd.Flags().String("trace", "", "Trace ID")
	taskStatusCmd.Flags().Bool("json", false, "Output machine-readable JSON")
	taskCmd.AddCommand(taskStatusCmd)
	rootCmd.AddCommand(taskCmd)
}

func runTaskStatus(cmd *cobra.Command, args []string) error {
	traceID, _ := cmd.Flags().GetString("trace")
	asJSON, _ := cmd.Flags().GetBool("json")
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return fmt.Errorf("--trace is required")
	}
	timeSvc, err := loadGroupTimeline()
	if err != nil {
		return err
	}
	defer timeSvc.Close()

	tasks, err := timeSvc.ListCascadeTasks(traceID)
	if err != nil {
		return err
	}
	transitions, err := timeSvc.ListCascadeTransitions(traceID, "", 500)
	if err != nil {
		return err
	}
	out := map[string]any{
		"traceId":            traceID,
		"taskCount":          len(tasks),
		"transitionCount":    len(transitions),
		"tasks":              tasks,
		"transitionsPreview": transitions,
	}
	return printTaskOutput(cmd.OutOrStdout(), out, asJSON)
}

func printTaskOutput(w io.Writer, payload any, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}
	m, ok := payload.(map[string]any)
	if !ok {
		_, err := fmt.Fprintln(w, payload)
		return err
	}
	_, _ = fmt.Fprintf(w, "Trace: %v\n", m["traceId"])
	_, _ = fmt.Fprintf(w, "Cascade tasks: %v\n", m["taskCount"])
	_, _ = fmt.Fprintf(w, "Transitions: %v\n", m["transitionCount"])
	tasks, _ := m["tasks"].([]any)
	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(w, "No cascading tasks recorded.")
		return nil
	}
	_, _ = fmt.Fprintln(w, "Tasks:")
	for i, item := range tasks {
		_, _ = fmt.Fprintf(w, "  %d. %v\n", i+1, item)
	}
	return nil
}
