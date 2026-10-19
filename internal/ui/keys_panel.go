package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/config"
)

// :keys lists every remappable action. enter waits for the next key press
// and binds it, ctrl+a adds it as an extra key, ctrl+x goes back to the
// default. Everything is written to config.toml as key.<action>.

type keyCapture struct {
	action string
	add    bool   // keep the old keys too
	swap   string // key waiting on a "take it from X?" answer
	from   string // the action that has it
}

func (m *Model) keysPanel() {
	m.openPicker("keys   enter rebinds · C-a adds a key · C-x resets · saved to "+config.Path(), m.keyItems(), nil)
	m.capture, m.keysNote = keyCapture{}, ""
	m.pick.onKey = func(m *Model, key string) (bool, tea.Cmd) { return m.keysPanelKey(key) }
	m.pick.footer = func(string) string {
		c := m.capture
		switch {
		case c.swap != "":
			return fmt.Sprintf("%s is already %s. y takes it, anything else cancels", shortKey(c.swap), c.from)
		case c.action != "" && c.add:
			return "press a key to add to " + c.action + " (esc cancels)"
		case c.action != "":
			return "press the new key for " + c.action + " (esc cancels)"
		}
		return m.keysNote
	}
}

func (m *Model) keyItems() []pickItem {
	items := make([]pickItem, len(keyDefs))
	for i, d := range keyDefs {
		items[i] = pickItem{title: d.Action, detail: padRight(m.km.label(d.Action), 16) + "  " + d.Help, value: d.Action}
	}
	return items
}

func (m *Model) keysPanelKey(key string) (bool, tea.Cmd) {
	c := &m.capture
	if c.swap != "" {
		if key == "y" {
			m.setBind(c.from, without(m.km.keys[c.from], c.swap))
			m.bindCaptured(c.swap)
		}
		*c = keyCapture{}
		return true, nil
	}
	if c.action != "" {
		if key == "esc" {
			*c = keyCapture{}
			return true, nil
		}
		if key == "ctrl+c" {
			m.setErr("ctrl+c can't be rebound")
			*c = keyCapture{}
			return true, nil
		}
		if other := m.km.conflict(c.action, key); other != "" {
			c.swap, c.from = key, other
			return true, nil
		}
		m.bindCaptured(key)
		*c = keyCapture{}
		return true, nil
	}

	cur := ""
	if len(m.pick.shown) > 0 {
		cur = m.pick.items[m.pick.shown[m.pick.cur]].value.(string)
	}
	switch key {
	case "enter":
		if cur != "" {
			*c = keyCapture{action: cur}
		}
		return true, nil
	case "ctrl+a":
		if cur != "" {
			*c = keyCapture{action: cur, add: true}
		}
		return true, nil
	case "ctrl+x":
		if cur != "" {
			m.cfg.Unset(config.KeyPrefix + cur)
			m.km = buildKeymap(m.cfg)
			m.saveBinds(cur + " back to " + m.km.label(cur))
		}
		return true, nil
	}
	return false, nil
}

// bindCaptured gives key to the action being captured.
func (m *Model) bindCaptured(key string) {
	a := m.capture.action
	keys := []string{key}
	if m.capture.add {
		keys = append(without(m.km.keys[a], key), key)
	}
	m.setBind(a, keys)
	m.saveBinds(a + " = " + m.km.label(a))
}

// setBind stores keys for an action, dropping the line when it matches the default.
func (m *Model) setBind(action string, keys []string) {
	v := strings.Join(keys, ", ")
	for _, d := range keyDefs {
		if d.Action == action && strings.Join(config.SplitKeys(d.Keys), ", ") == v {
			m.cfg.Unset(config.KeyPrefix + action)
			m.km = buildKeymap(m.cfg)
			return
		}
	}
	if err := m.cfg.Set(config.KeyPrefix+action, v); err != nil {
		m.setErr(err.Error())
		return
	}
	m.km = buildKeymap(m.cfg)
}

func (m *Model) saveBinds(status string) {
	m.km = buildKeymap(m.cfg)
	m.keysNote = status + ", saved"
	if err := m.cfg.Save(); err != nil {
		m.keysNote = "changed, but couldn't save: " + err.Error()
	}
	// refresh the rows in place, keeping the filter and cursor
	cur := m.pick.cur
	m.pick.items = m.keyItems()
	m.refilter()
	m.pick.cur = clamp(cur, 0, len(m.pick.shown)-1)
}

func without(keys []string, k string) []string {
	var out []string
	for _, x := range keys {
		if x != k {
			out = append(out, x)
		}
	}
	return out
}
