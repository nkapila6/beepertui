package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// Who reacted: the selected message gets a line naming them, and hovering a
// reaction row with the mouse shows the same in a tooltip.

// personName turns a participant id into a name: you, a chat member, or
// whoever sent a loaded message with that id.
func (m *Model) personName(id string) string {
	if m.selfIDs[id] {
		return "you"
	}
	if m.cur != nil {
		for _, p := range m.cur.Participants.Items {
			if p.ID == id {
				return p.Name()
			}
		}
	}
	for _, msg := range m.msgs {
		if msg.SenderID == id && msg.SenderName != "" {
			return msg.SenderName
		}
	}
	// network ids look like @name:server, the local part reads best
	if at := strings.TrimPrefix(id, "@"); at != id {
		if i := strings.Index(at, ":"); i > 0 {
			return at[:i]
		}
	}
	return id
}

// reactorsByKey groups who reacted with what, in first-seen order.
func (m *Model) reactorsByKey(msg beeper.Message) (keys []string, who map[string][]string) {
	who = map[string][]string{}
	for _, r := range msg.Reactions {
		if _, ok := who[r.ReactionKey]; !ok {
			keys = append(keys, r.ReactionKey)
		}
		who[r.ReactionKey] = append(who[r.ReactionKey], m.personName(r.ParticipantID))
	}
	return keys, who
}

func (m *Model) reactorsLine(msg beeper.Message) string {
	keys, who := m.reactorsByKey(msg)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + " " + strings.Join(who[k], ", ")
	}
	return strings.Join(parts, "  ·  ")
}

// hoverLayers draws the who-reacted tooltip next to the mouse.
func (m *Model) hoverLayers() []*lipgloss.Layer {
	if m.hover == nil || m.mode != modeNormal && m.mode != modeInsert {
		return nil
	}
	i := m.hover.msg
	if i >= len(m.msgs) || len(m.msgs[i].Reactions) == 0 {
		return nil
	}
	keys, who := m.reactorsByKey(m.msgs[i])
	lines := make([]string, len(keys))
	for k, key := range keys {
		lines[k] = key + "  " + stText.Render(truncate(strings.Join(who[key], ", "), 50))
	}
	box := stOverlay.Render(strings.Join(lines, "\n"))
	x := clamp(m.hover.x+2, 0, max(0, m.w-lipgloss.Width(box)))
	y := m.hover.y + 1
	if y+lipgloss.Height(box) > m.h-1 {
		y = max(0, m.hover.y-lipgloss.Height(box))
	}
	return []*lipgloss.Layer{lipgloss.NewLayer(box).X(x).Y(y).Z(5)}
}

type hoverState struct {
	msg, x, y int
}

// mouseMoved updates the hover target: a message's reaction row under the pointer.
func (m *Model) mouseMoved(x, y int) {
	m.hover = nil
	if x < m.chatWidth() || y < 2 || y >= m.composeTop() {
		return
	}
	rows := m.msgRows()
	row := y - 2
	if row < 0 || row >= len(rows) || rows[row].idx < 0 || !rows[row].reactions {
		return
	}
	m.hover = &hoverState{msg: rows[row].idx, x: x, y: y}
}
