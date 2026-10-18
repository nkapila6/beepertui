package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/auth"
	"github.com/nkapila6/beepertui/internal/beeper"
	"github.com/nkapila6/beepertui/internal/ui"
)

// set by goreleaser at build time
var version = "dev"

const usage = `beepertui: a terminal client for Beeper Desktop

usage:
  beepertui           open the app (logs in first if needed)
  beepertui login     approve beepertui in Beeper Desktop and save the token
  beepertui logout    forget and revoke the saved token
  beepertui status    show where the token comes from and if Beeper is reachable
  beepertui version

env:
  BEEPER_ACCESS_TOKEN  use this token instead of the saved one
  BEEPER_URL           API address (default http://localhost:23373)
  BEEPERTUI_IMAGES     kitty | blocks | off
  BEEPERTUI_NERDFONT   1 for nerd font icons
  BEEPERTUI_MOUSE      off to disable mouse capture

press ? inside for keys`

func main() {
	base := os.Getenv("BEEPER_URL")
	if base == "" {
		base = beeper.DefaultBaseURL
	}
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "-v", "--version", "version":
		fmt.Println("beepertui", version)
		return
	case "-h", "--help", "help":
		fmt.Println(usage)
		return
	case "login":
		if _, err := login(base); err != nil {
			fail(err)
		}
		return
	case "logout":
		if t := auth.Load(); t != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			auth.Revoke(ctx, base, t)
			cancel()
		}
		if err := auth.Delete(); err != nil {
			fail(err)
		}
		fmt.Println("logged out")
		return
	case "status":
		status(base)
		return
	case "":
	default:
		fail(fmt.Errorf("unknown command %q, try beepertui help", cmd))
	}

	token := os.Getenv("BEEPER_ACCESS_TOKEN")
	if token == "" {
		token = auth.Load()
	}
	if token == "" {
		fmt.Println("Not logged in yet.")
		t, err := login(base)
		if err != nil {
			fail(err)
		}
		token = t
	}

	m := ui.New(beeper.NewClient(base, token))
	defer m.Close()
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fail(err)
	}
}

func login(base string) (string, error) {
	if !reachable(base) {
		return "", fmt.Errorf("can't reach Beeper Desktop at %s. Open it and turn on Settings > Developers > Beeper Desktop API", base)
	}
	ctx := context.Background()
	token, err := auth.Login(ctx, base, func(s string) { fmt.Println(s) })
	if err != nil {
		return "", err
	}
	if err := auth.Save(token); err != nil {
		return "", fmt.Errorf("got a token but couldn't save it: %w", err)
	}
	fmt.Println("Logged in. Token saved to " + auth.Where() + ".")
	return token, nil
}

func reachable(base string) bool {
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get(base + "/v1/info")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func status(base string) {
	switch {
	case os.Getenv("BEEPER_ACCESS_TOKEN") != "":
		fmt.Println("token: from BEEPER_ACCESS_TOKEN")
	case auth.Load() != "":
		fmt.Println("token: saved in " + auth.Where())
	default:
		fmt.Println("token: none, run beepertui login")
	}
	if reachable(base) {
		fmt.Println("beeper: reachable at " + base)
	} else {
		fmt.Println("beeper: not reachable at " + base)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "beepertui:", err)
	os.Exit(1)
}
