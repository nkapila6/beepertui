package ui

import (
	"context"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if key == "ctrl+c" {
		return m.ctrlC()
	}
	switch m.mode {
	case modeInsert:
		return m.insertKey(k)
	case modeLine:
		return m.lineKeyPress(k)
	case modeFlash:
		return m.flashKey(key)
	case modeConfirm:
		return m.confirmKey(key)
	case modePicker:
		return m.pickerKey(k)
	case modeHelp:
		return m.helpKey(key)
	}
	return m.normalKey(key)
}

// ctrlC clears the draft while writing, cancels any prompt, and only quits from normal mode.
func (m *Model) ctrlC() tea.Cmd {
	switch m.mode {
	case modeNormal:
		m.Close()
		return tea.Quit
	case modeInsert:
		if m.compose.Value() != "" || m.replyTo != nil || m.editing != nil || m.attach != "" {
			m.compose.SetValue("")
			m.replyTo, m.editing = nil, nil
			m.setAttach("", false)
			m.setStatus("cleared")
			return nil
		}
		m.compose.Blur()
	case modeLine:
		m.closeLine()
		return nil
	case modePicker:
		m.closePicker()
		return nil
	}
	m.mode = modeNormal
	return nil
}

func (m *Model) normalKey(key string) tea.Cmd {
	if m.pendingG {
		m.pendingG = false
		if key == "g" {
			m.move(-1 << 30)
			return nil
		}
	}

	// works in both panes
	switch key {
	case "q":
		m.Close()
		return tea.Quit
	case "?":
		m.mode, m.helpOff = modeHelp, 0
		return nil
	case "tab":
		m.focus = 1 - m.focus
		return nil
	case "h", "left":
		m.focus = paneChats
		return nil
	case "l", "right":
		if m.cur != nil {
			m.focus = paneMsgs
		}
		return nil
	case "j", "down":
		return m.move(1)
	case "k", "up":
		return m.move(-1)
	case "ctrl+d":
		return m.move(m.halfPage())
	case "ctrl+u":
		return m.move(-m.halfPage())
	case "g":
		m.pendingG = true
		return nil
	case "G":
		return m.move(1 << 30)
	case "i", "a":
		return m.startInsert()
	case "/":
		return m.promptSearch()
	case ":":
		return m.promptCommand("")
	case "[":
		return m.cycleInbox(-1)
	case "]":
		return m.cycleInbox(1)
	case "ctrl+r":
		m.setStatus("refreshing")
		return tea.Batch(m.loadChats(), m.reloadOpenChat())
	case "s":
		m.startFlash('s')
		return nil
	case "c":
		m.startFlash('c')
		return nil
	case "C":
		m.startFlash('C')
		return nil
	case "esc":
		m.replyTo, m.editing = nil, nil
		m.setAttach("", false)
		if m.query != "" {
			m.query = ""
			return m.loadChats()
		}
		m.status = ""
		return nil
	case "n":
		return m.cmdDM("")
	case "N":
		return m.promptCommand("group ")
	}

	// chat-level actions target the cursor chat in the list, or the open chat in the message pane
	if ch := m.targetChat(); ch != nil {
		switch key {
		case "p":
			return m.toggleChat(*ch, "isPinned", !ch.IsPinned, "pinned", "unpinned")
		case "m":
			return m.toggleChat(*ch, "isMuted", !ch.IsMuted, "muted", "unmuted")
		case "L":
			return m.toggleChat(*ch, "isLowPriority", !ch.IsLowPriority, "moved to low priority", "moved to primary")
		case "A":
			return m.archive(*ch, !ch.IsArchived)
		case "u":
			return m.toggleUnread(*ch)
		case "!":
			return m.promptRemind(*ch)
		case "O":
			msgID := ""
			if m.focus == paneMsgs && m.msgCur < len(m.msgs) {
				msgID = m.msgs[m.msgCur].ID
			}
			id := ch.ID
			return m.run("opened in Beeper", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				return nil, m.c.Focus(ctx, id, msgID)
			})
		}
	}

	if m.focus == paneChats {
		if key == "enter" && m.chatCur < len(m.chats) {
			return m.openChat(m.chats[m.chatCur], "")
		}
		return nil
	}

	switch key {
	case "y", "r", "R", "e", "x", "o", "U":
		m.startFlash(rune(key[0]))
	case "enter":
		return m.startInsert()
	}
	return nil
}

func (m *Model) targetChat() *beeper.Chat {
	if m.focus == paneMsgs {
		return m.cur
	}
	if m.chatCur < len(m.chats) {
		return &m.chats[m.chatCur]
	}
	return nil
}

func (m *Model) halfPage() int {
	if m.focus == paneChats {
		return max(1, m.bodyHeight()/4)
	}
	return 5
}

func (m *Model) move(d int) tea.Cmd {
	if m.focus == paneChats {
		m.chatCur = clamp(m.chatCur+d, 0, len(m.chats)-1)
		m.fixChatScroll()
		return nil
	}
	if len(m.msgs) == 0 {
		return nil
	}
	atTop := m.msgCur == 0
	m.msgCur = clamp(m.msgCur+d, 0, len(m.msgs)-1)
	m.fixMsgScroll()
	if d < 0 && atTop {
		return m.loadOlder()
	}
	return nil
}

func (m *Model) cycleInbox(d int) tea.Cmd {
	m.inbox = (m.inbox + d + len(inboxes)) % len(inboxes)
	m.chatCur, m.chatOff = 0, 0
	m.focus = paneChats
	return m.loadChats()
}

func (m *Model) startInsert() tea.Cmd {
	if m.cur == nil {
		m.setErr("open a chat first")
		return nil
	}
	if m.cur.IsReadOnly {
		m.setErr("this chat is read-only")
		return nil
	}
	m.mode = modeInsert
	m.focus = paneMsgs
	return m.compose.Focus()
}

func (m *Model) insertKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.mode = modeNormal
		m.compose.Blur()
		return nil
	case "enter":
		return m.submitCompose()
	case "ctrl+v":
		return m.pasteFromClipboard()
	}
	var cmd tea.Cmd
	m.compose, cmd = m.compose.Update(k)
	return cmd
}

func (m *Model) submitCompose() tea.Cmd {
	text := strings.TrimSpace(m.compose.Value())
	if m.cur == nil || (text == "" && m.attach == "") {
		return nil
	}
	chatID := m.cur.ID
	m.compose.SetValue("")

	if m.editing != nil {
		msgID := m.editing.ID
		m.editing = nil
		return m.run("edited", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			return reloadAfter, m.c.EditMessage(ctx, chatID, msgID, text)
		})
	}

	replyTo := ""
	if m.replyTo != nil {
		replyTo = m.replyTo.ID
	}
	path, temp := m.attach, m.attachTemp
	m.replyTo, m.attach, m.attachTemp = nil, "", false
	m.msgCur = max(0, len(m.msgs)-1)
	if path != "" {
		return m.run("sent attachment", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			up, err := m.c.Upload(ctx, path)
			if err != nil {
				return nil, err
			}
			if temp {
				os.Remove(path)
			}
			_, err = m.c.SendAttachment(ctx, chatID, up, text, replyTo)
			return reloadAfter, err
		})
	}
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		_, err := m.c.SendMessage(ctx, chatID, text, replyTo)
		return reloadAfter, err
	})
}

func reloadAfter(m *Model) tea.Cmd { return m.reloadOpenChat() }

func (m *Model) confirmKey(key string) tea.Cmd {
	m.mode = modeNormal
	yes := m.confirmYes
	m.confirmYes = nil
	if key == "y" && yes != nil {
		return yes(m)
	}
	m.setStatus("cancelled")
	return nil
}

func (m *Model) confirm(text string, yes func(*Model) tea.Cmd) {
	m.mode = modeConfirm
	m.confirmText = text
	m.confirmYes = yes
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
