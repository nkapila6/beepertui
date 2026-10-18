package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/auth"
	"github.com/nkapila6/beepertui/internal/beeper"
	"github.com/nkapila6/beepertui/internal/config"
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
  beepertui config    show settings (config get|set|unset|edit|path)
  beepertui version

settings live in ~/.config/beepertui/config.toml (beepertui config).
env vars override them:
  BEEPER_ACCESS_TOKEN  use this token instead of the saved one
  BEEPER_URL           API address (default http://localhost:23373)
  BEEPERTUI_IMAGES     kitty | blocks | off
  BEEPERTUI_NERDFONT   1 for nerd font icons
  BEEPERTUI_MOUSE      off to disable mouse capture
  BEEPERTUI_AUTOSTART  0 to stop beepertui starting Beeper Desktop for you
  BEEPERTUI_KLIPY_KEY  KLIPY key for GIF search
  BEEPERTUI_GIPHY_KEY  GIPHY key for GIF search

press ? inside for keys`

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	cfg, err := config.Load()
	if err != nil {
		// a broken file shouldn't lock you out of fixing it
		if cmd != "config" {
			fail(fmt.Errorf("%w (fix it with beepertui config edit)", err))
		}
		fmt.Fprintln(os.Stderr, "warning:", err)
	}
	base := cfg.String("beeper_url")
	switch cmd {
	case "-v", "--version", "version":
		fmt.Println("beepertui", version)
		return
	case "-h", "--help", "help":
		fmt.Println(usage)
		return
	case "login":
		ensureBeeper(cfg, base)
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
	case "config":
		configCmd(cfg, os.Args[2:])
		return
	case "":
	default:
		fail(fmt.Errorf("unknown command %q, try beepertui help", cmd))
	}

	ensureBeeper(cfg, base)
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

	m := ui.New(beeper.NewClient(base, token), cfg)
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

// ensureBeeper starts Beeper Desktop hidden when its API isn't answering,
// then waits for it. Remote URLs are left alone since we can't start those.
func ensureBeeper(cfg *config.Config, base string) {
	if !cfg.Bool("autostart_beeper") || !isLocal(base) || reachable(base) {
		return
	}
	if err := launchBeeper(); err != nil {
		return // the usual "can't reach" error explains what to do
	}
	fmt.Print("starting Beeper Desktop in the background...")
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(time.Second) {
		if reachable(base) {
			fmt.Println(" ok")
			return
		}
	}
	fmt.Println(" still not answering after 30s")
}

func isLocal(base string) bool {
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func launchBeeper() error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// -g leaves focus in the terminal, -j launches it hidden
		c = exec.Command("open", "-g", "-j", "-b", "com.automattic.beeper.desktop")
	case "windows":
		exe := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "beeper", "Beeper.exe")
		if _, err := os.Stat(exe); err != nil {
			return err
		}
		c = exec.Command("cmd", "/c", "start", "/min", "", exe)
	default:
		var bin string
		for _, name := range []string{"beeper", "beeper-desktop"} {
			if p, err := exec.LookPath(name); err == nil {
				bin = p
				break
			}
		}
		if bin == "" {
			return errors.New("beeper not found in PATH")
		}
		c = exec.Command(bin)
		// own session, so closing the terminal doesn't take Beeper down with it
		if s, err := exec.LookPath("setsid"); err == nil {
			c = exec.Command(s, "-f", bin)
		}
	}
	return c.Start()
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

func configCmd(cfg *config.Config, args []string) {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "", "list", "show":
		fmt.Printf("# %s\n", config.Path())
		for _, s := range config.Settings {
			v, src := cfg.Value(s.Key)
			fmt.Printf("%-14s %-24s %-24s %s\n", s.Key, v, "("+src+")", s.Help)
		}
	case "get":
		if len(args) != 2 {
			fail(fmt.Errorf("usage: beepertui config get <key>"))
		}
		if err := config.Validate(args[1], cfgDefault(args[1])); err != nil && strings.HasPrefix(err.Error(), "unknown") {
			fail(err)
		}
		v, _ := cfg.Value(args[1])
		fmt.Println(v)
	case "set":
		if len(args) < 3 {
			fail(fmt.Errorf("usage: beepertui config set <key> <value>"))
		}
		val := strings.Join(args[2:], " ")
		if err := cfg.Set(args[1], val); err != nil {
			fail(err)
		}
		if err := cfg.Save(); err != nil {
			fail(err)
		}
		fmt.Printf("%s = %s\n", args[1], val)
		if _, src := cfg.Value(args[1]); strings.HasPrefix(src, "env") {
			fmt.Printf("note: %s is set and overrides this\n", strings.TrimPrefix(src, "env "))
		}
	case "unset", "reset":
		if len(args) != 2 {
			fail(fmt.Errorf("usage: beepertui config unset <key>"))
		}
		if err := cfg.Unset(args[1]); err != nil {
			fail(err)
		}
		if err := cfg.Save(); err != nil {
			fail(err)
		}
		v, _ := cfg.Value(args[1])
		fmt.Printf("%s back to %s\n", args[1], v)
	case "path":
		fmt.Println(config.Path())
	case "edit":
		if _, err := os.Stat(config.Path()); os.IsNotExist(err) {
			if err := cfg.Save(); err != nil {
				fail(err)
			}
		}
		editor := os.Getenv("VISUAL")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		var c *exec.Cmd
		switch {
		case editor == "" && runtime.GOOS == "windows":
			c = exec.Command("notepad", config.Path())
		case editor == "":
			c = exec.Command("vi", config.Path())
		default:
			// through the shell so EDITOR="code --wait" style values work
			c = exec.Command("sh", "-c", editor+` "$1"`, "sh", config.Path())
		}
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			fail(err)
		}
		if _, err := config.Load(); err != nil {
			fail(fmt.Errorf("saved, but it doesn't parse: %w", err))
		}
	default:
		fail(fmt.Errorf("unknown config command %q: use get, set, unset, edit or path", sub))
	}
}

func cfgDefault(key string) string {
	for _, s := range config.Settings {
		if s.Key == key {
			return s.Default
		}
	}
	return ""
}
