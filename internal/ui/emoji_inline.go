package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Inline emoji while writing: ":jo" suggests matching emoji above the input,
// tab or enter puts one in, and a finished ":joy:" swaps itself. ctrl+e opens
// the full searchable picker instead.

type emojiState struct {
	start int // rune index of the ':' on the cursor's line
	list  [][2]string
	sel   int
}

const maxEmojiSuggestions = 8

// emojiQuery finds a ":word" being typed before the cursor. The colon has to
// start a word, so times like 12:30 and "note:" don't trigger it.
func (m *Model) emojiQuery() (start int, q string, closed bool, ok bool) {
	r, pos := m.cursorLine()
	if pos == 0 {
		return 0, "", false, false
	}
	// a just-typed closing colon, as in ":joy:"
	end := pos
	if r[pos-1] == ':' {
		closed = true
		end = pos - 1
	}
	for i := end - 1; i >= 0; i-- {
		c := r[i]
		switch {
		case c == ':':
			if i > 0 && r[i-1] != ' ' && r[i-1] != '\t' {
				return 0, "", false, false
			}
			q = string(r[i+1 : end])
			if q == "" {
				return 0, "", false, false
			}
			return i, strings.ToLower(q), closed, true
		case c == '_' || c == '-' || c == '+' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9':
		default:
			return 0, "", false, false
		}
	}
	return 0, "", false, false
}

// updateEmoji refreshes the suggestions after each keystroke, and replaces a
// completed ":name:" straight away when it names an emoji.
func (m *Model) updateEmoji() {
	m.emoji = nil
	start, q, closed, ok := m.emojiQuery()
	if !ok {
		return
	}
	words := strings.ReplaceAll(q, "_", " ")
	var list [][2]string
	exact := -1
	for _, e := range emojiTable {
		if matchEmoji(pickItem{title: e[0], detail: e[1]}, words) {
			if exact < 0 && hasWord(e[1], words) {
				exact = len(list)
			}
			list = append(list, e)
			if len(list) == maxEmojiSuggestions {
				break
			}
		}
	}
	if closed {
		if exact >= 0 {
			m.insertEmojiAt(start, list[exact][0])
		}
		return
	}
	if len(list) > 0 {
		m.emoji = &emojiState{start: start, list: list}
	}
}

func hasWord(name, w string) bool {
	for _, f := range strings.Fields(name) {
		if f == w {
			return true
		}
	}
	return false
}

func (m *Model) emojiKey(key string) (bool, tea.Cmd) {
	st := m.emoji
	if st == nil {
		return false, nil
	}
	switch key {
	case "tab", "enter":
		m.insertEmojiAt(st.start, st.list[st.sel][0])
		m.emoji = nil
		return true, nil
	case "down", "ctrl+n", "right":
		st.sel = (st.sel + 1) % len(st.list)
		return true, nil
	case "up", "ctrl+p", "left", "shift+tab":
		st.sel = (st.sel - 1 + len(st.list)) % len(st.list)
		return true, nil
	case "esc":
		m.emoji = nil
		return true, nil
	}
	return false, nil
}

// insertEmojiAt replaces everything from the ':' at start up to the cursor with the emoji.
func (m *Model) insertEmojiAt(start int, e string) {
	_, pos := m.cursorLine()
	for range pos - start {
		m.compose, _ = m.compose.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m.compose.InsertString(e)
}

func (m *Model) renderEmojiSuggestions(w int) string {
	st := m.emoji
	if st == nil {
		return ""
	}
	parts := []string{stAccent.Render(":")}
	for i, e := range st.list {
		name := strings.Fields(e[1])[0]
		cell := e[0] + " " + name
		if i == st.sel {
			parts = append(parts, stTabOn.Render(cell))
		} else {
			parts = append(parts, stTab.Render(cell))
		}
	}
	return truncate(strings.Join(parts, ""), w) + stDim.Render("  tab picks")
}

// openEmojiPicker is ctrl+e: the full searchable picker, inserting at the cursor.
func (m *Model) openEmojiPicker() tea.Cmd {
	items := make([]pickItem, 0, len(m.recentReacts)+len(emojiTable))
	seen := map[string]bool{}
	for _, e := range m.recentReacts {
		items = append(items, pickItem{title: e, detail: "recent " + emojiName(e), value: e})
		seen[e] = true
	}
	for _, e := range emojiTable {
		if !seen[e[0]] {
			items = append(items, pickItem{title: e[0], detail: e[1], value: e[0]})
		}
	}
	m.compose.Blur()
	m.openPicker("insert emoji   type a name (joy, fire, +1)", items, func(m *Model, it pickItem) tea.Cmd {
		e := it.value.(string)
		m.rememberReact(e)
		cmd := m.startInsert()
		m.compose.InsertString(e)
		return cmd
	})
	m.pick.match = matchEmoji
	return nil
}
