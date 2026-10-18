package ui

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

func reloadChats(m *Model) tea.Cmd { return m.loadChats() }

func (m *Model) toggleChat(ch beeper.Chat, field string, val bool, on, off string) tea.Cmd {
	status := off
	if val {
		status = on
	}
	return m.run(status+": "+ch.Title, func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		return reloadChats, m.c.UpdateChat(ctx, ch.ID, map[string]any{field: val})
	})
}

func (m *Model) archive(ch beeper.Chat, archived bool) tea.Cmd {
	status := "unarchived"
	if archived {
		status = "archived"
	}
	return m.run(status+": "+ch.Title, func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		return reloadChats, m.c.Archive(ctx, ch.ID, archived)
	})
}

func (m *Model) toggleUnread(ch beeper.Chat) tea.Cmd {
	if ch.UnreadCount > 0 || ch.IsMarkedUnread {
		return m.run("marked read: "+ch.Title, func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			return reloadChats, m.c.MarkRead(ctx, ch.ID)
		})
	}
	return m.run("marked unread: "+ch.Title, func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		return reloadChats, m.c.MarkUnread(ctx, ch.ID)
	})
}

func (m *Model) promptRemind(ch beeper.Chat) tea.Cmd {
	return m.prompt("remind in", "", func(m *Model, s string) tea.Cmd {
		return m.setReminder(ch, s)
	})
}

func (m *Model) setReminder(ch beeper.Chat, spec string) tea.Cmd {
	dismiss := false
	if f := strings.Fields(spec); len(f) > 1 && f[len(f)-1] == "dismiss" {
		dismiss = true
		spec = strings.Join(f[:len(f)-1], " ")
	}
	at, err := parseWhen(spec, nowFunc())
	if err != nil {
		m.setErr(err.Error())
		return nil
	}
	return m.run("reminder set for "+at.Format("Mon Jan 2 15:04"), func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		return reloadChats, m.c.SetReminder(ctx, ch.ID, at, dismiss)
	})
}

type chatAction struct {
	key   string
	label string
	run   func(m *Model, ch beeper.Chat) tea.Cmd
}

func onOff(on bool, yes, no string) string {
	if on {
		return yes
	}
	return no
}

// chatActions is every chat-level action, one key each. C and right-click both use it.
func (m *Model) chatActions(ch beeper.Chat) []chatAction {
	unread := ch.UnreadCount > 0 || ch.IsMarkedUnread
	acts := []chatAction{
		{"o", "open", func(m *Model, ch beeper.Chat) tea.Cmd { return m.openChat(ch, "") }},
		{"p", onOff(ch.IsPinned, "unpin", "pin"), func(m *Model, ch beeper.Chat) tea.Cmd {
			return m.toggleChat(ch, "isPinned", !ch.IsPinned, "pinned", "unpinned")
		}},
		{"m", onOff(ch.IsMuted, "unmute", "mute"), func(m *Model, ch beeper.Chat) tea.Cmd {
			return m.toggleChat(ch, "isMuted", !ch.IsMuted, "muted", "unmuted")
		}},
		{"A", onOff(ch.IsArchived, "unarchive", "archive"), func(m *Model, ch beeper.Chat) tea.Cmd { return m.archive(ch, !ch.IsArchived) }},
		{"L", onOff(ch.IsLowPriority, "move to primary", "move to low priority"), func(m *Model, ch beeper.Chat) tea.Cmd {
			return m.toggleChat(ch, "isLowPriority", !ch.IsLowPriority, "moved to low priority", "moved to primary")
		}},
		{"u", onOff(unread, "mark read", "mark unread"), func(m *Model, ch beeper.Chat) tea.Cmd { return m.toggleUnread(ch) }},
		{"!", "remind me", func(m *Model, ch beeper.Chat) tea.Cmd { return m.promptRemind(ch) }},
		{"i", "info", func(m *Model, ch beeper.Chat) tea.Cmd { m.chatInfo(ch); return nil }},
		{"O", "show in Beeper", func(m *Model, ch beeper.Chat) tea.Cmd {
			return m.run("opened in Beeper", func(ctx context.Context) (func(*Model) tea.Cmd, error) { return nil, m.c.Focus(ctx, ch.ID, "") })
		}},
	}
	if ch.Reminder != nil {
		acts = append(acts, chatAction{"R", "clear reminder", func(m *Model, ch beeper.Chat) tea.Cmd {
			return m.run("reminder cleared", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				return reloadChats, m.c.ClearReminder(ctx, ch.ID)
			})
		}})
	}
	return acts
}

func (m *Model) chatMenu(ch beeper.Chat) {
	acts := m.chatActions(ch)
	items := make([]pickItem, len(acts))
	byKey := map[string]chatAction{}
	for i, a := range acts {
		items[i] = pickItem{title: a.key, detail: a.label, value: a}
		byKey[a.key] = a
	}
	m.openPicker(ch.Title, items, func(m *Model, it pickItem) tea.Cmd {
		return it.value.(chatAction).run(m, ch)
	})
	m.line.Prompt = "key> "
	// it's a menu, so letters run actions instead of filtering
	m.pick.onKey = func(m *Model, key string) (bool, tea.Cmd) {
		if a, ok := byKey[key]; ok {
			m.closePicker()
			return true, a.run(m, ch)
		}
		return len(key) == 1, nil
	}
}

// message actions, all reached through flash labels

func (m *Model) yank(msg beeper.Message) tea.Cmd {
	text := plainText(msg.Text)
	if text == "" {
		for _, a := range msg.Attachments {
			text = a.FileName
		}
	}
	if text == "" {
		m.setErr("nothing to yank")
		return nil
	}
	if err := copyText(text); err != nil {
		// OSC52 fallback for terminals over ssh etc
		m.setStatus(fmt.Sprintf("yanked %d chars (osc52)", len([]rune(text))))
		return tea.SetClipboard(text)
	}
	m.setStatus(fmt.Sprintf("yanked %d chars", len([]rune(text))))
	return nil
}

func (m *Model) reply(msg beeper.Message) tea.Cmd {
	mm := msg
	m.replyTo, m.editing = &mm, nil
	return m.startInsert()
}

func (m *Model) edit(msg beeper.Message) tea.Cmd {
	if !msg.IsSender {
		m.setErr("you can only edit your own messages")
		return nil
	}
	mm := msg
	m.editing, m.replyTo = &mm, nil
	m.compose.SetValue(plainText(msg.Text))
	m.compose.MoveToEnd()
	return m.startInsert()
}

func (m *Model) del(msg beeper.Message) tea.Cmd {
	if !msg.IsSender {
		m.setErr("you can only delete your own messages")
		return nil
	}
	chatID, id := msg.ChatID, msg.ID
	m.confirm("delete this message for everyone? (y/n)", func(m *Model) tea.Cmd {
		return m.run("deleted", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			return reloadAfter, m.c.DeleteMessage(ctx, chatID, id)
		})
	})
	return nil
}

var quickReacts = []string{"\U0001F44D", "\u2764\uFE0F", "\U0001F602", "\U0001F62E", "\U0001F622", "\U0001F64F"}

func (m *Model) react(msg beeper.Message) tea.Cmd {
	chatID, id := msg.ChatID, msg.ID
	send := func(m *Model, key string) tea.Cmd {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil
		}
		m.rememberReact(key)
		return m.run("reacted "+key, func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			return reloadAfter, m.c.AddReaction(ctx, chatID, id, key)
		})
	}

	// recent picks first, then the table (which already leads with the common ones)
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

	hint := ""
	for i, e := range quickReacts {
		hint += fmt.Sprintf(" %d %s ", i+1, e)
	}
	m.openPicker("react   "+hint+"  or type a name (fire, tada, :eyes:)", items, func(m *Model, it pickItem) tea.Cmd {
		return send(m, it.value.(string))
	})
	m.pick.match = matchEmoji
	m.pick.onKey = func(m *Model, key string) (bool, tea.Cmd) {
		typed := strings.TrimSpace(m.line.Value())
		// alt+enter sends the typed text itself as the reaction. Beeper shows text
		// reactions to everyone, which is half the fun.
		if key == "alt+enter" && typed != "" {
			m.closePicker()
			return true, send(m, typed)
		}
		// digits are quick reactions until you start typing
		if typed != "" || len(key) != 1 || key[0] < '1' || int(key[0]-'1') >= len(quickReacts) {
			return false, nil
		}
		m.closePicker()
		return true, send(m, quickReacts[key[0]-'1'])
	}
	// nothing matched, so whatever was typed (or pasted) goes out as is
	m.pick.onRaw = send
	m.pick.footer = func(q string) string {
		if q = strings.TrimSpace(q); q == "" {
			return ""
		}
		if len(m.pick.shown) == 0 {
			return "enter sends \"" + q + "\" as a text reaction"
		}
		return "alt+enter sends \"" + q + "\" as a text reaction"
	}
	return nil
}

func (m *Model) rememberReact(e string) {
	out := []string{e}
	for _, r := range m.recentReacts {
		if r != e && len(out) < 8 {
			out = append(out, r)
		}
	}
	m.recentReacts = out
}

func emojiName(e string) string {
	for _, row := range emojiTable {
		if row[0] == e {
			return row[1]
		}
	}
	return ""
}

// matchEmoji wants every typed word to start some word of the name, so "ok"
// finds the ok hand and not "smoking". Surrounding colons are ignored.
func matchEmoji(it pickItem, q string) bool {
	if strings.ContainsFunc(q, func(r rune) bool { return r > 0x7f }) {
		return it.title == strings.TrimSpace(q)
	}
	words := strings.Fields(it.detail)
	for _, qw := range strings.Fields(strings.ReplaceAll(q, ":", " ")) {
		ok := false
		for _, w := range words {
			if strings.HasPrefix(w, qw) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func (m *Model) unreact(msg beeper.Message) tea.Cmd {
	var mine []string
	for _, r := range msg.Reactions {
		if m.selfIDs[r.ParticipantID] {
			mine = append(mine, r.ReactionKey)
		}
	}
	chatID, id := msg.ChatID, msg.ID
	remove := func(key string) tea.Cmd {
		return m.run("removed "+key, func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			return reloadAfter, m.c.RemoveReaction(ctx, chatID, id, key)
		})
	}
	switch len(mine) {
	case 0:
		m.setErr("you haven't reacted to that message")
		return nil
	case 1:
		return remove(mine[0])
	}
	items := make([]pickItem, len(mine))
	for i, k := range mine {
		items[i] = pickItem{title: k, value: k}
	}
	m.openPicker("remove which reaction?", items, func(m *Model, it pickItem) tea.Cmd {
		return remove(it.value.(string))
	})
	return nil
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"')\]]+`)

type openTarget struct {
	label string
	url   string
	mime  string
	mxc   bool
}

func (m *Model) open(msg beeper.Message) tea.Cmd {
	var ts []openTarget
	seen := map[string]bool{}
	for _, a := range msg.Attachments {
		if a.SrcURL == "" {
			continue
		}
		name := a.FileName
		if name == "" {
			name = a.Type
		}
		ts = append(ts, openTarget{label: "file: " + name, url: a.SrcURL, mime: a.MimeType, mxc: !strings.HasPrefix(a.SrcURL, "http")})
		seen[a.SrcURL] = true
	}
	for _, u := range append(richLinks(msg.Text), urlRe.FindAllString(plainText(msg.Text), -1)...) {
		if !seen[u] {
			ts = append(ts, openTarget{label: u, url: u})
			seen[u] = true
		}
	}
	for _, l := range msg.Links {
		if l.URL != "" && !seen[l.URL] {
			ts = append(ts, openTarget{label: l.URL, url: l.URL})
			seen[l.URL] = true
		}
	}
	switch len(ts) {
	case 0:
		m.setErr("no links or attachments in that message")
		return nil
	case 1:
		return m.openTarget(ts[0])
	}
	items := make([]pickItem, len(ts))
	for i, t := range ts {
		items[i] = pickItem{title: t.label, value: t}
	}
	m.openPicker("open", items, func(m *Model, it pickItem) tea.Cmd {
		return m.openTarget(it.value.(openTarget))
	})
	return nil
}

func (m *Model) openTarget(t openTarget) tea.Cmd {
	return m.run("opened "+t.label, func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		u := t.url
		if t.mxc {
			local, err := m.c.Download(ctx, u)
			if err != nil {
				return nil, err
			}
			u = local
		}
		if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "file://") {
			return nil, fmt.Errorf("refusing to open %q", u)
		}
		// someone else's file could be an .app or script, so only hand media/docs to their default app
		if strings.HasPrefix(u, "file://") && !safeMime(t.mime) {
			return nil, reveal(u)
		}
		return nil, openURL(u)
	})
}

func safeMime(mt string) bool {
	for _, p := range []string{"image/", "video/", "audio/", "text/plain", "application/pdf"} {
		if strings.HasPrefix(mt, p) {
			return true
		}
	}
	return false
}
