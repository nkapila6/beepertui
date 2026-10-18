package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
	"github.com/nkapila6/beepertui/internal/ui"
)

// set by goreleaser at build time
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-v", "--version", "version":
			fmt.Println("beepertui", version)
			return
		case "-h", "--help", "help":
			fmt.Println("beepertui: a terminal client for Beeper Desktop")
			fmt.Println()
			fmt.Println("needs BEEPER_ACCESS_TOKEN (Beeper Desktop > Settings > Developers)")
			fmt.Println("optional BEEPER_URL (default http://localhost:23373), BEEPERTUI_IMAGES=kitty|blocks|off")
			fmt.Println("press ? inside for keys")
			return
		}
	}
	token := os.Getenv("BEEPER_ACCESS_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "BEEPER_ACCESS_TOKEN is not set.")
		fmt.Fprintln(os.Stderr, "Create one in Beeper Desktop under Settings > Developers, then export it.")
		os.Exit(1)
	}
	c := beeper.NewClient(os.Getenv("BEEPER_URL"), token)
	m := ui.New(c)
	defer m.Close()
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
