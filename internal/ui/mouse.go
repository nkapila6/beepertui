package ui

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Mouse support works by mapping screen coordinates back onto the layout in
// view.go: header on row 0, panes with a border from row 1, the message pane
// title on row 2, compose box under the panes.

func mouseDefault() bool { return os.Getenv("BEEPERTUI_MOUSE") != "off" }

func (m *Model) composeTop() int { return 1 + m.bodyHeight() + 2 }

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	if !m.mouse {
		return nil
	}
	ms := msg.Mouse()
	_, wheel := msg.(tea.MouseWheelMsg)
	_, click := msg.(tea.MouseClickMsg)
	if !wheel && !(click && ms.Button == tea.MouseLeft) {
		return nil
	}
	dir := 0
	if wheel {
		switch ms.Button {
		case tea.MouseWheelUp:
			dir = -1
		case tea.MouseWheelDown:
			dir = 1
		default:
			return nil
		}
	}

	// overlays take the mouse first
	switch m.mode {
	case modePopup:
		if wheel {
			return m.stepPopup(dir)
		}
		m.mode = modeNormal
		return m.closePopupImage()
	case modeHelp:
		if wheel {
			return m.helpKey(map[int]string{-1: "k", 1: "j"}[dir])
		}
		m.mode = modeNormal
		return nil
	case modePicker:
		if wheel {
			m.pick.cur = clamp(m.pick.cur+dir, 0, len(m.pick.shown)-1)
		}
		return nil
	case modeFlash, modeConfirm, modeLine:
		return nil
	}

	inChats := ms.X < m.chatWidth()
	if wheel {
		if inChats {
			m.chatCur = clamp(m.chatCur+dir*2, 0, len(m.chats)-1)
			m.fixChatScroll()
			return nil
		}
		if m.cur != nil && len(m.msgs) > 0 {
			atTop := m.msgCur == 0
			m.msgCur = clamp(m.msgCur+dir, 0, len(m.msgs)-1)
			m.fixMsgScroll()
			if dir < 0 && atTop {
				return m.loadOlder()
			}
		}
		return nil
	}

	// a click anywhere but the compose box ends insert mode, like clicking out of a text field
	if m.mode == modeInsert && !(ms.Y >= m.composeTop() && !inChats) {
		m.mode = modeNormal
		m.compose.Blur()
	}

	switch {
	case ms.Y == 0:
		return m.clickTab(ms.X)
	case ms.Y >= m.composeTop():
		if !inChats {
			return m.startInsert()
		}
		return nil
	case inChats:
		if ms.Y < 2 {
			return nil
		}
		i := m.chatOff + (ms.Y-2)/2
		if i >= len(m.chats) || i > m.chatOff+m.chatRows()-1 {
			return nil
		}
		m.chatCur = i
		m.fixChatScroll()
		return m.openChat(m.chats[i], "")
	default:
		row := ms.Y - 3
		rows := m.msgRows()
		if row < 0 || row >= len(rows) || rows[row].idx < 0 {
			return nil
		}
		i := rows[row].idx
		m.focus = paneMsgs
		// second click on an image opens the viewer
		if i == m.msgCur {
			if _, ok := messageImage(m.msgs[i]); ok {
				return m.viewImage(i)
			}
		}
		m.msgCur = i
		m.fixMsgScroll()
		return nil
	}
}

// clickTab switches inbox when a header tab is clicked. Widths mirror renderHeader.
func (m *Model) clickTab(x int) tea.Cmd {
	pos := lipgloss.Width(stAccent.Bold(true).Render(" beeper "))
	for i, n := range inboxes {
		w := len(n) + 2
		if x >= pos && x < pos+w {
			if i == m.inbox {
				return nil
			}
			return m.cycleInbox(i - m.inbox)
		}
		pos += w
	}
	return nil
}
