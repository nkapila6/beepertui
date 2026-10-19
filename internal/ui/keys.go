package ui

import (
	"context"
	"strings"
	"time"

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
	case modePopup:
		return m.popupKey(key)
	case modeLinks:
		return m.linkKey(key)
	case modeMenu:
		return m.menuKey(key)
	case modeGif:
		return m.gifKey(k)
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
			m.mention, m.mentioned = nil, nil
			return nil
		}
		m.compose.Blur()
	case modeLine:
		m.closeLine()
		return nil
	case modePicker:
		m.closePicker()
		return nil
	case modePopup:
		m.mode = modeNormal
		return m.closePopupImage()
	case modeMenu:
		m.closeMenu()
		return nil
	case modeGif:
		return m.closeGif()
	}
	m.mode = modeNormal
	return nil
}

func (m *Model) normalKey(key string) tea.Cmd {
	if key == "esc" {
		m.pendingKey = ""
		m.replyTo, m.editing = nil, nil
		m.setAttach("", false)
		if m.query != "" {
			m.query = ""
			return m.loadChats()
		}
		m.status = ""
		return nil
	}
	sections := []string{secAnywhere, secChat, secMsgs}
	if m.focus == paneChats {
		sections[2] = secChats
	}
	if p := m.pendingKey; p != "" {
		m.pendingKey = ""
		if a := m.km.action(p+" "+key, sections...); a != "" {
			return m.doAction(a, key)
		}
	}
	a := m.km.action(key, sections...)
	if a == "" {
		if m.km.prefix[key] {
			m.pendingKey = key
		}
		return nil
	}
	return m.doAction(a, key)
}

// doAction runs a normal-mode action. key is what was pressed, so flash
// can treat a second press as "this one".
func (m *Model) doAction(action, key string) tea.Cmd {
	if r, ok := flashActions[action]; ok {
		if (r == 'r' || r == 'R' || r == 'y' || r == 'e' || r == 'x' || r == 'o' || r == 'U' || r == 'v' || r == 'd' || r == 'F') && m.focus != paneMsgs {
			return nil
		}
		var cmd tea.Cmd
		if (r == 'c' || r == 'C') && m.chatsHidden() {
			// chat labels need the list on screen
			cmd = m.setChatsHidden(false)
		}
		m.startFlash(r, key)
		return cmd
	}

	switch action {
	case "quit":
		m.Close()
		return tea.Quit
	case "help":
		m.mode, m.helpOff = modeHelp, 0
		return nil
	case "switch_pane":
		m.focus = 1 - m.focus
		if m.focus == paneChats && m.chatsHidden() {
			return m.setChatsHidden(false)
		}
		return nil
	case "focus_chats":
		m.focus = paneChats
		if m.chatsHidden() {
			return m.setChatsHidden(false)
		}
		return nil
	case "focus_msgs":
		if m.cur != nil {
			m.focus = paneMsgs
		}
		return nil
	case "toggle_sidebar":
		return m.setChatsHidden(!m.chatsHidden())
	case "down":
		return m.move(1)
	case "up":
		return m.move(-1)
	case "half_down":
		return m.move(m.halfPage())
	case "half_up":
		return m.move(-m.halfPage())
	case "top":
		return m.move(-1 << 30)
	case "bottom":
		return m.move(1 << 30)
	case "next_chat", "prev_chat":
		// next / previous chat from anywhere, without leaving the message pane
		if len(m.chats) == 0 {
			return nil
		}
		d := 1
		if action == "prev_chat" {
			d = -1
		}
		m.chatCur = clamp(m.chatCur+d, 0, len(m.chats)-1)
		m.fixChatScroll()
		return m.openChat(m.chats[m.chatCur], "")
	case "write":
		return m.startInsert()
	case "search":
		// in a chat it searches that chat, in the list it filters chats
		if m.focus == paneMsgs && m.cur != nil {
			return m.findInChat("")
		}
		return m.promptSearch()
	case "command":
		return m.promptCommand("")
	case "prev_inbox":
		return m.cycleInbox(-1)
	case "next_inbox":
		return m.cycleInbox(1)
	case "refresh":
		m.setStatus("refreshing")
		return tea.Batch(m.loadChats(), m.reloadOpenChat())
	case "link_hints":
		m.startLinkHints()
		return nil
	case "paste":
		// paste an image without going into insert mode first
		return tea.Batch(m.startInsert(), m.pasteFromClipboard())
	case "new_dm":
		return m.cmdDM("")
	case "new_group":
		return m.promptCommand("group ")
	case "open_chat":
		if m.focus == paneChats && m.chatCur < len(m.chats) {
			return m.openChat(m.chats[m.chatCur], "")
		}
		return nil
	case "select":
		// enter on an image opens the viewer, anywhere else it starts a message
		if m.msgCur < len(m.msgs) {
			if _, ok := messageImage(m.msgs[m.msgCur]); ok {
				return m.viewImage(m.msgCur)
			}
		}
		return m.startInsert()
	}

	// chat-level actions target the cursor chat in the list, or the open chat in the message pane
	ch := m.targetChat()
	if ch == nil {
		return nil
	}
	switch action {
	case "pin":
		return m.toggleChat(*ch, "isPinned", !ch.IsPinned, "pinned", "unpinned")
	case "mute":
		return m.toggleChat(*ch, "isMuted", !ch.IsMuted, "muted", "unmuted")
	case "low_priority":
		return m.toggleChat(*ch, "isLowPriority", !ch.IsLowPriority, "moved to low priority", "moved to primary")
	case "archive":
		return m.archive(*ch, !ch.IsArchived)
	case "unread":
		return m.toggleUnread(*ch)
	case "remind":
		return m.promptRemind(*ch)
	case "open_in_beeper":
		msgID := ""
		if m.focus == paneMsgs && m.msgCur < len(m.msgs) {
			msgID = m.msgs[m.msgCur].ID
		}
		id := ch.ID
		return m.run("opened in Beeper", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			return nil, m.c.Focus(ctx, id, msgID)
		})
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
	if used, cmd := m.mentionKey(k.String()); used {
		return cmd
	}
	if used, cmd := m.emojiKey(k.String()); used {
		return cmd
	}
	if k.String() == "esc" {
		m.mode = modeNormal
		m.compose.Blur()
		return nil
	}
	switch m.km.action(k.String(), secWriting) {
	case "send":
		return m.submitCompose()
	case "paste_image":
		return m.pasteFromClipboard()
	case "media_picker":
		return m.openGifPicker("")
	case "emoji_picker":
		return m.openEmojiPicker()
	case "attach_file":
		return m.chooseFile()
	}
	var cmd tea.Cmd
	m.compose, cmd = m.compose.Update(k)
	m.updateMentions()
	m.updateEmoji()
	return cmd
}

func (m *Model) submitCompose() tea.Cmd {
	text := strings.TrimSpace(m.compose.Value())
	if m.cur == nil || (text == "" && m.attach == "") {
		return nil
	}
	text = m.withMentionLinks(text)
	m.mention, m.mentioned, m.mentionDismissed = nil, nil, -1
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
				removeTemp(path)
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

// reloadAfter refreshes the chat after you change something, then checks
// again a few times: Beeper can take seconds to turn a send (especially an
// upload) into a real message, and the live websocket may not be connected.
func reloadAfter(m *Model) tea.Cmd { return tea.Batch(m.reloadOpenChat(), followUp()) }

func followUp() tea.Cmd {
	var cmds []tea.Cmd
	for _, d := range []time.Duration{1500 * time.Millisecond, 4 * time.Second, 9 * time.Second} {
		cmds = append(cmds, tea.Tick(d, func(time.Time) tea.Msg { return reloadMsg{chats: true, msgs: true} }))
	}
	return tea.Batch(cmds...)
}

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
