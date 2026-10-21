package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type pickItem struct {
	title  string
	detail string
	value  any
}

type picker struct {
	title  string
	items  []pickItem
	shown  []int
	cur    int
	onPick func(*Model, pickItem) tea.Cmd
	match  func(it pickItem, q string) bool     // nil means substring
	onKey  func(*Model, string) (bool, tea.Cmd) // runs before normal handling
	onRaw  func(*Model, string) tea.Cmd         // enter with no matches
	onMove func(*Model) tea.Cmd                 // after the cursor or filter changes
	// floating draws a small box over the normal screen instead of taking
	// the body over, for choices you want to see applied (themes)
	floating bool
	footer   func(q string) string
}

// openPicker shows a filterable list. Typing filters, arrows or ctrl+j/k move, enter picks.
func (m *Model) openPicker(title string, items []pickItem, onPick func(*Model, pickItem) tea.Cmd) {
	m.pick = picker{title: title, items: items, onPick: onPick}
	m.line.Prompt = "filter> "
	m.line.SetValue("")
	m.line.Focus()
	m.mode = modePicker
	m.refilter()
}

func (m *Model) refilter() {
	q := strings.ToLower(m.line.Value())
	m.pick.shown = m.pick.shown[:0]
	for i, it := range m.pick.items {
		if q == "" || (m.pick.match != nil && m.pick.match(it, q)) ||
			(m.pick.match == nil && strings.Contains(strings.ToLower(it.title+" "+it.detail), q)) {
			m.pick.shown = append(m.pick.shown, i)
		}
	}
	m.pick.cur = 0
}

func (m *Model) closePicker() {
	m.mode = modeNormal
	m.line.Blur()
}

func (m *Model) pickerKey(k tea.KeyPressMsg) tea.Cmd {
	if m.pick.onKey != nil {
		if handled, cmd := m.pick.onKey(m, k.String()); handled {
			return cmd
		}
	}
	switch k.String() {
	case "esc":
		m.closePicker()
		return nil
	case "up", "ctrl+k", "ctrl+p":
		m.pick.cur = clamp(m.pick.cur-1, 0, len(m.pick.shown)-1)
		return m.pickMoved()
	case "down", "ctrl+j", "ctrl+n":
		m.pick.cur = clamp(m.pick.cur+1, 0, len(m.pick.shown)-1)
		return m.pickMoved()
	case "enter":
		m.closePicker()
		if len(m.pick.shown) == 0 && m.pick.onRaw != nil {
			return m.pick.onRaw(m, m.line.Value())
		}
		if m.pick.onPick == nil || len(m.pick.shown) == 0 {
			return nil
		}
		return m.pick.onPick(m, m.pick.items[m.pick.shown[m.pick.cur]])
	}
	var cmd tea.Cmd
	m.line, cmd = m.line.Update(k)
	m.refilter()
	return tea.Batch(cmd, m.pickMoved())
}

func (m *Model) pickMoved() tea.Cmd {
	if m.pick.onMove == nil {
		return nil
	}
	return m.pick.onMove(m)
}
