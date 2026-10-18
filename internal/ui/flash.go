package ui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// home row first, like flash.nvim
const labelChars = "asdfghjklqwertyuiopzxcvbnm"

type target struct {
	pane pane
	idx  int
}

type flashState struct {
	action rune
	pane   pane // pane the action came from, for "yy" style repeats
	labels map[target]string
	typed  string
}

func (f flashState) label(p pane, i int) (string, bool) {
	l, ok := f.labels[target{p, i}]
	return l, ok
}

var flashVerbs = map[rune]string{
	's': "jump", 'c': "chat", 'C': "chat action", 'y': "yank", 'r': "reply", 'R': "react", 'e': "edit", 'x': "delete", 'o': "open", 'U': "unreact", 'v': "view", 'd': "download",
}

func (m *Model) startFlash(action rune) {
	near := func(p pane, first, last, cursor int) []target {
		var ts []target
		for i := first; i <= last; i++ {
			ts = append(ts, target{p, i})
		}
		// nearest to the cursor gets the easiest label
		sort.SliceStable(ts, func(a, b int) bool { return absInt(ts[a].idx-cursor) < absInt(ts[b].idx-cursor) })
		return ts
	}
	var chats, msgs []target
	if first, last := m.chatRange(); len(m.chats) > 0 {
		chats = near(paneChats, first, last, m.chatCur)
	}
	if first, last := m.msgRange(); m.cur != nil && len(m.msgs) > 0 {
		msgs = near(paneMsgs, first, last, m.msgCur)
	}

	// s jumps anywhere, so it labels both panes, focused pane first.
	// Everything else acts on messages only.
	var ts []target
	switch {
	case action == 'c' || action == 'C':
		ts = chats
	case action != 's':
		ts = msgs
	case m.focus == paneChats:
		ts = append(chats, msgs...)
	default:
		ts = append(msgs, chats...)
	}
	if len(ts) == 0 {
		m.setErr("nothing on screen")
		return
	}

	// the action key itself is free so "yy" can mean "this one"
	chars := labelChars
	if action != 's' {
		chars = strings.ReplaceAll(chars, string(action), "")
	}
	labels := makeLabels(len(ts), chars)
	m.flash = flashState{action: action, pane: m.focus, labels: map[target]string{}}
	for i, t := range ts {
		m.flash.labels[t] = labels[i]
	}
	m.mode = modeFlash
}

// makeLabels hands out single keys where it can and two-key labels for the
// rest. Two-key labels start with letters no single label uses, so nothing is
// ever a prefix of something else.
func makeLabels(n int, chars string) []string {
	k := len(chars)
	if n <= k {
		return strings.Split(chars[:n], "")
	}
	prefixes := 1
	for (k-prefixes)+prefixes*k < n && prefixes < k {
		prefixes++
	}
	singles := k - prefixes
	out := strings.Split(chars[:singles], "")
	for _, p := range chars[singles:] {
		for _, c := range chars {
			if len(out) == n {
				return out
			}
			out = append(out, string(p)+string(c))
		}
	}
	return out
}

func (m *Model) flashKey(key string) tea.Cmd {
	if key == "esc" {
		m.mode = modeNormal
		return nil
	}
	f := &m.flash
	// repeating the action key acts on the cursor, vim style (yy, rr, ...)
	if f.typed == "" && key == string(f.action) && f.action != 's' {
		m.mode = modeNormal
		if f.action == 'C' {
			// the open chat when reading it, the cursor chat in the list
			if ch := m.targetChat(); ch != nil {
				m.chatMenu(*ch)
			}
			return nil
		}
		if f.action == 'c' {
			return m.flashDo(target{paneChats, m.chatCur})
		}
		return m.flashDo(target{paneMsgs, m.msgCur})
	}
	if len(key) != 1 {
		m.mode = modeNormal
		return nil
	}
	f.typed += key
	partial := false
	for t, l := range f.labels {
		if l == f.typed {
			m.mode = modeNormal
			return m.flashDo(t)
		}
		if strings.HasPrefix(l, f.typed) {
			partial = true
		}
	}
	if !partial {
		m.mode = modeNormal
		m.setErr("no label " + f.typed)
	}
	return nil
}

func (m *Model) flashDo(t target) tea.Cmd {
	i := t.idx
	if t.pane == paneChats {
		if i >= len(m.chats) {
			return nil
		}
		if m.flash.action == 'C' {
			m.chatMenu(m.chats[i])
			return nil
		}
		m.chatCur = i
		m.fixChatScroll()
		return m.openChat(m.chats[i], "")
	}
	if i >= len(m.msgs) {
		return nil
	}
	m.focus = paneMsgs
	m.msgCur = i
	m.fixMsgScroll()
	msg := m.msgs[i]
	switch m.flash.action {
	case 'y':
		return m.yank(msg)
	case 'r':
		return m.reply(msg)
	case 'R':
		return m.react(msg)
	case 'e':
		return m.edit(msg)
	case 'x':
		return m.del(msg)
	case 'o':
		return m.open(msg)
	case 'U':
		return m.unreact(msg)
	case 'v':
		return m.viewImage(i)
	case 'd':
		return m.downloadMsg(msg)
	}
	return nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
