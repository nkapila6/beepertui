package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
	"github.com/nkapila6/beepertui/internal/ui"
)

func main() {
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
