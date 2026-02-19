package cli

import (
	"encoding/json"
	"fmt"

	"github.com/KafClaw/KafClaw/internal/cliconfig"
	"github.com/spf13/cobra"
)

var doctorFix bool
var doctorGenerateGatewayToken bool
var doctorJSON bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run config and setup diagnostics",
	RunE: func(cmd *cobra.Command, args []string) error {
		report, err := cliconfig.RunDoctorWithOptions(cliconfig.DoctorOptions{
			Fix:                  doctorFix,
			GenerateGatewayToken: doctorGenerateGatewayToken,
		})
		if err != nil {
			return err
		}
		if doctorJSON {
			payload := map[string]any{
				"status":  "ok",
				"command": "doctor",
				"result":  report,
			}
			failures := 0
			for _, check := range report.Checks {
				if check.Status == cliconfig.DoctorFail {
					failures++
				}
			}
			if failures > 0 {
				payload["status"] = "error"
				payload["error"] = fmt.Sprintf("doctor found %d failing check(s)", failures)
			}
			b, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			if failures > 0 {
				return fmt.Errorf("doctor found %d failing check(s)", failures)
			}
			return nil
		}

		failures := 0
		for _, check := range report.Checks {
			symbol := "PASS"
			if check.Status == cliconfig.DoctorWarn {
				symbol = "WARN"
			}
			if check.Status == cliconfig.DoctorFail {
				symbol = "FAIL"
				failures++
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s\n", symbol, check.Name, check.Message)
		}

		if failures > 0 {
			return fmt.Errorf("doctor found %d failing check(s)", failures)
		}
		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Apply safe fixes (merge env files, enforce loopback gateway host)")
	doctorCmd.Flags().BoolVar(&doctorGenerateGatewayToken, "generate-gateway-token", false, "Generate and persist a new gateway auth token")
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Output machine-readable JSON report")
	rootCmd.AddCommand(doctorCmd)
}
