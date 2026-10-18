package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// notifyNewMessages compares fresh chats with the unread counts seen last
// time and alerts for chats that gained messages. Chats seen for the first
// time (startup, switching inbox) never alert. Muted chats and your own
// messages don't either, and neither does the chat you're looking at.
func (m *Model) notifyNewMessages(chats []beeper.Chat) tea.Cmd {
	var fresh []beeper.Chat
	for _, ch := range chats {
		old, known := m.seenUnread[ch.ID]
		m.seenUnread[ch.ID] = ch.UnreadCount
		if !known || ch.UnreadCount <= old || ch.IsMuted || ch.Preview == nil || ch.Preview.IsSender {
			continue
		}
		if m.focused && m.cur != nil && m.cur.ID == ch.ID {
			continue
		}
		fresh = append(fresh, ch)
	}
	if len(fresh) == 0 || !m.notifications {
		return nil
	}

	var title, body string
	if len(fresh) > 2 {
		// a burst (say, after reconnecting) gets one summary, not a pile of popups
		title = "beepertui"
		body = fmt.Sprintf("new messages in %d chats", len(fresh))
	} else {
		ch := fresh[0]
		title, body = messageAlert(ch, m.notifPreview)
		if len(fresh) == 2 {
			body += fmt.Sprintf("  (+ %s)", fresh[1].Title)
		}
	}

	if m.focused {
		return m.showToast("●", title+": "+body)
	}
	return m.desktopNotify(title, body)
}

func messageAlert(ch beeper.Chat, preview bool) (title, body string) {
	p := ch.Preview
	who := firstNonEmpty(p.SenderName, ch.Title)
	title = who
	if ch.Type == "group" && who != ch.Title {
		title = who + " in " + ch.Title
	}
	body = "new message"
	if preview {
		text := oneLine(plainText(p.Text))
		if text == "" && len(p.Attachments) > 0 {
			text = "sent " + strings.ToLower(firstNonEmpty(p.Attachments[0].Type, "a file"))
		}
		if text != "" {
			body = truncate(text, 160)
		}
	}
	return title, body
}
