package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// While typing a : command, a panel above the command line lists what
// matches, with arguments and a description, so nobody has to remember them.

const paletteRows = 12

// paletteMatches is what the panel shows for the current input: prefix
// matches first, then commands whose name or description contain the text.
// Once arguments are being typed it narrows to that one command.
func paletteMatches(input string) []command {
	name, _, hasArgs := strings.Cut(strings.TrimLeft(input, " "), " ")
	name = strings.ToLower(name)
	var prefix, other []command
	for _, c := range commands {
		switch {
		case hasArgs:
			if c.name == name {
				return []command{c}
			}
		case strings.HasPrefix(c.name, name):
			prefix = append(prefix, c)
		case name != "" && (strings.Contains(c.name, name) || len(name) >= 3 && helpWordPrefix(c.help, name)):
			other = append(other, c)
		}
	}
	if hasArgs {
		return nil
	}
	return append(prefix, other...)
}

// paletteKey handles keys that drive the panel. It reports whether it used the key.
func (m *Model) paletteKey(key string) (bool, tea.Cmd) {
	matches := paletteMatches(m.line.Value())
	switch key {
	case "down", "ctrl+n", "ctrl+j":
		if len(matches) > 0 {
			m.paletteSel = (m.paletteSel + 1) % len(matches)
		}
		return true, nil
	case "up", "ctrl+p", "ctrl+k":
		if len(matches) > 0 {
			m.paletteSel = (m.paletteSel - 1 + len(matches)) % len(matches)
		}
		return true, nil
	case "tab":
		if c, ok := pick(matches, m.paletteSel); ok && !strings.Contains(m.line.Value(), " ") {
			m.line.SetValue(c.name + " ")
			m.line.CursorEnd()
			m.paletteSel = 0
		}
		return true, nil
	case "enter":
		// a half-typed name runs whatever is highlighted
		v := strings.TrimSpace(m.line.Value())
		if c, ok := pick(matches, m.paletteSel); ok && !strings.Contains(v, " ") && !isCommand(v) {
			m.closeLine()
			return true, m.runCommand(c.name)
		}
	}
	return false, nil
}

func pick(cs []command, i int) (command, bool) {
	if i < 0 || i >= len(cs) {
		return command{}, false
	}
	return cs[i], true
}

func isCommand(name string) bool {
	for _, c := range commands {
		if c.name == name {
			return true
		}
	}
	return name == "quit"
}

func (m *Model) paletteLayers() []*lipgloss.Layer {
	if m.mode != modeLine || m.lineKind != ":" {
		return nil
	}
	matches := paletteMatches(m.line.Value())
	if len(matches) == 0 {
		return nil
	}
	sel := clamp(m.paletteSel, 0, len(matches)-1)
	start := clamp(sel-paletteRows+1, 0, max(0, len(matches)-paletteRows))
	end := min(len(matches), start+paletteRows)

	nameW := 0
	for _, c := range matches {
		nameW = max(nameW, lipgloss.Width(":"+c.name+" "+c.args))
	}
	nameW = min(nameW, 36)
	width := min(m.w-2, 100)
	helpW := max(10, width-nameW-6)

	var rows []string
	for i := start; i < end; i++ {
		c := matches[i]
		left := padRight(truncate(":"+c.name+" "+c.args, nameW), nameW)
		help := truncate(c.help, helpW)
		if i == sel {
			rows = append(rows, stTabOn.Render(left+"  "+padRight(help, helpW)))
			continue
		}
		name := stAccent.Render(":" + c.name)
		args := stDim.Render(strings.TrimPrefix(left, ":"+c.name))
		rows = append(rows, " "+name+args+"  "+stText.Render(help))
	}
	title := stBold.Render("commands") + stDim.Render(fmt.Sprintf("  %d  ↑↓ move  tab complete  enter run", len(matches)))
	if len(matches) > paletteRows {
		title += stDim.Render(fmt.Sprintf("  (%d-%d)", start+1, end))
	}
	box := stOverlay.Render(title + "\n" + strings.Join(rows, "\n"))
	y := max(0, m.h-1-lipgloss.Height(box))
	return []*lipgloss.Layer{lipgloss.NewLayer(box).X(0).Y(y).Z(4)}
}

// helpWordPrefix matches the start of a word in a description, so "rem" finds
// the reminder command but "re" doesn't match "direct".
func helpWordPrefix(help, q string) bool {
	for _, w := range strings.FieldsFunc(strings.ToLower(help), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if strings.HasPrefix(w, q) {
			return true
		}
	}
	return false
}
