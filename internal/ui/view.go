package ui

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

const gutter = 3

// layout

func (m *Model) composeHeight() int {
	h := 2 + m.composeLines()
	if m.replyTo != nil || m.editing != nil || m.attach != "" {
		h++
	}
	if m.mention != nil || m.emoji != nil {
		h++
	}
	return h
}

// bodyHeight is the inner height of the two panes.
func (m *Model) bodyHeight() int { return max(2, m.h-2-m.composeHeight()-2) }

func (m *Model) chatWidth() int { return clamp(m.w*m.chatPct/100, 24, max(44, m.w*m.chatPct/100)) }
func (m *Model) msgWidth() int  { return max(20, m.w-m.chatWidth()) }

// chat list scrolling, two rows per chat

func (m *Model) chatRows() int { return max(1, m.bodyHeight()/2) }

func (m *Model) chatRange() (int, int) {
	return m.chatOff, min(len(m.chats)-1, m.chatOff+m.chatRows()-1)
}

func (m *Model) fixChatScroll() {
	rows := m.chatRows()
	if m.chatCur < m.chatOff {
		m.chatOff = m.chatCur
	}
	if m.chatCur >= m.chatOff+rows {
		m.chatOff = m.chatCur - rows + 1
	}
	m.chatOff = clamp(m.chatOff, 0, max(0, len(m.chats)-rows))
}

// message scrolling is anchored on the bottom message, like a chat app

func (m *Model) msgTextWidth() int { return max(10, m.msgWidth()-2-gutter-1) }

func (m *Model) blockHeight(i int) int { return len(m.msgBlock(i)) }

// msgSpace is the message pane height minus the chat title row.
func (m *Model) msgSpace() int { return max(1, m.bodyHeight()) }

func (m *Model) msgRange() (int, int) {
	if len(m.msgs) == 0 {
		return 0, -1
	}
	last := clamp(m.msgBottom, 0, len(m.msgs)-1)
	h, first := m.msgSpace(), last
	used := m.blockHeight(last)
	for first > 0 && used < h {
		first--
		used += m.blockHeight(first)
	}
	return first, last
}

func (m *Model) fixMsgScroll() {
	if len(m.msgs) == 0 {
		m.msgBottom, m.msgCur = 0, 0
		return
	}
	m.msgCur = clamp(m.msgCur, 0, len(m.msgs)-1)
	m.msgBottom = clamp(m.msgBottom, 0, len(m.msgs)-1)
	if m.msgCur > m.msgBottom {
		m.msgBottom = m.msgCur
	}
	if first, _ := m.msgRange(); m.msgCur < first || (m.msgCur == first && first > 0) {
		h, used, b := m.msgSpace(), 0, m.msgCur
		for j := m.msgCur; j < len(m.msgs); j++ {
			used += m.blockHeight(j)
			if used > h {
				break
			}
			b = j
		}
		m.msgBottom = b
	}
}

// rendering

func (m *Model) View() tea.View {
	v := tea.NewView(m.withOverlays(m.render()))
	v.AltScreen = true
	v.ReportFocus = true // so notifications know whether you're looking
	if m.mouse {
		// all motion, so hovering a reaction can show who reacted
		v.MouseMode = tea.MouseModeAllMotion
	}
	unread := 0
	for _, ch := range m.chats {
		if !ch.IsMuted {
			unread += ch.UnreadCount
		}
	}
	v.WindowTitle = "beeper"
	if unread > 0 {
		v.WindowTitle = fmt.Sprintf("beeper (%d)", unread)
	}
	return v
}

func (m *Model) render() string {
	if m.w == 0 {
		return ""
	}
	var body string
	switch m.mode {
	case modePicker:
		body = m.renderPicker()
	case modeHelp:
		body = m.renderHelp()
	case modePopup:
		body = m.renderPopup()
	case modeGif:
		body = m.renderGif()
	default:
		chats := titledPane(m.paneStyle(paneChats), m.chatWidth(), m.bodyHeight()+2, m.chatsTitle(), m.renderChats())
		msgs := titledPane(m.paneStyle(paneMsgs), m.msgWidth(), m.bodyHeight()+2, m.msgsTitle(), m.renderMsgs())
		body = lipgloss.JoinHorizontal(lipgloss.Top, chats, msgs)
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), body, m.renderCompose(), m.renderStatus())
}

func (m *Model) paneStyle(p pane) lipgloss.Style {
	if m.focus == p && m.mode != modeInsert {
		return stPaneFocus
	}
	return stPane
}

func (m *Model) renderHeader() string {
	parts := []string{stAccent.Bold(true).Render(" beeper ")}
	for i, n := range inboxes {
		if i == m.inbox {
			parts = append(parts, stTabOn.Render(n))
		} else {
			parts = append(parts, stTab.Render(n))
		}
	}
	if m.label.ID != "" {
		parts = append(parts, stAccent.Render(" label:"+m.label.Name))
	}
	if m.query != "" {
		parts = append(parts, stAccent.Render(" /"+m.query))
	}
	left := strings.Join(parts, "")
	right := ""
	if m.inflight > 0 {
		right = stAccent.Render(spinFrames[m.spin%len(spinFrames)]) + " "
	}
	pad := max(1, m.w-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", pad) + right
}

func (m *Model) chatsTitle() string {
	st := stDim
	if m.focus == paneChats {
		st = stAccent.Bold(true)
	}
	t := "Chats " + stDim.Render("· "+inboxes[m.inbox])
	unread := 0
	for _, ch := range m.chats {
		if ch.UnreadCount > 0 && !ch.IsMuted {
			unread++
		}
	}
	if unread > 0 {
		t += stDim.Render(fmt.Sprintf(" · %d unread", unread))
	}
	return st.Render("Chats ") + strings.TrimPrefix(t, "Chats ")
}

func (m *Model) msgsTitle() string {
	if m.cur == nil {
		return stDim.Render("Messages")
	}
	st := stDim.Bold(true)
	if m.focus == paneMsgs {
		st = stAccent.Bold(true)
	}
	t := st.Render(oneLine(m.cur.Title)) + stDim.Render(" · "+m.netLabel(m.cur.Network)+" "+m.cur.Network)
	if m.cur.IsMuted {
		t += stDim.Render(" · " + m.icon("muted", "muted"))
	}
	return t
}

func (m *Model) renderChats() string {
	w := m.chatWidth() - 2
	if len(m.chats) == 0 {
		return stDim.Render(" no chats")
	}
	first, last := m.chatRange()
	var rows []string
	for i := first; i <= last; i++ {
		ch := m.chats[i]
		sel := i == m.chatCur && m.focus == paneChats
		// background has to go on every piece; wrapping styled text in another
		// style doesn't work because each piece resets it
		paint := func(st lipgloss.Style, s string) string {
			if sel {
				// dim text is the same slot as the selection, so lift it to the default ink
				if st.GetForeground() == colMuted {
					st = st.Foreground(colText)
				}
				st = st.Background(colSel)
			}
			return st.Render(s)
		}
		plain := lipgloss.NewStyle()

		g := paint(plain, "  ")
		if m.mode == modeFlash {
			if l, ok := m.flash.label(paneChats, i); ok {
				g = stLabel.Render(padRight(l, 2))
			}
		} else if i == m.chatCur {
			g = paint(stCursor, "▌ ")
		}

		var right []string
		if ch.IsPinned {
			right = append(right, m.icon("pinned", "^"))
		}
		if ch.IsMuted {
			right = append(right, m.icon("muted", "~"))
		}
		if ch.Reminder != nil {
			right = append(right, m.icon("remind", "!"))
		}
		right = append(right, m.netLabel(ch.Network))
		rs := paint(stDim, strings.Join(right, " "))
		if n := ch.UnreadCount; n > 0 || ch.IsMarkedUnread {
			badge := " * "
			if n > 0 {
				badge = fmt.Sprintf(" %d ", n)
			}
			rs = stUnread.Render(badge) + paint(plain, " ") + rs
		}
		// avatar column, 4 wide plus a space, when enabled
		av := [2]string{"", ""}
		aw := 0
		if m.showAvatars {
			av = m.avatarLines(ch)
			av[0] += paint(plain, " ")
			av[1] += paint(plain, " ")
			aw = avatarCols + 1
		}
		tw := max(1, w-2-aw-lipgloss.Width(rs)-1)
		titleSt := stText
		switch {
		case ch.IsMuted:
			titleSt = stDim // muted chats fade back
		case ch.UnreadCount > 0 || ch.IsMarkedUnread:
			titleSt = stBold
		}
		l1 := g + av[0] + paint(titleSt, padRight(truncate(oneLine(ch.Title), tw), tw)+" ") + rs

		prev := ""
		ts := ""
		if p := ch.Preview; p != nil {
			who := p.SenderName
			if p.IsSender {
				who = "you"
			}
			body := plainText(p.Text)
			if body == "" && len(p.Attachments) > 0 {
				body = "[" + strings.ToLower(firstNonEmpty(p.Attachments[0].Type, "file")) + "]"
			}
			if ch.Type == "group" || p.IsSender {
				body = who + ": " + body
			}
			prev = oneLine(body)
			ts = shortTime(p.Timestamp)
		} else {
			// chats outside the preview window still get something useful
			prev = ch.Network
			if ch.Type == "group" && ch.Participants.Total > 0 {
				prev += fmt.Sprintf(" - %d members", ch.Participants.Total)
			}
			ts = shortTime(ch.LastActivity)
		}
		pw := max(1, w-2-aw-lipgloss.Width(ts)-1)
		l2 := paint(plain, "  ") + av[1] + paint(stDim, padRight(truncate(prev, pw), pw)+" "+ts)

		rows = append(rows, l1, l2)
	}
	return strings.Join(rows, "\n")
}

type msgRow struct {
	idx  int // message index, -1 for padding above short chats
	text string
	top  bool // first line of the message itself, where labels go
	lead bool // day rule, "new" rule or spacing above the message
	kind lineKind
}

// msgRows is the message pane line by line, exactly msgSpace() rows. The mouse
// code uses it too, to map a click back to a message.
func (m *Model) msgRows() []msgRow {
	if m.cur == nil || len(m.msgs) == 0 {
		return nil
	}
	first, last := m.msgRange()
	var rows []msgRow
	for i := first; i <= last; i++ {
		lines, pre, kinds := m.msgBlockParts(i)
		for j, l := range lines {
			rows = append(rows, msgRow{idx: i, text: l, top: j == pre, lead: j < pre, kind: kinds[j]})
		}
	}
	space := m.msgSpace()
	if len(rows) > space {
		rows = rows[len(rows)-space:]
		// a cropped block's label goes on its first visible non-lead line
		hasTop := false
		for k := 0; k < len(rows) && rows[k].idx == rows[0].idx; k++ {
			hasTop = hasTop || rows[k].top
		}
		if !hasTop {
			for k := 0; k < len(rows) && rows[k].idx == rows[0].idx; k++ {
				if !rows[k].lead {
					rows[k].top = true
					break
				}
			}
		}
	}
	pad := make([]msgRow, space-len(rows))
	for k := range pad {
		pad[k].idx = -1
	}
	return append(pad, rows...)
}

func (m *Model) renderMsgs() string {
	if m.cur == nil {
		return stDim.Render(" pick a chat (enter, or s for labels)")
	}
	if len(m.msgs) == 0 {
		return stDim.Render(" no messages")
	}
	rows := m.msgRows()
	var out []string
	for _, r := range rows {
		if r.idx < 0 {
			out = append(out, "")
			continue
		}
		g := strings.Repeat(" ", gutter)
		switch {
		case m.mode == modeFlash:
			if l, ok := m.flash.label(paneMsgs, r.idx); ok && r.top {
				g = stLabel.Render(padRight(l, 2)) + " "
			}
		case r.idx == m.msgCur && m.focus == paneMsgs && !r.lead:
			g = stCursor.Render("▌") + "  "
		}
		out = append(out, g+r.text)
	}
	return strings.Join(out, "\n")
}

// msgBlock renders one message without the gutter. Consecutive messages from the
// same sender within a few minutes skip the header.
func (m *Model) msgBlock(i int) []string {
	lines, _, _ := m.msgBlockParts(i)
	return lines
}

type lineKind int

const (
	lineContent lineKind = iota
	lineLead             // day rule, "new" rule, spacing
	lineTick             // the line carrying the ✓ / ✓✓
	lineReact            // reaction pills and who reacted
	lineSeen             // who saw it
)

// msgBlockParts renders one message plus whatever goes above it (day rule,
// "new" rule, spacing). pre is how many of the lines are that lead-in, so the
// cursor bar and flash label can skip them; kinds says what each line is,
// for mouse hover.
func (m *Model) msgBlockParts(i int) (lines []string, pre int, kinds []lineKind) {
	msg := m.msgs[i]
	w := m.msgTextWidth()

	newDay := i == 0 || !sameDay(m.msgs[i-1].Timestamp, msg.Timestamp)
	if newDay {
		lines = append(lines, rule(dayLabel(msg.Timestamp), w, stDim))
	}
	if i == m.newMarker() {
		lines = append(lines, rule("new", w, lipgloss.NewStyle().Foreground(colErr).Bold(true)))
	}

	var quoted *beeper.Message
	if msg.LinkedMessageID != "" {
		for j := range m.msgs {
			if m.msgs[j].ID == msg.LinkedMessageID {
				quoted = &m.msgs[j]
				break
			}
		}
	}

	// replies always get a header, so the quote never hangs under someone else's name
	showHeader := true
	if i > 0 && msg.LinkedMessageID == "" {
		prev := m.msgs[i-1]
		showHeader = newDay || prev.SenderID != msg.SenderID || msg.Timestamp.Sub(prev.Timestamp) > 5*time.Minute
	}
	if showHeader && !m.compact && i > 0 && len(lines) == 0 {
		lines = append(lines, "")
	}
	pre = len(lines)
	if showHeader {
		who := firstNonEmpty(msg.SenderName, msg.SenderID)
		if msg.IsSender {
			who = "you"
		}
		hdr := senderStyle(msg).Render(truncate(who, w-14))
		if msg.IsSender && quoted != nil {
			hdr += stDim.Render(" replied to ") + senderStyle(*quoted).Bold(false).Render(truncate(quotedName(*quoted), w/3))
		}
		hdr += " " + stDim.Render(shortTime(msg.Timestamp))
		lines = append(lines, hdr)
	}

	if msg.LinkedMessageID != "" {
		lines = append(lines, quoteCard(quoted, w)...)
	}

	switch {
	case msg.IsDeleted:
		lines = append(lines, stDim.Italic(true).Render("message deleted"))
	case msg.Text != "":
		text := renderRich(msg.Text)
		if msg.EditedTimestamp != nil {
			text += stDim.Render(" (edited)")
		}
		lines = append(lines, strings.Split(lipgloss.Wrap(text, w, " -/"), "\n")...)
	}
	for _, l := range msg.Links {
		lines = append(lines, linkPreview(l, w)...)
	}
	for _, a := range msg.Attachments {
		kind := strings.ToLower(firstNonEmpty(a.Type, "file"))
		label := "[" + kind + "] " + a.FileName
		if src := thumbSource(a); src != "" && m.showImages {
			if img := m.thumbLines(src, w); img != nil {
				lines = append(lines, img...)
				if a.Type != "video" {
					continue
				}
				label = "[video] " + a.FileName + "  (o plays)"
			} else if t := m.thumbs[src]; t != nil && t.loading {
				label += "  loading..."
			}
		}
		lines = append(lines, stDim.Render(truncate(label, w)))
	}
	if len(lines) == pre || (showHeader && len(lines) == pre+1 && msg.Text == "" && len(msg.Attachments) == 0 && !msg.IsDeleted) {
		lines = append(lines, stDim.Render("["+strings.ToLower(firstNonEmpty(msg.Type, "message"))+"]"))
	}

	tickLine := -1
	if t := m.tick(msg); t != "" && len(lines) > pre {
		last := len(lines) - 1
		if lipgloss.Width(lines[last])+lipgloss.Width(t) <= w {
			lines[last] += t
		} else {
			lines = append(lines, t)
		}
		tickLine = len(lines) - 1
	}
	reactStart := len(lines)

	if len(msg.Reactions) > 0 {
		counts := map[string]int{}
		var order []string
		for _, r := range msg.Reactions {
			if counts[r.ReactionKey] == 0 {
				order = append(order, r.ReactionKey)
			}
			counts[r.ReactionKey]++
		}
		mine := map[string]bool{}
		for _, r := range msg.Reactions {
			if m.selfIDs[r.ParticipantID] {
				mine[r.ReactionKey] = true
			}
		}
		var rs []string
		for _, k := range order {
			s := k
			if counts[k] > 1 {
				s += fmt.Sprintf(" %d", counts[k])
			}
			st := stReaction
			if mine[k] {
				st = stReactionMine
			}
			rs = append(rs, st.Render(" "+s+" "))
		}
		lines = append(lines, strings.Join(rs, " "))
		// the selected message spells out who reacted
		if i == m.msgCur && m.focus == paneMsgs {
			lines = append(lines, stDim.Render(truncate(m.reactorsLine(msg), w)))
		}
	}
	seenStart := len(lines)
	// seen status: always under your latest message, and under any of yours
	// you scroll onto
	if msg.IsSender && (i == m.lastOwnMessage() || i == m.msgCur && m.focus == paneMsgs) {
		if sl := m.seenLine(msg); sl != "" {
			lines = append(lines, stDim.Render(truncate(sl, w)))
		}
	}

	kinds = make([]lineKind, len(lines))
	for j := range kinds {
		switch {
		case j < pre:
			kinds[j] = lineLead
		case j == tickLine:
			kinds[j] = lineTick
		case j >= seenStart:
			kinds[j] = lineSeen
		case j >= reactStart:
			kinds[j] = lineReact
		}
	}
	return lines, pre, kinds
}

func (m *Model) renderCompose() string {
	w := m.msgWidth()
	cw := m.chatWidth()
	var banner string
	switch {
	case m.editing != nil:
		banner = stAccent.Render("editing: ") + stDim.Render(truncate(oneLine(plainText(m.editing.Text)), w-16))
	case m.replyTo != nil:
		who := firstNonEmpty(m.replyTo.SenderName, "them")
		if m.replyTo.IsSender {
			who = "yourself"
		}
		banner = stAccent.Render("replying to "+who+": ") + stDim.Render(truncate(oneLine(plainText(m.replyTo.Text)), w-30))
	}
	if m.attach != "" {
		if banner != "" {
			banner += "  "
		}
		banner += stAccent.Render("attach: ") + stDim.Render(truncate(m.attach, w-14))
	}
	content := m.compose.View()
	if banner != "" {
		content = banner + "\n" + content
	}
	if el := m.renderEmojiSuggestions(w - 4); el != "" {
		content = el + "\n" + content
	}
	if ml := m.renderMentions(w - 4); ml != "" {
		content = ml + "\n" + content
	}
	st := stPane
	if m.mode == modeInsert {
		st = stPaneFocus
	}
	var hints []string
	for _, h := range m.paneHints() {
		hints = append(hints, stDim.Render(" "+truncate(h, cw-2)))
	}
	left := lipgloss.Place(cw, m.composeHeight(), lipgloss.Left, lipgloss.Center, strings.Join(hints, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, st.Width(w).Render(content))
}

func (m *Model) renderStatus() string {
	var mode lipgloss.Style
	var name string
	switch m.mode {
	case modeInsert:
		mode, name = stModeIns, "INSERT"
	case modeFlash:
		mode, name = stModeAlt, strings.ToUpper(flashVerbs[m.flash.action])
	case modeLine, modePicker, modeGif:
		return m.line.View()
	case modeConfirm:
		return stModeAlt.Render("CONFIRM") + " " + m.confirmText
	case modePopup:
		mode, name = stModeAlt, "VIEW"
	case modeLinks:
		mode, name = stModeAlt, "LINKS"
	case modeMenu:
		mode, name = stModeAlt, "MENU"
	default:
		mode, name = stMode, "NORMAL"
	}

	msg := m.status
	if m.mode == modeLinks {
		msg = "type a label, then o opens or y yanks, esc cancels"
	}
	if m.mode == modeFlash {
		msg = "type a label"
		switch m.flash.action {
		case 's':
		case 'c':
			msg += ", c again for the chat list"
		default:
			msg += fmt.Sprintf(", %c again for the cursor", m.flash.action)
		}
		msg += ", esc cancels"
	}
	msgSt := stDim
	if m.isErr {
		msgSt = stErr
	}

	// right side: connection and unread total
	segBg := colSel
	onSeg := lipgloss.NewStyle().Background(segBg)
	// every piece carries the segment background, nested styles would reset it
	conn := onSeg.Foreground(colSelf).Render(m.icon("dot", "●")) + onSeg.Foreground(colText).Render(" live")
	if !m.live {
		conn = onSeg.Foreground(colOther).Render(m.icon("dot", "●")) + onSeg.Foreground(colText).Render(" polling")
	}
	unread := 0
	for _, ch := range m.chats {
		if !ch.IsMuted {
			unread += ch.UnreadCount
		}
	}

	left := m.segments([]segment{
		{name, mode},
		{m.statusChat(), lipgloss.NewStyle().Background(segBg)},
	}, false)
	rightSegs := []segment{{conn, onSeg}}
	if unread > 0 {
		rightSegs = append(rightSegs, segment{fmt.Sprintf("%d unread", unread), stMode})
	}
	right := m.segments(rightSegs, true)

	room := m.w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	mid := " " + msgSt.Render(truncate(msg, max(0, room)))
	pad := max(0, m.w-lipgloss.Width(left)-lipgloss.Width(mid)-lipgloss.Width(right))
	return left + mid + strings.Repeat(" ", pad) + right
}

func (m *Model) statusChat() string {
	if m.cur == nil {
		return "no chat"
	}
	return truncate(oneLine(m.cur.Title), 28) + " " + m.netLabel(m.cur.Network)
}

type segment struct {
	text string
	st   lipgloss.Style
}

// segments draws lualine-style blocks. With a nerd font they get rounded
// powerline caps, otherwise they just butt up against each other.
func (m *Model) segments(segs []segment, rightSide bool) string {
	var b strings.Builder
	for i, sg := range segs {
		bg := fill(sg.st)
		if m.nerdFont && rightSide {
			b.WriteString(lipgloss.NewStyle().Foreground(bg).Render("\uE0B6"))
		}
		b.WriteString(sg.st.Padding(0, 1).Render(sg.text))
		if m.nerdFont && !rightSide {
			b.WriteString(lipgloss.NewStyle().Foreground(bg).Render("\uE0B4"))
		}
		if !m.nerdFont && i < len(segs)-1 {
			b.WriteString(" ")
		}
	}
	return b.String()
}

func (m *Model) overlayBox(title string, lines []string) string {
	w := min(m.w-4, 90)
	h := m.bodyHeight() + 2
	box := stOverlay.Width(w).Render(stBold.Render(title) + "\n\n" + strings.Join(lines, "\n"))
	return lipgloss.Place(m.w, h, lipgloss.Center, lipgloss.Center, box)
}

func (m *Model) renderPicker() string {
	maxRows := max(1, m.bodyHeight()-6)
	start := 0
	if m.pick.cur >= maxRows {
		start = m.pick.cur - maxRows + 1
	}
	w := min(m.w-4, 90) - 6
	tw := 0
	for _, i := range m.pick.shown {
		tw = max(tw, lipgloss.Width(m.pick.items[i].title))
	}
	tw = min(tw, 24)
	var lines []string
	for k := start; k < len(m.pick.shown) && k < start+maxRows; k++ {
		it := m.pick.items[m.pick.shown[k]]
		l := it.title
		if it.detail != "" {
			l = padRight(truncate(it.title, tw), tw) + "  " + it.detail
		}
		l = truncate(oneLine(l), w)
		if k == m.pick.cur {
			l = stCursor.Render("> ") + stBold.Render(l)
		} else {
			l = "  " + stText.Render(l)
		}
		lines = append(lines, l)
	}
	if len(m.pick.shown) == 0 {
		lines = append(lines, stDim.Render("  no matches"))
	}
	if m.pick.footer != nil {
		if f := m.pick.footer(m.line.Value()); f != "" {
			lines = append(lines, "", stAccent.Render("  "+f))
		}
	}
	return m.overlayBox(m.pick.title, lines)
}

// linkPreview is the little card under a message with a link, like Beeper's.
func linkPreview(l beeper.LinkPreview, w int) []string {
	if l.Title == "" && l.Summary == "" {
		return nil
	}
	bar := stAccent.Render("▎ ")
	var out []string
	if l.Title != "" {
		out = append(out, bar+stBold.Hyperlink(l.URL).Render(truncate(oneLine(l.Title), w-2)))
	}
	if l.Summary != "" {
		wrapped := strings.Split(lipgloss.Wrap(oneLine(l.Summary), w-2, " "), "\n")
		if len(wrapped) > 2 {
			wrapped = wrapped[:2]
			wrapped[1] = truncate(wrapped[1], w-3) + "…"
		}
		for _, s := range wrapped {
			out = append(out, bar+stDim.Render(s))
		}
	}
	if u, err := url.Parse(l.URL); err == nil && u.Host != "" {
		out = append(out, bar+stDim.Render(strings.TrimPrefix(u.Host, "www.")))
	}
	return out
}

// helpers

func shortTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.Local()
	now := nowFunc()
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format("15:04")
	case now.Sub(t) < 6*24*time.Hour:
		return t.Format("Mon 15:04")
	case y1 == y2:
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2 2006")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
}

func padRight(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

var netAbbr = map[string]string{
	"whatsapp": "WA", "telegram": "TG", "signal": "SIG", "imessage": "iM", "slack": "SL",
	"discord": "DC", "instagram": "IG", "messenger": "FB", "facebook messenger": "FB",
	"linkedin": "LI", "x": "X", "twitter": "X", "google messages": "GM", "beeper": "BP",
	"matrix": "MX", "google chat": "GC", "sms": "SMS",
}

func netShort(n string) string {
	if a, ok := netAbbr[strings.ToLower(n)]; ok {
		return a
	}
	if len(n) > 3 {
		return n[:3]
	}
	return n
}

func quotedName(o beeper.Message) string {
	if o.IsSender {
		return "you"
	}
	return firstNonEmpty(o.SenderName, o.SenderID, "?")
}

// quoteCard is the indented block above a reply: a bar in the quoted
// sender's colour, their name, then the start of what they said.
func quoteCard(o *beeper.Message, w int) []string {
	if o == nil {
		return []string{"  " + stDim.Render("┃ reply to an older message")}
	}
	st := senderStyle(*o)
	bar := "  " + st.Bold(false).Render("┃") + " "
	text := oneLine(plainText(o.Text))
	if text == "" && len(o.Attachments) > 0 {
		text = "[" + strings.ToLower(firstNonEmpty(o.Attachments[0].Type, "file")) + "]"
	}
	return []string{
		bar + st.Render(truncate(quotedName(*o), w-4)),
		bar + stDim.Render(truncate(firstNonEmpty(text, "message"), w-4)),
	}
}
