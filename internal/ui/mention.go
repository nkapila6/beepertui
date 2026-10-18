package ui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// Typing @ in the compose box suggests chat members. Picking one inserts
// "@Name" as plain text and remembers who that is; on send each remembered
// "@Name" becomes a matrix.to markdown link, which Beeper turns into a real
// mention on the network.

type mentionState struct {
	start int // rune index of the @
	list  []beeper.User
	sel   int
}

const maxMentionSuggestions = 6

// mentionQuery finds an @word being typed right before the cursor.
// cursorLine is the text of the line the cursor is on, and the cursor's rune column in it.
func (m *Model) cursorLine() ([]rune, int) {
	lines := strings.Split(m.compose.Value(), "\n")
	row := clamp(m.compose.Line(), 0, len(lines)-1)
	r := []rune(lines[row])
	return r, clamp(m.compose.Column(), 0, len(r))
}

func (m *Model) mentionQuery() (start int, q string, ok bool) {
	r, pos := m.cursorLine()
	for i := pos - 1; i >= 0; i-- {
		switch r[i] {
		case ' ', '\t':
			return 0, "", false
		case '@':
			if i > 0 && r[i-1] != ' ' {
				return 0, "", false // an email address, not a mention
			}
			return i, string(r[i+1 : pos]), true
		}
	}
	return 0, "", false
}

func (m *Model) updateMentions() {
	m.mention = nil
	if m.cur == nil {
		return
	}
	start, q, ok := m.mentionQuery()
	if !ok {
		m.mentionDismissed = -1
		return
	}
	if start == m.mentionDismissed {
		return
	}
	q = strings.ToLower(q)
	type cand struct {
		u     beeper.User
		score int
	}
	var cs []cand
	for _, u := range m.cur.Participants.Items {
		if u.IsSelf || m.selfIDs[u.ID] {
			continue
		}
		name := strings.ToLower(u.Name())
		score := -1
		switch {
		case q == "" || strings.HasPrefix(name, q):
			score = 0
		case strings.Contains(" "+name, " "+q):
			score = 1 // starts a later word, like a surname
		case strings.Contains(name, q) || strings.HasPrefix(strings.ToLower(u.Username), q):
			score = 2
		}
		if score >= 0 {
			cs = append(cs, cand{u, score})
		}
	}
	if len(cs) == 0 {
		return
	}
	sort.SliceStable(cs, func(a, b int) bool { return cs[a].score < cs[b].score })
	st := &mentionState{start: start}
	for _, c := range cs[:min(len(cs), maxMentionSuggestions)] {
		st.list = append(st.list, c.u)
	}
	m.mention = st
}

// mentionKey handles keys while suggestions are showing. It reports whether it used the key.
func (m *Model) mentionKey(key string) (bool, tea.Cmd) {
	st := m.mention
	if st == nil {
		return false, nil
	}
	switch key {
	case "tab", "enter":
		m.acceptMention(st.list[st.sel])
		return true, nil
	case "down", "ctrl+n", "ctrl+j":
		st.sel = (st.sel + 1) % len(st.list)
		return true, nil
	case "up", "ctrl+p", "ctrl+k", "shift+tab":
		st.sel = (st.sel - 1 + len(st.list)) % len(st.list)
		return true, nil
	case "esc":
		// hide suggestions for this @ but stay in insert mode
		m.mentionDismissed = st.start
		m.mention = nil
		return true, nil
	}
	return false, nil
}

func (m *Model) acceptMention(u beeper.User) {
	_, pos := m.cursorLine()
	tag := "@" + u.Name()
	// delete the typed "@query" and type the full name in its place, which
	// keeps the cursor right where it should be in a multi-line draft
	for range pos - m.mention.start {
		m.compose, _ = m.compose.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m.compose.InsertString(tag + " ")
	if m.mentioned == nil {
		m.mentioned = map[string]string{}
	}
	m.mentioned[tag] = u.ID
	m.mention = nil
}

// withMentionLinks swaps every picked "@Name" for a markdown mention link.
// Longest names first, so "@Ann Lee" wins over "@Ann".
func (m *Model) withMentionLinks(text string) string {
	tags := make([]string, 0, len(m.mentioned))
	for t := range m.mentioned {
		tags = append(tags, t)
	}
	sort.Slice(tags, func(a, b int) bool { return len(tags[a]) > len(tags[b]) })
	for _, t := range tags {
		if !strings.Contains(text, t) {
			continue
		}
		label := strings.NewReplacer("[", `\[`, "]", `\]`).Replace(strings.TrimPrefix(t, "@"))
		text = strings.ReplaceAll(text, t, "["+label+"](https://matrix.to/#/"+m.mentioned[t]+")")
	}
	return text
}

func (m *Model) renderMentions(w int) string {
	st := m.mention
	if st == nil {
		return ""
	}
	parts := []string{stAccent.Render("@")}
	for i, u := range st.list {
		name := truncate(u.Name(), 24)
		if i == st.sel {
			parts = append(parts, stTabOn.Render(name))
		} else {
			parts = append(parts, stTab.Render(name))
		}
	}
	return truncate(strings.Join(parts, ""), w) + stDim.Render("  tab picks")
}
