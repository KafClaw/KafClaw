package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install kafclaw to /usr/local/bin",
	RunE:  runInstall,
}

var installJSON bool

func init() {
	installCmd.Flags().BoolVar(&installJSON, "json", false, "Output machine-readable JSON")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	if !installJSON {
		printHeader("📦 KafClaw Install")
	}

	exe, err := os.Executable()
	if err != nil {
		return printInstallResult(cmd, "error", "", fmt.Sprintf("failed to resolve executable: %v", err))
	}

	targetDir := "/usr/local/bin"
	if os.Geteuid() != 0 {
		targetDir = filepath.Join(os.Getenv("HOME"), ".local", "bin")
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return printInstallResult(cmd, "error", "", fmt.Sprintf("install failed: %v", err))
	}
	targetPath := filepath.Join(targetDir, "kafclaw")
	cmdCopy := exec.Command("cp", exe, targetPath)
	cmdCopy.Stdout = os.Stdout
	cmdCopy.Stderr = os.Stderr
	if err := cmdCopy.Run(); err != nil {
		return printInstallResult(cmd, "error", "", fmt.Sprintf("install failed: %v", err))
	}
	return printInstallResult(cmd, "ok", targetPath, "")
}

func printInstallResult(cmd *cobra.Command, status, targetPath, errMsg string) error {
	if installJSON {
		payload := map[string]any{
			"status":  status,
			"command": "install",
		}
		if targetPath != "" {
			payload["result"] = map[string]any{
				"targetPath": targetPath,
			}
		}
		if errMsg != "" {
			payload["error"] = errMsg
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
	} else {
		if status == "ok" {
			fmt.Fprintf(cmd.OutOrStdout(), "Installed to %s\n", targetPath)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), errMsg)
		}
	}
	if status != "ok" {
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}
