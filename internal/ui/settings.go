package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nkapila6/beepertui/internal/config"
)

// applySetting pushes one resolved config value into the running app.
func (m *Model) applySetting(key string) {
	c := m.cfg
	if strings.HasPrefix(key, config.KeyPrefix) {
		m.km = buildKeymap(c)
		return
	}
	switch key {
	case "nerd_font":
		m.nerdFont = c.Bool(key)
	case "images":
		m.imageMode = detectImageMode(c.String(key))
	case "image_size":
		setImageSize(c.String(key))
		for _, t := range m.thumbs {
			t.lines, t.placedCols, t.placedRows = nil, 0, 0
		}
	case "avatars":
		m.showAvatars = c.Bool(key)
	case "compact":
		m.compact = c.Bool(key)
	case "mouse":
		m.mouse = c.Bool(key)
	case "notifications":
		m.notifications = c.Bool(key)
	case "vim_compose":
		m.vimCompose = c.Bool(key)
	case "notification_preview":
		m.notifPreview = c.Bool(key)
	case "chat_width":
		m.chatPct = c.Int(key)
	case "inbox":
		for i, n := range inboxes {
			if n == c.String(key) {
				m.inbox = i
			}
		}
	case "accent":
		if v, err := config.ParseColor(c.String(key)); err == nil {
			colAccent = lipgloss.Color(v)
			buildStyles()
		}
	}
}

// setSetting is :set key value. It applies live and saves to the config file.
func (m *Model) setSetting(arg string) tea.Cmd {
	key, val, _ := strings.Cut(strings.TrimSpace(arg), " ")
	val = strings.TrimSpace(val)
	if key == "" {
		m.settingsPicker()
		return nil
	}
	if val == "" {
		m.promptSetting(key)
		return nil
	}
	if err := m.cfg.Set(key, val); err != nil {
		m.setErr(err.Error())
		return nil
	}
	if err := m.cfg.Save(); err != nil {
		m.setErr("applied, but couldn't save: " + err.Error())
	}
	m.applySetting(key)
	m.fixChatScroll()
	m.fixMsgScroll()
	note := ""
	if _, src := m.cfg.Value(key); strings.HasPrefix(src, "env") {
		note = fmt.Sprintf(" (but %s overrides it)", src)
	}
	m.setStatus(fmt.Sprintf("%s = %s, saved to %s%s", key, val, config.Path(), note))
	return tea.Batch(m.loadThumbs(), m.loadAvatars(), m.placeImages(), m.loadChats())
}

// settingsPicker lists every setting with its value; enter asks for a new one.
func (m *Model) settingsPicker() {
	items := make([]pickItem, len(config.Settings))
	for i, s := range config.Settings {
		v, src := m.cfg.Value(s.Key)
		items[i] = pickItem{title: s.Key, detail: v + "  " + src + "  " + s.Help, value: s.Key}
	}
	m.openPicker("settings   enter to change, saved to "+config.Path(), items, func(m *Model, it pickItem) tea.Cmd {
		m.promptSetting(it.value.(string))
		return m.line.Focus()
	})
}

func (m *Model) promptSetting(key string) {
	cur, _ := m.cfg.Value(key)
	kind := ""
	for _, s := range config.Settings {
		if s.Key == key && len(s.Choices) > 0 {
			kind = " (" + strings.Join(s.Choices, "|") + ")"
		}
	}
	m.prompt(key+kind, cur, func(m *Model, v string) tea.Cmd { return m.setSetting(key + " " + v) })
}

// toggleSetting flips a bool setting (or sets it to on/off) and saves it.
func (m *Model) toggleSetting(key, arg string) tea.Cmd {
	v := !m.cfg.Bool(key)
	switch arg {
	case "on", "true":
		v = true
	case "off", "false":
		v = false
	}
	return m.setSetting(fmt.Sprintf("%s %t", key, v))
}
