package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

type helpSection struct {
	title string
	pane  int // -1 for anywhere, otherwise the pane it's about
	rows  [][]string
}

func helpSections() []helpSection {
	cmds := make([][]string, 0, len(commands))
	for _, c := range commands {
		cmds = append(cmds, []string{strings.TrimSpace(":" + c.name + " " + c.args), c.help})
	}
	return []helpSection{
		{"Anywhere", -1, [][]string{
			{"tab  h  l", "switch pane"},
			{"/", "search chats (esc clears)"},
			{"[  ]", "prev / next inbox"},
			{"c", "flash: switch to any chat"},
			{"C", "flash: pick a chat, then act on it"},
			{"n  N", "new DM / new group"},
			{":", "command line"},
			{"C-r", "refresh"},
			{"?", "this help"},
			{"q  C-c", "quit (C-c elsewhere cancels)"},
		}},
		{"Chat list (left)", int(paneChats), [][]string{
			{"j  k", "move"},
			{"gg  G", "top / bottom"},
			{"C-d  C-u", "half page"},
			{"enter", "open chat"},
			{"s", "flash jump to a chat"},
			{"p", "pin / unpin"},
			{"m", "mute / unmute"},
			{"A", "archive / unarchive"},
			{"L", "low priority / primary"},
			{"u", "mark read / unread"},
			{"!", "set a reminder"},
			{"O", "open in Beeper Desktop"},
		}},
		{"Messages (right)", int(paneMsgs), [][]string{
			{"j  k", "move (k at top loads older)"},
			{"gg  G", "oldest / newest"},
			{"i  a  enter", "write a message"},
			{"s", "flash jump to a message"},
			{"y", "yank text to clipboard"},
			{"r", "reply"},
			{"R", "react"},
			{"e", "edit (yours)"},
			{"x", "delete (yours, asks first)"},
			{"o", "open link or attachment"},
			{"enter on image", "image viewer"},
			{"v", "view an image big"},
			{"d", "download attachment to ~/Downloads"},
			{"U", "remove your reaction"},
			{"O", "show message in Beeper"},
			{"p m A L u !", "act on the open chat"},
		}},
		{"Flash labels", -1, [][]string{
			{"s", "everything on screen, both panes"},
			{"c  C", "chats only (switch / action menu)"},
			{"y r R e x o U v d", "messages only"},
			{"<label>", "act on that message"},
			{"yy rr CC ...", "act on the cursor item"},
			{"esc", "cancel"},
		}},
		{"Writing", -1, [][]string{
			{"enter", "send (stays in insert)"},
			{"esc", "back to normal, text kept"},
			{"C-c", "clear the message (again to leave)"},
			{"C-v", "attach clipboard image or copied file"},
			{"drag a file in", "attaches it"},
			{":attach <path>", "send a file with the next message"},
			{"esc in normal", "drop reply / edit / attach"},
		}},
		{"Reacting", -1, [][]string{
			{"1-6", "quick reaction"},
			{"type", "search by name: fire, tada, +1"},
			{":eyes:", "colons work too"},
			{"paste + enter", "any emoji as is"},
			{"alt+enter", "send what you typed as a text reaction"},
			{"no match + enter", "same, e.g. \"lol\" or \"same\""},
			{"up  down", "pick from the list"},
		}},
		{"Image viewer", -1, [][]string{
			{"d", "download to ~/Downloads"},
			{"o", "open in default app"},
			{"n  p", "next / previous image in chat"},
			{"esc  q", "close"},
		}},
		{"Lists and prompts", -1, [][]string{
			{"type", "filter"},
			{"up down C-j C-k", "move"},
			{"enter", "pick"},
			{"tab", "complete a :command"},
			{"esc", "cancel"},
		}},
		{"Commands", -1, cmds},
	}
}

// helpLines renders all sections in as many columns as fit, packed shortest-column-first.
func (m *Model) helpLines() []string {
	gap := 1
	cols := clamp((m.w-2)/(40+gap), 1, 3)
	boxW := (m.w-2)/cols - gap

	colBlocks := make([][]string, cols)
	colH := make([]int, cols)
	for _, s := range helpSections() {
		border := stDim
		titleSt := stAccent.Bold(true)
		if s.pane >= 0 && pane(s.pane) == m.focus {
			border = stAccent
			titleSt = stModeAlt
		}
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(border).
			BorderColumn(false).
			Width(boxW).
			Wrap(true).
			Rows(s.rows...).
			StyleFunc(func(row, col int) lipgloss.Style {
				if col == 0 {
					return stOther.Padding(0, 1)
				}
				return stText.Padding(0, 1)
			})
		title := titleSt.Render(" " + s.title + " ")
		if s.pane >= 0 && pane(s.pane) == m.focus {
			title += stDim.Render("  (you are here)")
		}
		block := title + "\n" + t.Render()
		c := 0
		for i := range colH {
			if colH[i] < colH[c] {
				c = i
			}
		}
		colBlocks[c] = append(colBlocks[c], block)
		colH[c] += lipgloss.Height(block)
	}

	rendered := make([]string, 0, cols*2)
	for i, blocks := range colBlocks {
		if i > 0 {
			rendered = append(rendered, strings.Repeat(" ", gap))
		}
		rendered = append(rendered, lipgloss.NewStyle().Width(boxW).Render(strings.Join(blocks, "\n")))
	}
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, rendered...), "\n")
}

func (m *Model) helpHeight() int { return m.bodyHeight() + 2 - 1 }

func (m *Model) renderHelp() string {
	lines := m.helpLines()
	h := m.helpHeight()
	off := clamp(m.helpOff, 0, max(0, len(lines)-h))
	end := min(len(lines), off+h)
	hdr := stBold.Render(" keys") + stDim.Render("   j/k scroll, esc closes")
	if len(lines) > h {
		hdr += stDim.Render(fmt.Sprintf("   (%d-%d of %d)", off+1, end, len(lines)))
	}
	out := append([]string{hdr}, lines[off:end]...)
	for len(out) < h+1 {
		out = append(out, "")
	}
	return " " + strings.Join(out, "\n ")
}

func (m *Model) helpKey(key string) tea.Cmd {
	maxOff := max(0, len(m.helpLines())-m.helpHeight())
	switch key {
	case "j", "down":
		m.helpOff++
	case "k", "up":
		m.helpOff--
	case "ctrl+d", "space":
		m.helpOff += m.helpHeight() / 2
	case "ctrl+u":
		m.helpOff -= m.helpHeight() / 2
	case "g":
		m.helpOff = 0
	case "G":
		m.helpOff = maxOff
	case "esc", "q", "?", "enter":
		m.mode = modeNormal
	}
	m.helpOff = clamp(m.helpOff, 0, maxOff)
	return nil
}

// paneHints is the cheat sheet under the chat list, for whatever is focused right now.
func (m *Model) paneHints() []string {
	switch {
	case m.mode == modeInsert:
		return []string{"enter  send", "esc    normal mode", "C-v    paste image"}
	case m.focus == paneMsgs:
		return []string{"y yank  r reply  R react", "e edit  x del  o open", "c chats  C chat menu  ?"}
	default:
		return []string{"enter open  s/c jump", "C menu  p pin  m mute", "/ search  ? all keys"}
	}
}
