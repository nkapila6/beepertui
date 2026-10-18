package ui

import (
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
	seq  int
}

type toastDoneMsg struct{ seq int }

func (m *Model) notify(text string) tea.Cmd {
	m.toastSeq++
	m.toast = &toast{text: text, seq: m.toastSeq}
	seq := m.toastSeq
	cmds := []tea.Cmd{tea.Tick(toastFor, func(time.Time) tea.Msg { return toastDoneMsg{seq} })}
	if seqStr := desktopNotification("beepertui", text); seqStr != "" {
		cmds = append(cmds, tea.Raw(seqStr))
	}
	return tea.Batch(cmds...)
}

// desktopNotification builds the escape for terminals that turn it into a
// system notification, and nothing for the rest so no junk lands on screen.
func desktopNotification(title, body string) string {
	clean := strings.NewReplacer("\x1b", "", "\x07", "", ";", ",").Replace(body)
	if os.Getenv("TMUX") != "" {
		return ""
	}
	if os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return "\x1b]99;i=1:d=0;" + title + "\x1b\\" + "\x1b]99;i=1:d=1:p=body;" + clean + "\x1b\\"
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "ghostty", "iTerm.app", "WezTerm", "TUIOS":
		return "\x1b]9;" + clean + "\x07"
	}
	return ""
}

// withToast draws the toast over the finished frame.
func (m *Model) withToast(frame string) string {
	if m.toast == nil {
		return frame
	}
	box := stOverlay.BorderForeground(colSelf).Render(stSelf.Render("✓ ") + stText.Render(truncate(m.toast.text, max(10, m.w/2))))
	x := max(0, m.w-lipgloss.Width(box)-1)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(frame),
		lipgloss.NewLayer(box).X(x).Y(1).Z(1),
	).Render()
}
