// Package clinst provides CLI tool installation helpers for OAuth-based providers.
package clinst

import (
	"fmt"
	"os/exec"
)

// EnsureGeminiCLI checks if the Gemini CLI is available and installs it if absent.
func EnsureGeminiCLI() error {
	if _, err := exec.LookPath("gemini"); err == nil {
		return nil
	}
	fmt.Println("Gemini CLI not found. Installing @google/gemini-cli via npm...")
	cmd := exec.Command("npm", "install", "-g", "@google/gemini-cli")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install gemini CLI: %w (you can install manually: npm install -g @google/gemini-cli)", err)
	}
	if _, err := exec.LookPath("gemini"); err != nil {
		return fmt.Errorf("gemini CLI installed but not found in PATH")
	}
	return nil
}

// EnsureCodexCLI checks if the Codex CLI is available and installs it if absent.
func EnsureCodexCLI() error {
	if _, err := exec.LookPath("codex"); err == nil {
		return nil
	}
	fmt.Println("Codex CLI not found. Installing @openai/codex via npm...")
	cmd := exec.Command("npm", "install", "-g", "@openai/codex")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install codex CLI: %w (you can install manually: npm install -g @openai/codex)", err)
	}
	if _, err := exec.LookPath("codex"); err != nil {
		return fmt.Errorf("codex CLI installed but not found in PATH")
	}
	return nil
}
