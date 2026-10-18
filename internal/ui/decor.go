package ui

import (
	"encoding/json"
	"hash/fnv"
	"image/color"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// sender colours: palette slots that aren't already taken by self (green),
// the accent (blue) or flash labels, so names stay readable and follow the theme
// plain red is left out on purpose: some themes set color1 almost to the background
var senderColors = []color.Color{
	lipgloss.Yellow, lipgloss.Magenta, lipgloss.Cyan, lipgloss.BrightRed,
	lipgloss.BrightYellow, lipgloss.BrightMagenta, lipgloss.BrightCyan,
}

func hashColor(key string) color.Color {
	h := fnv.New32a()
	h.Write([]byte(key))
	return senderColors[h.Sum32()%uint32(len(senderColors))]
}

func senderStyle(msg beeper.Message) lipgloss.Style {
	if msg.IsSender {
		return stSelf
	}
	return lipgloss.NewStyle().Bold(true).Foreground(hashColor(msg.SenderID))
}

// rule draws "──── text ────" centred in width.
func rule(text string, width int, st lipgloss.Style) string {
	label := " " + text + " "
	side := max(2, (width-lipgloss.Width(label))/2)
	return stDim.Render(strings.Repeat("─", side)) + st.Render(label) + stDim.Render(strings.Repeat("─", max(0, width-side-lipgloss.Width(label))))
}

func dayLabel(t time.Time) string {
	t = t.Local()
	now := nowFunc()
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "Today"
	case y1 == y2 && m1 == m2 && d1 == d2-1, now.AddDate(0, 0, -1).Format("2006-01-02") == t.Format("2006-01-02"):
		return "Yesterday"
	case now.Sub(t) < 6*24*time.Hour:
		return t.Format("Monday")
	case y1 == y2:
		return t.Format("Mon, Jan 2")
	}
	return t.Format("Mon, Jan 2 2006")
}

func sameDay(a, b time.Time) bool {
	return a.Local().Format("2006-01-02") == b.Local().Format("2006-01-02")
}

// laterSortKey compares Beeper sort keys, which are numeric strings of varying length.
func laterSortKey(a, b string) bool {
	if x, err1 := strconv.ParseUint(a, 10, 64); err1 == nil {
		if y, err2 := strconv.ParseUint(b, 10, 64); err2 == nil {
			return x > y
		}
	}
	if len(a) != len(b) {
		return len(a) > len(b)
	}
	return a > b
}

// seenByAnyone reads the loosely typed "seen" field: a bool, a timestamp, or a map per participant.
func seenByAnyone(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "false" {
		return false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		for _, v := range obj {
			if string(v) != "false" && string(v) != "null" {
				return true
			}
		}
		return false
	}
	return true // true, or a timestamp
}

// tick is the delivery mark after your own messages.
func (m *Model) tick(msg beeper.Message) string {
	if !msg.IsSender {
		return ""
	}
	status := ""
	if msg.SendStatus != nil {
		status = msg.SendStatus.Status
	}
	switch {
	case status == "PENDING":
		return stDim.Render(" " + m.icon("sending", "sending"))
	case strings.HasPrefix(status, "FAIL"):
		return stErr.Render(" " + m.icon("failed", "failed"))
	case seenByAnyone(msg.Seen):
		return stAccent.Render(" " + m.icon("seen", "✓✓"))
	}
	return stDim.Render(" " + m.icon("sent", "✓"))
}

// titledPane draws a rounded pane with its title set into the top border.
func titledPane(st lipgloss.Style, w, h int, title, content string) string {
	out := strings.Split(st.Width(w).Height(h).Render(content), "\n")
	if len(out) == 0 {
		return ""
	}
	bc := lipgloss.NewStyle().Foreground(st.GetBorderTopForeground())
	// title is already styled, so cut it with an ANSI-aware truncate
	title = ansi.Truncate(title, max(1, w-6), "…")
	fill := max(0, w-3-lipgloss.Width(title)-2)
	out[0] = bc.Render("╭─") + " " + title + " " + bc.Render(strings.Repeat("─", fill)+"╮")
	return strings.Join(out, "\n")
}

// spinner, only ticking while requests are in flight

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type spinMsg struct{}

func spinTick() tea.Cmd {
	return tea.Tick(90*time.Millisecond, func(time.Time) tea.Msg { return spinMsg{} })
}

// nerd font glyphs, opt in

var nerdIcons = map[string]string{
	"sending": "\U000F0150", // md-clock_outline
	"failed":  "\U000F05D6", // md-alert_circle_outline
	"seen":    "\U000F012D", // md-check_all
	"sent":    "\U000F012C", // md-check
	"pinned":  "\U000F0403", // md-pin
	"muted":   "\U000F009B", // md-bell_off
	"remind":  "\U000F0020", // md-alarm
	"dot":     "",          // oct-dot_fill
}

// icon returns the nerd font glyph when enabled, otherwise the plain fallback.
func (m *Model) icon(name, fallback string) string {
	if m.nerdFont {
		if g, ok := nerdIcons[name]; ok {
			return g
		}
	}
	return fallback
}

// network icons from nerd fonts. There's no Signal logo, so it gets the lock bubble.
var netIcons = map[string]string{
	"whatsapp":           "\U000F05A3",
	"telegram":           "",
	"signal":             "\U000F0FCC",
	"imessage":           "",
	"slack":              "",
	"discord":            "\U000F066F",
	"instagram":          "",
	"messenger":          "\U000F020E",
	"facebook messenger": "\U000F020E",
	"linkedin":           "",
	"x":                  "",
	"twitter":            "",
	"google messages":    "\U000F0369",
	"sms":                "",
	"beeper":             "\U000F0628",
	"matrix":             "\U000F0628",
	"google chat":        "\U000F0B79",
}

func (m *Model) netLabel(network string) string {
	if m.nerdFont {
		if g, ok := netIcons[strings.ToLower(network)]; ok {
			return g
		}
		return "" // fa-comment
	}
	return netShort(network)
}

// newMarker is the index of the first message from someone else after the
// read mark, or -1. That's where the "new" rule goes.
func (m *Model) newMarker() int {
	if m.readMark == "" {
		return -1
	}
	for i, msg := range m.msgs {
		if !msg.IsSender && laterSortKey(msg.SortKey, m.readMark) {
			return i
		}
	}
	return -1
}
