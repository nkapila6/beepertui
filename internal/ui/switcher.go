package ui

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// The chat switcher (cc or :chats) is a picker over every chat, across all
// inboxes, filtered as you type. It's the no-flash way to get anywhere.

func (m *Model) openSwitcher() tea.Cmd {
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		// plain list covers every inbox and comes with previews
		p, err := m.c.ListChats(ctx, 200)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			m.showSwitcher(p.Items, "")
			return nil
		}, nil
	})
}

func (m *Model) showSwitcher(chats []beeper.Chat, searched string) {
	items := make([]pickItem, 0, len(chats))
	for _, ch := range chats {
		detail := netShort(ch.Network)
		if ch.UnreadCount > 0 {
			detail += fmt.Sprintf("  %d unread", ch.UnreadCount)
		}
		if ch.IsArchived {
			detail += "  archived"
		}
		if ch.Preview != nil {
			detail += "  " + oneLine(plainText(ch.Preview.Text))
		}
		items = append(items, pickItem{title: oneLine(ch.Title), detail: detail, value: ch})
	}
	title := "go to chat   type to filter, enter opens"
	if searched != "" {
		title = fmt.Sprintf("search results for %q", searched)
	}
	m.openPicker(title, items, func(m *Model, it pickItem) tea.Cmd {
		return m.openChat(it.value.(beeper.Chat), "")
	})
	m.pick.match = func(it pickItem, q string) bool {
		// fuzzy on the name, plain substring on network and preview, or everything matches everything
		return fuzzy(it.title, q) || len(q) > 2 && strings.Contains(strings.ToLower(it.detail), strings.ToLower(q))
	}
	// nothing loaded matches, so ask Beeper to search everything
	m.pick.onRaw = func(m *Model, q string) tea.Cmd {
		q = strings.TrimSpace(q)
		if q == "" {
			return nil
		}
		return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			p, err := m.c.SearchChats(ctx, q, 50)
			if err != nil {
				return nil, err
			}
			return func(m *Model) tea.Cmd {
				if len(p.Items) == 0 {
					m.setErr("no chats match " + q)
					return nil
				}
				m.showSwitcher(p.Items, q)
				return nil
			}, nil
		})
	}
	m.pick.footer = func(q string) string {
		if strings.TrimSpace(q) != "" && len(m.pick.shown) == 0 {
			return "enter searches all of Beeper for \"" + strings.TrimSpace(q) + "\""
		}
		return ""
	}
}

// fuzzy reports whether q's letters appear in s in order, ignoring case and spaces, so "tms" finds "Team Standup".
func fuzzy(s, q string) bool {
	s = strings.ToLower(s)
	i := 0
	rs := []rune(s)
	for _, c := range strings.ToLower(q) {
		if unicode.IsSpace(c) {
			continue
		}
		for i < len(rs) && rs[i] != c {
			i++
		}
		if i == len(rs) {
			return false
		}
		i++
	}
	return true
}
