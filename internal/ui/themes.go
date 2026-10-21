package ui

import (
	"context"
	"image/color"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/config"
	"github.com/nkapila6/beepertui/internal/theme"
)

// Themes repaint the terminal's palette while the app runs (see the theme
// package). Everything already draws with palette slots, so nothing here
// restyles; it only sends the escape codes and tells modern's tints what the
// new background and text colours are.

// applyTheme switches to a theme by name. nvim themes not imported yet are
// imported first, which starts nvim, so that runs off the UI thread.
func (m *Model) applyTheme(name string) tea.Cmd {
	if name == "" || name == theme.Terminal {
		if !m.themed {
			return nil
		}
		m.themed, m.termBg, m.termFg = false, nil, nil
		colWash, colChip, colHair = nil, nil, nil
		buildStyles()
		// ask again, so modern's tints follow the terminal's own colours
		return tea.Batch(tea.Raw(theme.Reset()), m.termColors())
	}
	t, err := theme.Load(config.Dir(), name)
	if err == theme.ErrNotImported {
		scheme := strings.TrimPrefix(name, "nvim:")
		return m.run("imported "+scheme+" from nvim", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			t, err := theme.ImportNvim(ctx, config.Dir(), scheme)
			if err != nil {
				return nil, err
			}
			return func(m *Model) tea.Cmd { return m.useTheme(t) }, nil
		})
	}
	if err != nil {
		m.setErr(err.Error())
		return nil
	}
	return m.useTheme(t)
}

func (m *Model) useTheme(t theme.Theme) tea.Cmd {
	m.themed = true
	m.termBg, m.termFg = hexColor(t.Bg), hexColor(t.Fg)
	m.deriveTints()
	return tea.Raw(t.Sequences())
}

// Themed says a theme changed the terminal's colours, so they need resetting on exit.
func (m *Model) Themed() bool { return m.themed }

func hexColor(s string) color.Color {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil {
		return nil
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

// cmdTheme is :theme. No argument opens the picker, "refresh" re-imports the
// current nvim theme, anything else is a theme name to save.
func (m *Model) cmdTheme(arg string) tea.Cmd {
	switch arg = strings.TrimSpace(arg); arg {
	case "":
		return m.themePicker()
	case "refresh":
		cur := m.cfg.String("theme")
		scheme, ok := strings.CutPrefix(cur, "nvim:")
		if !ok {
			m.setErr("refresh is for nvim themes, this one is " + cur)
			return nil
		}
		return m.run("re-imported "+scheme+" from nvim", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			t, err := theme.ImportNvim(ctx, config.Dir(), scheme)
			if err != nil {
				return nil, err
			}
			return func(m *Model) tea.Cmd { return m.useTheme(t) }, nil
		})
	}
	return m.setSetting("theme " + arg)
}

type themeItem struct {
	name     string
	imported bool // false for nvim schemes that need importing first
}

// themePicker lists the terminal's own colours, built-ins, saved files and
// every nvim colorscheme. Moving previews, enter keeps it, esc goes back.
func (m *Model) themePicker() tea.Cmd {
	prev := m.cfg.String("theme")
	saved := map[string]bool{}
	items := []pickItem{{title: theme.Terminal, detail: "your terminal's own colours", value: themeItem{theme.Terminal, true}}}
	for _, n := range theme.BuiltinNames() {
		items = append(items, pickItem{title: n, detail: "built in", value: themeItem{n, true}})
	}
	for _, n := range theme.Saved(config.Dir()) {
		saved[n] = true
		detail := "saved"
		if strings.HasPrefix(n, "nvim:") {
			detail = "from nvim"
		}
		items = append(items, pickItem{title: n, detail: detail, value: themeItem{n, true}})
	}
	m.openPicker("themes · enter keeps · esc goes back", items, func(m *Model, it pickItem) tea.Cmd {
		return m.setSetting("theme " + it.value.(themeItem).name)
	})
	for i, it := range m.pick.items {
		if it.value.(themeItem).name == prev {
			m.pick.cur = i
		}
	}
	preview := func(m *Model) tea.Cmd {
		if len(m.pick.shown) == 0 {
			return nil
		}
		ti := m.pick.items[m.pick.shown[m.pick.cur]].value.(themeItem)
		if !ti.imported {
			return nil // importing takes a moment, so only on enter
		}
		return m.applyTheme(ti.name)
	}
	m.pick.onMove = preview
	m.pick.floating = true
	m.pick.onKey = func(m *Model, key string) (bool, tea.Cmd) {
		if key == "esc" {
			m.closePicker()
			return true, m.applyTheme(prev)
		}
		return false, nil
	}
	m.pick.footer = func(string) string {
		if len(m.pick.shown) > 0 && !m.pick.items[m.pick.shown[m.pick.cur]].value.(themeItem).imported {
			return "enter imports this from nvim (a second or two)"
		}
		return ""
	}
	// nvim's list takes a moment, so it joins the picker when it arrives
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		names, err := theme.NvimColorschemes(ctx)
		if err != nil {
			return nil, nil // no nvim is fine, the rest still works
		}
		return func(m *Model) tea.Cmd {
			if m.mode != modePicker || !strings.HasPrefix(m.pick.title, "themes") {
				return nil
			}
			for _, n := range names {
				if saved["nvim:"+n] {
					continue
				}
				m.pick.items = append(m.pick.items, pickItem{title: "nvim:" + n, detail: "nvim colorscheme", value: themeItem{"nvim:" + n, false}})
			}
			cur := m.pick.cur
			m.refilter()
			m.pick.cur = clamp(cur, 0, len(m.pick.shown)-1)
			return nil
		}, nil
	})
}
