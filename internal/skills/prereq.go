package skills

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// PrereqResult captures prerequisite status/install output.
type PrereqResult struct {
	Name      string   `json:"name"`
	Installed bool     `json:"installed"`
	Messages  []string `json:"messages,omitempty"`
}

// CheckPrerequisite checks whether a named prerequisite is present.
func CheckPrerequisite(name string) (*PrereqResult, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "google-cli", "gcloud":
		return &PrereqResult{
			Name:      "google-cli",
			Installed: HasBinary("gcloud"),
			Messages:  []string{"checks `gcloud` in PATH"},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported prerequisite: %s", name)
	}
}

// InstallPrerequisite installs a named prerequisite using an OS-specific routine.
func InstallPrerequisite(name string, dryRun bool) (*PrereqResult, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "google-cli", "gcloud":
		return installGoogleCLI(dryRun)
	default:
		return nil, fmt.Errorf("unsupported prerequisite: %s", name)
	}
}

func installGoogleCLI(dryRun bool) (*PrereqResult, error) {
	if HasBinary("gcloud") {
		return &PrereqResult{
			Name:      "google-cli",
			Installed: true,
			Messages:  []string{"gcloud already present in PATH"},
		}, nil
	}
	switch runtime.GOOS {
	case "linux":
		return installGoogleCLILinuxAPT(dryRun)
	case "darwin":
		return installGoogleCLIDarwinBrew(dryRun)
	default:
		return nil, fmt.Errorf("google-cli install not implemented for os=%s", runtime.GOOS)
	}
}

func installGoogleCLIDarwinBrew(dryRun bool) (*PrereqResult, error) {
	if !HasBinary("brew") {
		return nil, errors.New("homebrew not found; install brew first")
	}
	cmds := [][]string{
		{"brew", "update"},
		{"brew", "install", "--cask", "google-cloud-sdk"},
	}
	msgs := formatCommands(cmds)
	if dryRun {
		return &PrereqResult{Name: "google-cli", Installed: false, Messages: msgs}, nil
	}
	for _, c := range cmds {
		if err := runCommand(3*time.Minute, c[0], c[1:]...); err != nil {
			return nil, err
		}
	}
	return CheckPrerequisite("google-cli")
}

func installGoogleCLILinuxAPT(dryRun bool) (*PrereqResult, error) {
	if !HasBinary("apt-get") {
		return nil, errors.New("apt-get not found; only apt-based install is currently supported")
	}
	if !HasBinary("sudo") {
		return nil, errors.New("sudo is required for apt install routine")
	}
	if !HasBinary("gpg") {
		return nil, errors.New("gpg is required")
	}

	cmds := [][]string{
		{"sudo", "apt-get", "update"},
		{"sudo", "apt-get", "install", "-y", "ca-certificates", "gnupg", "curl"},
		{"sudo", "install", "-m", "0755", "-d", "/usr/share/keyrings"},
		{"sudo", "tee", "/etc/apt/sources.list.d/google-cloud-sdk.list"},
		{"sudo", "apt-get", "update"},
		{"sudo", "apt-get", "install", "-y", "google-cloud-cli"},
	}
	msgs := formatCommands(cmds)
	if dryRun {
		return &PrereqResult{Name: "google-cli", Installed: false, Messages: msgs}, nil
	}

	key, err := fetchGoogleCloudAPTKey()
	if err != nil {
		return nil, err
	}
	dearmored, err := dearmorGPGKey(key)
	if err != nil {
		return nil, err
	}

	if err := runCommand(5*time.Minute, "sudo", "apt-get", "update"); err != nil {
		return nil, err
	}
	if err := runCommand(5*time.Minute, "sudo", "apt-get", "install", "-y", "ca-certificates", "gnupg", "curl"); err != nil {
		return nil, err
	}
	if err := runCommand(1*time.Minute, "sudo", "install", "-m", "0755", "-d", "/usr/share/keyrings"); err != nil {
		return nil, err
	}
	if err := runCommandWithStdin(1*time.Minute, dearmored, "sudo", "tee", "/usr/share/keyrings/cloud.google.gpg"); err != nil {
		return nil, err
	}
	repoLine := "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main\n"
	if err := runCommandWithStdin(1*time.Minute, []byte(repoLine), "sudo", "tee", "/etc/apt/sources.list.d/google-cloud-sdk.list"); err != nil {
		return nil, err
	}
	if err := runCommand(5*time.Minute, "sudo", "apt-get", "update"); err != nil {
		return nil, err
	}
	if err := runCommand(10*time.Minute, "sudo", "apt-get", "install", "-y", "google-cloud-cli"); err != nil {
		return nil, err
	}
	return CheckPrerequisite("google-cli")
}

func fetchGoogleCloudAPTKey() ([]byte, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://packages.cloud.google.com/apt/doc/apt-key.gpg", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to fetch apt key: %s", resp.Status)
	}
	var b bytes.Buffer
	if _, err := b.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	if b.Len() == 0 {
		return nil, errors.New("fetched apt key is empty")
	}
	return b.Bytes(), nil
}

func dearmorGPGKey(key []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gpg", "--dearmor")
	cmd.Stdin = bytes.NewReader(key)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gpg --dearmor failed: %w (%s)", err, out.String())
	}
	if out.Len() == 0 {
		return nil, errors.New("gpg --dearmor produced empty output")
	}
	return out.Bytes(), nil
}

func runCommand(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%s %s failed: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func runCommandWithStdin(timeout time.Duration, stdin []byte, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%s %s failed: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return fmt.Errorf("%s %s failed: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func formatCommands(cmds [][]string) []string {
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, strings.Join(c, " "))
	}
	return out
}
