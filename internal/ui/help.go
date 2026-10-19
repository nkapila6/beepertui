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

func helpSections(km keymap) []helpSection {
	cmds := make([][]string, 0, len(commands))
	for _, c := range commands {
		cmds = append(cmds, []string{strings.TrimSpace(":" + c.name + " " + c.args), c.help})
	}
	rows := func(sec string) [][]string {
		var out [][]string
		for _, d := range keyDefs {
			if d.Section == sec {
				out = append(out, []string{km.label(d.Action), d.Help})
			}
		}
		return out
	}
	anywhere := append(rows(secAnywhere),
		[]string{"C-c", "quit (elsewhere cancels)"},
		[]string{"esc", "drop reply / edit / attach, clear a filter"},
		[]string{":keys", "change any of these keys"},
	)
	return []helpSection{
		{"Anywhere", -1, anywhere},
		{"Chat list (left)", int(paneChats), append(rows(secChats), rows(secChat)...)},
		{"Messages (right)", int(paneMsgs), append(rows(secMsgs), []string{km.label("pin") + " ...", "chat keys act on the open chat"})},
		{"Flash labels", -1, [][]string{
			{"<label>", "act on that item"},
			{"same key again", "act on the cursor item (yy, rr, cc ...)"},
			{"esc", "cancel"},
		}},
		{"Writing", -1, append(rows(secWriting),
			[]string{"esc", "back to normal, text kept"},
			[]string{"C-c", "clear the message (again to leave)"},
			[]string{"S-enter A-enter C-j", "new line"},
			[]string{"@name", "mention someone (tab picks, C-n/C-p move)"},
			[]string{":joy", "inline emoji (tab picks, :joy: swaps itself)"},
			[]string{"drag a file in", "attaches it"},
			[]string{":attach <path>", "send a file with the next message"},
		)},
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
		{"Mouse", -1, [][]string{
			{"click chat", "open it"},
			{"click message", "select (again on an image: view)"},
			{"click tab", "switch inbox"},
			{"click compose", "start writing"},
			{"click a link", "open it in the browser"},
			{"right click", "menu for that chat or message"},
			{"wheel", "scroll chats / messages / images"},
			{":mouse off", "give selection back to the terminal"},
		}},
		{"Looks", -1, [][]string{
			{":icons", "nerd font icons (BEEPERTUI_NERDFONT=1)"},
			{":avatars", "chat avatars"},
			{":compact", "no gap between senders"},
			{":images  :mouse", "inline images, mouse capture"},
		}},
		{"Media picker", -1, [][]string{
			{"tab  S-tab", "next / previous source"},
			{"type", "search the current source"},
			{"↑ ↓", "pick a result"},
			{"enter", "send (memes and cats ask for a caption)"},
			{":media :gif :meme", "open on All / GIFs / Memes"},
			{":nekos :emoji :cataas", "Reactions / Stickers / Cats"},
			{":xkcd :status :memeapi", "xkcd / Status / Reddit"},
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
	for _, s := range helpSections(m.km) {
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
	k := m.km.first
	switch {
	case m.mode == modeInsert:
		return []string{k("send") + " send  " + k("attach_file") + " file", k("paste_image") + " paste  " + k("media_picker") + " gif", "esc normal mode"}
	case m.focus == paneMsgs:
		return []string{k("yank") + " yank  " + k("reply") + " reply  " + k("react") + " react", k("edit") + " edit  " + k("delete") + " del  " + k("open") + " open", k("chats") + " chats  " + k("chat_action") + " chat menu  " + k("help")}
	default:
		return []string{k("open_chat") + " open  " + k("jump") + "/" + k("chats") + " jump", k("chat_action") + " menu  " + k("pin") + " pin  " + k("mute") + " mute", k("search") + " search  " + k("help") + " all keys"}
	}
}
