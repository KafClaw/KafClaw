package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install kafclaw to /usr/local/bin",
	Run:   runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) {
	printHeader("📦 KafClaw Install")

	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Failed to resolve executable: %v\n", err)
		return
	}

	targetDir := "/usr/local/bin"
	if os.Geteuid() != 0 {
		targetDir = filepath.Join(os.Getenv("HOME"), ".local", "bin")
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		fmt.Printf("Install failed: %v\n", err)
		return
	}
	targetPath := filepath.Join(targetDir, "kafclaw")
	cmdCopy := exec.Command("cp", exe, targetPath)
	cmdCopy.Stdout = os.Stdout
	cmdCopy.Stderr = os.Stderr
	if err := cmdCopy.Run(); err != nil {
		fmt.Printf("Install failed: %v\n", err)
		return
	}
	fmt.Printf("Installed to %s\n", targetPath)
}
