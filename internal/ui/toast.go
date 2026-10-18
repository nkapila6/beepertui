package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// A toast floats in the top right for a few seconds, for things worth more
// than a status line (downloads). The same text also goes out as a desktop
// notification where the terminal is known to handle one.

const toastFor = 4 * time.Second

type toast struct {
	text string
	mark string
	seq  int
}

type toastDoneMsg struct{ seq int }

// notify is for things you did (downloads, forwards): a toast, plus a desktop
// notification if you've looked away.
func (m *Model) notify(text string) tea.Cmd {
	return tea.Batch(m.showToast("✓", text), m.desktopNotify("beepertui", text))
}

func (m *Model) showToast(mark, text string) tea.Cmd {
	m.toastSeq++
	m.toast = &toast{text: text, mark: mark, seq: m.toastSeq}
	seq := m.toastSeq
	return tea.Tick(toastFor, func(time.Time) tea.Msg { return toastDoneMsg{seq} })
}

// desktopNotify sends a system notification through the terminal, when
// notifications are on and the terminal is known to support one.
func (m *Model) desktopNotify(title, body string) tea.Cmd {
	if !m.notifications {
		return nil
	}
	m.notifySeq++
	if esc := desktopNotification(m.notifySeq, title, body); esc != "" {
		return tea.Raw(esc)
	}
	return nil
}

// desktopNotification builds the escape for terminals that turn it into a
// system notification, and nothing for the rest so no junk lands on screen.
//
//	kitty           OSC 99 (title and body, its own protocol)
//	Ghostty, WezTerm OSC 777 notify (title and body)
//	iTerm2, tuios   OSC 9 (body only, so the title goes in front)
func desktopNotification(id int, title, body string) string {
	clean := func(s string) string {
		return strings.NewReplacer("\x1b", "", "\x07", "", ";", ",", "\n", " ").Replace(s)
	}
	title, body = clean(title), clean(body)
	switch {
	case os.Getenv("TMUX") != "":
		return ""
	case os.Getenv("TUIOS_WINDOW_ID") != "" || os.Getenv("TERM_PROGRAM") == "TUIOS":
		// tuios panes may claim to be ghostty, but tuios itself takes OSC 9
		return "\x1b]9;" + title + ": " + body + "\x07"
	case os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "":
		return fmt.Sprintf("\x1b]99;i=%d:d=0;%s\x1b\\\x1b]99;i=%d:d=1:p=body;%s\x1b\\", id, title, id, body)
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "ghostty", "WezTerm":
		return "\x1b]777;notify;" + title + ";" + body + "\x07"
	case "iTerm.app":
		return "\x1b]9;" + title + ": " + body + "\x07"
	}
	return ""
}

// withOverlays draws the floating bits (toast, link hints, right-click menu)
// over the finished frame. With none showing, the frame goes out untouched.
func (m *Model) withOverlays(frame string) string {
	var layers []*lipgloss.Layer
	if m.toast != nil {
		box := stOverlay.BorderForeground(colSelf).Render(stSelf.Render(m.toast.mark+" ") + stText.Render(truncate(m.toast.text, max(10, m.w/2))))
		x := max(0, m.w-lipgloss.Width(box)-1)
		layers = append(layers, lipgloss.NewLayer(box).X(x).Y(1).Z(1))
	}
	layers = append(layers, m.hintLayers()...)
	layers = append(layers, m.menuLayers()...)
	layers = append(layers, m.paletteLayers()...)
	layers = append(layers, m.hoverLayers()...)
	if len(layers) == 0 {
		return frame
	}
	return lipgloss.NewCompositor(append([]*lipgloss.Layer{lipgloss.NewLayer(frame)}, layers...)...).Render()
}
