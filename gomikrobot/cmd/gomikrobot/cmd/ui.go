package cmd

import (
	"fmt"

	"github.com/fatih/color"
)

func printHeader(title string) {
	fmt.Println(color.CyanString(logo))
	if title != "" {
		fmt.Println(title)
		fmt.Println("─────────────────────")
	}
}
