package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install gomikrobot to /usr/local/bin",
	Run:   runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) {
	printHeader("📦 GoMikroBot Install")

	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Failed to resolve executable: %v\n", err)
		return
	}

	script := filepath.Join(filepath.Dir(exe), "scripts", "install.sh")
	if _, err := os.Stat(script); err == nil {
		cmdRun := exec.Command("bash", script, exe)
		cmdRun.Stdout = os.Stdout
		cmdRun.Stderr = os.Stderr
		if err := cmdRun.Run(); err != nil {
			fmt.Printf("Install failed: %v\n", err)
		}
		return
	}

	targetDir := "/usr/local/bin"
	targetPath := filepath.Join(targetDir, "gomikrobot")
	cmdCopy := exec.Command("cp", exe, targetPath)
	cmdCopy.Stdout = os.Stdout
	cmdCopy.Stderr = os.Stderr
	if err := cmdCopy.Run(); err != nil {
		fmt.Printf("Install failed (try with sudo): %v\n", err)
		return
	}
	fmt.Printf("Installed to %s\n", targetPath)
}
