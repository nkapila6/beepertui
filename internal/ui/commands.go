package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// bottom-line prompt shared by search, commands, reactions and the multi-step flows

func (m *Model) prompt(kind, initial string, submit func(*Model, string) tea.Cmd) tea.Cmd {
	m.mode = modeLine
	m.lineKind = kind
	m.lineSubmit = submit
	m.lineKey = nil
	m.line.Prompt = kind + "> "
	m.line.SetValue(initial)
	m.line.CursorEnd()
	return m.line.Focus()
}

func (m *Model) closeLine() {
	m.mode = modeNormal
	m.line.Blur()
	m.lineKey = nil
}

func (m *Model) lineKeyPress(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if m.lineKey != nil {
		if handled, cmd := m.lineKey(m, key); handled {
			return cmd
		}
	}
	switch key {
	case "esc":
		m.closeLine()
		return nil
	case "enter":
		v := m.line.Value()
		submit := m.lineSubmit
		m.closeLine()
		if submit != nil {
			return submit(m, v)
		}
		return nil
	case "tab":
		if m.lineKind == ":" {
			m.completeCommand()
			return nil
		}
	}
	var cmd tea.Cmd
	m.line, cmd = m.line.Update(k)
	return cmd
}

func (m *Model) promptSearch() tea.Cmd {
	return m.prompt("/", m.query, func(m *Model, s string) tea.Cmd {
		m.query = strings.TrimSpace(s)
		m.chatCur, m.chatOff = 0, 0
		m.focus = paneChats
		return m.loadChats()
	})
}

func (m *Model) promptCommand(initial string) tea.Cmd {
	return m.prompt(":", initial, func(m *Model, s string) tea.Cmd { return m.runCommand(s) })
}

type command struct {
	name, args, help string
	run              func(m *Model, arg string) tea.Cmd
}

func needChat(f func(m *Model, ch beeper.Chat, arg string) tea.Cmd) func(*Model, string) tea.Cmd {
	return func(m *Model, arg string) tea.Cmd {
		ch := m.targetChat()
		if ch == nil {
			m.setErr("no chat selected")
			return nil
		}
		return f(m, *ch, arg)
	}
}

func flag(field string, val bool, on, off string) func(*Model, string) tea.Cmd {
	return needChat(func(m *Model, ch beeper.Chat, _ string) tea.Cmd {
		return m.toggleChat(ch, field, val, on, off)
	})
}

var commands []command

func init() {
	commands = []command{
		{"pin", "", "pin chat", flag("isPinned", true, "pinned", "")},
		{"unpin", "", "unpin chat", flag("isPinned", false, "", "unpinned")},
		{"mute", "", "mute chat", flag("isMuted", true, "muted", "")},
		{"unmute", "", "unmute chat", flag("isMuted", false, "", "unmuted")},
		{"lowpri", "", "move chat to low priority", flag("isLowPriority", true, "moved to low priority", "")},
		{"primary", "", "move chat back to primary", flag("isLowPriority", false, "", "moved to primary")},
		{"archive", "", "archive chat", needChat(func(m *Model, ch beeper.Chat, _ string) tea.Cmd { return m.archive(ch, true) })},
		{"unarchive", "", "unarchive chat", needChat(func(m *Model, ch beeper.Chat, _ string) tea.Cmd { return m.archive(ch, false) })},
		{"read", "", "mark chat read", needChat(func(m *Model, ch beeper.Chat, _ string) tea.Cmd {
			return m.run("marked read", func(ctx context.Context) (func(*Model) tea.Cmd, error) { return reloadChats, m.c.MarkRead(ctx, ch.ID) })
		})},
		{"unread", "", "mark chat unread", needChat(func(m *Model, ch beeper.Chat, _ string) tea.Cmd {
			return m.run("marked unread", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				return reloadChats, m.c.MarkUnread(ctx, ch.ID)
			})
		})},
		{"rename", "<title>", "rename chat", needChat(func(m *Model, ch beeper.Chat, arg string) tea.Cmd {
			if arg == "" {
				return m.prompt("rename", ch.Title, func(m *Model, s string) tea.Cmd { return m.runCommand("rename " + s) })
			}
			return m.run("renamed", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				return reloadChats, m.c.UpdateChat(ctx, ch.ID, map[string]any{"title": arg})
			})
		})},
		{"desc", "<text>", "set group description", needChat(func(m *Model, ch beeper.Chat, arg string) tea.Cmd {
			return m.run("description updated", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				return reloadChats, m.c.UpdateChat(ctx, ch.ID, map[string]any{"description": arg})
			})
		})},
		{"remind", "<when> [dismiss]", "reminder: 30m, 2h, 1d, tomorrow, 17:30. dismiss = cancel on reply", needChat(func(m *Model, ch beeper.Chat, arg string) tea.Cmd {
			if arg == "" {
				return m.promptRemind(ch)
			}
			return m.setReminder(ch, arg)
		})},
		{"unremind", "", "clear reminder", needChat(func(m *Model, ch beeper.Chat, _ string) tea.Cmd {
			return m.run("reminder cleared", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				return reloadChats, m.c.ClearReminder(ctx, ch.ID)
			})
		})},
		{"expiry", "<dur|off>", "disappearing messages timer, e.g. 1h, 24h, 168h, off", needChat(func(m *Model, ch beeper.Chat, arg string) tea.Cmd {
			secs := 0
			if arg != "off" && arg != "0" {
				d, err := time.ParseDuration(arg)
				if err != nil {
					m.setErr("expiry wants a duration like 24h, or off")
					return nil
				}
				secs = int(d.Seconds())
			}
			return m.run("disappearing timer updated", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				return reloadChats, m.c.UpdateChat(ctx, ch.ID, map[string]any{"messageExpirySeconds": secs})
			})
		})},
		{"notify", "", "notify anyway (push a muted-chat notification)", needChat(func(m *Model, ch beeper.Chat, _ string) tea.Cmd {
			return m.run("notified", func(ctx context.Context) (func(*Model) tea.Cmd, error) { return nil, m.c.NotifyAnyway(ctx, ch.ID) })
		})},
		{"open", "", "open chat in Beeper Desktop", needChat(func(m *Model, ch beeper.Chat, _ string) tea.Cmd {
			return m.run("opened in Beeper", func(ctx context.Context) (func(*Model) tea.Cmd, error) { return nil, m.c.Focus(ctx, ch.ID, "") })
		})},
		{"info", "", "show chat details", needChat(func(m *Model, ch beeper.Chat, _ string) tea.Cmd { m.chatInfo(ch); return nil })},
		{"attach", "[path]", "stage a file for the next message (no path: file dialog)", func(m *Model, arg string) tea.Cmd {
			if m.cur == nil {
				m.setErr("open a chat first")
				return nil
			}
			if arg == "" {
				return m.chooseFile()
			}
			p := expandHome(arg)
			if _, err := os.Stat(p); err != nil {
				m.setErr(err.Error())
				return nil
			}
			m.setAttach(p, false)
			return m.startInsert()
		}},
		{"files", "[folder]", "browse folders and attach a file", func(m *Model, arg string) tea.Cmd { return m.browseFiles(expandHome(arg)) }},
		{"paste", "", "attach the image or file on the clipboard (C-v while writing)", func(m *Model, _ string) tea.Cmd {
			return tea.Batch(m.startInsert(), m.pasteFromClipboard())
		}},
		{"chats", "", "go to any chat by name (same as cc)", func(m *Model, _ string) tea.Cmd { return m.openSwitcher() }},
		{"gif", "[words]", "search and send a GIF (C-g while writing)", func(m *Model, arg string) tea.Cmd {
			if c := m.startInsert(); m.mode != modeInsert {
				return c
			}
			return m.openGifPicker(arg)
		}},
		{"dm", "[name|handle|phone]", "start a direct chat", func(m *Model, arg string) tea.Cmd { return m.cmdDM(arg) }},
		{"group", "<title>", "create a group chat", func(m *Model, arg string) tea.Cmd { return m.cmdGroup(arg) }},
		{"search", "<words>", "search all messages", func(m *Model, arg string) tea.Cmd { return m.cmdSearch(arg) }},
		{"inbox", "primary|low-priority|archive|unread", "switch inbox", func(m *Model, arg string) tea.Cmd {
			for i, n := range inboxes {
				if strings.HasPrefix(n, arg) {
					m.inbox = i
					m.chatCur, m.chatOff = 0, 0
					return m.loadChats()
				}
			}
			m.setErr("unknown inbox")
			return nil
		}},
		{"label", "[name|none]", "filter inbox by label", func(m *Model, arg string) tea.Cmd { return m.cmdLabel(arg) }},
		{"accounts", "", "list connected accounts", func(m *Model, _ string) tea.Cmd { return m.cmdAccounts() }},
		{"images", "[on|off]", "toggle inline image previews", func(m *Model, arg string) tea.Cmd {
			m.showImages = arg == "on" || (arg == "" && !m.showImages)
			m.fixMsgScroll()
			if m.showImages {
				m.setStatus("images on")
				return m.loadThumbs()
			}
			m.setStatus("images off")
			return nil
		}},
		{"mouse", "[on|off]", "toggle mouse (off lets the terminal select text)", func(m *Model, arg string) tea.Cmd {
			m.mouse = arg == "on" || (arg == "" && !m.mouse)
			m.setStatus(map[bool]string{true: "mouse on", false: "mouse off, terminal selection works again"}[m.mouse])
			return nil
		}},
		{"icons", "[on|off]", "nerd font icons (or BEEPERTUI_NERDFONT=1)", func(m *Model, arg string) tea.Cmd {
			m.nerdFont = arg == "on" || (arg == "" && !m.nerdFont)
			m.setStatus(map[bool]string{true: "nerd font icons on", false: "nerd font icons off"}[m.nerdFont])
			return nil
		}},
		{"avatars", "[on|off]", "chat avatars in the list", func(m *Model, arg string) tea.Cmd {
			m.showAvatars = arg == "on" || (arg == "" && !m.showAvatars)
			return m.loadAvatars()
		}},
		{"compact", "[on|off]", "drop the blank line between senders", func(m *Model, arg string) tea.Cmd {
			m.compact = arg == "on" || (arg == "" && !m.compact)
			m.fixMsgScroll()
			return nil
		}},
		{"older", "", "load older messages", func(m *Model, _ string) tea.Cmd { return m.loadOlder() }},
		{"refresh", "", "refetch everything", func(m *Model, _ string) tea.Cmd { return tea.Batch(m.loadChats(), m.reloadOpenChat()) }},
		{"help", "", "show keys", func(m *Model, _ string) tea.Cmd { m.mode, m.helpOff = modeHelp, 0; return nil }},
		{"q", "", "quit", func(m *Model, _ string) tea.Cmd { m.Close(); return tea.Quit }},
	}
}

func (m *Model) runCommand(s string) tea.Cmd {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	name, arg, _ := strings.Cut(s, " ")
	arg = strings.TrimSpace(arg)
	if name == "quit" {
		name = "q"
	}
	var match []command
	for _, c := range commands {
		if c.name == name {
			match = []command{c}
			break
		}
		if strings.HasPrefix(c.name, name) {
			match = append(match, c)
		}
	}
	switch len(match) {
	case 0:
		m.setErr("unknown command: " + name + " (try :help)")
		return nil
	case 1:
		return match[0].run(m, arg)
	}
	names := make([]string, len(match))
	for i, c := range match {
		names[i] = c.name
	}
	m.setErr("ambiguous: " + strings.Join(names, ", "))
	return nil
}

func (m *Model) completeCommand() {
	v := m.line.Value()
	if strings.Contains(v, " ") {
		return
	}
	var names []string
	for _, c := range commands {
		if strings.HasPrefix(c.name, v) {
			names = append(names, c.name)
		}
	}
	if len(names) == 0 {
		return
	}
	lcp := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, lcp) {
			lcp = lcp[:len(lcp)-1]
		}
	}
	if len(names) == 1 {
		lcp += " "
	}
	m.line.SetValue(lcp)
	m.line.CursorEnd()
	m.setStatus(strings.Join(names, "  "))
}

func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, rest)
		}
	}
	return p
}

// multi-step flows

func (m *Model) pickAccount(then func(m *Model, a beeper.Account) tea.Cmd) tea.Cmd {
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		accts, err := m.c.Accounts(ctx)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			var items []pickItem
			for _, a := range accts {
				if a.Status != "connected" && a.Status != "backfilling" {
					continue
				}
				items = append(items, pickItem{title: networkName(a), detail: a.User.Name(), value: a})
			}
			if len(items) == 1 {
				return then(m, items[0].value.(beeper.Account))
			}
			m.openPicker("which account?", items, func(m *Model, it pickItem) tea.Cmd {
				return then(m, it.value.(beeper.Account))
			})
			return nil
		}, nil
	})
}

func networkName(a beeper.Account) string {
	if a.Network != "" {
		return a.Network
	}
	return a.AccountID
}

func (m *Model) cmdDM(query string) tea.Cmd {
	return m.pickAccount(func(m *Model, a beeper.Account) tea.Cmd {
		find := func(m *Model, q string) tea.Cmd {
			q = strings.TrimSpace(q)
			if q == "" {
				return nil
			}
			return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				users, err := m.c.SearchContacts(ctx, a.AccountID, q)
				if err != nil {
					return nil, err
				}
				return func(m *Model) tea.Cmd {
					items := make([]pickItem, 0, len(users)+1)
					for _, u := range users {
						items = append(items, pickItem{title: u.Name(), detail: firstNonEmpty(u.Username, u.PhoneNumber, u.Email), value: u})
					}
					// let the bridge try to resolve whatever was typed as a handle/phone/email
					raw := beeper.User{Username: q}
					if strings.HasPrefix(q, "+") {
						raw = beeper.User{PhoneNumber: q}
					} else if strings.Contains(q, "@") && strings.Contains(q, ".") {
						raw = beeper.User{Email: q}
					}
					items = append(items, pickItem{title: "use \"" + q + "\" as is", value: raw})
					m.openPicker("message who?", items, func(m *Model, it pickItem) tea.Cmd {
						u := it.value.(beeper.User)
						return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
							id, err := m.c.StartDirect(ctx, a.AccountID, u)
							if err != nil {
								return nil, err
							}
							return func(m *Model) tea.Cmd {
								return tea.Batch(m.openChatByID(id, ""), m.loadChats(), m.startInsert())
							}, nil
						})
					})
					return nil
				}, nil
			})
		}
		if query != "" {
			return find(m, query)
		}
		return m.prompt("dm "+networkName(a), "", find)
	})
}

func (m *Model) cmdGroup(title string) tea.Cmd {
	if title == "" {
		return m.prompt("group title", "", func(m *Model, s string) tea.Cmd {
			if strings.TrimSpace(s) == "" {
				return nil
			}
			return m.cmdGroup(strings.TrimSpace(s))
		})
	}
	return m.pickAccount(func(m *Model, a beeper.Account) tea.Cmd {
		return m.prompt("members, comma separated", "", func(m *Model, s string) tea.Cmd {
			var names []string
			for _, n := range strings.Split(s, ",") {
				if n = strings.TrimSpace(n); n != "" {
					names = append(names, n)
				}
			}
			if len(names) == 0 {
				return nil
			}
			return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				var ids, shown []string
				for _, n := range names {
					users, err := m.c.SearchContacts(ctx, a.AccountID, n)
					if err != nil {
						return nil, err
					}
					if len(users) == 0 {
						return nil, fmt.Errorf("no contact matches %q", n)
					}
					ids = append(ids, users[0].ID)
					shown = append(shown, users[0].Name())
				}
				return func(m *Model) tea.Cmd {
					m.confirm(fmt.Sprintf("create %q with %s? (y/n)", title, strings.Join(shown, ", ")), func(m *Model) tea.Cmd {
						return m.run("group created", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
							id, err := m.c.CreateGroup(ctx, a.AccountID, title, ids)
							if err != nil {
								return nil, err
							}
							return func(m *Model) tea.Cmd { return tea.Batch(m.openChatByID(id, ""), m.loadChats()) }, nil
						})
					})
					return nil
				}, nil
			})
		})
	})
}

func (m *Model) cmdSearch(q string) tea.Cmd {
	if q == "" {
		return m.prompt("search messages", "", func(m *Model, s string) tea.Cmd {
			if strings.TrimSpace(s) == "" {
				return nil
			}
			return m.cmdSearch(s)
		})
	}
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		p, err := m.c.SearchMessages(ctx, q, 50)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			if len(p.Items) == 0 {
				m.setErr("no messages match " + q)
				return nil
			}
			titles := map[string]string{}
			for _, ch := range m.chats {
				titles[ch.ID] = ch.Title
			}
			items := make([]pickItem, len(p.Items))
			for i, mm := range p.Items {
				where := titles[mm.ChatID]
				if where == "" {
					where = mm.SenderName
				}
				items[i] = pickItem{
					title:  fmt.Sprintf("%s  %s", shortTime(mm.Timestamp), where),
					detail: oneLine(firstNonEmpty(mm.SenderName, "?") + ": " + plainText(mm.Text)),
					value:  mm,
				}
			}
			m.openPicker(fmt.Sprintf("%d results for %q", len(items), q), items, func(m *Model, it pickItem) tea.Cmd {
				mm := it.value.(beeper.Message)
				return m.openChatByID(mm.ChatID, mm.ID)
			})
			return nil
		}, nil
	})
}

func (m *Model) cmdLabel(name string) tea.Cmd {
	if name == "none" {
		m.label = beeper.Label{}
		return m.loadChats()
	}
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		labels, err := m.c.Labels(ctx)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			for _, l := range labels {
				if name != "" && strings.EqualFold(l.Name, name) {
					m.label = l
					return m.loadChats()
				}
			}
			if len(labels) == 0 {
				m.setErr("you have no labels")
				return nil
			}
			items := []pickItem{{title: "(no label filter)", value: beeper.Label{}}}
			for _, l := range labels {
				items = append(items, pickItem{title: l.Name, value: l})
			}
			m.openPicker("label", items, func(m *Model, it pickItem) tea.Cmd {
				m.label = it.value.(beeper.Label)
				m.chatCur, m.chatOff = 0, 0
				return m.loadChats()
			})
			return nil
		}, nil
	})
}

func (m *Model) cmdAccounts() tea.Cmd {
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		accts, err := m.c.Accounts(ctx)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			sort.Slice(accts, func(i, j int) bool { return networkName(accts[i]) < networkName(accts[j]) })
			items := make([]pickItem, len(accts))
			for i, a := range accts {
				st := a.Status
				if a.StatusText != "" {
					st += " - " + a.StatusText
				}
				items[i] = pickItem{title: networkName(a), detail: a.User.Name() + "  [" + st + "]", value: a}
			}
			m.openPicker("accounts (enter opens Beeper to fix one)", items, func(m *Model, _ pickItem) tea.Cmd {
				return m.run("opened Beeper", func(ctx context.Context) (func(*Model) tea.Cmd, error) { return nil, m.c.Focus(ctx, "", "") })
			})
			return nil
		}, nil
	})
}

func (m *Model) chatInfo(ch beeper.Chat) {
	items := []pickItem{
		{title: "title", detail: ch.Title},
		{title: "network", detail: ch.Network},
		{title: "type", detail: ch.Type},
	}
	if ch.Description != "" {
		items = append(items, pickItem{title: "description", detail: oneLine(ch.Description)})
	}
	var flags []string
	for _, f := range []struct {
		on   bool
		name string
	}{{ch.IsPinned, "pinned"}, {ch.IsMuted, "muted"}, {ch.IsArchived, "archived"}, {ch.IsLowPriority, "low priority"}, {ch.IsReadOnly, "read-only"}} {
		if f.on {
			flags = append(flags, f.name)
		}
	}
	if len(flags) > 0 {
		items = append(items, pickItem{title: "flags", detail: strings.Join(flags, ", ")})
	}
	if ch.Reminder != nil {
		items = append(items, pickItem{title: "reminder", detail: ch.Reminder.RemindAt.Local().Format("Mon Jan 2 15:04")})
	}
	if ch.ExpirySeconds > 0 {
		items = append(items, pickItem{title: "disappearing", detail: (time.Duration(ch.ExpirySeconds) * time.Second).String()})
	}
	for _, l := range ch.Labels {
		items = append(items, pickItem{title: "label", detail: l.Name})
	}
	for _, p := range ch.Participants.Items {
		items = append(items, pickItem{title: "member", detail: p.Name() + "  " + firstNonEmpty(p.Username, p.PhoneNumber, p.Email)})
	}
	if ch.Participants.HasMore {
		items = append(items, pickItem{title: "member", detail: fmt.Sprintf("... %d total", ch.Participants.Total)})
	}
	m.openPicker("chat info", items, nil)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
