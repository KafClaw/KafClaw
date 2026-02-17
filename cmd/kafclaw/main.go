// Package main is the entry point for the KafClaw CLI.
package main

import (
	"os"

	"github.com/KafClaw/KafClaw/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
