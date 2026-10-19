// Package config reads and writes ~/.config/beepertui/config.toml.
//
// The file is a flat list of `key = value` lines, a subset of TOML, so no
// parser dependency is needed and it stays easy to edit by hand. Environment
// variables win over the file, and the file wins over defaults.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Setting describes one key: its default, what it accepts, and the env var that overrides it.
type Setting struct {
	Key     string
	Default string
	Help    string
	Env     string
	Choices []string // fixed values, or nil
	Kind    string   // "bool", "int:lo-hi", "color", "string", or "choice"
}

var Settings = []Setting{
	{Key: "nerd_font", Default: "false", Kind: "bool", Env: "BEEPERTUI_NERDFONT", Help: "nerd font icons and powerline status line"},
	{Key: "images", Default: "auto", Kind: "choice", Choices: []string{"auto", "kitty", "blocks", "off"}, Env: "BEEPERTUI_IMAGES", Help: "inline image style; auto picks kitty in Ghostty/kitty"},
	{Key: "image_size", Default: "medium", Kind: "choice", Choices: []string{"small", "medium", "large"}, Help: "how big inline images get"},
	{Key: "avatars", Default: "true", Kind: "bool", Help: "chat avatars in the list"},
	{Key: "compact", Default: "false", Kind: "bool", Help: "no blank line between senders"},
	{Key: "mouse", Default: "true", Kind: "bool", Env: "BEEPERTUI_MOUSE", Help: "mouse capture (off gives selection back to the terminal)"},
	{Key: "notifications", Default: "true", Kind: "bool", Help: "notifications for new messages (desktop when the terminal is in the background, a toast otherwise), downloads and forwards"},
	{Key: "notification_preview", Default: "true", Kind: "bool", Help: "show the message text in notifications"},
	{Key: "chat_width", Default: "32", Kind: "int:20-60", Help: "chat list width, percent of the window"},
	{Key: "inbox", Default: "primary", Kind: "choice", Choices: []string{"primary", "low-priority", "archive", "unread"}, Help: "inbox shown at startup"},
	{Key: "accent", Default: "blue", Kind: "color", Help: "accent colour: a palette name (blue, cyan, magenta...), 0-15, or #hex"},
	{Key: "gif_provider", Default: "auto", Kind: "choice", Choices: []string{"auto", "klipy", "giphy", "memegen"}, Help: "GIF search: auto uses whichever key is set (BEEPERTUI_KLIPY_KEY / BEEPERTUI_GIPHY_KEY), else free memegen.link memes"},
	{Key: "beeper_url", Default: "http://localhost:23373", Kind: "string", Env: "BEEPER_URL", Help: "Beeper Desktop API address"},
	{Key: "autostart_beeper", Default: "true", Kind: "bool", Env: "BEEPERTUI_AUTOSTART", Help: "start Beeper Desktop hidden in the background when it isn't running"},
}

var colorNames = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"bright-black", "bright-red", "bright-green", "bright-yellow", "bright-blue", "bright-magenta", "bright-cyan", "bright-white"}

// Config holds the values from the file only (not env or defaults).
type Config struct {
	values map[string]string
	path   string
}

// Dir is ~/.config/beepertui on macOS and Linux (XDG_CONFIG_HOME respected), and the usual app data folder on Windows.
func Dir() string {
	if runtime.GOOS == "windows" {
		if d, err := os.UserConfigDir(); err == nil {
			return filepath.Join(d, "beepertui")
		}
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "beepertui")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "beepertui")
}

func Path() string { return filepath.Join(Dir(), "config.toml") }

// KeyBind is a remappable action. The ui package registers them, and the
// file stores changes as key.<action> = "k1, k2".
type KeyBind struct {
	Action  string
	Keys    string // defaults, comma separated
	Section string
	Help    string
}

// Binds holds every remappable action, filled in by the ui package.
var Binds []KeyBind

const KeyPrefix = "key."

func lookup(key string) (Setting, bool) {
	for _, s := range Settings {
		if s.Key == key {
			return s, true
		}
	}
	if a, ok := strings.CutPrefix(key, KeyPrefix); ok {
		for _, k := range Binds {
			if k.Action == a {
				return Setting{Key: key, Default: k.Keys, Kind: "keys", Help: k.Help}, true
			}
		}
	}
	return Setting{}, false
}

// SplitKeys turns "j, down" into its keys. An empty value means unbound.
func SplitKeys(v string) []string {
	var out []string
	for _, k := range strings.Split(v, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// Load reads the config file. A missing file is just an empty config.
func Load() (*Config, error) {
	c := &Config{values: map[string]string{}, path: Path()}
	f, err := os.Open(c.path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return c, fmt.Errorf("%s line %d: expected key = value", c.path, n)
		}
		k = strings.TrimSpace(k)
		v = unquote(strings.TrimSpace(v))
		if _, known := lookup(k); !known {
			return c, fmt.Errorf("%s line %d: unknown setting %q", c.path, n, k)
		}
		if err := Validate(k, v); err != nil {
			return c, fmt.Errorf("%s line %d: %w", c.path, n, err)
		}
		c.values[k] = v
	}
	return c, sc.Err()
}

func unquote(v string) string {
	if i := strings.Index(v, " #"); i >= 0 && !strings.HasPrefix(v, `"`) {
		v = strings.TrimSpace(v[:i])
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if u, err := strconv.Unquote(v); err == nil {
			return u
		}
	}
	return v
}

// Validate checks a value against its setting.
func Validate(key, val string) error {
	s, ok := lookup(key)
	if !ok {
		return fmt.Errorf("unknown setting %q (see beepertui config)", key)
	}
	switch {
	case s.Kind == "bool":
		if _, err := strconv.ParseBool(val); err != nil {
			return fmt.Errorf("%s wants true or false", key)
		}
	case s.Kind == "choice":
		for _, c := range s.Choices {
			if val == c {
				return nil
			}
		}
		return fmt.Errorf("%s wants one of %s", key, strings.Join(s.Choices, ", "))
	case strings.HasPrefix(s.Kind, "int:"):
		var lo, hi int
		fmt.Sscanf(strings.TrimPrefix(s.Kind, "int:"), "%d-%d", &lo, &hi)
		n, err := strconv.Atoi(val)
		if err != nil || n < lo || n > hi {
			return fmt.Errorf("%s wants a number from %d to %d", key, lo, hi)
		}
	case s.Kind == "keys":
		for _, k := range SplitKeys(val) {
			// these always have to work, so you can't lock yourself out
			if k == "esc" || k == "ctrl+c" {
				return fmt.Errorf("%s can't be rebound", k)
			}
		}
	case s.Kind == "color":
		if _, err := ParseColor(val); err != nil {
			return err
		}
	}
	return nil
}

// ParseColor turns a palette name, 0-15 or #rrggbb into what lipgloss.Color takes.
func ParseColor(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	for i, n := range colorNames {
		if v == n || v == strings.ReplaceAll(n, "-", "") || v == strings.ReplaceAll(n, "-", "_") {
			return strconv.Itoa(i), nil
		}
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 255 {
		return v, nil
	}
	if len(v) == 7 && v[0] == '#' {
		if _, err := strconv.ParseUint(v[1:], 16, 32); err == nil {
			return v, nil
		}
	}
	return "", fmt.Errorf("accent wants a colour name (%s), 0-255, or #rrggbb", strings.Join(colorNames[:8], ", "))
}

// Value resolves a key: env var, then file, then default. It also says where the value came from.
func (c *Config) Value(key string) (string, string) {
	s, _ := lookup(key)
	if s.Env != "" {
		if v := os.Getenv(s.Env); v != "" {
			return envValue(s, v), "env " + s.Env
		}
	}
	if v, ok := c.values[key]; ok {
		return v, "config"
	}
	return s.Default, "default"
}

// envValue maps the older env var spellings (1, on) onto the setting's values.
func envValue(s Setting, v string) string {
	if s.Kind == "bool" {
		switch strings.ToLower(v) {
		case "1", "on", "yes", "true":
			return "true"
		case "0", "off", "no", "false":
			return "false"
		}
	}
	return v
}

func (c *Config) Bool(key string) bool {
	v, _ := c.Value(key)
	b, _ := strconv.ParseBool(v)
	return b
}

func (c *Config) Int(key string) int {
	v, _ := c.Value(key)
	n, _ := strconv.Atoi(v)
	return n
}

func (c *Config) String(key string) string {
	v, _ := c.Value(key)
	return v
}

// Set validates and stores a value in memory. Call Save to write it.
func (c *Config) Set(key, val string) error {
	if err := Validate(key, val); err != nil {
		return err
	}
	c.values[key] = val
	return nil
}

func (c *Config) Unset(key string) error {
	if _, ok := lookup(key); !ok {
		return fmt.Errorf("unknown setting %q", key)
	}
	delete(c.values, key)
	return nil
}

// Save writes the file back, keeping every setting documented as a comment.
func (c *Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# beepertui settings. Edit here or with `beepertui config set <key> <value>`.\n")
	b.WriteString("# Commented lines show the defaults. Env vars (where noted) win over this file.\n\n")
	keys := make([]string, 0, len(Settings))
	for _, s := range Settings {
		keys = append(keys, s.Key)
	}
	for _, k := range keys {
		s, _ := lookup(k)
		help := s.Help
		if s.Env != "" {
			help += " (env " + s.Env + ")"
		}
		fmt.Fprintf(&b, "# %s\n", help)
		if v, ok := c.values[k]; ok {
			fmt.Fprintf(&b, "%s = %s\n\n", k, quote(v))
		} else {
			fmt.Fprintf(&b, "# %s = %s\n\n", k, quote(s.Default))
		}
	}
	var binds []string
	for k := range c.values {
		if strings.HasPrefix(k, KeyPrefix) {
			binds = append(binds, k)
		}
	}
	sort.Strings(binds)
	b.WriteString("# key binds you've changed, as key.<action> = \"key, key\". `beepertui config keys` or :keys lists them all.\n")
	for _, k := range binds {
		fmt.Fprintf(&b, "%s = %s\n", k, strconv.Quote(c.values[k]))
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func quote(v string) string {
	if _, err := strconv.ParseBool(v); err == nil {
		return v
	}
	if _, err := strconv.Atoi(v); err == nil {
		return v
	}
	return strconv.Quote(v)
}

// Keys lists the setting names, sorted, for completion and errors.
func Keys() []string {
	out := make([]string, len(Settings))
	for i, s := range Settings {
		out[i] = s.Key
	}
	sort.Strings(out)
	return out
}
