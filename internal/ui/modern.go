package ui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// The modern layout (layout = modern). Same rows and columns as classic, so
// the mouse, flash labels, link hints and images line up without knowing
// which one is on; only the drawing differs:
//
//	no boxes     panes are plain areas, a single hairline between them
//	quiet text   one accent, everything secondary dim
//	soft bands   selection is a faint wash behind the row plus an accent edge
//	chips        initials beside each sender, counts and keys as small tints
//
// The wash, hairline and chip shades sit between the terminal's own
// background and text, which it reports when asked (OSC 10/11). That keeps
// it following any theme or tuios look; without an answer it falls back to
// the 16 ANSI colours.

// tints, nil until the terminal answers
var colWash, colHair, colChip color.Color

// modernOn mirrors the layout setting for buildStyles, which has no model.
var modernOn bool

func (m *Model) termColors() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, tea.RequestForegroundColor)
}

// deriveTints mixes the shades once both terminal colours are known.
func (m *Model) deriveTints() {
	if m.termBg == nil {
		return
	}
	fg := m.termFg
	if fg == nil {
		fg = color.White
		if !isDark(m.termBg) {
			fg = color.Black
		}
	}
	colWash = mixColor(m.termBg, fg, 0.07)
	colChip = mixColor(m.termBg, fg, 0.14)
	colHair = mixColor(m.termBg, fg, 0.20)
	buildStyles()
}

func mixColor(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	mix := func(x, y uint32) uint8 { return uint8((float64(x)*(1-t) + float64(y)*t) / 257) }
	return color.RGBA{mix(ar, br), mix(ag, bg), mix(ab, bb), 255}
}

func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return (0.299*float64(r)+0.587*float64(g)+0.114*float64(b))/65535 < 0.5
}

func hair() color.Color {
	if colHair != nil {
		return colHair
	}
	return colMuted
}

// withBg paints bg behind already-styled text, padded to w cells. Every
// reset in the text would drop the background, so it goes back on after each.
func withBg(s string, w int, bg color.Color) string {
	if bg == nil {
		return s
	}
	on := strings.TrimSuffix(lipgloss.NewStyle().Background(bg).Render(" "), " \x1b[m")
	s = on + strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+on)
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s + "\x1b[m"
}

// modernPane draws a pane without a box: a title row, the content, and a
// spare bottom row, inside the same w x h+2 cells titledPane uses. The left
// column of the message pane is the hairline between the two.
func (m *Model) modernPane(p pane, w, h int, title, content string) string {
	inner := max(1, w-2)
	body := strings.Split(lipgloss.NewStyle().Width(inner).Height(h).MaxHeight(h).Render(content), "\n")
	edge := " "
	if p == paneMsgs && !m.chatsHidden() {
		edge = lipgloss.NewStyle().Foreground(hair()).Render("│")
	}
	rows := make([]string, 0, h+2)
	rows = append(rows, edge+padRight(truncateANSI(title, inner), inner)+" ")
	for _, l := range body {
		rows = append(rows, edge+padRight(l, inner)+" ")
	}
	bottom := strings.Repeat(" ", inner)
	if p == paneMsgs {
		// the line the compose box sits under
		bottom = lipgloss.NewStyle().Foreground(hair()).Render(strings.Repeat("─", inner))
	}
	rows = append(rows, edge+bottom+" ")
	return strings.Join(rows, "\n")
}

func truncateANSI(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

// modernHeader is the inbox switcher: plain words, the current one underlined.
func (m *Model) modernHeader() string {
	parts := []string{stAccent.Bold(true).Render(" beeper") + "   "}
	for i, n := range inboxes {
		if i == m.inbox {
			parts = append(parts, lipgloss.NewStyle().Foreground(colAccent).Bold(true).Underline(true).Render(n))
		} else {
			parts = append(parts, stDim.Render(n))
		}
		parts = append(parts, "   ")
	}
	if m.label.ID != "" {
		parts = append(parts, stAccent.Render("label:"+m.label.Name)+"  ")
	}
	if m.query != "" {
		parts = append(parts, stAccent.Render("/"+m.query))
	}
	left := strings.Join(parts, "")
	right := ""
	if m.inflight > 0 {
		right = stAccent.Render(spinFrames[m.spin%len(spinFrames)]) + " "
	}
	pad := max(1, m.w-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", pad) + right
}

func (m *Model) modernChatsTitle() string {
	unread := 0
	for _, ch := range m.chats {
		if ch.UnreadCount > 0 && !ch.IsMuted {
			unread++
		}
	}
	st := stDim
	if m.focus == paneChats {
		st = stBold
	}
	t := st.Render("Chats")
	if unread > 0 {
		t += stDim.Render(fmt.Sprintf("  %d unread", unread))
	}
	return t
}

func (m *Model) modernMsgsTitle() string {
	if m.cur == nil {
		return ""
	}
	t := stBold.Render(oneLine(m.cur.Title))
	info := []string{m.netLabel(m.cur.Network) + " " + m.cur.Network}
	if n := m.cur.Participants.Total; m.cur.Type == "group" && n > 0 {
		info = append(info, fmt.Sprintf("%d people", n))
	}
	if m.cur.IsMuted {
		info = append(info, m.icon("muted", "muted"))
	}
	return t + stDim.Render("  "+strings.Join(info, " · "))
}

// senderChip is a sender's initials on their colour, the face beside a message.
func senderChip(msg beeper.Message) string {
	name := msg.SenderName
	if msg.IsSender && name == "" {
		name = "me"
	}
	ini := initials(firstNonEmpty(name, msg.SenderID))
	c := senderStyle(msg).GetForeground()
	return lipgloss.NewStyle().Foreground(c).Background(chipBg()).Bold(true).Render(padRight(ini, 2))
}

// chipIndent lines a message's body up under the name, past the chip.
const chipIndent = "   "

// modernDayRule is "Today ────" from the left edge, quieter than classic's centred rule.
func modernDayRule(label string, w int, st lipgloss.Style) string {
	l := st.Render(label) + " "
	return l + lipgloss.NewStyle().Foreground(hair()).Render(strings.Repeat("─", max(0, w-lipgloss.Width(l))))
}

func (m *Model) modernCompose() string {
	w := m.msgWidth()
	cw := m.chatWidth()
	var top []string
	if ml := m.renderMentions(w - 4); ml != "" {
		top = append(top, ml)
	}
	if el := m.renderEmojiSuggestions(w - 4); el != "" {
		top = append(top, el)
	}
	switch {
	case m.editing != nil:
		top = append(top, stAccent.Render("editing  ")+stDim.Render(truncate(oneLine(plainText(m.editing.Text)), w-16)))
	case m.replyTo != nil:
		who := firstNonEmpty(m.replyTo.SenderName, "them")
		if m.replyTo.IsSender {
			who = "yourself"
		}
		top = append(top, stAccent.Render("↳ "+who+"  ")+stDim.Render(truncate(oneLine(plainText(m.replyTo.Text)), w-30)))
	}
	if m.attach != "" {
		top = append(top, stAccent.Render("+ ")+stDim.Render(truncate(m.attach, w-8)))
	}

	prompt := stDim.Render("›")
	if m.mode == modeInsert {
		prompt = stAccent.Bold(true).Render("›")
	}
	send := ""
	if m.mode == modeInsert && !m.vim.normal {
		send = lipgloss.NewStyle().Foreground(colText).Background(chipBg()).Render(" " + m.km.first("send") + " send ")
	}
	box := strings.Split(m.compose.View(), "\n")
	for i := range box {
		lead := "  "
		if i == 0 {
			lead = prompt + " "
		}
		box[i] = lead + box[i]
	}
	if send != "" {
		// the textarea pads its lines to full width, so cut that back to make room
		room := max(0, w-3-lipgloss.Width(send))
		box[0] = padRight(truncateANSI(box[0], room), room) + send
	}
	lines := append(top, box...)
	// same height as classic's bordered box: a row above and below
	h := m.composeHeight()
	out := []string{""}
	out = append(out, lines...)
	for len(out) < h {
		out = append(out, "")
	}
	edge := " "
	if cw > 0 {
		edge = lipgloss.NewStyle().Foreground(hair()).Render("│")
	}
	for i := range out {
		out[i] = edge + padRight(truncateANSI(out[i], w-2), w-1)
	}
	right := strings.Join(out[:h], "\n")
	if cw == 0 {
		return right
	}
	left := lipgloss.Place(cw, h, lipgloss.Left, lipgloss.Top, "")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func chipBg() color.Color {
	if colChip != nil {
		return colChip
	}
	return colSel
}

// modernStatus is one quiet line: the mode and chat on the left, key hints in
// the middle, connection and unread on the right. No coloured blocks.
func (m *Model) modernStatus() string {
	var name string
	nameSt := stAccent.Bold(true)
	switch m.mode {
	case modeInsert:
		name, nameSt = "insert", stSelf
		if m.vim.normal {
			name, nameSt = "vim"+strings.TrimRight(" "+m.vim.count+m.vim.pending, " "), lipgloss.NewStyle().Foreground(colLabel).Bold(true)
		}
	case modeFlash:
		name, nameSt = flashVerbs[m.flash.action], lipgloss.NewStyle().Foreground(colLabel).Bold(true)
	case modeLine, modePicker, modeGif:
		return m.line.View()
	case modeConfirm:
		return lipgloss.NewStyle().Foreground(colLabel).Bold(true).Render(" confirm ") + m.confirmText
	case modePopup:
		name = "view"
	case modeLinks:
		name = "links"
	case modeMenu:
		name = "menu"
	default:
		name = "normal"
	}
	left := " " + nameSt.Render(name)
	if m.cur != nil {
		left += stDim.Render("  " + truncate(oneLine(m.cur.Title), 24))
	}

	msg := m.status
	if m.mode == modeLinks {
		msg = "type a label, then o opens or y yanks, esc cancels"
	}
	if m.mode == modeFlash {
		msg = "type a label, esc cancels"
		if m.flash.action != 's' {
			msg = "type a label, " + shortKey(m.flash.key) + " again for the cursor, esc cancels"
		}
	}
	mid := ""
	switch {
	case msg != "" && m.isErr:
		mid = stErr.Render(msg)
	case msg != "":
		mid = stText.Render(msg)
	default:
		// nothing to say, so show what the keys do here
		var hs []string
		for _, h := range m.paneHints() {
			for _, pair := range strings.Split(h, "  ") {
				k, label, ok := strings.Cut(strings.TrimSpace(pair), " ")
				if !ok {
					continue
				}
				hs = append(hs, stBold.Render(k)+" "+stDim.Render(label))
			}
		}
		room := max(0, m.w-lipgloss.Width(left)-30)
		for len(hs) > 0 && lipgloss.Width(strings.Join(hs, "   ")) > room {
			hs = hs[:len(hs)-1]
		}
		mid = strings.Join(hs, "   ")
	}

	dot := stSelf.Render(m.icon("dot", "●")) + stDim.Render(" live")
	if !m.live {
		dot = stOther.Render(m.icon("dot", "●")) + stDim.Render(" polling")
	}
	unread := 0
	for _, ch := range m.chats {
		if !ch.IsMuted {
			unread += ch.UnreadCount
		}
	}
	right := dot
	if unread > 0 {
		right = stAccent.Bold(true).Render(fmt.Sprint(unread)) + stDim.Render(" unread   ") + right
	}
	right += " "

	room := max(0, m.w-lipgloss.Width(left)-lipgloss.Width(right)-6)
	mid = truncateANSI(mid, room)
	pad := max(1, m.w-lipgloss.Width(left)-3-lipgloss.Width(mid)-lipgloss.Width(right))
	return left + "   " + mid + strings.Repeat(" ", pad) + right
}
