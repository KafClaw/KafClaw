// Package main is the entry point for gonanobot CLI.
package main

import (
	"os"

	"github.com/kamir/gomikrobot/cmd/gomikrobot/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
