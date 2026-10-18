package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// Right-click menus float next to the pointer. Every entry also has a key.

type menuItem struct {
	key   string
	label string
	run   func(m *Model) tea.Cmd
}

type ctxMenu struct {
	x, y  int
	title string
	items []menuItem
	cur   int
}

func (m *Model) openMenu(x, y int, title string, items []menuItem) {
	if len(items) == 0 {
		return
	}
	m.menu = &ctxMenu{x: x, y: y, title: title, items: items}
	m.mode = modeMenu
}

func (m *Model) messageMenu(i int, link string) []menuItem {
	msg := m.msgs[i]
	var items []menuItem
	if link != "" {
		items = append(items,
			menuItem{"o", "open link", func(m *Model) tea.Cmd { return m.openLink(link) }},
			menuItem{"Y", "copy link", func(m *Model) tea.Cmd {
				if copyText(link) != nil {
					return tea.SetClipboard(link)
				}
				m.setStatus("yanked " + link)
				return nil
			}},
		)
	}
	items = append(items,
		menuItem{"r", "reply", func(m *Model) tea.Cmd { return m.reply(msg) }},
		menuItem{"R", "react", func(m *Model) tea.Cmd { return m.react(msg) }},
		menuItem{"y", "copy text", func(m *Model) tea.Cmd { return m.yank(msg) }},
	)
	if _, ok := messageImage(msg); ok {
		items = append(items, menuItem{"v", "view image", func(m *Model) tea.Cmd { return m.viewImage(i) }})
	}
	if len(msg.Attachments) > 0 {
		items = append(items, menuItem{"d", "download", func(m *Model) tea.Cmd { return m.downloadMsg(msg) }})
	}
	if link == "" && (len(msg.Attachments) > 0 || len(msg.Links) > 0 || urlRe.MatchString(plainText(msg.Text))) {
		items = append(items, menuItem{"o", "open link / file", func(m *Model) tea.Cmd { return m.open(msg) }})
	}
	if msg.IsSender {
		items = append(items,
			menuItem{"e", "edit", func(m *Model) tea.Cmd { return m.edit(msg) }},
			menuItem{"x", "delete", func(m *Model) tea.Cmd { return m.del(msg) }},
		)
	}
	for _, r := range msg.Reactions {
		if m.selfIDs[r.ParticipantID] {
			items = append(items, menuItem{"U", "remove my reaction", func(m *Model) tea.Cmd { return m.unreact(msg) }})
			break
		}
	}
	items = append(items, menuItem{"O", "show in Beeper", func(m *Model) tea.Cmd {
		return m.run("opened in Beeper", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			return nil, m.c.Focus(ctx, msg.ChatID, msg.ID)
		})
	}})
	return items
}

func (m *Model) chatMenuItems(ch beeper.Chat) []menuItem {
	acts := m.chatActions(ch)
	items := make([]menuItem, len(acts))
	for i, a := range acts {
		a := a
		items[i] = menuItem{a.key, a.label, func(m *Model) tea.Cmd { return a.run(m, ch) }}
	}
	return items
}

func (m *Model) menuKey(key string) tea.Cmd {
	mn := m.menu
	if mn == nil {
		m.mode = modeNormal
		return nil
	}
	switch key {
	case "esc", "q":
		m.closeMenu()
		return nil
	case "j", "down", "ctrl+n":
		mn.cur = (mn.cur + 1) % len(mn.items)
		return nil
	case "k", "up", "ctrl+p":
		mn.cur = (mn.cur - 1 + len(mn.items)) % len(mn.items)
		return nil
	case "enter":
		return m.runMenu(mn.cur)
	}
	for i, it := range mn.items {
		if it.key == key {
			return m.runMenu(i)
		}
	}
	return nil
}

func (m *Model) runMenu(i int) tea.Cmd {
	it := m.menu.items[i]
	m.closeMenu()
	return it.run(m)
}

func (m *Model) closeMenu() {
	m.menu = nil
	m.mode = modeNormal
}

func (m *Model) menuBox() string {
	mn := m.menu
	var lines []string
	if mn.title != "" {
		lines = append(lines, stBold.Render(truncate(oneLine(mn.title), 30)))
	}
	for i, it := range mn.items {
		row := stAccent.Render(it.key) + "  " + it.label
		if i == mn.cur {
			row = stTabOn.Render(it.key + "  " + it.label)
		}
		lines = append(lines, row)
	}
	return stOverlay.Render(strings.Join(lines, "\n"))
}

// menuRect is where the menu sits: at the pointer, pushed back on screen if it would overflow.
func (m *Model) menuRect() (x, y, w, h int) {
	box := m.menuBox()
	w, h = lipgloss.Width(box), lipgloss.Height(box)
	x = clamp(m.menu.x, 0, max(0, m.w-w))
	y = clamp(m.menu.y, 0, max(0, m.h-h))
	return
}

func (m *Model) menuLayers() []*lipgloss.Layer {
	if m.mode != modeMenu || m.menu == nil {
		return nil
	}
	x, y, _, _ := m.menuRect()
	return []*lipgloss.Layer{lipgloss.NewLayer(m.menuBox()).X(x).Y(y).Z(3)}
}

// menuMouse handles a click while the menu is open: an item runs, anywhere else closes it.
func (m *Model) menuMouse(x, y int) tea.Cmd {
	mx, my, w, h := m.menuRect()
	if x < mx || x >= mx+w || y < my || y >= my+h {
		m.closeMenu()
		return nil
	}
	top := my + 1 // border
	if m.menu.title != "" {
		top++
	}
	if i := y - top; i >= 0 && i < len(m.menu.items) {
		return m.runMenu(i)
	}
	return nil
}
