// Package theme loads colour themes and turns them into the escape codes
// that repaint the terminal's palette while beepertui runs.
//
// The app only ever draws with the terminal's 16 ANSI colours and its default
// text and background, so a theme doesn't restyle anything: it sets those 18
// slots (OSC 4/10/11) on start and resets them (OSC 104/110/111) on quit.
//
// Themes come from three places: built-ins, .toml files in the themes folder,
// and Neovim colorschemes ("nvim:<name>"), which are imported once by running
// nvim headless and cached as a file.
package theme

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Terminal means no theme: the terminal's own colours.
const Terminal = "terminal"

type Theme struct {
	Name   string
	Bg, Fg string
	Colors [16]string // #rrggbb
}

// Sequences sets the terminal to the theme.
func (t Theme) Sequences() string {
	var b strings.Builder
	// bright black is the app's dim text; some themes make it a background
	// shade, which would leave previews and times unreadable
	if lumGap(t.Colors[8], t.Bg) < 0.22 {
		t.Colors[8] = mixHex(t.Bg, t.Fg, 0.5)
	}
	for i, c := range t.Colors {
		fmt.Fprintf(&b, "\x1b]4;%d;%s\x1b\\", i, c)
	}
	fmt.Fprintf(&b, "\x1b]10;%s\x1b\\\x1b]11;%s\x1b\\\x1b]12;%s\x1b\\", t.Fg, t.Bg, t.Fg)
	return b.String()
}

// Reset puts the terminal's own palette, text, background and cursor colours back.
func Reset() string {
	return "\x1b]104\x1b\\\x1b]110\x1b\\\x1b]111\x1b\\\x1b]112\x1b\\"
}

// Dir is where theme files live: ~/.config/beepertui/themes.
func Dir(configDir string) string { return filepath.Join(configDir, "themes") }

// fileName is where a theme is saved; nvim imports get a prefix so they don't
// clash with hand-made ones.
func fileName(name string) string {
	if n, ok := strings.CutPrefix(name, "nvim:"); ok {
		name = "nvim-" + n
	}
	return strings.NewReplacer("/", "-", "\\", "-", "..", "-").Replace(name) + ".toml"
}

// Load finds a theme by name: a built-in, then a file. nvim themes that
// haven't been imported yet return ErrNotImported, so the caller can import
// them (slow, it starts nvim) off the UI thread.
func Load(configDir, name string) (Theme, error) {
	if t, ok := Builtins[name]; ok {
		t.Name = name
		return t, nil
	}
	t, err := ReadFile(filepath.Join(Dir(configDir), fileName(name)))
	if os.IsNotExist(err) {
		if strings.HasPrefix(name, "nvim:") {
			return Theme{}, ErrNotImported
		}
		return Theme{}, fmt.Errorf("no theme %q (see :theme)", name)
	}
	t.Name = name
	return t, err
}

var ErrNotImported = fmt.Errorf("not imported yet")

// ReadFile parses a theme file: bg, fg and color0..color15 as "#rrggbb", one
// per line, # comments allowed.
func ReadFile(path string) (Theme, error) {
	f, err := os.Open(path)
	if err != nil {
		return Theme{}, err
	}
	defer f.Close()
	var t Theme
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return t, fmt.Errorf("%s line %d: expected key = value", path, n)
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if !isHex(v) {
			return t, fmt.Errorf("%s line %d: %q isn't a #rrggbb colour", path, n, v)
		}
		switch {
		case k == "bg" || k == "background":
			t.Bg = v
		case k == "fg" || k == "foreground":
			t.Fg = v
		case strings.HasPrefix(k, "color"):
			i, err := strconv.Atoi(strings.TrimPrefix(k, "color"))
			if err != nil || i < 0 || i > 15 {
				return t, fmt.Errorf("%s line %d: colours go from color0 to color15", path, n)
			}
			t.Colors[i] = v
		}
	}
	if err := sc.Err(); err != nil {
		return t, err
	}
	return t, t.complete()
}

// complete fills gaps (brights from normals and back) and checks the rest.
func (t *Theme) complete() error {
	for i := 0; i < 8; i++ {
		if t.Colors[i+8] == "" {
			t.Colors[i+8] = t.Colors[i]
		}
		if t.Colors[i] == "" {
			t.Colors[i] = t.Colors[i+8]
		}
	}
	if t.Bg == "" || t.Fg == "" {
		return fmt.Errorf("a theme needs bg and fg")
	}
	for i, c := range t.Colors {
		if c == "" {
			return fmt.Errorf("color%d is missing", i)
		}
	}
	return nil
}

// Save writes a theme file.
func Save(configDir, name string, t Theme) error {
	dir := Dir(configDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# beepertui theme %q\nbg = %q\nfg = %q\n", name, t.Bg, t.Fg)
	for i, c := range t.Colors {
		fmt.Fprintf(&b, "color%d = %q\n", i, c)
	}
	return os.WriteFile(filepath.Join(dir, fileName(name)), []byte(b.String()), 0o644)
}

// Saved lists the theme files in the folder, by the name to use in config.
func Saved(configDir string) []string {
	entries, _ := os.ReadDir(Dir(configDir))
	var out []string
	for _, e := range entries {
		n, ok := strings.CutSuffix(e.Name(), ".toml")
		if !ok || e.IsDir() {
			continue
		}
		if v, ok := strings.CutPrefix(n, "nvim-"); ok {
			n = "nvim:" + v
		}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// BuiltinNames is the built-ins in a stable order.
func BuiltinNames() []string {
	out := make([]string, 0, len(Builtins))
	for n := range Builtins {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func isHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(s[1:], 16, 32)
	return err == nil
}

func rgb(h string) (r, g, b float64) {
	v, _ := strconv.ParseUint(strings.TrimPrefix(h, "#"), 16, 32)
	return float64(v>>16&255) / 255, float64(v>>8&255) / 255, float64(v&255) / 255
}

func lumGap(a, b string) float64 {
	lum := func(h string) float64 { r, g, bl := rgb(h); return 0.299*r + 0.587*g + 0.114*bl }
	d := lum(a) - lum(b)
	if d < 0 {
		d = -d
	}
	return d
}

func mixHex(a, b string, t float64) string {
	ar, ag, ab := rgb(a)
	br, bg, bb := rgb(b)
	m := func(x, y float64) int { return int((x*(1-t) + y*t) * 255) }
	return fmt.Sprintf("#%02x%02x%02x", m(ar, br), m(ag, bg), m(ab, bb))
}
